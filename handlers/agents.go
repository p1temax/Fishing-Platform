package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const agentOfflineAfter = 60 * time.Second

type agentRegisterRequest struct {
	RegistrationToken string   `json:"registration_token"`
	AgentID           string   `json:"agent_id"`
	Name              string   `json:"name"`
	Hostname          string   `json:"hostname"`
	IPAddress         string   `json:"ip_address"`
	OS                string   `json:"os"`
	Arch              string   `json:"arch"`
	Version           string   `json:"version"`
	Capabilities      []string `json:"capabilities"`
	Tags              []string `json:"tags"`
}

type agentHeartbeatRequest struct {
	Name          string   `json:"name"`
	Hostname      string   `json:"hostname"`
	IPAddress     string   `json:"ip_address"`
	OS            string   `json:"os"`
	Arch          string   `json:"arch"`
	Version       string   `json:"version"`
	Capabilities  []string `json:"capabilities"`
	Tags          []string `json:"tags"`
	DeploymentIDs []uint   `json:"deployment_ids"`
}

type agentResponse struct {
	ID              uint               `json:"id"`
	AgentID         string             `json:"agent_id"`
	Name            string             `json:"name"`
	Hostname        string             `json:"hostname"`
	IPAddress       string             `json:"ip_address"`
	OS              string             `json:"os"`
	Arch            string             `json:"arch"`
	Version         string             `json:"version"`
	Status          models.AgentStatus `json:"status"`
	Capabilities    []string           `json:"capabilities"`
	Tags            []string           `json:"tags"`
	LastSeenAt      *time.Time         `json:"last_seen_at"`
	CreatedAt       time.Time          `json:"created_at"`
	UpdatedAt       time.Time          `json:"updated_at"`
	DeploymentCount int64              `json:"deployment_count"`
}

func encodeStringList(values []string) string {
	cleaned := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	payload, err := json.Marshal(cleaned)
	if err != nil {
		return "[]"
	}
	return string(payload)
}

func decodeStringList(raw string) []string {
	var values []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return []string{}
	}
	return values
}

func hashAgentToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func generateURLToken(prefix string, size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func constantTimeEqual(left string, right string) bool {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" || right == "" || len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func normalizeAgentName(req agentRegisterRequest, agentID string) string {
	for _, candidate := range []string{req.Name, req.Hostname, agentID} {
		candidate = strings.TrimSpace(candidate)
		if candidate != "" {
			return candidate
		}
	}
	return "Unnamed Agent"
}

func applyAgentRegistration(agent *models.Agent, req agentRegisterRequest, token string, now time.Time) {
	agent.Name = normalizeAgentName(req, agent.AgentID)
	agent.TokenHash = hashAgentToken(token)
	agent.Hostname = strings.TrimSpace(req.Hostname)
	agent.IPAddress = strings.TrimSpace(req.IPAddress)
	agent.OS = strings.TrimSpace(req.OS)
	agent.Arch = strings.TrimSpace(req.Arch)
	agent.Version = strings.TrimSpace(req.Version)
	agent.Status = models.AgentStatusOnline
	agent.Capabilities = encodeStringList(req.Capabilities)
	agent.Tags = encodeStringList(req.Tags)
	agent.LastSeenAt = &now
}

func applyAgentHeartbeat(agent *models.Agent, req agentHeartbeatRequest, now time.Time) {
	if strings.TrimSpace(req.Name) != "" {
		agent.Name = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.Hostname) != "" {
		agent.Hostname = strings.TrimSpace(req.Hostname)
	}
	if strings.TrimSpace(req.IPAddress) != "" {
		agent.IPAddress = strings.TrimSpace(req.IPAddress)
	}
	if strings.TrimSpace(req.OS) != "" {
		agent.OS = strings.TrimSpace(req.OS)
	}
	if strings.TrimSpace(req.Arch) != "" {
		agent.Arch = strings.TrimSpace(req.Arch)
	}
	if strings.TrimSpace(req.Version) != "" {
		agent.Version = strings.TrimSpace(req.Version)
	}
	if req.Capabilities != nil {
		agent.Capabilities = encodeStringList(req.Capabilities)
	}
	if req.Tags != nil {
		agent.Tags = encodeStringList(req.Tags)
	}
	agent.Status = models.AgentStatusOnline
	agent.LastSeenAt = &now
}

func currentAgentStatus(agent models.Agent, now time.Time) models.AgentStatus {
	if agent.LastSeenAt == nil || now.Sub(*agent.LastSeenAt) > agentOfflineAfter {
		return models.AgentStatusOffline
	}
	return models.AgentStatusOnline
}

func toAgentResponse(agent models.Agent) agentResponse {
	return agentResponse{
		ID:           agent.ID,
		AgentID:      agent.AgentID,
		Name:         agent.Name,
		Hostname:     agent.Hostname,
		IPAddress:    agent.IPAddress,
		OS:           agent.OS,
		Arch:         agent.Arch,
		Version:      agent.Version,
		Status:       currentAgentStatus(agent, time.Now()),
		Capabilities: decodeStringList(agent.Capabilities),
		Tags:         decodeStringList(agent.Tags),
		LastSeenAt:   agent.LastSeenAt,
		CreatedAt:    agent.CreatedAt,
		UpdatedAt:    agent.UpdatedAt,
	}
}

func authenticateAgent(c *gin.Context) (*models.Agent, bool) {
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
		return nil, false
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if !(len(parts) == 2 && parts[0] == "Bearer") {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format"})
		return nil, false
	}

	token := strings.TrimSpace(parts[1])
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Agent token required"})
		return nil, false
	}

	var agent models.Agent
	db := config.GetDB()
	if err := db.Where("token_hash = ?", hashAgentToken(token)).First(&agent).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid agent token"})
		return nil, false
	}

	return &agent, true
}

// RegisterAgent registers or re-registers an agent and returns its bearer token once.
func RegisterAgent(c *gin.Context) {
	var req agentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	expectedToken := config.AgentRegistrationToken()
	if expectedToken == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Agent registration token is not configured"})
		return
	}
	if !constantTimeEqual(req.RegistrationToken, expectedToken) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid agent registration token"})
		return
	}

	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" {
		generatedID, err := generateURLToken("agent_", 12)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate agent ID"})
			return
		}
		agentID = generatedID
	}

	agentToken, err := generateURLToken("agent_token_", 32)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate agent token"})
		return
	}

	db := config.GetDB()
	now := time.Now()
	var agent models.Agent
	result := db.Where("agent_id = ?", agentID).First(&agent)
	if result.Error == nil {
		applyAgentRegistration(&agent, req, agentToken, now)
		if agent.IPAddress == "" { agent.IPAddress = c.ClientIP() }
		if err := db.Save(&agent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update agent"})
			return
		}
	} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		agent = models.Agent{AgentID: agentID}
		applyAgentRegistration(&agent, req, agentToken, now)
		if agent.IPAddress == "" { agent.IPAddress = c.ClientIP() }
		if err := db.Create(&agent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create agent"})
			return
		}
	} else {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load agent"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"agent": agentResponse{
			ID:           agent.ID,
			AgentID:      agent.AgentID,
			Name:         agent.Name,
			Hostname:     agent.Hostname,
			IPAddress:    agent.IPAddress,
			OS:           agent.OS,
			Arch:         agent.Arch,
			Version:      agent.Version,
			Status:       models.AgentStatusOnline,
			Capabilities: decodeStringList(agent.Capabilities),
			Tags:         decodeStringList(agent.Tags),
			LastSeenAt:   agent.LastSeenAt,
			CreatedAt:    agent.CreatedAt,
			UpdatedAt:    agent.UpdatedAt,
		},
		"token": agentToken,
	})
}

// AgentHeartbeat updates agent liveness and runtime metadata.
func AgentHeartbeat(c *gin.Context) {
	agent, ok := authenticateAgent(c)
	if !ok {
		return
	}

	var req agentHeartbeatRequest
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	now := time.Now()
	applyAgentHeartbeat(agent, req, now)
	if agent.IPAddress == "" { agent.IPAddress = c.ClientIP() }
	db := config.GetDB()
	if err := db.Save(agent).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update agent heartbeat"})
		return
	}
	// Reconcile deployments after an agent restart. Missing desired deployments
	// receive a fresh idempotent deploy task, without duplicating active work.
	running := make(map[uint]struct{}, len(req.DeploymentIDs))
	for _, id := range req.DeploymentIDs {
		running[id] = struct{}{}
	}
	var deployments []models.ProjectDeployment
	if err := db.Where("agent_id = ? AND desired_status = ?", agent.ID, models.DeploymentStatusRunning).Find(&deployments).Error; err == nil {
		for _, deployment := range deployments {
			if _, ok := running[deployment.ID]; ok {
				db.Model(&deployment).Updates(map[string]any{"actual_status": models.DeploymentStatusRunning, "last_seen_at": now})
				continue
			}
			var active int64
			db.Model(&models.AgentTask{}).Where("deployment_id = ? AND status IN ?", deployment.ID,
				[]models.AgentTaskStatus{models.AgentTaskStatusPending, models.AgentTaskStatusLeased, models.AgentTaskStatusRunning}).Count(&active)
			if active == 0 {
				deployment.ActualStatus = models.DeploymentStatusPending
				_ = db.Save(&deployment).Error
				_ = enqueueAgentTask(db, deployment, "project.deploy")
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":             "ok",
		"agent":              toAgentResponse(*agent),
		"server_time":        now,
		"task_poll_interval": 3,
	})
}

// GetAgents retrieves all registered agents for the admin console.
func GetAgents(c *gin.Context) {
	db := config.GetDB()
	var agents []models.Agent
	if err := db.Order("last_seen_at DESC, id DESC").Find(&agents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve agents"})
		return
	}

	response := make([]agentResponse, 0, len(agents))
	for _, agent := range agents {
		item := toAgentResponse(agent)
		db.Model(&models.ProjectDeployment{}).
			Where("agent_id = ? AND desired_status <> ?", agent.ID, models.DeploymentStatusRemoved).
			Count(&item.DeploymentCount)
		response = append(response, item)
	}

	c.JSON(http.StatusOK, response)
}

// GetAgent retrieves a single registered agent.
func GetAgent(c *gin.Context) {
	agentID, err := strconv.ParseUint(c.Param("id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	db := config.GetDB()
	var agent models.Agent
	if err := db.First(&agent, uint(agentID)).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Agent not found"})
		return
	}

	response := toAgentResponse(agent)
	db.Model(&models.ProjectDeployment{}).
		Where("agent_id = ? AND desired_status <> ?", agent.ID, models.DeploymentStatusRemoved).
		Count(&response.DeploymentCount)
	c.JSON(http.StatusOK, response)
}
