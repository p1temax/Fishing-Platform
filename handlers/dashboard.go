package handlers

import (
	"net/http"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/hostmetrics"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
)

// GetStatistics retrieves dashboard statistics
func GetStatistics(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()

	var projectCount int64
	if err := scopeByOwner(db.Model(&models.Project{}), user).Where("deletion_pending = ?", false).Count(&projectCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve project count"})
		return
	}

	var robotCount int64
	if err := scopeByOwner(db.Model(&models.Robot{}), user).Count(&robotCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve robot count"})
		return
	}

	var messageCount int64
	messageQuery := db.Model(&models.Message{})
	if !user.Role.IsAdmin() {
		messageQuery = messageQuery.Where("project_id IN (?)", db.Model(&models.Project{}).Select("id").Where("created_by = ?", user.ID))
	}
	if err := messageQuery.Count(&messageCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve message count"})
		return
	}

	var agents []models.Agent
	if err := db.Find(&agents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve agents"})
		return
	}
	var onlineAgentCount int64
	for _, agent := range agents {
		if currentAgentStatus(agent, time.Now()) == models.AgentStatusOnline {
			onlineAgentCount++
		}
	}
	agentCount := int64(len(agents))
	offlineAgentCount := agentCount - onlineAgentCount

	var activeDeploymentCount int64
	deploymentQuery := db.Model(&models.ProjectDeployment{}).
		Where("actual_status = ? AND desired_status <> ?", models.DeploymentStatusRunning, models.DeploymentStatusRemoved)
	if !user.Role.IsAdmin() {
		deploymentQuery = deploymentQuery.Where("project_id IN (?)", db.Model(&models.Project{}).Select("id").Where("created_by = ?", user.ID))
	}
	if err := deploymentQuery.Count(&activeDeploymentCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve active deployment count"})
		return
	}

	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var todayCredentialCount int64
	credQuery := db.Model(&models.Credential{}).Where("created_at >= ?", startOfDay)
	if !user.Role.IsAdmin() {
		credQuery = credQuery.Where("project_id IN (?)", db.Model(&models.Project{}).Select("id").Where("created_by = ?", user.ID))
	}
	if err := credQuery.Count(&todayCredentialCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve today's credential count"})
		return
	}

	// A project is running when its local runtime is running or at least one
	// distributed Agent deployment is running. Count projects only once even
	// when the same project is deployed to multiple Agents.
	var runningProjectCount int64
	runningQuery := scopeByOwner(db.Model(&models.Project{}), user).
		Where("deletion_pending = ?", false).
		Where(
			"flask_status = ? OR EXISTS (SELECT 1 FROM project_deployments WHERE project_deployments.project_id = projects.id AND project_deployments.actual_status = ? AND project_deployments.desired_status <> ?)",
			models.ContainerStatusRunning,
			models.DeploymentStatusRunning,
			models.DeploymentStatusRemoved,
		)
	if err := runningQuery.Count(&runningProjectCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve running project count"})
		return
	}

	hostSamples, err := hostmetrics.ListRecent()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve host metric history"})
		return
	}

	statistics := gin.H{
		"projectCount":          projectCount,
		"robotCount":            robotCount,
		"messageCount":          messageCount,
		"agentCount":            agentCount,
		"onlineAgentCount":      onlineAgentCount,
		"offlineAgentCount":     offlineAgentCount,
		"activeDeploymentCount": activeDeploymentCount,
		"todayCredentialCount":  todayCredentialCount,
		"runningProjectCount":   runningProjectCount,
		"hostMetrics":           utils.CollectHostMetrics(),
		"hostMetricSamples":     hostSamples,
	}

	c.JSON(http.StatusOK, statistics)
}
