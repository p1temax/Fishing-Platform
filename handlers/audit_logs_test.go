package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"fishing-platform-backend/audit"
	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupAuditTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "audit.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	config.DB = db
	return db
}

func TestListAuditLogsFiltersAndDefaultOrder(t *testing.T) {
	db := setupAuditTestDB(t)
	now := time.Now()
	older := now.Add(-2 * time.Hour)
	newer := now.Add(-1 * time.Hour)

	rows := []models.AuditLog{
		{ActorName: "alice", Action: "create_project", Summary: "alice create project #1", ClientIP: "1.1.1.1", CreatedAt: older, StatusCode: 200, Success: true},
		{ActorName: "bob", Action: "delete_robot", Summary: "bob delete robot #2", ClientIP: "2.2.2.2", CreatedAt: newer, StatusCode: 200, Success: true},
		{ActorName: "alice", Action: "login_failed", Summary: "alice login failed", ClientIP: "3.3.3.3", CreatedAt: now, StatusCode: 401, Success: false},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/audit-logs/", ListAuditLogs)

	t.Run("default newest first", func(t *testing.T) {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/audit-logs/", nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			Items []models.AuditLog `json:"items"`
			Total int64             `json:"total"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Total != 3 {
			t.Fatalf("total = %d", payload.Total)
		}
		if len(payload.Items) != 3 || payload.Items[0].Action != "login_failed" {
			t.Fatalf("unexpected order: %+v", payload.Items)
		}
	})

	t.Run("filter by action and actor", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/audit-logs/?action=create_project&actor=ali", nil)
		router.ServeHTTP(rec, req)
		var payload struct {
			Items []models.AuditLog `json:"items"`
			Total int64             `json:"total"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &payload)
		if payload.Total != 1 || payload.Items[0].ActorName != "alice" {
			t.Fatalf("unexpected filter result: %+v", payload)
		}
	})

	t.Run("filter by keyword", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/audit-logs/?keyword=2.2.2.2", nil)
		router.ServeHTTP(rec, req)
		var payload struct {
			Items []models.AuditLog `json:"items"`
			Total int64             `json:"total"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &payload)
		if payload.Total != 1 || payload.Items[0].Action != "delete_robot" {
			t.Fatalf("unexpected keyword result: %+v", payload)
		}
	})
}

func TestListAuditActions(t *testing.T) {
	db := setupAuditTestDB(t)
	for _, action := range []string{"create_project", "login_success", "create_project"} {
		if err := db.Create(&models.AuditLog{Action: action, CreatedAt: time.Now()}).Error; err != nil {
			t.Fatal(err)
		}
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/audit-logs/actions/", ListAuditActions)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/audit-logs/actions/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var actions []string
	if err := json.Unmarshal(rec.Body.Bytes(), &actions); err != nil {
		t.Fatal(err)
	}
	if len(actions) != 2 {
		t.Fatalf("actions = %#v", actions)
	}
}

func TestSanitizeAuditDetailRedactsSecrets(t *testing.T) {
	raw := `{"username":"alice","password":"s3cret","token":"abc"}`
	got := audit.SanitizeAuditDetail(raw)
	if got == "" || containsLiteral(got, "s3cret") || containsLiteral(got, `"abc"`) {
		t.Fatalf("detail not redacted: %s", got)
	}
	if !containsLiteral(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker: %s", got)
	}
}

func TestDeriveAuditAction(t *testing.T) {
	cases := map[string]string{
		audit.DeriveAuditAction(http.MethodPost, "/api/projects/"):                      "create_project",
		audit.DeriveAuditAction(http.MethodDelete, "/api/robots/3/"):                    "delete_robot",
		audit.DeriveAuditAction(http.MethodGet, "/api/projects/9/credentials/"):         "view_credentials",
		audit.DeriveAuditAction(http.MethodPost, "/api/robots/3/online/"):               "online_robot",
		audit.DeriveAuditAction(http.MethodPost, "/api/projects/1/deployments/2/stop/"): "stop_deployment",
	}
	for got, want := range cases {
		if got != want {
			t.Fatalf("got %q want %q", got, want)
		}
	}
}

func containsLiteral(s, lit string) bool {
	return len(s) >= len(lit) && (s == lit || len(lit) == 0 || (len(s) > 0 && (stringIndex(s, lit) >= 0)))
}

func stringIndex(s, lit string) int {
	for i := 0; i+len(lit) <= len(s); i++ {
		if s[i:i+len(lit)] == lit {
			return i
		}
	}
	return -1
}
