package middleware

import (
	"bytes"
	"io"
	"net/http"
	"strings"

	"fishing-platform-backend/audit"
	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
)

type auditStatusWriter struct {
	gin.ResponseWriter
	status int
}

func (w *auditStatusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// AuditMiddleware records operator actions for mutating requests and selected
// sensitive reads. It never blocks the request on audit write failure.
func AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !shouldAuditRequest(c.Request.Method, c.Request.URL.Path) {
			c.Next()
			return
		}

		var bodyPreview string
		if c.Request.Body != nil && c.Request.Method != http.MethodGet {
			// Read enough for handlers (e.g. HTML upsert), but only store a
			// short preview in the audit log. Previously restoring only 4KB
			// truncated large JSON bodies and broke binding downstream.
			const maxBodyBytes = 32 << 20 // 32 MiB
			const maxPreviewBytes = 4096
			raw, err := io.ReadAll(io.LimitReader(c.Request.Body, maxBodyBytes))
			_ = c.Request.Body.Close()
			if err == nil {
				if len(raw) > maxPreviewBytes {
					bodyPreview = string(raw[:maxPreviewBytes])
				} else {
					bodyPreview = string(raw)
				}
				c.Request.Body = io.NopCloser(bytes.NewBuffer(raw))
			} else {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(nil))
			}
		}

		writer := &auditStatusWriter{ResponseWriter: c.Writer, status: http.StatusOK}
		c.Writer = writer

		c.Next()

		status := writer.status
		if status == 0 {
			status = c.Writer.Status()
		}

		actorID, actorName := actorFromContext(c)
		resourceType, resourceID, _ := audit.ParseAPIResource(c.Request.URL.Path)
		action := audit.DeriveAuditAction(c.Request.Method, c.Request.URL.Path)
		summary := audit.BuildAuditSummary(action, actorName, resourceType, resourceID, status)

		audit.RecordAudit(config.GetDB(), audit.AuditEntry{
			ActorID:      actorID,
			ActorName:    actorName,
			Action:       action,
			ResourceType: resourceType,
			ResourceID:   resourceID,
			Method:       c.Request.Method,
			Path:         c.Request.URL.Path,
			StatusCode:   status,
			Summary:      summary,
			Detail:       audit.SanitizeAuditDetail(bodyPreview),
			ClientIP:     c.ClientIP(),
			UserAgent:    c.Request.UserAgent(),
		})
	}
}

func shouldAuditRequest(method, path string) bool {
	method = strings.ToUpper(method)
	path = strings.TrimSpace(path)
	pathLower := strings.ToLower(path)

	// Never audit the audit-log list endpoints themselves.
	if strings.HasPrefix(pathLower, "/api/audit-logs") {
		return false
	}
	// Skip high-frequency machine push-log ingestion.
	if strings.HasPrefix(pathLower, "/api/robot_push_logs") {
		return false
	}

	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return strings.HasPrefix(pathLower, "/api/")
	case http.MethodGet:
		return isSensitiveRead(pathLower)
	default:
		return false
	}
}

func isSensitiveRead(pathLower string) bool {
	switch {
	case strings.HasPrefix(pathLower, "/api/credentials"):
		return true
	case strings.Contains(pathLower, "/credentials"):
		return true
	case strings.Contains(pathLower, "/container_log"):
		return true
	case strings.Contains(pathLower, "/logs") && strings.HasPrefix(pathLower, "/api/projects/"):
		// Central project logs: /api/projects/:id/logs/
		return true
	default:
		return false
	}
}

func actorFromContext(c *gin.Context) (*uint, string) {
	raw, ok := c.Get("user")
	if !ok {
		return nil, ""
	}
	user, ok := raw.(models.User)
	if !ok {
		return nil, ""
	}
	id := user.ID
	return &id, user.Username
}
