package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDashboardStatisticsIncludeLocalAndDistributedProjects(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "dashboard.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	config.DB = db

	admin := models.User{Username: "admin", Password: "x", Role: models.UserRoleAdmin}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}

	local := models.Project{Name: "local-running", ContainerStatus: models.ContainerStatusRunning, CreatedBy: admin.ID}
	remote := models.Project{Name: "remote-running", ContainerStatus: models.ContainerStatusStopped, CreatedBy: admin.ID}
	stopped := models.Project{Name: "stopped", ContainerStatus: models.ContainerStatusStopped, CreatedBy: admin.ID}
	deleting := models.Project{Name: "deleting", ContainerStatus: models.ContainerStatusRunning, DeletionPending: true, CreatedBy: admin.ID}
	for _, project := range []*models.Project{&local, &remote, &stopped, &deleting} {
		if err := db.Create(project).Error; err != nil {
			t.Fatal(err)
		}
	}
	agent := models.Agent{AgentID: "dashboard-agent", Name: "Dashboard Agent", TokenHash: "dashboard-agent-token-hash"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		deployment := models.ProjectDeployment{
			ProjectID: remote.ID, AgentID: agent.ID,
			DesiredStatus: models.DeploymentStatusRunning,
			ActualStatus:  models.DeploymentStatusRunning,
		}
		// The schema permits one deployment per project/agent, so use a second
		// Agent to verify that multiple running deployments count as one project.
		if i == 1 {
			secondAgent := models.Agent{AgentID: "dashboard-agent-2", Name: "Dashboard Agent 2", TokenHash: "dashboard-agent-token-hash-2"}
			if err := db.Create(&secondAgent).Error; err != nil {
				t.Fatal(err)
			}
			deployment.AgentID = secondAgent.ID
		}
		if err := db.Create(&deployment).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&models.MailCampaign{
		Subject: "dashboard-mail", SmtpServiceID: 1, BodyTemplate: "hi", CreatedBy: admin.ID,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Message{ProjectID: remote.ID}).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/dashboard", func(c *gin.Context) {
		c.Set("user", admin)
		GetStatistics(c)
	})
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		ProjectCount        int64 `json:"projectCount"`
		RunningProjectCount int64 `json:"runningProjectCount"`
		MailCampaignCount   int64 `json:"mailCampaignCount"`
		MessageCount        int64 `json:"messageCount"`
		AgentCount          int64 `json:"agentCount"`
		OnlineAgentCount    int64 `json:"onlineAgentCount"`
		OfflineAgentCount   int64 `json:"offlineAgentCount"`
		ActiveDeployments   int64 `json:"activeDeploymentCount"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.ProjectCount != 3 {
		t.Fatalf("projectCount = %d, want 3", result.ProjectCount)
	}
	if result.RunningProjectCount != 2 {
		t.Fatalf("runningProjectCount = %d, want 2", result.RunningProjectCount)
	}
	if result.MailCampaignCount != 1 || result.MessageCount != 1 {
		t.Fatalf("mailCampaignCount/messageCount = %d/%d, want 1/1", result.MailCampaignCount, result.MessageCount)
	}
	if result.AgentCount != 2 || result.OnlineAgentCount != 0 || result.OfflineAgentCount != 2 {
		t.Fatalf("agent counts = %d/%d/%d, want 2/0/2", result.AgentCount, result.OnlineAgentCount, result.OfflineAgentCount)
	}
	if result.ActiveDeployments != 2 {
		t.Fatalf("activeDeploymentCount = %d, want 2", result.ActiveDeployments)
	}
}
