package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type rewriteUploadRequest struct {
	Name         string `json:"name" binding:"required"`
	HTML         string `json:"html" binding:"required"`
	Description  string `json:"description"`
	URL          string `json:"url"`
	SubmitURL    string `json:"submit_url"`
	RedirectURL  string `json:"redirect_url"`
	OriginalHost string `json:"original_host"`
}

// RewriteUploadPhishingPage AI-rewrites uploaded HTML then upserts the phishing page.
// Progress is streamed as SSE events (same shape as URL mirror).
func RewriteUploadPhishingPage(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}

	var req rewriteUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and html are required"})
		return
	}
	name := strings.TrimSpace(req.Name)
	html := strings.TrimSpace(req.HTML)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Page name is required"})
		return
	}
	if html == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "HTML content is required"})
		return
	}
	if len(html) > 30<<20 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "HTML files cannot exceed 30 MB"})
		return
	}

	profile, apiKey, active := ActiveAIConfig()
	if !active {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No AI model is enabled. Open System → AI Settings to configure and test one."})
		return
	}
	if strings.TrimSpace(apiKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enabled AI profile has no API key. Open System → AI Settings to fix and test it."})
		return
	}

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming unsupported"})
		return
	}

	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)
	flusher.Flush()

	var writeMu sync.Mutex
	writeSSE := func(eventName string, payload []byte) bool {
		writeMu.Lock()
		defer writeMu.Unlock()
		if c.Request.Context().Err() != nil {
			return false
		}
		if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventName, payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	emit := func(ev mirrorProgressEvent) bool {
		raw, err := json.Marshal(ev)
		if err != nil {
			return false
		}
		return writeSSE("progress", raw)
	}
	fail := func(stage, msg string) {
		_ = emit(mirrorProgressEvent{
			Stage:       "failed",
			StageFailed: stage,
			Error:       msg,
			Message:     msg,
		})
	}

	submitURL := strings.TrimSpace(req.SubmitURL)
	if submitURL == "" {
		submitURL = utils.DefaultMirrorSubmitURL
	}
	redirectURL := strings.TrimSpace(req.RedirectURL)
	pageURL := strings.TrimSpace(req.URL)
	originalHost := strings.TrimSpace(req.OriginalHost)
	if originalHost == "" && pageURL != "" {
		if parsed, err := url.Parse(pageURL); err == nil {
			originalHost = parsed.Host
		}
	}

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout+45*time.Second)
	defer cancel()

	pingStop := make(chan struct{})
	defer close(pingStop)
	go func() {
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-pingStop:
				return
			case <-c.Request.Context().Done():
				return
			case <-ticker.C:
				_ = writeSSE("ping", []byte("{}"))
			}
		}
	}()

	if !emit(mirrorProgressEvent{
		Stage:   "preparing",
		Percent: 20,
		Message: "Preparing uploaded HTML for AI",
		Detail: map[string]interface{}{
			"host":  originalHost,
			"bytes": len(html),
		},
	}) {
		return
	}
	aiHTML := utils.StripHeavyBase64DataURIs(html)

	if !emit(mirrorProgressEvent{
		Stage:   "rewriting",
		Percent: 40,
		Message: "AI rewriting login form",
		Detail: map[string]interface{}{
			"model": profile.Model,
			"host":  originalHost,
			"bytes": len(aiHTML),
		},
	}) {
		return
	}
	rewritten, err := utils.RewriteHTMLWithAI(ctx, profile.BaseURL, apiKey, profile.Model, timeout, utils.MirrorRewriteInput{
		RawHTML:      aiHTML,
		SubmitURL:    submitURL,
		RedirectURL:  redirectURL,
		OriginalHost: originalHost,
	})
	if err != nil {
		fail("rewriting", "AI rewrite failed: "+err.Error())
		return
	}
	if !emit(mirrorProgressEvent{Stage: "rewriting", Percent: 80, Message: "AI rewrite finished"}) {
		return
	}

	if !emit(mirrorProgressEvent{Stage: "validating", Percent: 90, Message: "Validating rewritten HTML"}) {
		return
	}
	if err := utils.ValidateRewrittenHTML(rewritten, submitURL); err != nil {
		fail("validating", "AI rewrite failed validation: "+err.Error())
		return
	}

	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = "Uploaded HTML (AI rewritten)"
	}

	if !emit(mirrorProgressEvent{Stage: "saving", Percent: 95, Message: "Saving rewritten page"}) {
		return
	}

	db := config.GetDB()
	overwritten := false
	var existing models.PhishingPage
	findErr := db.Where("created_by = ? AND LOWER(name) = LOWER(?)", user.ID, name).
		First(&existing).Error
	if findErr == nil {
		existing.Name = name
		if pageURL != "" {
			existing.URL = pageURL
		}
		existing.Description = desc
		existing.Html = rewritten
		if err := db.Save(&existing).Error; err != nil {
			fail("saving", "Failed to update phishing page")
			return
		}
		overwritten = true
		_ = emit(mirrorProgressEvent{
			Stage:   "done",
			Percent: 100,
			Message: "Page rewritten and saved",
			Detail:  map[string]interface{}{"overwritten": true},
			Page:    existing,
		})
		return
	}
	if findErr != nil && !errors.Is(findErr, gorm.ErrRecordNotFound) {
		fail("saving", "Failed to look up phishing page")
		return
	}

	page := models.PhishingPage{
		Name:        name,
		URL:         pageURL,
		Description: desc,
		Html:        rewritten,
		CreatedBy:   user.ID,
	}
	if err := db.Create(&page).Error; err != nil {
		fail("saving", "Failed to save phishing page")
		return
	}
	_ = emit(mirrorProgressEvent{
		Stage:   "done",
		Percent: 100,
		Message: "Page rewritten and saved",
		Detail:  map[string]interface{}{"overwritten": overwritten},
		Page:    page,
	})
}
