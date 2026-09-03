package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newAgentRuntimeTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "agent-runtime.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	config.DB = db
	return db
}

func TestAgentRuntimeTaskArtifactLogsAndSubmission(t *testing.T) {
	db := newAgentRuntimeTestDB(t)
	token := "agent-secret"
	agent := models.Agent{AgentID: "runtime-a", Name: "Runtime A", TokenHash: hashAgentToken(token)}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	otherToken := "other-agent-secret"
	otherAgent := models.Agent{AgentID: "runtime-b", Name: "Runtime B", TokenHash: hashAgentToken(otherToken)}
	if err := db.Create(&otherAgent).Error; err != nil {
		t.Fatal(err)
	}
	htmlPath := filepath.Join(t.TempDir(), "index.html")
	if err := os.WriteFile(htmlPath, []byte("<html>agent artifact</html>"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := models.Project{Name: "runtime-project", HtmlFilePath: htmlPath, FrontendRoute: "/training", ContainerRoute: "/api/submit", DeploymentRevision: 1}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}
	if err := syncProjectDeployments(db, &project, []uint{agent.ID}); err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/lease", LeaseAgentTasks)
	router.POST("/tasks/:taskID/start", StartAgentTask)
	router.POST("/tasks/:taskID/complete", CompleteAgentTask)
	router.GET("/deployments/:id/artifact", DownloadDeploymentArtifact)
	router.POST("/deployments/:id/logs", UploadDeploymentLogs)
	router.POST("/deployments/:id/submit", SubmitDeploymentData)
	auth := func(req *http.Request) { req.Header.Set("Authorization", "Bearer "+token) }

	leaseReq := httptest.NewRequest(http.MethodGet, "/lease?limit=5", nil)
	auth(leaseReq)
	leaseRes := httptest.NewRecorder()
	router.ServeHTTP(leaseRes, leaseReq)
	if leaseRes.Code != http.StatusOK {
		t.Fatalf("lease: %d %s", leaseRes.Code, leaseRes.Body.String())
	}
	var leased struct {
		Tasks []TaskForTest `json:"tasks"`
	}
	if err := json.Unmarshal(leaseRes.Body.Bytes(), &leased); err != nil {
		t.Fatal(err)
	}
	if len(leased.Tasks) != 1 {
		t.Fatalf("leased tasks = %d, want 1", len(leased.Tasks))
	}
	task := leased.Tasks[0]
	if task.Payload.RuntimePort != agentProjectPortStart {
		t.Fatalf("runtime port = %d, want controller-assigned %d", task.Payload.RuntimePort, agentProjectPortStart)
	}

	startReq := httptest.NewRequest(http.MethodPost, "/tasks/"+task.TaskID+"/start", strings.NewReader("{}"))
	auth(startReq)
	startReq.Header.Set("Content-Type", "application/json")
	startRes := httptest.NewRecorder()
	router.ServeHTTP(startRes, startReq)
	if startRes.Code != http.StatusOK {
		t.Fatalf("start: %d %s", startRes.Code, startRes.Body.String())
	}

	artifactReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/deployments/%d/artifact", task.DeploymentID), nil)
	auth(artifactReq)
	artifactRes := httptest.NewRecorder()
	router.ServeHTTP(artifactRes, artifactReq)
	if artifactRes.Code != http.StatusOK || !strings.Contains(artifactRes.Body.String(), "agent artifact") {
		t.Fatalf("artifact: %d %s", artifactRes.Code, artifactRes.Body.String())
	}
	foreignReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/deployments/%d/artifact", task.DeploymentID), nil)
	foreignReq.Header.Set("Authorization", "Bearer "+otherToken)
	foreignRes := httptest.NewRecorder()
	router.ServeHTTP(foreignRes, foreignReq)
	if foreignRes.Code != http.StatusNotFound {
		t.Fatalf("cross-agent artifact status = %d, want 404", foreignRes.Code)
	}

	logBody := `{"entries":[{"sequence":1,"task_id":"` + task.TaskID + `","stream":"system","level":"info","message":"deployed"}]}`
	logReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/deployments/%d/logs", task.DeploymentID), strings.NewReader(logBody))
	auth(logReq)
	logReq.Header.Set("Content-Type", "application/json")
	logRes := httptest.NewRecorder()
	router.ServeHTTP(logRes, logReq)
	if logRes.Code != http.StatusOK {
		t.Fatalf("logs: %d %s", logRes.Code, logRes.Body.String())
	}

	form := "username=user&password=pass&captchavalue=1234"
	submitReq := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/deployments/%d/submit", task.DeploymentID), strings.NewReader(form))
	auth(submitReq)
	submitReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	submitReq.Header.Set("X-Forwarded-For", "198.51.100.8")
	submitRes := httptest.NewRecorder()
	router.ServeHTTP(submitRes, submitReq)
	if submitRes.Code != http.StatusOK {
		t.Fatalf("submit: %d %s", submitRes.Code, submitRes.Body.String())
	}

	completeBody, _ := json.Marshal(map[string]any{"success": true, "revision": 1, "runtime_port": 12000, "runtime_url": "http://agent:12000/training"})
	completeReq := httptest.NewRequest(http.MethodPost, "/tasks/"+task.TaskID+"/complete", bytes.NewReader(completeBody))
	auth(completeReq)
	completeReq.Header.Set("Content-Type", "application/json")
	completeRes := httptest.NewRecorder()
	router.ServeHTTP(completeRes, completeReq)
	if completeRes.Code != http.StatusOK {
		t.Fatalf("complete: %d %s", completeRes.Code, completeRes.Body.String())
	}

	var credential models.Credential
	if err := db.Where("project_id = ?", project.ID).First(&credential).Error; err != nil {
		t.Fatal(err)
	}
	if credential.Username != "user" || credential.IPAddress != "198.51.100.8" {
		t.Fatalf("unexpected credential: %+v", credential)
	}
	var storedLog models.ProjectLog
	if err := db.Where("project_id = ?", project.ID).First(&storedLog).Error; err != nil {
		t.Fatal(err)
	}
	if storedLog.DeploymentID != task.DeploymentID || storedLog.AgentID != agent.ID {
		t.Fatalf("unexpected log ownership: %+v", storedLog)
	}
	var deployment models.ProjectDeployment
	if err := db.First(&deployment, task.DeploymentID).Error; err != nil {
		t.Fatal(err)
	}
	if deployment.ActualStatus != models.DeploymentStatusRunning {
		t.Fatalf("deployment status = %s", deployment.ActualStatus)
	}
}

type TaskForTest struct {
	TaskID       string `json:"task_id"`
	DeploymentID uint   `json:"deployment_id"`
	Payload      struct {
		RuntimePort uint `json:"runtime_port"`
	} `json:"payload"`
}

func TestSyncProjectDeploymentsSupportsManyToMany(t *testing.T) {
	db := newAgentRuntimeTestDB(t)
	agentA := models.Agent{AgentID: "a", Name: "A", TokenHash: "hash-a"}
	agentB := models.Agent{AgentID: "b", Name: "B", TokenHash: "hash-b"}
	if err := db.Create(&agentA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&agentB).Error; err != nil {
		t.Fatal(err)
	}
	projectA := models.Project{Name: "project-a", DeploymentRevision: 1}
	projectB := models.Project{Name: "project-b", DeploymentRevision: 1}
	if err := db.Create(&projectA).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&projectB).Error; err != nil {
		t.Fatal(err)
	}

	if err := syncProjectDeployments(db, &projectA, []uint{agentA.ID, agentB.ID}); err != nil {
		t.Fatal(err)
	}
	if err := syncProjectDeployments(db, &projectB, []uint{agentA.ID}); err != nil {
		t.Fatal(err)
	}

	var deploymentCount int64
	db.Model(&models.ProjectDeployment{}).Count(&deploymentCount)
	if deploymentCount != 3 {
		t.Fatalf("deployments = %d, want 3", deploymentCount)
	}
	var agentAProjects int64
	db.Model(&models.ProjectDeployment{}).Where("agent_id = ?", agentA.ID).Count(&agentAProjects)
	if agentAProjects != 2 {
		t.Fatalf("agent A projects = %d, want 2", agentAProjects)
	}
	var projectAAgents int64
	db.Model(&models.ProjectDeployment{}).Where("project_id = ?", projectA.ID).Count(&projectAAgents)
	if projectAAgents != 2 {
		t.Fatalf("project A agents = %d, want 2", projectAAgents)
	}
	var taskCount int64
	db.Model(&models.AgentTask{}).Count(&taskCount)
	if taskCount != 3 {
		t.Fatalf("tasks = %d, want 3", taskCount)
	}
	var agentADeployments []models.ProjectDeployment
	if err := db.Where("agent_id = ?", agentA.ID).Order("runtime_port ASC").Find(&agentADeployments).Error; err != nil {
		t.Fatal(err)
	}
	if agentADeployments[0].RuntimePort != 10000 || agentADeployments[1].RuntimePort != 10001 {
		t.Fatalf("agent A ports = %d, %d; want 10000, 10001", agentADeployments[0].RuntimePort, agentADeployments[1].RuntimePort)
	}
}

func TestProjectLogsArePartitionedByProjectAndDeployment(t *testing.T) {
	db := newAgentRuntimeTestDB(t)
	logs := []models.ProjectLog{
		{ProjectID: 1, AgentID: 1, DeploymentID: 11, Sequence: 1, Message: "p1-a"},
		{ProjectID: 1, AgentID: 2, DeploymentID: 12, Sequence: 1, Message: "p1-b"},
		{ProjectID: 2, AgentID: 1, DeploymentID: 21, Sequence: 1, Message: "p2-a"},
	}
	for i := range logs {
		if err := db.Create(&logs[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	var projectOne []models.ProjectLog
	if err := db.Where("project_id = ?", 1).Find(&projectOne).Error; err != nil {
		t.Fatal(err)
	}
	if len(projectOne) != 2 {
		t.Fatalf("project 1 logs = %d, want 2", len(projectOne))
	}
	if projectOne[0].DeploymentID == projectOne[1].DeploymentID {
		t.Fatal("project logs lost deployment distinction")
	}
}
