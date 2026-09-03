package handlers

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type robotSummary struct {
	ID        uint               `json:"id"`
	Name      string             `json:"name"`
	RobotType models.RobotType   `json:"robot_type"`
	Status    models.RobotStatus `json:"status"`
}

type projectHealthResponse struct {
	ID              uint                       `json:"id"`
	ProjectID       uint                       `json:"project_id"`
	AgentID         string                     `json:"agent_id"`
	AgentName       string                     `json:"agent_name"`
	RunMode         models.ProjectRunMode      `json:"run_mode"`
	ContainerStatus models.ContainerStatus     `json:"container_status"`
	RuntimeOK       bool                       `json:"runtime_ok"`
	Status          models.ProjectHealthStatus `json:"status"`
	URL             string                     `json:"url"`
	HTTPStatus      int                        `json:"http_status"`
	LatencyMS       int64                      `json:"latency_ms"`
	Message         string                     `json:"message"`
	ErrorMessage    string                     `json:"error_message"`
	CheckedAt       time.Time                  `json:"checked_at"`
	CreatedAt       time.Time                  `json:"created_at"`
}

type projectListResponse struct {
	ID                 uint                   `json:"id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	RunMode            models.ProjectRunMode  `json:"run_mode"`
	FrontendRoute      string                 `json:"frontend_route"`
	ContainerRoute     string                 `json:"container_route"`
	UseHTTPS           bool                   `json:"use_https"`
	HasSSLCert         bool                   `json:"has_ssl_cert"`
	ContainerStatus    models.ContainerStatus `json:"container_status"`
	DockerImage        string                 `json:"docker_image"`
	ContainerID        string                 `json:"container_id"`
	Port               uint                   `json:"port"`
	LoginURL           string                 `json:"login_url"`
	BuildStatus        models.BuildStatus     `json:"build_status"`
	DeploymentRevision int64                  `json:"deployment_revision"`
	TaskID             string                 `json:"task_id"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	RobotIDs           []uint                 `json:"robot_ids"`
	Robots             []robotSummary         `json:"robots"`
	CredentialCount    int                    `json:"credential_count"`
	Health             *projectHealthResponse `json:"health,omitempty"`
	AgentIDs           []uint                 `json:"agent_ids"`
	Deployments        []deploymentResponse   `json:"deployments"`
}

type projectDetailResponse struct {
	ID                 uint                   `json:"id"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	RunMode            models.ProjectRunMode  `json:"run_mode"`
	FrontendRoute      string                 `json:"frontend_route"`
	ContainerRoute     string                 `json:"container_route"`
	UseHTTPS           bool                   `json:"use_https"`
	HasSSLCert         bool                   `json:"has_ssl_cert"`
	ContainerCode      string                 `json:"container_code"`
	ContainerStatus    models.ContainerStatus `json:"container_status"`
	ContainerLog       string                 `json:"container_log"`
	HtmlFilePath       string                 `json:"html_file_path"`
	DockerImage        string                 `json:"docker_image"`
	ContainerID        string                 `json:"container_id"`
	Port               uint                   `json:"port"`
	LoginURL           string                 `json:"login_url"`
	BuildStatus        models.BuildStatus     `json:"build_status"`
	DeploymentRevision int64                  `json:"deployment_revision"`
	TaskID             string                 `json:"task_id"`
	CreatedAt          time.Time              `json:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at"`
	Robots             []robotSummary         `json:"robots"`
	RobotIDs           []uint                 `json:"robot_ids"`
	Health             *projectHealthResponse `json:"health,omitempty"`
	AgentIDs           []uint                 `json:"agent_ids"`
	Deployments        []deploymentResponse   `json:"deployments"`
}

type projectUpdateRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	RunMode        string `json:"run_mode"`
	FrontendRoute  string `json:"frontend_route"`
	ContainerRoute string `json:"container_route"`
	LoginURL       string `json:"login_url"`
	UseHTTPS       bool   `json:"use_https"`
	Port           uint   `json:"port"`
	Robots         []uint `json:"robots"`
	AgentIDs       []uint `json:"agent_ids"`
}

func projectDeploymentResponses(deployments []models.ProjectDeployment) ([]uint, []deploymentResponse) {
	agentIDs := make([]uint, 0, len(deployments))
	items := make([]deploymentResponse, 0, len(deployments))
	for _, deployment := range deployments {
		if deployment.DesiredStatus == models.DeploymentStatusRemoved {
			continue
		}
		agentIDs = append(agentIDs, deployment.AgentID)
		items = append(items, toDeploymentResponse(deployment))
	}
	return agentIDs, items
}

type projectHealthReportRequest struct {
	ProjectID       uint                       `json:"project_id"`
	AgentID         string                     `json:"agent_id"`
	AgentName       string                     `json:"agent_name"`
	RunMode         models.ProjectRunMode      `json:"run_mode"`
	ContainerStatus models.ContainerStatus     `json:"container_status"`
	RuntimeOK       bool                       `json:"runtime_ok"`
	Status          models.ProjectHealthStatus `json:"status"`
	URL             string                     `json:"url"`
	HTTPStatus      int                        `json:"http_status"`
	LatencyMS       int64                      `json:"latency_ms"`
	Message         string                     `json:"message"`
	ErrorMessage    string                     `json:"error_message"`
	CheckedAt       time.Time                  `json:"checked_at"`
}

const (
	flaskRequirementsContent = "flask==2.2.2\nrequests==2.26.0\nWerkzeug==2.2.2\nflask-cors==4.0.0\n"
	localContainerIDPrefix   = "local:"
	maxHTMLUploadSizeBytes   = 30 << 20
)

var errHTMLFileTooLarge = errors.New("HTML file exceeds the 30MB upload limit")

func toRobotSummaries(robots []models.Robot) []robotSummary {
	summaries := make([]robotSummary, 0, len(robots))
	for _, robot := range robots {
		summaries = append(summaries, robotSummary{
			ID:        robot.ID,
			Name:      robot.Name,
			RobotType: robot.RobotType,
			Status:    robot.Status,
		})
	}
	return summaries
}

func toRobotIDs(robots []models.Robot) []uint {
	ids := make([]uint, 0, len(robots))
	for _, robot := range robots {
		ids = append(ids, robot.ID)
	}
	return ids
}

func toProjectHealthResponse(health models.ProjectHealthCheck) projectHealthResponse {
	return projectHealthResponse{
		ID:              health.ID,
		ProjectID:       health.ProjectID,
		AgentID:         health.AgentID,
		AgentName:       health.AgentName,
		RunMode:         health.RunMode,
		ContainerStatus: health.ContainerStatus,
		RuntimeOK:       health.RuntimeOK,
		Status:          health.Status,
		URL:             health.URL,
		HTTPStatus:      health.HTTPStatus,
		LatencyMS:       health.LatencyMS,
		Message:         health.Message,
		ErrorMessage:    health.ErrorMessage,
		CheckedAt:       health.CheckedAt,
		CreatedAt:       health.CreatedAt,
	}
}

func toProjectHealthResponsePtr(health *models.ProjectHealthCheck) *projectHealthResponse {
	if health == nil {
		return nil
	}
	response := toProjectHealthResponse(*health)
	return &response
}

func toProjectListResponse(project models.Project, health *models.ProjectHealthCheck) projectListResponse {
	agentIDs, deployments := projectDeploymentResponses(project.Deployments)
	return projectListResponse{
		ID:                 project.ID,
		Name:               project.Name,
		Description:        project.Description,
		RunMode:            normalizedProjectRunMode(project.RunMode),
		FrontendRoute:      project.FrontendRoute,
		ContainerRoute:     project.ContainerRoute,
		UseHTTPS:           project.UseHTTPS,
		HasSSLCert:         hasProjectSSLCert(project),
		ContainerStatus:    project.ContainerStatus,
		DockerImage:        project.DockerImage,
		ContainerID:        project.ContainerID,
		Port:               project.Port,
		LoginURL:           project.LoginURL,
		BuildStatus:        project.BuildStatus,
		DeploymentRevision: project.DeploymentRevision,
		TaskID:             project.TaskID,
		CreatedAt:          project.CreatedAt,
		UpdatedAt:          project.UpdatedAt,
		RobotIDs:           toRobotIDs(project.Robots),
		Robots:             toRobotSummaries(project.Robots),
		CredentialCount:    len(project.Credentials),
		Health:             toProjectHealthResponsePtr(health),
		AgentIDs:           agentIDs,
		Deployments:        deployments,
	}
}

func toProjectDetailResponse(project models.Project, health *models.ProjectHealthCheck) projectDetailResponse {
	agentIDs, deployments := projectDeploymentResponses(project.Deployments)
	return projectDetailResponse{
		ID:                 project.ID,
		Name:               project.Name,
		Description:        project.Description,
		RunMode:            normalizedProjectRunMode(project.RunMode),
		FrontendRoute:      project.FrontendRoute,
		ContainerRoute:     project.ContainerRoute,
		UseHTTPS:           project.UseHTTPS,
		HasSSLCert:         hasProjectSSLCert(project),
		ContainerCode:      project.ContainerCode,
		ContainerStatus:    project.ContainerStatus,
		ContainerLog:       project.ContainerLog,
		HtmlFilePath:       project.HtmlFilePath,
		DockerImage:        project.DockerImage,
		ContainerID:        project.ContainerID,
		Port:               project.Port,
		LoginURL:           project.LoginURL,
		BuildStatus:        project.BuildStatus,
		DeploymentRevision: project.DeploymentRevision,
		TaskID:             project.TaskID,
		CreatedAt:          project.CreatedAt,
		UpdatedAt:          project.UpdatedAt,
		Robots:             toRobotSummaries(project.Robots),
		RobotIDs:           toRobotIDs(project.Robots),
		Health:             toProjectHealthResponsePtr(health),
		AgentIDs:           agentIDs,
		Deployments:        deployments,
	}
}

func loadLatestProjectHealth(db *gorm.DB, projectID uint) *models.ProjectHealthCheck {
	var health models.ProjectHealthCheck
	if err := db.Where("project_id = ?", projectID).
		Order("checked_at DESC, id DESC").
		First(&health).Error; err != nil {
		return nil
	}
	return &health
}

func loadLatestProjectHealthMap(db *gorm.DB, projects []models.Project) map[uint]*models.ProjectHealthCheck {
	result := make(map[uint]*models.ProjectHealthCheck, len(projects))
	for _, project := range projects {
		if health := loadLatestProjectHealth(db, project.ID); health != nil {
			result[project.ID] = health
		}
	}
	return result
}

func containerRouteFromForm(c *gin.Context) string {
	return c.PostForm("container_route")
}

func containerRouteFromRequest(req projectUpdateRequest) string {
	return req.ContainerRoute
}

func parseRobotIDs(raw string) ([]uint, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	if strings.HasPrefix(raw, "[") {
		var ids []uint
		if err := json.Unmarshal([]byte(raw), &ids); err == nil {
			return ids, nil
		}
	}

	raw = strings.Trim(raw, "[]")
	parts := strings.Split(raw, ",")
	ids := make([]uint, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		id, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return nil, err
		}
		ids = append(ids, uint(id))
	}
	return ids, nil
}

func loadRobotsByIDs(db *gorm.DB, robotIDs []uint) ([]models.Robot, error) {
	if len(robotIDs) == 0 {
		return nil, nil
	}

	robots := make([]models.Robot, 0, len(robotIDs))
	for _, robotID := range robotIDs {
		var robot models.Robot
		if err := db.First(&robot, robotID).Error; err != nil {
			return nil, err
		}
		robots = append(robots, robot)
	}

	return robots, nil
}

func normalizeRoutePath(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	if !strings.HasPrefix(value, "/") {
		return "/" + value
	}
	return value
}

func normalizedProjectRunMode(value models.ProjectRunMode) models.ProjectRunMode {
	switch value {
	case models.ProjectRunModeLocal:
		return models.ProjectRunModeLocal
	case models.ProjectRunModeDocker:
		return models.ProjectRunModeDocker
	default:
		return models.ProjectRunModeDocker
	}
}

func parseProjectRunMode(value string) (models.ProjectRunMode, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", string(models.ProjectRunModeDocker):
		return models.ProjectRunModeDocker, nil
	case string(models.ProjectRunModeLocal):
		return models.ProjectRunModeLocal, nil
	default:
		return "", fmt.Errorf("invalid run mode: %s", value)
	}
}

func validateProjectName(value string) error {
	name := strings.TrimSpace(value)
	if name == "" {
		return errors.New("project name is required")
	}

	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return errors.New("project name must contain only English letters, numbers, and hyphens, without spaces or other special characters")
		}
	}

	return nil
}

func hasProjectSSLCert(project models.Project) bool {
	return strings.TrimSpace(project.SSLCertPath) != "" && strings.TrimSpace(project.SSLKeyPath) != ""
}

func localProjectAppDir(projectID uint) string {
	return filepath.Join("data", "local_apps", fmt.Sprintf("project_%d", projectID))
}

func localProjectLogPath(projectID uint) string {
	return filepath.Join(localProjectAppDir(projectID), "flask.log")
}

func makeLocalContainerID(pid int) string {
	return fmt.Sprintf("%s%d", localContainerIDPrefix, pid)
}

func parseLocalContainerPID(containerID string) (int, bool) {
	if !strings.HasPrefix(containerID, localContainerIDPrefix) {
		return 0, false
	}

	pid, err := strconv.Atoi(strings.TrimPrefix(containerID, localContainerIDPrefix))
	return pid, err == nil && pid > 0
}

func requestBackendBaseURL(c *gin.Context) string {
	scheme := strings.TrimSpace(c.GetHeader("X-Forwarded-Proto"))
	if scheme == "" {
		scheme = "http"
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}

	host := strings.TrimSpace(c.Request.Host)
	if host == "" {
		host = fmt.Sprintf("127.0.0.1:%d", config.ServerPort())
	}

	return scheme + "://" + host
}

func projectServiceURL(c *gin.Context, project models.Project) string {
	host := c.Request.Host
	if host == "" {
		host = "127.0.0.1"
	}
	for i, r := range host {
		if r == ':' {
			host = host[:i]
			break
		}
	}

	protocol := "http"
	if project.UseHTTPS {
		protocol = "https"
	}

	route := normalizeRoutePath(project.FrontendRoute, "/")
	if route == "/" {
		route = ""
	}

	return fmt.Sprintf("%s://%s:%d%s", protocol, host, project.Port, route)
}

func prepareProjectRuntimeFiles(project *models.Project, targetDir string) (string, error) {
	if strings.TrimSpace(targetDir) == "" {
		return "", errors.New("runtime directory is required")
	}
	if project.HtmlFilePath == "" {
		return "", errors.New("no HTML file uploaded")
	}
	if _, err := os.Stat(project.HtmlFilePath); err != nil {
		return "", fmt.Errorf("HTML file not found: %w", err)
	}

	project.FrontendRoute = normalizeRoutePath(project.FrontendRoute, "/")
	project.ContainerRoute = normalizeRoutePath(project.ContainerRoute, "/api/submit")

	if err := os.RemoveAll(targetDir); err != nil {
		return "", fmt.Errorf("failed to reset runtime directory: %w", err)
	}

	templatesDir := filepath.Join(targetDir, "templates")
	staticDir := filepath.Join(targetDir, "static")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create templates directory: %w", err)
	}
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create static directory: %w", err)
	}

	if err := writeProjectIndexHTML(project, filepath.Join(templatesDir, "index.html")); err != nil {
		return "", fmt.Errorf("failed to write HTML file: %w", err)
	}

	robotConfigs := make([]utils.RobotConfig, len(project.Robots))
	for i, robot := range project.Robots {
		robotConfigs[i] = utils.RobotConfig{
			ID:      robot.ID,
			Webhook: robot.Webhook,
			Type:    string(robot.RobotType),
			Secret:  robot.Secret,
		}
	}

	containerSecret := config.ContainerSecret()
	if containerSecret == "" {
		return "", errors.New("CONTAINER_SECRET is not configured")
	}

	containerCode := utils.GenerateContainerCode(
		project.Name,
		project.ID,
		project.FrontendRoute,
		project.ContainerRoute,
		project.LoginURL,
		containerSecret,
		robotConfigs,
	)

	if err := os.WriteFile(filepath.Join(targetDir, "app.py"), []byte(containerCode), 0644); err != nil {
		return "", fmt.Errorf("failed to write app.py: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "requirements.txt"), []byte(flaskRequirementsContent), 0644); err != nil {
		return "", fmt.Errorf("failed to write requirements.txt: %w", err)
	}

	return containerCode, nil
}

func writeProjectIndexHTML(project *models.Project, dstPath string) error {
	rendered, err := utils.RenderProjectHTMLFile(project.HtmlFilePath, utils.ProjectHTMLVars{
		SubmitURL:   project.ContainerRoute,
		RedirectURL: project.LoginURL,
	})
	if err != nil {
		return err
	}
	return os.WriteFile(dstPath, rendered, 0644)
}

func copyFile(srcPath string, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	_, err = io.Copy(dst, src)
	return err
}

func readTailFile(path string, maxBytes int64) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", err
	}

	offset := int64(0)
	if info.Size() > maxBytes {
		offset = info.Size() - maxBytes
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return "", err
	}

	content, err := io.ReadAll(file)
	if err != nil {
		return "", err
	}

	if offset > 0 {
		return "...日志已截断，仅显示最后部分...\n" + string(content), nil
	}
	return string(content), nil
}

func saveUploadedHTMLFile(c *gin.Context) (string, error) {
	file, header, err := c.Request.FormFile("html_file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil
		}
		return "", err
	}
	defer file.Close()

	if header.Size > maxHTMLUploadSizeBytes {
		return "", errHTMLFileTooLarge
	}

	uploadDir := "./data/html_files"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", err
	}

	ext := filepath.Ext(header.Filename)
	filename := strconv.FormatInt(time.Now().UnixNano(), 10) + ext
	filePath := filepath.Join(uploadDir, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		return "", err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, file); err != nil {
		return "", err
	}

	return filePath, nil
}

func saveUploadedCertificateFiles(c *gin.Context) (string, string, error) {
	certFile, certHeader, certErr := c.Request.FormFile("ssl_cert_file")
	if certErr == nil {
		defer certFile.Close()
	}

	keyFile, keyHeader, keyErr := c.Request.FormFile("ssl_key_file")
	if keyErr == nil {
		defer keyFile.Close()
	}

	certMissing := errors.Is(certErr, http.ErrMissingFile)
	keyMissing := errors.Is(keyErr, http.ErrMissingFile)

	if certMissing && keyMissing {
		return "", "", nil
	}
	if certMissing || keyMissing {
		return "", "", errors.New("please upload both SSL certificate and key files, or leave both empty")
	}
	if certErr != nil {
		return "", "", certErr
	}
	if keyErr != nil {
		return "", "", keyErr
	}

	uploadDir := "./data/project_certificates"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", "", err
	}

	saveFile := func(file multipart.File, header *multipart.FileHeader, suffix string) (string, error) {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext == "" {
			ext = ".pem"
		}
		filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), suffix, ext)
		filePath := filepath.Join(uploadDir, filename)

		dst, err := os.Create(filePath)
		if err != nil {
			return "", err
		}
		defer dst.Close()

		if _, err := io.Copy(dst, file); err != nil {
			return "", err
		}

		return filePath, nil
	}

	certPath, err := saveFile(certFile, certHeader, "cert")
	if err != nil {
		return "", "", err
	}

	keyPath, err := saveFile(keyFile, keyHeader, "key")
	if err != nil {
		return "", "", err
	}

	return certPath, keyPath, nil
}

// CreateProject creates a new project
func CreateProject(c *gin.Context) {
	var project models.Project
	var selectedAgentIDs []uint

	// Parse multipart form
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data", "details": err.Error()})
		return
	}

	// Extract form fields
	project.Name = c.PostForm("name")
	project.Name = strings.TrimSpace(project.Name)
	project.Description = c.PostForm("description")
	runMode, err := parseProjectRunMode(c.PostForm("run_mode"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	project.RunMode = runMode
	project.FrontendRoute = c.PostForm("frontend_route")
	project.ContainerRoute = containerRouteFromForm(c)
	project.LoginURL = c.PostForm("login_url")
	project.UseHTTPS = strings.EqualFold(strings.TrimSpace(c.PostForm("use_https")), "true")

	// Parse port
	portStr := c.PostForm("port")
	if portStr != "" {
		if port, err := strconv.ParseUint(portStr, 10, 32); err == nil {
			project.Port = uint(port)
		}
	}

	// Handle robots array
	robotsStr := c.PostForm("robots")
	if robotsStr != "" {
		db := config.GetDB()
		robotIDs, err := parseRobotIDs(robotsStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid robots field", "details": err.Error()})
			return
		}
		project.Robots, err = loadRobotsByIDs(db, robotIDs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid robot selection", "details": err.Error()})
			return
		}
	}
	agentsStr := c.PostForm("agent_ids")
	if agentsStr != "" {
		selectedAgentIDs, err = parseRobotIDs(agentsStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent_ids field", "details": err.Error()})
			return
		}
	}

	// Handle file upload
	filePath, err := saveUploadedHTMLFile(c)
	if err != nil {
		if errors.Is(err, errHTMLFileTooLarge) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "HTML file exceeds the 30MB upload limit"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded HTML", "details": err.Error()})
		return
	}
	if filePath != "" {
		project.HtmlFilePath = filePath
	}

	certPath, keyPath, err := saveUploadedCertificateFiles(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to process SSL certificate files", "details": err.Error()})
		return
	}
	if certPath != "" && keyPath != "" {
		project.SSLCertPath = certPath
		project.SSLKeyPath = keyPath
	}

	if project.UseHTTPS && !hasProjectSSLCert(project) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SSL certificate and key files are required when SSL is enabled"})
		return
	}

	// Set default values
	project.FrontendRoute = normalizeRoutePath(project.FrontendRoute, "/")
	project.ContainerRoute = normalizeRoutePath(project.ContainerRoute, "/api/submit")
	project.RunMode = normalizedProjectRunMode(project.RunMode)
	if project.ContainerStatus == "" {
		project.ContainerStatus = models.ContainerStatusStopped
	}
	if project.BuildStatus == "" {
		project.BuildStatus = models.BuildStatusPending
	}
	if project.Port == 0 {
		project.Port = 6000
	}
	project.DeploymentRevision = 1

	// Validate required fields
	if err := validateProjectName(project.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	project.CreatedBy = user.ID
	db := config.GetDB()
	if !ensureOwnedRobotIDs(c, db, user, robotIDsFromModels(project.Robots)) {
		return
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		return syncProjectDeployments(tx, &project, selectedAgentIDs)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create project", "details": err.Error()})
		return
	}

	// Reload project with associations
	db.Preload("Robots").Preload("Deployments.Agent").First(&project, project.ID)

	c.JSON(http.StatusCreated, toProjectDetailResponse(project, nil))
}

// GetProjects retrieves all projects
func GetProjects(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var projects []models.Project

	if err := scopeByOwner(db, user).Preload("Robots").Preload("Deployments.Agent").Preload("Credentials", func(tx *gorm.DB) *gorm.DB {
		return tx.Select("id", "project_id")
	}).Find(&projects).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve projects"})
		return
	}

	healthByProjectID := loadLatestProjectHealthMap(db, projects)
	response := make([]projectListResponse, 0, len(projects))
	for _, project := range projects {
		response = append(response, toProjectListResponse(project, healthByProjectID[project.ID]))
	}

	c.JSON(http.StatusOK, response)
}

// GetProject retrieves a single project by ID
func GetProject(c *gin.Context) {
	id := c.Param("id")
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var project models.Project

	if err := scopeByOwner(db, user).Preload("Robots").Preload("Deployments.Agent").First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	c.JSON(http.StatusOK, toProjectDetailResponse(project, loadLatestProjectHealth(db, project.ID)))
}

// UpdateProject updates an existing project
func UpdateProject(c *gin.Context) {
	id := c.Param("id")
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()

	var project models.Project
	if err := scopeByOwner(db, user).Preload("Deployments").First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	ownerID := project.CreatedBy
	oldRunMode := normalizedProjectRunMode(project.RunMode)
	var selectedAgentIDs []uint

	contentType := c.GetHeader("Content-Type")
	if strings.HasPrefix(contentType, "multipart/form-data") {
		if err := c.Request.ParseMultipartForm(32 << 20); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to parse form data", "details": err.Error()})
			return
		}

		project.Name = strings.TrimSpace(c.PostForm("name"))
		project.Description = c.PostForm("description")
		runMode, err := parseProjectRunMode(c.PostForm("run_mode"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		project.RunMode = runMode
		project.FrontendRoute = c.PostForm("frontend_route")
		project.ContainerRoute = containerRouteFromForm(c)
		project.LoginURL = c.PostForm("login_url")
		project.UseHTTPS = strings.EqualFold(strings.TrimSpace(c.PostForm("use_https")), "true")
		selectedAgentIDs, err = parseRobotIDs(c.PostForm("agent_ids"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent_ids field", "details": err.Error()})
			return
		}

		portStr := c.PostForm("port")
		if portStr != "" {
			port, err := strconv.ParseUint(portStr, 10, 32)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid port"})
				return
			}
			project.Port = uint(port)
		}

		robotIDs, err := parseRobotIDs(c.PostForm("robots"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid robots field", "details": err.Error()})
			return
		}
		project.Robots, err = loadRobotsByIDs(db, robotIDs)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid robot selection", "details": err.Error()})
			return
		}

		filePath, err := saveUploadedHTMLFile(c)
		if err != nil {
			if errors.Is(err, errHTMLFileTooLarge) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "HTML file exceeds the 30MB upload limit"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save uploaded HTML", "details": err.Error()})
			return
		}
		if filePath != "" {
			project.HtmlFilePath = filePath
		}

		certPath, keyPath, err := saveUploadedCertificateFiles(c)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to process SSL certificate files", "details": err.Error()})
			return
		}
		if certPath != "" && keyPath != "" {
			project.SSLCertPath = certPath
			project.SSLKeyPath = keyPath
		}
	} else {
		var req projectUpdateRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
			return
		}

		project.Name = strings.TrimSpace(req.Name)
		project.Description = req.Description
		runMode, err := parseProjectRunMode(req.RunMode)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		project.RunMode = runMode
		project.FrontendRoute = req.FrontendRoute
		project.ContainerRoute = containerRouteFromRequest(req)
		project.LoginURL = req.LoginURL
		project.UseHTTPS = req.UseHTTPS
		project.Port = req.Port
		selectedAgentIDs = req.AgentIDs

		robots, err := loadRobotsByIDs(db, req.Robots)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid robot selection", "details": err.Error()})
			return
		}
		project.Robots = robots
	}

	if project.UseHTTPS && !hasProjectSSLCert(project) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SSL certificate and key files are required when SSL is enabled"})
		return
	}

	if err := validateProjectName(project.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !ensureOwnedRobotIDs(c, db, user, robotIDsFromModels(project.Robots)) {
		return
	}

	project.FrontendRoute = normalizeRoutePath(project.FrontendRoute, "/")
	project.ContainerRoute = normalizeRoutePath(project.ContainerRoute, "/api/submit")
	project.RunMode = normalizedProjectRunMode(project.RunMode)
	project.CreatedBy = ownerID

	if project.ContainerStatus == models.ContainerStatusRunning && oldRunMode != project.RunMode {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Please stop the running service before changing run mode"})
		return
	}
	if oldRunMode != project.RunMode {
		project.BuildStatus = models.BuildStatusPending
		project.TaskID = ""
		project.DockerImage = ""
		project.ContainerCode = ""
		project.ContainerLog = "执行方式已切换，请重新构建或生成运行代码"
	}

	project.DeploymentRevision++
	if project.DeploymentRevision < 1 {
		project.DeploymentRevision = 1
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&project).Error; err != nil {
			return err
		}
		if err := tx.Model(&project).Association("Robots").Replace(project.Robots); err != nil {
			return err
		}
		return syncProjectDeployments(tx, &project, selectedAgentIDs)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project", "details": err.Error()})
		return
	}

	if err := db.Preload("Robots").Preload("Deployments.Agent").First(&project, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reload project"})
		return
	}

	c.JSON(http.StatusOK, toProjectDetailResponse(project, loadLatestProjectHealth(db, project.ID)))
}

// DeleteProject deletes a project
func DeleteProject(c *gin.Context) {
	id := c.Param("id")
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()

	var project models.Project
	if err := scopeByOwner(db, user).First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	// Note: In production, you would also:
	// 1. Stop and remove Docker containers
	// 2. Delete Docker images
	// 3. Delete HTML files
	// 4. Delete associated credentials and messages
	if pid, ok := parseLocalContainerPID(project.ContainerID); ok {
		if err := utils.StopLocalProcess(pid); err != nil {
			log.Printf("[DeleteProject] WARNING: failed to stop local Flask process %d: %v", pid, err)
		}
	}
	if err := os.RemoveAll(localProjectAppDir(project.ID)); err != nil {
		log.Printf("[DeleteProject] WARNING: failed to remove local app directory: %v", err)
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectLog{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.AgentTask{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectDeployment{}).Error; err != nil {
			return err
		}
		return tx.Delete(&project).Error
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete project"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// BuildProject starts the Docker build process
func BuildProject(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var project models.Project
	if err := db.Preload("Robots").First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	// Check if already building
	if project.BuildStatus == models.BuildStatusBuilding {
		c.JSON(http.StatusConflict, gin.H{"status": "already_building", "task_id": project.TaskID})
		return
	}

	// Check if HTML file exists
	if project.HtmlFilePath == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No HTML file uploaded"})
		return
	}

	project.RunMode = normalizedProjectRunMode(project.RunMode)
	if project.RunMode == models.ProjectRunModeLocal && project.ContainerStatus == models.ContainerStatusRunning {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "msg": "本地 Flask 正在运行，请先停止后再重新生成代码。"})
		return
	}

	// Generate a task ID
	taskID := "task_" + strconv.Itoa(int(project.ID)) + "_" + strconv.FormatInt(time.Now().Unix(), 10)

	// Update project with task ID and building status
	project.BuildStatus = models.BuildStatusBuilding
	project.TaskID = taskID
	project.DockerImage = ""
	if err := db.Save(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update project", "details": err.Error()})
		return
	}

	// Start background build process
	go func() {
		if project.RunMode == models.ProjectRunModeLocal {
			buildLocalProjectInBackground(db, project.ID, taskID)
			return
		}
		buildProjectInBackground(db, project.ID, taskID)
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"status":   "build_started",
		"run_mode": project.RunMode,
		"task_id":  taskID,
	})
}

func buildLocalProjectInBackground(db *gorm.DB, projectID uint, taskID string) {
	log.Println("[Local Build Background] === Starting local Flask build for project", projectID, "task:", taskID)

	db = config.GetDB()
	var project models.Project
	if err := db.Preload("Robots").First(&project, projectID).Error; err != nil {
		log.Println("[Local Build Background] ERROR: Failed to fetch project:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to fetch project")
		return
	}

	project.RunMode = normalizedProjectRunMode(project.RunMode)
	if project.RunMode != models.ProjectRunModeLocal {
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Project is not configured for local Flask mode")
		return
	}

	updateBuildProgress(db, projectID, 20, "Preparing local Flask runtime...")
	appDir := localProjectAppDir(project.ID)
	containerCode, err := prepareProjectRuntimeFiles(&project, appDir)
	if err != nil {
		log.Println("[Local Build Background] ERROR: Failed to prepare runtime files:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Local Flask build failed: "+err.Error())
		return
	}

	updateBuildProgress(db, projectID, 100, "Local Flask runtime generated successfully!")

	updates := map[string]interface{}{
		"RunMode":       models.ProjectRunModeLocal,
		"DockerImage":   "",
		"ContainerCode": containerCode,
		"ContainerLog":  "本地 Flask 代码已生成\n运行目录: " + appDir,
		"BuildStatus":   models.BuildStatusSuccess,
	}
	if err := db.Model(&models.Project{}).Where("id = ?", projectID).Updates(updates).Error; err != nil {
		log.Println("[Local Build Background] ERROR: Failed to update project:", err)
	} else {
		log.Println("[Local Build Background] ✅ Local Flask runtime generated for project", projectID)
	}
}

// buildProjectInBackground runs the build process in the background
func buildProjectInBackground(db *gorm.DB, projectID uint, taskID string) {
	log.Println("[Build Background] === Starting Docker build for project", projectID, "task:", taskID)

	// Get a fresh database connection and project data
	db = config.GetDB()
	var project models.Project
	if err := db.Preload("Robots").First(&project, projectID).Error; err != nil {
		log.Println("[Build Background] ERROR: Failed to fetch project:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to fetch project")
		return
	}

	project.FrontendRoute = normalizeRoutePath(project.FrontendRoute, "/")
	project.ContainerRoute = normalizeRoutePath(project.ContainerRoute, "/api/submit")
	if err := db.Model(&models.Project{}).Where("id = ?", projectID).Updates(map[string]interface{}{
		"FrontendRoute":  project.FrontendRoute,
		"ContainerRoute": project.ContainerRoute,
	}).Error; err != nil {
		log.Println("[Build Background] ERROR: Failed to normalize routes:", err)
	}

	log.Printf("[Build Background] Project: %s, HTML: %s", project.Name, project.HtmlFilePath)

	// Check HTML file exists
	if project.HtmlFilePath == "" {
		log.Println("[Build Background] ERROR: No HTML file path")
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "No HTML file uploaded")
		return
	}

	if _, err := os.Stat(project.HtmlFilePath); os.IsNotExist(err) {
		log.Println("[Build Background] ERROR: HTML file does not exist:", project.HtmlFilePath)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "HTML file not found")
		return
	}

	// Update progress: 10% - Creating build context
	updateBuildProgress(db, projectID, 10, "Creating build context...")

	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "container-build-*")
	if err != nil {
		log.Println("[Build Background] ERROR: Failed to create temp dir:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to create build directory")
		return
	}
	defer os.RemoveAll(tempDir)

	log.Println("[Build Background] Created temp directory:", tempDir)

	// Create directory structure
	templatesDir := filepath.Join(tempDir, "templates")
	staticDir := filepath.Join(tempDir, "static")
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		log.Println("[Build Background] ERROR: Failed to create templates dir:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to create templates directory")
		return
	}
	if err := os.MkdirAll(staticDir, 0755); err != nil {
		log.Println("[Build Background] ERROR: Failed to create static dir:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to create static directory")
		return
	}

	// Update progress: 20% - Copying HTML file
	updateBuildProgress(db, projectID, 20, "Copying HTML file...")

	// Write HTML with project placeholders resolved.
	dstHTML := filepath.Join(templatesDir, "index.html")
	if err := writeProjectIndexHTML(&project, dstHTML); err != nil {
		log.Println("[Build Background] ERROR: Failed to write index.html:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to write HTML file")
		return
	}
	log.Println("[Build Background] HTML file written successfully")

	// Update progress: 40% - Generating container code
	updateBuildProgress(db, projectID, 40, "Generating container application code...")

	// Prepare robot configs
	robotConfigs := make([]utils.RobotConfig, len(project.Robots))
	for i, robot := range project.Robots {
		robotConfigs[i] = utils.RobotConfig{
			ID:      robot.ID,
			Webhook: robot.Webhook,
			Type:    string(robot.RobotType),
			Secret:  robot.Secret,
		}
	}

	// Get container secret from configuration
	containerSecret := config.ContainerSecret()
	if containerSecret == "" {
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "CONTAINER_SECRET is not configured")
		return
	}

	// Generate container application code
	containerCode := utils.GenerateContainerCode(
		project.Name,
		project.ID,
		project.FrontendRoute,
		project.ContainerRoute,
		project.LoginURL,
		containerSecret,
		robotConfigs,
	)

	// Save container application code to file
	appPath := filepath.Join(tempDir, "app.py")
	if err := os.WriteFile(appPath, []byte(containerCode), 0644); err != nil {
		log.Println("[Build Background] ERROR: Failed to write app.py:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to write container code")
		return
	}
	log.Println("[Build Background] Container code generated successfully")

	// Update progress: 50% - Creating Dockerfile and requirements
	updateBuildProgress(db, projectID, 50, "Creating Dockerfile and requirements...")

	// Create requirements.txt
	requirementsPath := filepath.Join(tempDir, "requirements.txt")
	if err := os.WriteFile(requirementsPath, []byte(flaskRequirementsContent), 0644); err != nil {
		log.Println("[Build Background] ERROR: Failed to write requirements.txt:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to create requirements.txt")
		return
	}

	// Create Dockerfile
	dockerfileContent := fmt.Sprintf(`FROM %s
WORKDIR /app
COPY . /app
RUN pip install -i https://mirrors.aliyun.com/pypi/simple -r requirements.txt
EXPOSE 5000
CMD ["python", "app.py"]
`, utils.RequiredProjectBaseImage)
	if err := os.WriteFile(filepath.Join(tempDir, "Dockerfile"), []byte(dockerfileContent), 0644); err != nil {
		log.Println("[Build Background] ERROR: Failed to write Dockerfile:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to create Dockerfile")
		return
	}
	log.Println("[Build Background] Dockerfile created successfully")

	// Update progress: 60% - Starting Docker build
	updateBuildProgress(db, projectID, 60, "Building Docker image...")

	// Initialize Docker client
	dockerClient, err := utils.NewDockerClient()
	if err != nil {
		log.Println("[Build Background] ERROR: Failed to create Docker client:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Failed to connect to Docker. Please ensure Docker is running.")
		return
	}

	// List files in tempDir for debugging
	log.Println("[Build Background] Files in build context:")
	filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(tempDir, path)
		log.Printf("[Build Background]   %s", relPath)
		return nil
	})

	// Verify Dockerfile exists
	if _, err := os.Stat(filepath.Join(tempDir, "Dockerfile")); os.IsNotExist(err) {
		log.Println("[Build Background] ERROR: Dockerfile does not exist")
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Dockerfile not found")
		return
	} else {
		log.Println("[Build Background] Dockerfile verified")
	}

	// Build Docker image
	imageName := buildDockerImageName(project.ID, project.Name, time.Now())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	log.Println("[Build Background] Starting Docker build for image:", imageName)
	if err := dockerClient.BuildImage(ctx, tempDir, imageName); err != nil {
		log.Println("[Build Background] ERROR: Docker build failed:", err)
		updateBuildStatus(db, projectID, models.BuildStatusFailed, "Docker build failed: "+err.Error())
		return
	}

	// Update progress: 100% - Build complete
	updateBuildProgress(db, projectID, 100, "Build completed successfully!")

	// Update project with success status
	updates := map[string]interface{}{
		"DockerImage":   imageName,
		"ContainerCode": containerCode,
		"BuildStatus":   models.BuildStatusSuccess,
	}

	if err := db.Model(&models.Project{}).Where("id = ?", projectID).Updates(updates).Error; err != nil {
		log.Println("[Build Background] ERROR: Failed to update project:", err)
	} else {
		log.Println("[Build Background] ✅ Build completed successfully for project", projectID)
		log.Println("[Build Background] Docker image:", imageName)
	}
}

func buildDockerImageName(projectID uint, projectName string, builtAt time.Time) string {
	return fmt.Sprintf("%d-%s-%s", projectID, strings.TrimSpace(projectName), builtAt.Format("20060102150405"))
}

// updateBuildProgress updates build progress
func updateBuildProgress(db *gorm.DB, projectID uint, progress int, status string) {
	log.Printf("[Build Background] Progress: %d%% - %s", progress, status)
	// TODO: Store progress in a separate table or use Redis for real-time updates
}

// updateBuildStatus updates build status and saves to database
func updateBuildStatus(db *gorm.DB, projectID uint, status models.BuildStatus, message string) {
	updates := map[string]interface{}{
		"BuildStatus":  status,
		"ContainerLog": message,
	}
	if err := db.Model(&models.Project{}).Where("id = ?", projectID).Updates(updates).Error; err != nil {
		log.Println("[Build Background] ERROR: Failed to update build status:", err)
	}
}

// BuildStatus retrieves the build status of a project
func BuildStatus(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var project models.Project
	if err := db.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	response := gin.H{
		"status":   project.BuildStatus,
		"run_mode": normalizedProjectRunMode(project.RunMode),
		"progress": gin.H{
			"current":    0,
			"total":      100,
			"percentage": 0,
			"status":     "等待中...",
		},
		"db_status": project.BuildStatus,
	}

	if project.BuildStatus == models.BuildStatusBuilding {
		response["progress"] = gin.H{
			"current":    50,
			"total":      100,
			"percentage": 50,
			"status":     "构建中...",
		}
	}

	if project.BuildStatus == models.BuildStatusSuccess {
		response["progress"] = gin.H{
			"current":    100,
			"total":      100,
			"percentage": 100,
			"status":     "构建成功！",
		}
	}

	if project.BuildStatus == models.BuildStatusFailed {
		response["progress"] = gin.H{
			"current":    100,
			"total":      100,
			"percentage": 100,
			"status":     "构建失败",
		}
	}

	c.JSON(http.StatusOK, response)
}

// StartLocalFlask generates the Flask runtime files and starts them as a local Python process.
func StartLocalFlask(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var project models.Project
	if err := db.Preload("Robots").First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	project.RunMode = normalizedProjectRunMode(project.RunMode)
	if project.RunMode != models.ProjectRunModeLocal {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "该项目配置为 Docker 运行，请使用容器启动。"})
		return
	}

	if project.Port == 0 {
		project.Port = 6000
	}

	if project.ContainerStatus == models.ContainerStatusRunning && project.ContainerID != "" {
		if _, ok := parseLocalContainerPID(project.ContainerID); ok {
			c.JSON(http.StatusOK, gin.H{
				"status":       "already_running",
				"run_mode":     "local",
				"container_id": project.ContainerID,
				"port":         project.Port,
				"url":          projectServiceURL(c, project),
			})
			return
		}

		c.JSON(http.StatusConflict, gin.H{"status": "error", "msg": "Docker 容器正在运行，请先停止后再启动本地 Flask。"})
		return
	}

	if project.ContainerStatus == models.ContainerStatusStarting || project.ContainerStatus == models.ContainerStatusStopping {
		c.JSON(http.StatusConflict, gin.H{"status": "busy", "msg": "项目正在启动或停止，请稍后再试。"})
		return
	}

	if project.UseHTTPS && !hasProjectSSLCert(project) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "已启用SSL，但项目未配置证书和私钥文件。"})
		return
	}

	project.ContainerStatus = models.ContainerStatusStarting
	project.ContainerLog = "正在准备本地 Flask 运行目录..."
	if err := db.Save(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "更新项目状态失败"})
		return
	}

	appDir := localProjectAppDir(project.ID)
	containerCode, err := prepareProjectRuntimeFiles(&project, appDir)
	if err != nil {
		project.ContainerStatus = models.ContainerStatusStopped
		project.ContainerLog = "本地 Flask 准备失败: " + err.Error()
		project.BuildStatus = models.BuildStatusFailed
		db.Save(&project)
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": project.ContainerLog})
		return
	}

	project.ContainerCode = containerCode
	project.BuildStatus = models.BuildStatusSuccess
	project.ContainerLog = "本地 Flask 运行目录已生成，正在启动..."
	if err := db.Save(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "保存本地 Flask 代码失败"})
		return
	}

	process, err := utils.StartLocalFlaskApp(utils.LocalFlaskConfig{
		AppDir:         appDir,
		LogPath:        localProjectLogPath(project.ID),
		Port:           project.Port,
		BackendBaseURL: requestBackendBaseURL(c),
		UseHTTPS:       project.UseHTTPS,
		SSLCertPath:    project.SSLCertPath,
		SSLKeyPath:     project.SSLKeyPath,
	})
	if err != nil {
		project.ContainerStatus = models.ContainerStatusStopped
		project.ContainerID = ""
		project.ContainerLog = fmt.Sprintf("本地 Flask 启动失败: %s\n运行目录: %s", err.Error(), appDir)
		db.Save(&project)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": project.ContainerLog})
		return
	}

	project.ContainerStatus = models.ContainerStatusRunning
	project.ContainerID = makeLocalContainerID(process.PID)
	project.ContainerLog = fmt.Sprintf("本地 Flask 已启动\n进程ID: %d\n运行目录: %s\nPython: %s", process.PID, appDir, process.PythonPath)
	if err := db.Save(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "保存本地 Flask 启动状态失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "started",
		"run_mode":     "local",
		"container_id": project.ContainerID,
		"port":         project.Port,
		"url":          projectServiceURL(c, project),
	})
}

// StopLocalFlask stops a local Flask process for a project.
func StopLocalFlask(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var project models.Project
	if err := db.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	pid, ok := parseLocalContainerPID(project.ContainerID)
	if !ok {
		if project.ContainerStatus != models.ContainerStatusRunning {
			c.JSON(http.StatusBadRequest, gin.H{"status": "already_stopped", "msg": "该项目已停止"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "当前运行的不是本地 Flask 进程，请使用容器停止操作。"})
		return
	}

	if err := stopLocalFlaskProject(db, &project, pid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":   "stopped",
		"run_mode": "local",
		"msg":      "本地 Flask 服务已停止",
	})
}

// RestartLocalFlask restarts the local Flask process for a project.
func RestartLocalFlask(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var project models.Project
	if err := db.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}
	if normalizedProjectRunMode(project.RunMode) != models.ProjectRunModeLocal {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "该项目配置为 Docker 运行，请使用容器重启。"})
		return
	}

	if pid, ok := parseLocalContainerPID(project.ContainerID); ok {
		if err := stopLocalFlaskProject(db, &project, pid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": err.Error()})
			return
		}
	}

	StartLocalFlask(c)
}

func stopLocalFlaskProject(db *gorm.DB, project *models.Project, pid int) error {
	project.ContainerStatus = models.ContainerStatusStopping
	project.ContainerLog = "正在停止本地 Flask 服务..."
	if err := db.Save(project).Error; err != nil {
		return fmt.Errorf("更新项目状态失败")
	}

	if err := utils.StopLocalProcess(pid); err != nil {
		project.ContainerLog = "本地 Flask 停止时返回错误: " + err.Error()
	} else {
		project.ContainerLog = fmt.Sprintf("本地 Flask 已停止\n进程ID: %d", pid)
	}

	project.ContainerStatus = models.ContainerStatusStopped
	project.ContainerID = ""
	if err := db.Save(project).Error; err != nil {
		return fmt.Errorf("保存项目停止状态失败")
	}

	return nil
}

// StartContainer starts the generated container for a project
func StartContainer(c *gin.Context) {
	id := c.Param("id")
	log.Printf("[StartContainer] Starting container for project ID: %s", id)

	db := config.GetDB()

	var project models.Project
	if err := db.First(&project, id).Error; err != nil {
		log.Printf("[StartContainer] ERROR: Project not found: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	log.Printf("[StartContainer] Project: %s, BuildStatus: %s, DockerImage: %s",
		project.Name, project.BuildStatus, project.DockerImage)

	project.RunMode = normalizedProjectRunMode(project.RunMode)
	if project.RunMode != models.ProjectRunModeDocker {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "该项目配置为本地 Flask 运行，请使用本地启动。"})
		return
	}

	if _, ok := parseLocalContainerPID(project.ContainerID); ok && project.ContainerStatus == models.ContainerStatusRunning {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "msg": "本地 Flask 服务正在运行，请先停止后再启动 Docker 容器。"})
		return
	}

	if project.BuildStatus != models.BuildStatusSuccess || project.DockerImage == "" {
		log.Printf("[StartContainer] ERROR: Project not built successfully")
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "项目尚未成功构建，请先构建镜像。"})
		return
	}
	if project.UseHTTPS && !hasProjectSSLCert(project) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "已启用SSL，但项目未配置证书和私钥文件。"})
		return
	}

	// Initialize Docker client
	log.Printf("[StartContainer] Connecting to Docker...")
	dockerClient, err := utils.NewDockerClient()
	if err != nil {
		log.Printf("[StartContainer] ERROR: Failed to connect to Docker: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "Failed to connect to Docker: " + err.Error()})
		return
	}
	log.Printf("[StartContainer] Docker client connected")

	// Update status to starting
	project.ContainerStatus = models.ContainerStatusStarting
	project.ContainerLog = "正在启动容器..."
	if err := db.Save(&project).Error; err != nil {
		log.Printf("[StartContainer] ERROR: Failed to update status: %v", err)
	}
	log.Printf("[StartContainer] Status updated to: %s", project.ContainerStatus)

	// Create container
	log.Printf("[StartContainer] Creating container with image: %s, port: %d", project.DockerImage, project.Port)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	containerID, err := dockerClient.CreateContainer(ctx, project.DockerImage, int(project.Port), project.ID, project.UseHTTPS, project.SSLCertPath, project.SSLKeyPath)
	if err != nil {
		log.Printf("[StartContainer] ERROR: Failed to create container: %v", err)
		project.ContainerStatus = models.ContainerStatusStopped
		project.ContainerLog = "创建容器失败: " + err.Error()
		db.Save(&project)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": project.ContainerLog})
		return
	}
	log.Printf("[StartContainer] Container created with ID: %s", containerID)

	// Start container
	log.Printf("[StartContainer] Starting container %s...", containerID)
	if err := dockerClient.StartContainer(ctx, containerID); err != nil {
		log.Printf("[StartContainer] ERROR: Failed to start container: %v", err)
		project.ContainerStatus = models.ContainerStatusStopped
		project.ContainerLog = "启动容器失败: " + err.Error()
		db.Save(&project)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": project.ContainerLog})
		return
	}
	log.Printf("[StartContainer] Container started successfully")

	// Update project status
	project.ContainerStatus = models.ContainerStatusRunning
	project.ContainerLog = "容器已成功启动"
	project.ContainerID = containerID
	if err := db.Save(&project).Error; err != nil {
		log.Printf("[StartContainer] ERROR: Failed to save project: %v", err)
	}

	host := c.Request.Host
	if len(host) > 0 {
		// Extract host without port
		for i, r := range host {
			if r == ':' {
				host = host[:i]
				break
			}
		}
	}

	log.Printf("[StartContainer] Returning success response")
	c.JSON(http.StatusOK, gin.H{
		"status":       "started",
		"container_id": containerID,
		"port":         project.Port,
		"url":          "http://" + host + ":" + strconv.Itoa(int(project.Port)),
	})
}

// StopContainer stops the generated container for a project
func StopContainer(c *gin.Context) {
	id := c.Param("id")
	log.Printf("[StopContainer] Stopping container for project ID: %s", id)

	db := config.GetDB()

	var project models.Project
	if err := db.First(&project, id).Error; err != nil {
		log.Printf("[StopContainer] ERROR: Project not found: %v", err)
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	log.Printf("[StopContainer] Project: %s, ContainerStatus: %s, ContainerID: %s",
		project.Name, project.ContainerStatus, project.ContainerID)

	if project.ContainerStatus != models.ContainerStatusRunning {
		log.Printf("[StopContainer] Project is not running")
		c.JSON(http.StatusBadRequest, gin.H{"status": "already_stopped", "msg": "该项目已停止"})
		return
	}

	if pid, ok := parseLocalContainerPID(project.ContainerID); ok {
		if err := stopLocalFlaskProject(db, &project, pid); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status":   "stopped",
			"run_mode": "local",
			"msg":      "本地 Flask 服务已停止",
		})
		return
	}

	if project.ContainerID == "" {
		log.Printf("[StopContainer] ERROR: No container ID found for project")
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "未找到容器ID"})
		return
	}

	// Update status to stopping
	project.ContainerStatus = models.ContainerStatusStopping
	project.ContainerLog = "正在停止容器..."
	if err := db.Save(&project).Error; err != nil {
		log.Printf("[StopContainer] ERROR: Failed to update status: %v", err)
	}
	log.Printf("[StopContainer] Status updated to: %s", project.ContainerStatus)

	// Initialize Docker client
	log.Printf("[StopContainer] Connecting to Docker...")
	dockerClient, err := utils.NewDockerClient()
	if err != nil {
		log.Printf("[StopContainer] ERROR: Failed to connect to Docker: %v", err)
		project.ContainerStatus = models.ContainerStatusRunning
		project.ContainerLog = "连接Docker失败: " + err.Error()
		db.Save(&project)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": project.ContainerLog})
		return
	}
	log.Printf("[StopContainer] Docker client connected")

	// Stop container
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	log.Printf("[StopContainer] Stopping container %s...", project.ContainerID)
	if err := dockerClient.StopContainer(ctx, project.ContainerID); err != nil {
		log.Printf("[StopContainer] ERROR: Failed to stop container: %v", err)
		project.ContainerStatus = models.ContainerStatusRunning
		project.ContainerLog = "停止容器失败: " + err.Error()
		db.Save(&project)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": project.ContainerLog})
		return
	}
	log.Printf("[StopContainer] Container stopped successfully")

	// Remove container
	log.Printf("[StopContainer] Removing container %s...", project.ContainerID)
	if err := dockerClient.RemoveContainer(ctx, project.ContainerID); err != nil {
		log.Printf("[StopContainer] WARNING: Failed to remove container: %v", err)
		// Continue even if removal fails, as container is stopped
	} else {
		log.Printf("[StopContainer] Container removed successfully")
	}

	// Update project status
	project.ContainerStatus = models.ContainerStatusStopped
	project.ContainerLog = "容器已成功停止并移除"
	project.ContainerID = ""
	if err := db.Save(&project).Error; err != nil {
		log.Printf("[StopContainer] ERROR: Failed to save project: %v", err)
	}

	log.Printf("[StopContainer] Returning success response")
	c.JSON(http.StatusOK, gin.H{
		"status": "stopped",
		"msg":    "容器已成功停止并移除",
	})
}

// RestartContainer restarts the generated container for a project
func RestartContainer(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var project models.Project
	if err := db.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	project.RunMode = normalizedProjectRunMode(project.RunMode)
	if project.RunMode != models.ProjectRunModeDocker {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "该项目配置为本地 Flask 运行，请使用本地重启。"})
		return
	}

	if _, ok := parseLocalContainerPID(project.ContainerID); ok && project.ContainerStatus == models.ContainerStatusRunning {
		c.JSON(http.StatusConflict, gin.H{"status": "error", "msg": "本地 Flask 服务正在运行，请先停止后再重启 Docker 容器。"})
		return
	}

	if project.BuildStatus != models.BuildStatusSuccess || project.DockerImage == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "项目尚未成功构建，请先构建镜像。"})
		return
	}
	if project.UseHTTPS && !hasProjectSSLCert(project) {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "msg": "已启用SSL，但项目未配置证书和私钥文件。"})
		return
	}

	dockerClient, err := utils.NewDockerClient()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "Failed to connect to Docker: " + err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if project.ContainerID != "" {
		if err := dockerClient.StopContainer(ctx, project.ContainerID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "停止容器失败: " + err.Error()})
			return
		}
		if err := dockerClient.RemoveContainer(ctx, project.ContainerID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "移除容器失败: " + err.Error()})
			return
		}
	}

	project.ContainerStatus = models.ContainerStatusStarting
	project.ContainerLog = "正在重启容器..."
	project.ContainerID = ""
	if err := db.Save(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "更新项目状态失败"})
		return
	}

	containerID, err := dockerClient.CreateContainer(ctx, project.DockerImage, int(project.Port), project.ID, project.UseHTTPS, project.SSLCertPath, project.SSLKeyPath)
	if err != nil {
		project.ContainerStatus = models.ContainerStatusStopped
		project.ContainerLog = "创建容器失败: " + err.Error()
		db.Save(&project)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": project.ContainerLog})
		return
	}

	if err := dockerClient.StartContainer(ctx, containerID); err != nil {
		project.ContainerStatus = models.ContainerStatusStopped
		project.ContainerLog = "启动容器失败: " + err.Error()
		db.Save(&project)
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": project.ContainerLog})
		return
	}

	project.ContainerStatus = models.ContainerStatusRunning
	project.ContainerLog = "容器已成功重启"
	project.ContainerID = containerID
	if err := db.Save(&project).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"status": "error", "msg": "保存项目状态失败"})
		return
	}

	host := c.Request.Host
	if len(host) > 0 {
		for i, r := range host {
			if r == ':' {
				host = host[:i]
				break
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":       "started",
		"container_id": containerID,
		"port":         project.Port,
		"url":          "http://" + host + ":" + strconv.Itoa(int(project.Port)),
	})
}

type projectHTTPProbeResult struct {
	OK         bool
	URL        string
	HTTPStatus int
	LatencyMS  int64
	Error      string
}

func normalizeProjectHealthStatus(status models.ProjectHealthStatus) models.ProjectHealthStatus {
	switch status {
	case models.ProjectHealthStatusHealthy,
		models.ProjectHealthStatusUnhealthy,
		models.ProjectHealthStatusStopped,
		models.ProjectHealthStatusUnknown:
		return status
	default:
		return models.ProjectHealthStatusUnknown
	}
}

func normalizeContainerStatus(status models.ContainerStatus) models.ContainerStatus {
	switch status {
	case models.ContainerStatusRunning,
		models.ContainerStatusStopped,
		models.ContainerStatusStarting,
		models.ContainerStatusStopping:
		return status
	default:
		return models.ContainerStatusStopped
	}
}

func projectProbeBaseURL(project models.Project) string {
	protocol := "http"
	if project.UseHTTPS {
		protocol = "https"
	}
	if project.Port == 0 {
		project.Port = 6000
	}
	return fmt.Sprintf("%s://127.0.0.1:%d", protocol, project.Port)
}

func projectRuntimeHealthURL(project models.Project) string {
	return projectProbeBaseURL(project) + "/_fp_health"
}

func projectRuntimePageURL(project models.Project) string {
	route := normalizeRoutePath(project.FrontendRoute, "/")
	if route == "/" {
		route = ""
	}
	return projectProbeBaseURL(project) + route
}

func probeHTTPURL(ctx context.Context, targetURL string, useHTTPS bool) projectHTTPProbeResult {
	result := projectHTTPProbeResult{URL: targetURL}
	if strings.TrimSpace(targetURL) == "" {
		result.Error = "health check URL is empty"
		return result
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if useHTTPS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	client := &http.Client{
		Timeout:   5 * time.Second,
		Transport: transport,
	}

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, targetURL, nil)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	req.Header.Set("User-Agent", "FishingPlatformHealthCheck/1.0")

	startedAt := time.Now()
	resp, err := client.Do(req)
	result.LatencyMS = time.Since(startedAt).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1024))
	result.HTTPStatus = resp.StatusCode
	result.OK = resp.StatusCode >= 200 && resp.StatusCode < 400
	if !result.OK {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
	}
	return result
}

func probeProjectHTTP(ctx context.Context, project models.Project) projectHTTPProbeResult {
	healthProbe := probeHTTPURL(ctx, projectRuntimeHealthURL(project), project.UseHTTPS)
	if healthProbe.OK {
		return healthProbe
	}
	if healthProbe.HTTPStatus != http.StatusNotFound && healthProbe.HTTPStatus != http.StatusMethodNotAllowed {
		return healthProbe
	}
	return probeHTTPURL(ctx, projectRuntimePageURL(project), project.UseHTTPS)
}

func dockerStatusToContainerStatus(status string) models.ContainerStatus {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running":
		return models.ContainerStatusRunning
	case "created", "restarting":
		return models.ContainerStatusStarting
	case "removing", "paused":
		return models.ContainerStatusStopping
	default:
		return models.ContainerStatusStopped
	}
}

func inspectProjectRuntime(ctx context.Context, project models.Project) (bool, models.ContainerStatus, string, error) {
	project.RunMode = normalizedProjectRunMode(project.RunMode)
	if project.RunMode == models.ProjectRunModeLocal || strings.HasPrefix(project.ContainerID, localContainerIDPrefix) {
		if utils.IsTCPPortOpen("127.0.0.1", project.Port, 800*time.Millisecond) {
			return true, models.ContainerStatusRunning, fmt.Sprintf("Local Flask port %d is open", project.Port), nil
		}
		return false, models.ContainerStatusStopped, fmt.Sprintf("Local Flask port %d is closed", project.Port), nil
	}

	if strings.TrimSpace(project.ContainerID) == "" {
		return false, models.ContainerStatusStopped, "Docker container ID is empty", nil
	}

	dockerClient, err := utils.NewDockerClient()
	if err != nil {
		return false, normalizeContainerStatus(project.ContainerStatus), "Unable to connect to Docker", err
	}

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	status, err := dockerClient.GetContainerStatus(probeCtx, project.ContainerID)
	if err != nil {
		return false, normalizeContainerStatus(project.ContainerStatus), "Unable to inspect Docker container", err
	}

	containerStatus := dockerStatusToContainerStatus(status)
	if containerStatus == models.ContainerStatusRunning {
		return true, containerStatus, "Docker container is running", nil
	}
	return false, containerStatus, "Docker container status is " + status, nil
}

func performProjectHealthCheck(ctx context.Context, project models.Project) models.ProjectHealthCheck {
	project.RunMode = normalizedProjectRunMode(project.RunMode)
	if project.Port == 0 {
		project.Port = 6000
	}

	checkedAt := time.Now()
	health := models.ProjectHealthCheck{
		ProjectID:       project.ID,
		AgentID:         "local",
		AgentName:       "Local Host",
		RunMode:         project.RunMode,
		ContainerStatus: normalizeContainerStatus(project.ContainerStatus),
		Status:          models.ProjectHealthStatusUnknown,
		URL:             projectRuntimeHealthURL(project),
		CheckedAt:       checkedAt,
	}

	runtimeOK, actualStatus, runtimeMessage, err := inspectProjectRuntime(ctx, project)
	health.RuntimeOK = runtimeOK
	health.ContainerStatus = actualStatus
	if err != nil {
		health.Status = models.ProjectHealthStatusUnknown
		health.Message = runtimeMessage
		health.ErrorMessage = err.Error()
		return health
	}
	if !runtimeOK {
		health.Status = models.ProjectHealthStatusStopped
		if project.ContainerStatus == models.ContainerStatusRunning || project.ContainerStatus == models.ContainerStatusStarting {
			health.Message = runtimeMessage + "; local record will be synchronized to stopped"
		} else {
			health.Message = runtimeMessage
		}
		return health
	}

	httpResult := probeProjectHTTP(ctx, project)
	health.URL = httpResult.URL
	health.HTTPStatus = httpResult.HTTPStatus
	health.LatencyMS = httpResult.LatencyMS
	if httpResult.OK {
		health.Status = models.ProjectHealthStatusHealthy
		health.Message = "Runtime is running and HTTP endpoint responded"
		return health
	}

	health.Status = models.ProjectHealthStatusUnhealthy
	health.Message = "Runtime is running but HTTP check failed"
	health.ErrorMessage = httpResult.Error
	return health
}

func syncProjectRuntimeStatusFromHealth(db *gorm.DB, project models.Project, health models.ProjectHealthCheck) {
	updates := map[string]interface{}{}
	switch health.Status {
	case models.ProjectHealthStatusHealthy:
		if project.ContainerStatus != models.ContainerStatusRunning {
			updates["ContainerStatus"] = models.ContainerStatusRunning
			updates["ContainerLog"] = "健康检查确认服务正在运行"
		}
	case models.ProjectHealthStatusStopped:
		if project.ContainerStatus != models.ContainerStatusStopped {
			updates["ContainerStatus"] = models.ContainerStatusStopped
			updates["ContainerLog"] = health.Message
			if strings.HasPrefix(project.ContainerID, localContainerIDPrefix) {
				updates["ContainerID"] = ""
			}
		}
	}

	if len(updates) == 0 {
		return
	}
	if err := db.Model(&models.Project{}).Where("id = ?", project.ID).Updates(updates).Error; err != nil {
		log.Printf("[HealthCheck] failed to sync project %d runtime status: %v", project.ID, err)
	}
}

// CheckProjectHealth checks the current runtime from the platform host and records the result.
func CheckProjectHealth(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	db := config.GetDB()
	var project models.Project
	if err := db.First(&project, uint(projectID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	health := performProjectHealthCheck(c.Request.Context(), project)
	if err := db.Create(&health).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save health check", "details": err.Error()})
		return
	}
	syncProjectRuntimeStatusFromHealth(db, project, health)

	c.JSON(http.StatusOK, gin.H{
		"status": health.Status,
		"health": toProjectHealthResponse(health),
	})
}

// GetProjectHealthChecks returns recent health check logs for a project.
func GetProjectHealthChecks(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	limit := 20
	if rawLimit := strings.TrimSpace(c.Query("limit")); rawLimit != "" {
		if parsed, err := strconv.Atoi(rawLimit); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 100 {
		limit = 100
	}

	db := config.GetDB()
	var project models.Project
	if err := db.First(&project, uint(projectID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	var healthChecks []models.ProjectHealthCheck
	if err := db.Where("project_id = ?", uint(projectID)).
		Order("checked_at DESC, id DESC").
		Limit(limit).
		Find(&healthChecks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve health checks"})
		return
	}

	response := make([]projectHealthResponse, 0, len(healthChecks))
	for _, health := range healthChecks {
		response = append(response, toProjectHealthResponse(health))
	}
	c.JSON(http.StatusOK, response)
}

// CreateProjectHealthReport records a health report submitted by a future distributed agent.
func CreateProjectHealthReport(c *gin.Context) {
	var req projectHealthReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if req.ProjectID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "project_id is required"})
		return
	}

	db := config.GetDB()
	var project models.Project
	if err := db.First(&project, req.ProjectID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	if strings.TrimSpace(req.AgentID) == "" {
		req.AgentID = "external"
	}
	if strings.TrimSpace(req.AgentName) == "" {
		req.AgentName = "External Agent"
	}
	if req.RunMode == "" {
		req.RunMode = normalizedProjectRunMode(project.RunMode)
	}
	if req.ContainerStatus == "" {
		req.ContainerStatus = normalizeContainerStatus(project.ContainerStatus)
	}
	if req.CheckedAt.IsZero() {
		req.CheckedAt = time.Now()
	}
	status := normalizeProjectHealthStatus(req.Status)
	if req.Status == "" {
		if req.RuntimeOK {
			status = models.ProjectHealthStatusHealthy
		} else {
			status = models.ProjectHealthStatusUnknown
		}
	}

	health := models.ProjectHealthCheck{
		ProjectID:       project.ID,
		AgentID:         strings.TrimSpace(req.AgentID),
		AgentName:       strings.TrimSpace(req.AgentName),
		RunMode:         normalizedProjectRunMode(req.RunMode),
		ContainerStatus: normalizeContainerStatus(req.ContainerStatus),
		RuntimeOK:       req.RuntimeOK,
		Status:          status,
		URL:             strings.TrimSpace(req.URL),
		HTTPStatus:      req.HTTPStatus,
		LatencyMS:       req.LatencyMS,
		Message:         req.Message,
		ErrorMessage:    req.ErrorMessage,
		CheckedAt:       req.CheckedAt,
	}
	if err := db.Create(&health).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save health report", "details": err.Error()})
		return
	}
	syncProjectRuntimeStatusFromHealth(db, project, health)

	c.JSON(http.StatusCreated, gin.H{
		"status": health.Status,
		"health": toProjectHealthResponse(health),
	})
}

// ContainerLog retrieves the container logs
func ContainerLog(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var project models.Project
	if err := db.First(&project, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}
	if !ensureProjectAccess(c, project) {
		return
	}

	if pid, ok := parseLocalContainerPID(project.ContainerID); ok || strings.Contains(project.ContainerLog, "本地 Flask") {
		logContent := "=== Local Flask Logs ===\n"
		if ok {
			logContent += "Process ID: " + strconv.Itoa(pid) + "\n"
		}
		logContent += "Runtime directory: " + localProjectAppDir(project.ID) + "\n"
		logContent += "Port: " + strconv.Itoa(int(project.Port)) + "\n"
		logContent += "Status: " + string(project.ContainerStatus) + "\n"
		logContent += "=====================================\n\n"

		localLogs, err := readTailFile(localProjectLogPath(project.ID), 64*1024)
		if err != nil {
			logContent += "Unable to read Local Flask logs: " + err.Error() + "\n"
		} else {
			logContent += localLogs + "\n"
		}
		logContent += "\nRecent operations:\n" + project.ContainerLog

		c.JSON(http.StatusOK, gin.H{"log": logContent})
		return
	}

	// Fetch actual Docker container logs
	var logContent string

	if project.ContainerStatus == models.ContainerStatusRunning && project.ContainerID != "" {
		// Container is running, fetch real logs from Docker
		dockerClient, err := utils.NewDockerClient()
		if err != nil {
			logContent = "=== Container Logs ===\n"
			logContent += "Error: unable to connect to Docker: " + err.Error() + "\n"
			logContent += "=====================================\n"
			logContent += project.ContainerLog
		} else {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			containerLogs, err := dockerClient.GetContainerLogs(ctx, project.ContainerID)
			if err != nil {
				logContent = "=== Container Logs ===\n"
				logContent += "Error: failed to retrieve container logs: " + err.Error() + "\n"
				logContent += "Container ID: " + project.ContainerID + "\n"
				logContent += "=====================================\n"
				logContent += project.ContainerLog
			} else {
				logContent = "=== Container Logs ===\n"
				logContent += "Container ID: " + project.ContainerID + "\n"
				logContent += "Docker image: " + project.DockerImage + "\n"
				logContent += "Port: " + strconv.Itoa(int(project.Port)) + "\n"
				logContent += "=====================================\n\n"
				logContent += containerLogs
			}
		}
	} else {
		// Container is not running
		logContent = "=== Container Logs ===\n"
		logContent += "Status: " + string(project.ContainerStatus) + "\n"
		logContent += "Container ID: " + project.ContainerID + "\n"
		logContent += "Docker image: " + project.DockerImage + "\n"
		logContent += "Port: " + strconv.Itoa(int(project.Port)) + "\n"
		logContent += "=====================================\n\n"

		if project.ContainerStatus == models.ContainerStatusStopped {
			logContent += "The container is stopped; live logs are unavailable.\n\n"
		} else if project.ContainerStatus == models.ContainerStatusStopping {
			logContent += "The container is stopping...\n\n"
		}

		logContent += "Recent operations:\n" + project.ContainerLog
	}

	c.JSON(http.StatusOK, gin.H{"log": logContent})
}

// GetProjectCredentials retrieves credentials for a project
func GetProjectCredentials(c *gin.Context) {
	id := c.Param("id")
	projectID, err := strconv.ParseUint(id, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}

	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var project models.Project
	if err := scopeByOwner(db, user).First(&project, uint(projectID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project not found"})
		return
	}

	var credentials []models.Credential
	if err := db.Where("project_id = ?", project.ID).Order("created_at DESC").Find(&credentials).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve credentials"})
		return
	}

	fillCredentialLocations(credentials)
	c.JSON(http.StatusOK, credentials)
}
