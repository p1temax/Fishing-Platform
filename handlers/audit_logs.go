package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
)

// ListAuditLogs returns paginated audit logs with filters.
// Sorted by created_at DESC. No delete/update endpoints exist by design.
func ListAuditLogs(c *gin.Context) {
	db := config.GetDB()
	query := db.Model(&models.AuditLog{})

	if action := strings.TrimSpace(c.Query("action")); action != "" {
		query = query.Where("action = ?", action)
	}
	if actor := strings.TrimSpace(c.Query("actor")); actor != "" {
		query = query.Where("actor_name LIKE ?", "%"+escapeLike(actor)+"%")
	}
	if keyword := strings.TrimSpace(c.Query("keyword")); keyword != "" {
		like := "%" + escapeLike(keyword) + "%"
		query = query.Where(
			"summary LIKE ? OR path LIKE ? OR detail LIKE ? OR client_ip LIKE ? OR resource_id LIKE ? OR action LIKE ?",
			like, like, like, like, like, like,
		)
	}
	if from, ok := parseAuditTime(c.Query("from"), false); ok {
		query = query.Where("created_at >= ?", from)
	}
	if to, ok := parseAuditTime(c.Query("to"), true); ok {
		query = query.Where("created_at <= ?", to)
	}

	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 50)
	if pageSize > 200 {
		pageSize = 200
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count audit logs"})
		return
	}

	var items []models.AuditLog
	offset := (page - 1) * pageSize
	if err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(pageSize).Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list audit logs"})
		return
	}
	if items == nil {
		items = []models.AuditLog{}
	}

	c.JSON(http.StatusOK, gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ListAuditActions returns distinct action names for filter dropdowns.
func ListAuditActions(c *gin.Context) {
	db := config.GetDB()
	var actions []string
	if err := db.Model(&models.AuditLog{}).
		Distinct("action").
		Where("action <> ''").
		Order("action ASC").
		Pluck("action", &actions).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list audit actions"})
		return
	}
	if actions == nil {
		actions = []string{}
	}
	c.JSON(http.StatusOK, actions)
}

func parsePositiveInt(raw string, fallback int) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		return fallback
	}
	return n
}

func parseAuditTime(raw string, endOfDay bool) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, true
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04", raw, time.Local); err == nil {
		return t, true
	}
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", raw, time.Local); err == nil {
		return t, true
	}
	if t, err := time.ParseInLocation("2006-01-02", raw, time.Local); err == nil {
		if endOfDay {
			return t.Add(24*time.Hour - time.Nanosecond), true
		}
		return t, true
	}
	return time.Time{}, false
}

func escapeLike(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(value)
}
