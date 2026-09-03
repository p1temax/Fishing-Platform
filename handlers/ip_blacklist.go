package handlers

import (
	"errors"
	"net/http"
	"net/netip"
	"strings"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ipBlacklistPayload struct {
	IPAddress    string `json:"ip_address"`
	Reason       string `json:"reason"`
	Enabled      *bool  `json:"enabled"`
	CredentialID *uint  `json:"credential_id"`
}

func normalizeBlacklistIP(value string) (string, error) {
	candidate := strings.TrimSpace(value)
	if candidate == "" {
		return "", errors.New("IP address is required")
	}

	if strings.Contains(candidate, "/") {
		prefix, err := netip.ParsePrefix(candidate)
		if err != nil {
			return "", errors.New("Invalid IP or CIDR")
		}
		return prefix.Masked().String(), nil
	}

	addr, err := netip.ParseAddr(candidate)
	if err != nil {
		return "", errors.New("Invalid IP or CIDR")
	}
	return addr.String(), nil
}

func isIPBlacklisted(db *gorm.DB, ipAddress string) bool {
	addr, err := netip.ParseAddr(strings.TrimSpace(ipAddress))
	if err != nil {
		return false
	}

	var entries []models.IPBlacklist
	if err := db.Where("enabled = ?", true).Find(&entries).Error; err != nil {
		return false
	}

	for _, entry := range entries {
		value := strings.TrimSpace(entry.IPAddress)
		if value == "" {
			continue
		}
		if strings.Contains(value, "/") {
			prefix, err := netip.ParsePrefix(value)
			if err == nil && prefix.Contains(addr) {
				return true
			}
			continue
		}
		blockedAddr, err := netip.ParseAddr(value)
		if err == nil && blockedAddr == addr {
			return true
		}
	}

	return false
}

func GetIPBlacklist(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var entries []models.IPBlacklist

	if err := scopeByOwner(db, user).Order("created_at DESC").Find(&entries).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve IP blacklist"})
		return
	}

	c.JSON(http.StatusOK, entries)
}

func GetIPBlacklistEntry(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	id := c.Param("id")
	var entry models.IPBlacklist

	if err := scopeByOwner(db, user).First(&entry, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "IP blacklist entry not found"})
		return
	}

	c.JSON(http.StatusOK, entry)
}

func CreateIPBlacklistEntry(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	var payload ipBlacklistPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if payload.CredentialID == nil || *payload.CredentialID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IP blacklist entries can only be created from captured credential IPs"})
		return
	}

	db := config.GetDB()
	var credential models.Credential
	credQuery := db.Model(&models.Credential{}).Where("id = ?", *payload.CredentialID)
	if !user.Role.IsAdmin() {
		credQuery = credQuery.Where(
			"project_id IN (?)",
			db.Model(&models.Project{}).Select("id").Where("created_by = ?", user.ID),
		)
	}
	if err := credQuery.First(&credential).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Credential not found or not accessible"})
		return
	}

	credentialIP, err := normalizeBlacklistIP(credential.IPAddress)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Credential has no valid IP address"})
		return
	}

	ipAddress := credentialIP
	if strings.TrimSpace(payload.IPAddress) != "" {
		requestedIP, err := normalizeBlacklistIP(payload.IPAddress)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if requestedIP != credentialIP {
			c.JSON(http.StatusBadRequest, gin.H{"error": "IP must match the credential source IP"})
			return
		}
		ipAddress = requestedIP
	}

	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	reason := strings.TrimSpace(payload.Reason)
	if reason == "" {
		reason = "Blocked from captured credential"
	}

	entry := models.IPBlacklist{
		IPAddress: ipAddress,
		Reason:    reason,
		Enabled:   enabled,
		CreatedBy: user.ID,
	}

	if err := db.Create(&entry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create IP blacklist entry"})
		return
	}

	if entry.Enabled {
		if err := utils.ApplyHostIPBlock(entry.IPAddress); err != nil {
			_ = db.Delete(&entry).Error
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to apply host firewall rule: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusCreated, entry)
}

func UpdateIPBlacklistEntry(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	id := c.Param("id")

	var entry models.IPBlacklist
	if err := scopeByOwner(db, user).First(&entry, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "IP blacklist entry not found"})
		return
	}
	previousEntry := entry
	ownerID := entry.CreatedBy

	var payload ipBlacklistPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	// IP address is immutable after creation; only reason/enabled can change.
	entry.Reason = strings.TrimSpace(payload.Reason)
	if payload.Enabled != nil {
		entry.Enabled = *payload.Enabled
	}
	entry.CreatedBy = ownerID
	entry.IPAddress = previousEntry.IPAddress

	if entry.Enabled && !previousEntry.Enabled {
		if err := utils.ApplyHostIPBlock(entry.IPAddress); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to apply host firewall rule: " + err.Error()})
			return
		}
	}

	if err := db.Save(&entry).Error; err != nil {
		if entry.Enabled && !previousEntry.Enabled {
			_ = utils.RemoveHostIPBlock(entry.IPAddress)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update IP blacklist entry"})
		return
	}

	if previousEntry.Enabled && !entry.Enabled {
		if err := utils.RemoveHostIPBlock(previousEntry.IPAddress); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Updated database, but failed to remove host firewall rule: " + err.Error()})
			return
		}
	}

	c.JSON(http.StatusOK, entry)
}

func DeleteIPBlacklistEntry(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	id := c.Param("id")

	var entry models.IPBlacklist
	if err := scopeByOwner(db, user).First(&entry, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "IP blacklist entry not found"})
		return
	}

	if entry.Enabled {
		if err := utils.RemoveHostIPBlock(entry.IPAddress); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove host firewall rule: " + err.Error()})
			return
		}
	}

	if err := db.Delete(&entry).Error; err != nil {
		if entry.Enabled {
			_ = utils.ApplyHostIPBlock(entry.IPAddress)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete IP blacklist entry"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func SyncEnabledIPBlacklistToHostFirewall(db *gorm.DB) error {
	var entries []models.IPBlacklist
	if err := db.Where("enabled = ?", true).Find(&entries).Error; err != nil {
		return err
	}

	for _, entry := range entries {
		if err := utils.ApplyHostIPBlock(entry.IPAddress); err != nil {
			return err
		}
	}

	return nil
}
