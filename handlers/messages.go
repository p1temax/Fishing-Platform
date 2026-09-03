package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"fishing-platform-backend/models"
	"fishing-platform-backend/config"
)

// CreateMessage creates a new message
func CreateMessage(c *gin.Context) {
	var message models.Message
	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	db := config.GetDB()
	if err := db.Create(&message).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create message"})
		return
	}

	c.JSON(http.StatusCreated, message)
}

// GetMessages retrieves messages, optionally filtered by project_id
func GetMessages(c *gin.Context) {
	db := config.GetDB()
	var messages []models.Message

	projectID := c.Query("project_id")
	query := db.Order("created_at DESC")

	if projectID != "" {
		query = query.Where("project_id = ?", projectID)
	}

	if err := query.Find(&messages).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve messages"})
		return
	}

	// Limit to 5 messages
	if len(messages) > 5 {
		messages = messages[:5]
	}

	c.JSON(http.StatusOK, messages)
}

// GetMessage retrieves a single message by ID
func GetMessage(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()
	var message models.Message

	if err := db.First(&message, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}

	c.JSON(http.StatusOK, message)
}

// UpdateMessage updates an existing message
func UpdateMessage(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var message models.Message
	if err := db.First(&message, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}

	if err := c.ShouldBindJSON(&message); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	if err := db.Save(&message).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update message"})
		return
	}

	c.JSON(http.StatusOK, message)
}

// DeleteMessage deletes a message
func DeleteMessage(c *gin.Context) {
	id := c.Param("id")
	db := config.GetDB()

	var message models.Message
	if err := db.First(&message, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Message not found"})
		return
	}

	if err := db.Delete(&message).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete message"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}