package handlers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	defaultAIBaseURL    = "https://api.x.ai/v1"
	defaultAIModel      = "grok-4.5"
	defaultAITimeoutSec = 120
)

type aiProfileDTO struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Enabled    bool   `json:"enabled"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	APIKeySet  bool   `json:"api_key_set"`
	Model      string `json:"model"`
	TimeoutSec int    `json:"timeout_sec"`
}

type aiSettingsDTO struct {
	Profiles []aiProfileDTO `json:"profiles"`
}

type aiSettingsPayload struct {
	Profiles []aiProfileDTO `json:"profiles"`
}

// GetAISettings returns AI profiles from the database (admin only, keys masked).
func GetAISettings(c *gin.Context) {
	db := config.GetDB()
	var rows []models.AIProfile
	if err := db.Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load AI settings"})
		return
	}
	c.JSON(http.StatusOK, aiSettingsDTO{Profiles: toAIProfileDTOs(rows)})
}

// UpdateAISettings replaces AI profiles in the database. At most one may be enabled.
func UpdateAISettings(c *gin.Context) {
	var payload aiSettingsPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if payload.Profiles == nil {
		payload.Profiles = []aiProfileDTO{}
	}

	db := config.GetDB()
	var existing []models.AIProfile
	if err := db.Find(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load AI settings"})
		return
	}
	existingByID := map[string]models.AIProfile{}
	for _, row := range existing {
		existingByID[row.ID] = row
	}

	merged := make([]models.AIProfile, 0, len(payload.Profiles))
	enabledCount := 0
	for _, p := range payload.Profiles {
		name := strings.TrimSpace(p.Name)
		model := strings.TrimSpace(p.Model)
		baseURL := utils.NormalizeOpenAICompatibleBaseURL(p.BaseURL)
		if name == "" || model == "" || baseURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "each profile requires name, model and base_url"})
			return
		}
		id := strings.TrimSpace(p.ID)
		if id == "" {
			id = newAIProfileID()
		}
		timeout := p.TimeoutSec
		if timeout <= 0 {
			timeout = defaultAITimeoutSec
		}

		apiKeyPlain := strings.TrimSpace(p.APIKey)
		encryptedKey := ""
		if apiKeyPlain == "" || looksMaskedSecret(apiKeyPlain) {
			if old, ok := existingByID[id]; ok {
				encryptedKey = old.APIKey
			}
		} else {
			enc, err := utils.Encrypt(apiKeyPlain)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt API key"})
				return
			}
			encryptedKey = enc
		}

		if p.Enabled {
			enabledCount++
		}
		merged = append(merged, models.AIProfile{
			ID:         id,
			Name:       name,
			Enabled:    p.Enabled,
			BaseURL:    baseURL,
			APIKey:     encryptedKey,
			Model:      model,
			TimeoutSec: timeout,
		})
	}
	if enabledCount > 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only one AI profile can be enabled"})
		return
	}

	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("1 = 1").Delete(&models.AIProfile{}).Error; err != nil {
			return err
		}
		for i := range merged {
			if err := tx.Create(&merged[i]).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save AI settings"})
		return
	}

	c.JSON(http.StatusOK, aiSettingsDTO{Profiles: toAIProfileDTOs(merged)})
}

type aiTestRequest struct {
	ID         string `json:"id"`
	BaseURL    string `json:"base_url"`
	APIKey     string `json:"api_key"`
	Model      string `json:"model"`
	TimeoutSec int    `json:"timeout_sec"`
}

// TestAISettings probes chat/completions (page mirror) and /responses + web_search
// (info gathering) for a saved profile or inline form values.
func TestAISettings(c *gin.Context) {
	var req aiTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	baseURL := utils.NormalizeOpenAICompatibleBaseURL(req.BaseURL)
	model := strings.TrimSpace(req.Model)
	apiKey := strings.TrimSpace(req.APIKey)
	timeoutSec := req.TimeoutSec

	if id := strings.TrimSpace(req.ID); id != "" {
		db := config.GetDB()
		var row models.AIProfile
		if err := db.Where("id = ?", id).First(&row).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "AI profile not found"})
			return
		}
		if baseURL == "" {
			baseURL = utils.NormalizeOpenAICompatibleBaseURL(row.BaseURL)
		}
		if model == "" {
			model = strings.TrimSpace(row.Model)
		}
		if timeoutSec <= 0 {
			timeoutSec = row.TimeoutSec
		}
		if apiKey == "" || looksMaskedSecret(apiKey) {
			if strings.TrimSpace(row.APIKey) != "" {
				if dec, err := utils.Decrypt(row.APIKey); err == nil {
					apiKey = dec
				} else {
					apiKey = row.APIKey
				}
			}
		}
	}

	if baseURL == "" || model == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "base_url and model are required"})
		return
	}
	if apiKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API key is required"})
		return
	}
	if timeoutSec <= 0 {
		timeoutSec = 60
	}
	if timeoutSec > 120 {
		timeoutSec = 120
	}

	chatTimeout := 30 * time.Second
	webTimeout := time.Duration(timeoutSec) * time.Second
	if webTimeout < 45*time.Second {
		webTimeout = 45 * time.Second
	}
	if webTimeout > 90*time.Second {
		webTimeout = 90 * time.Second
	}

	chatCtx, chatCancel := context.WithTimeout(c.Request.Context(), chatTimeout)
	chatResult, chatErr := utils.ProbeChatCompletions(chatCtx, baseURL, apiKey, model, chatTimeout)
	chatCancel()

	webCtx, webCancel := context.WithTimeout(c.Request.Context(), webTimeout)
	webResult, webErr := utils.ProbeWebSearch(webCtx, baseURL, apiKey, model, webTimeout)
	webCancel()

	chatPayload := gin.H{
		"ok":         chatErr == nil,
		"endpoint":   chatResult.Endpoint,
		"latency_ms": chatResult.LatencyMs,
		"status":     chatResult.StatusCode,
	}
	if chatErr != nil {
		chatPayload["error"] = chatErr.Error()
	}
	webPayload := gin.H{
		"ok":         webErr == nil,
		"endpoint":   webResult.Endpoint,
		"latency_ms": webResult.LatencyMs,
		"status":     webResult.StatusCode,
		"mode":       webResult.Mode,
	}
	if webErr != nil {
		webPayload["error"] = webErr.Error()
	}

	// Backward-compatible top-level fields mirror the chat probe.
	resp := gin.H{
		"ok":         chatErr == nil,
		"model":      model,
		"endpoint":   chatResult.Endpoint,
		"latency_ms": chatResult.LatencyMs,
		"status":     chatResult.StatusCode,
		"chat":       chatPayload,
		"web_search": webPayload,
	}
	if chatErr != nil {
		resp["error"] = chatErr.Error()
	}
	c.JSON(http.StatusOK, resp)
}

// ActiveAIConfig returns the enabled profile with decrypted API key for server use.
func ActiveAIConfig() (models.AIProfile, string, bool) {
	db := config.GetDB()
	var profile models.AIProfile
	if err := db.Where("enabled = ?", true).Order("updated_at DESC").First(&profile).Error; err != nil {
		return models.AIProfile{}, "", false
	}
	plain := ""
	if strings.TrimSpace(profile.APIKey) != "" {
		if dec, err := utils.Decrypt(profile.APIKey); err == nil {
			plain = dec
		} else {
			plain = profile.APIKey
		}
	}
	return profile, plain, true
}

func toAIProfileDTOs(rows []models.AIProfile) []aiProfileDTO {
	out := make([]aiProfileDTO, 0, len(rows))
	for _, row := range rows {
		plain := ""
		if strings.TrimSpace(row.APIKey) != "" {
			if dec, err := utils.Decrypt(row.APIKey); err == nil && dec != "" {
				plain = dec
			} else {
				plain = row.APIKey
			}
		}
		out = append(out, aiProfileDTO{
			ID:         row.ID,
			Name:       row.Name,
			Enabled:    row.Enabled,
			BaseURL:    row.BaseURL,
			APIKey:     maskSecret(plain),
			APIKeySet:  plain != "",
			Model:      row.Model,
			TimeoutSec: row.TimeoutSec,
		})
	}
	return out
}

func newAIProfileID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("ai-%d", len(buf))
	}
	return "ai-" + hex.EncodeToString(buf)
}

func maskSecret(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if len(value) <= 8 {
		return "********"
	}
	return value[:4] + strings.Repeat("*", len(value)-8) + value[len(value)-4:]
}

func looksMaskedSecret(value string) bool {
	return strings.Contains(value, "*")
}
