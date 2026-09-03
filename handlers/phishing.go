package handlers

import (
	"errors"
	"net/http"
	"strings"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type phishingPageSummary struct {
	ID          uint      `json:"id"`
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Description string    `json:"description"`
	CreatedBy   uint      `json:"created_by"`
	CreatedAt   any       `json:"created_at"`
	UpdatedAt   any       `json:"updated_at"`
}

func toPhishingPageSummary(page models.PhishingPage) phishingPageSummary {
	return phishingPageSummary{
		ID:          page.ID,
		Name:        page.Name,
		URL:         page.URL,
		Description: page.Description,
		CreatedBy:   page.CreatedBy,
		CreatedAt:   page.CreatedAt,
		UpdatedAt:   page.UpdatedAt,
	}
}

type upsertPhishingPageRequest struct {
	Name        string `json:"name" binding:"required"`
	URL         string `json:"url"`
	Description string `json:"description"`
	Html        string `json:"html" binding:"required"`
}

// CreatePhishingPage creates a new phishing page
func CreatePhishingPage(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	var phishingPage models.PhishingPage
	if err := c.ShouldBindJSON(&phishingPage); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	phishingPage.Name = strings.TrimSpace(phishingPage.Name)
	phishingPage.URL = strings.TrimSpace(phishingPage.URL)
	if phishingPage.Name == "" || phishingPage.Html == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name and HTML are required"})
		return
	}
	phishingPage.CreatedBy = user.ID

	db := config.GetDB()
	if err := db.Create(&phishingPage).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create phishing page"})
		return
	}

	c.JSON(http.StatusCreated, phishingPage)
}

// UpsertPhishingPage creates a phishing page or overwrites an owned page with the same name.
func UpsertPhishingPage(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}

	var req upsertPhishingPageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	name := strings.TrimSpace(req.Name)
	html := strings.TrimSpace(req.Html)
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

	db := config.GetDB()
	url := strings.TrimSpace(req.URL)
	desc := strings.TrimSpace(req.Description)
	if desc == "" {
		desc = "Uploaded HTML"
	}

	applyFields := func(page *models.PhishingPage) {
		page.Name = name
		if url != "" {
			page.URL = url
		}
		page.Description = desc
		page.Html = html
	}

	// Always match the current user's own pages (even for admin) so a
	// same-name upload replaces instead of creating another card.
	var existing models.PhishingPage
	findErr := db.Where("created_by = ? AND LOWER(name) = LOWER(?)", user.ID, name).
		First(&existing).Error
	if findErr == nil {
		applyFields(&existing)
		if err := db.Save(&existing).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update phishing page"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"page": existing, "overwritten": true})
		return
	}
	if !errors.Is(findErr, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to look up phishing page"})
		return
	}

	page := models.PhishingPage{
		Name:        name,
		URL:         url,
		Description: desc,
		Html:        html,
		CreatedBy:   user.ID,
	}
	if err := db.Create(&page).Error; err != nil {
		// Concurrent same-name create lost the race; update the winner.
		var raced models.PhishingPage
		if retryErr := db.Where("created_by = ? AND LOWER(name) = LOWER(?)", user.ID, name).
			First(&raced).Error; retryErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create phishing page"})
			return
		}
		applyFields(&raced)
		if saveErr := db.Save(&raced).Error; saveErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update phishing page"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"page": raced, "overwritten": true})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"page": page, "overwritten": false})
}

// GetPhishingPages retrieves phishing pages (without HTML bodies for list performance).
func GetPhishingPages(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var phishingPages []models.PhishingPage

	if err := scopeByOwner(db, user).
		Select("id", "name", "url", "description", "created_by", "created_at", "updated_at").
		Order("created_at DESC").
		Find(&phishingPages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve phishing pages"})
		return
	}

	out := make([]phishingPageSummary, 0, len(phishingPages))
	for _, page := range phishingPages {
		out = append(out, toPhishingPageSummary(page))
	}
	c.JSON(http.StatusOK, out)
}

// GetPhishingPage retrieves a single phishing page by ID
func GetPhishingPage(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()
	var phishingPage models.PhishingPage

	if err := scopeByOwner(db, user).First(&phishingPage, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Phishing page not found"})
		return
	}

	c.JSON(http.StatusOK, phishingPage)
}

// UpdatePhishingPage updates an existing phishing page
func UpdatePhishingPage(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()

	var phishingPage models.PhishingPage
	if err := scopeByOwner(db, user).First(&phishingPage, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Phishing page not found"})
		return
	}
	ownerID := phishingPage.CreatedBy

	var payload models.PhishingPage
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	phishingPage.Name = strings.TrimSpace(payload.Name)
	phishingPage.URL = strings.TrimSpace(payload.URL)
	phishingPage.Description = payload.Description
	phishingPage.Html = payload.Html
	phishingPage.CreatedBy = ownerID
	if phishingPage.Name == "" || phishingPage.Html == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name and HTML are required"})
		return
	}

	if err := db.Save(&phishingPage).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update phishing page"})
		return
	}

	c.JSON(http.StatusOK, phishingPage)
}

// DeletePhishingPage deletes a phishing page
func DeletePhishingPage(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()

	var phishingPage models.PhishingPage
	if err := scopeByOwner(db, user).First(&phishingPage, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Phishing page not found"})
		return
	}

	if err := db.Delete(&phishingPage).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete phishing page"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}
