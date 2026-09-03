package audit

import (
	"encoding/json"
	"log"
	"net/http"
	"regexp"
	"strings"
	"time"

	"fishing-platform-backend/models"
	"gorm.io/gorm"
)

var sensitiveJSONKeys = regexp.MustCompile(`(?i)"(password|secret|token|webhook|authorization|access|refresh|captchavalue)"\s*:\s*"(?:\\.|[^"\\])*"`)

// AuditEntry is the input payload for persisting an audit log row.
type AuditEntry struct {
	ActorID      *uint
	ActorName    string
	Action       string
	ResourceType string
	ResourceID   string
	Method       string
	Path         string
	StatusCode   int
	Summary      string
	Detail       string
	ClientIP     string
	UserAgent    string
}

// RecordAudit persists an immutable audit log. Failures are logged but never returned
// to callers so audit write issues do not break business flows.
func RecordAudit(db *gorm.DB, entry AuditEntry) {
	if db == nil {
		return
	}

	action := strings.TrimSpace(entry.Action)
	if action == "" {
		action = "unknown"
	}

	status := entry.StatusCode
	if status == 0 {
		status = http.StatusOK
	}

	row := models.AuditLog{
		ActorID:      entry.ActorID,
		ActorName:    truncate(strings.TrimSpace(entry.ActorName), 100),
		Action:       truncate(action, 64),
		ResourceType: truncate(strings.TrimSpace(entry.ResourceType), 64),
		ResourceID:   truncate(strings.TrimSpace(entry.ResourceID), 64),
		Method:       truncate(strings.ToUpper(strings.TrimSpace(entry.Method)), 16),
		Path:         truncate(strings.TrimSpace(entry.Path), 255),
		StatusCode:   status,
		Success:      status >= 200 && status < 400,
		Summary:      truncate(strings.TrimSpace(entry.Summary), 500),
		Detail:       SanitizeAuditDetail(entry.Detail),
		ClientIP:     truncate(strings.TrimSpace(entry.ClientIP), 64),
		UserAgent:    truncate(strings.TrimSpace(entry.UserAgent), 255),
		CreatedAt:    time.Now(),
	}

	if err := db.Create(&row).Error; err != nil {
		log.Printf("audit log write failed: %v", err)
	}
}

// SanitizeAuditDetail redacts common secret fields and caps size.
func SanitizeAuditDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return ""
	}
	// Redact JSON string values for sensitive keys.
	detail = sensitiveJSONKeys.ReplaceAllString(detail, `"$1":"[REDACTED]"`)
	lower := strings.ToLower(detail)
	for _, key := range []string{"password=", "secret=", "token=", "authorization="} {
		if idx := strings.Index(lower, key); idx >= 0 {
			// Best-effort form-body redaction: truncate from the secret key onward.
			detail = detail[:idx] + key + "[REDACTED]"
			break
		}
	}
	return truncate(detail, 4000)
}

// ParseAPIResource extracts resource type/id from common /api/... paths.
func ParseAPIResource(path string) (resourceType, resourceID, subResource string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", ""
	}
	// Strip query string.
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	// Expect: api, <resource>, [id], [sub], ...
	if len(parts) < 2 || parts[0] != "api" {
		return "", "", ""
	}

	resourceType = normalizeResourceType(parts[1])
	if len(parts) >= 3 && !isActionSegment(parts[2]) {
		resourceID = parts[2]
	}
	if len(parts) >= 4 {
		subResource = normalizeResourceType(parts[3])
		// Nested: /api/projects/:id/credentials/ → resource credentials under project
		if isNestedSensitive(subResource) {
			return subResource, resourceID, subResource
		}
		if len(parts) >= 5 && !isActionSegment(parts[4]) {
			// /api/projects/:id/deployments/:deploymentID/...
			if subResource != "" && resourceID != "" {
				return subResource, parts[4], parts[3]
			}
		}
	}
	return resourceType, resourceID, subResource
}

// DeriveAuditAction builds a stable action name from method + path.
func DeriveAuditAction(method, path string) string {
	method = strings.ToUpper(strings.TrimSpace(method))
	resourceType, resourceID, sub := ParseAPIResource(path)
	pathLower := strings.ToLower(path)

	switch {
	case strings.Contains(pathLower, "/credentials"):
		if method == http.MethodGet {
			return "view_credentials"
		}
	case strings.Contains(pathLower, "/container_log"):
		return "view_container_log"
	case strings.Contains(pathLower, "/logs") && method == http.MethodGet:
		return "view_project_logs"
	}

	verb := methodToVerb(method)
	name := resourceType
	if name == "" {
		name = "resource"
	}
	// Prefer leaf resource for nested write paths like deployments control.
	if sub != "" && (method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete) {
		if strings.Contains(pathLower, "/deployments/") {
			name = "deployment"
			if action := deploymentAction(pathLower); action != "" {
				return action + "_deployment"
			}
		} else if isNestedSensitive(sub) {
			name = sub
		}
	}

	// Special POST actions: /online/, /offline/, /test/, /build/, /send/, etc.
	if method == http.MethodPost {
		if action := trailingAction(pathLower); action != "" {
			return action + "_" + singularize(name)
		}
	}

	_ = resourceID
	return verb + "_" + singularize(name)
}

// BuildAuditSummary creates a short human-readable summary.
func BuildAuditSummary(action, actorName, resourceType, resourceID string, statusCode int) string {
	actor := strings.TrimSpace(actorName)
	if actor == "" {
		actor = "anonymous"
	}
	target := strings.TrimSpace(resourceType)
	if resourceID != "" {
		if target != "" {
			target = target + " #" + resourceID
		} else {
			target = "#" + resourceID
		}
	}
	if target == "" {
		target = "resource"
	}
	outcome := "ok"
	if statusCode >= 400 || statusCode == 0 {
		outcome = "failed"
	}
	return truncate(actor+" "+action+" "+target+" ("+outcome+")", 500)
}

// DetailFromJSONMap returns sanitized JSON for audit detail storage.
func DetailFromJSONMap(values map[string]any) string {
	if len(values) == 0 {
		return ""
	}
	redacted := map[string]any{}
	for k, v := range values {
		if isSensitiveKey(k) {
			redacted[k] = "[REDACTED]"
			continue
		}
		redacted[k] = v
	}
	b, err := json.Marshal(redacted)
	if err != nil {
		return ""
	}
	return SanitizeAuditDetail(string(b))
}

func methodToVerb(method string) string {
	switch method {
	case http.MethodPost:
		return "create"
	case http.MethodPut, http.MethodPatch:
		return "update"
	case http.MethodDelete:
		return "delete"
	case http.MethodGet:
		return "view"
	default:
		return strings.ToLower(method)
	}
}

func trailingAction(pathLower string) string {
	trimmed := strings.Trim(pathLower, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	switch last {
	case "online", "offline", "test", "build", "send", "start_container", "stop_container",
		"restart_container", "start_local", "stop_local", "restart_local":
		return last
	}
	return ""
}

func deploymentAction(pathLower string) string {
	trimmed := strings.Trim(pathLower, "/")
	parts := strings.Split(trimmed, "/")
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	switch last {
	case "start", "stop", "restart", "delete", "rebuild":
		return last
	}
	return "control"
}

func normalizeResourceType(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.ReplaceAll(raw, "-", "_")
	switch raw {
	case "ip_blacklist":
		return "ip_blacklist"
	case "smtp_services":
		return "smtp_service"
	case "mail_campaigns":
		return "mail_campaign"
	case "phishing_pages":
		return "phishing_page"
	case "robots":
		return "robot"
	case "projects":
		return "project"
	case "agents":
		return "agent"
	case "messages":
		return "message"
	case "credentials":
		return "credential"
	case "audit_logs":
		return "audit_log"
	case "robot_push_logs":
		return "robot_push_log"
	}
	return singularize(raw)
}

func singularize(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	if name == "" {
		return "resource"
	}
	if strings.HasSuffix(name, "ies") && len(name) > 3 {
		return name[:len(name)-3] + "y"
	}
	if strings.HasSuffix(name, "ses") || strings.HasSuffix(name, "sses") {
		return strings.TrimSuffix(name, "es")
	}
	if strings.HasSuffix(name, "s") && !strings.HasSuffix(name, "ss") {
		return strings.TrimSuffix(name, "s")
	}
	return name
}

func isActionSegment(seg string) bool {
	seg = strings.ToLower(seg)
	switch seg {
	case "online", "offline", "test", "build", "build_status", "send", "start_container",
		"stop_container", "restart_container", "start_local", "stop_local", "restart_local",
		"container_log", "logs", "credentials", "deployments", "push_logs", "messages",
		"recipients", "events", "actions":
		return true
	}
	return false
}

func isNestedSensitive(sub string) bool {
	switch sub {
	case "credential", "credentials", "container_log", "log", "logs":
		return true
	}
	return false
}

func isSensitiveKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "password", "secret", "token", "webhook", "authorization", "access", "refresh", "captchavalue":
		return true
	}
	return false
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
