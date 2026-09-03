package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"fishing-platform-backend/config"
	"fishing-platform-backend/middleware"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestAISettingsStoredInDatabaseNotConfig(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "ai.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	config.DB = db
	middleware.InitJWT()
	_ = utils.InitEncryption("test-encryption-key-for-ai-settings-123")

	hashed, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	admin := models.User{Username: "admin", Password: string(hashed), Role: models.UserRoleAdmin}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api")
	group.Use(middleware.AuthMiddleware(), middleware.RequireAdmin())
	group.GET("/ai-settings/", GetAISettings)
	group.PUT("/ai-settings/", UpdateAISettings)

	token, err := middleware.GenerateToken(admin)
	if err != nil {
		t.Fatal(err)
	}

	body := []byte(`{"profiles":[{"name":"SpaceXAI","enabled":true,"base_url":"https://api.x.ai/v1","api_key":"sk-test-key-12345678","model":"grok-4.5","timeout_sec":60},{"name":"Backup","enabled":true,"base_url":"https://api.openai.com/v1","api_key":"sk-backup-key","model":"gpt-4.1","timeout_sec":60}]}`)
	req := httptest.NewRequest(http.MethodPut, "/api/ai-settings/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("two enabled status=%d body=%s", rec.Code, rec.Body.String())
	}

	body = []byte(`{"profiles":[{"name":"SpaceXAI","enabled":true,"base_url":"https://api.x.ai/v1","api_key":"sk-test-key-12345678","model":"grok-4.5","timeout_sec":60},{"name":"Backup","enabled":false,"base_url":"https://api.openai.com/v1","api_key":"sk-backup-key","model":"gpt-4.1","timeout_sec":60}]}`)
	req = httptest.NewRequest(http.MethodPut, "/api/ai-settings/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	var saved int64
	if err := db.Model(&models.AIProfile{}).Count(&saved).Error; err != nil {
		t.Fatal(err)
	}
	if saved != 2 {
		t.Fatalf("saved=%d", saved)
	}

	profile, key, ok := ActiveAIConfig()
	if !ok || profile.Model != "grok-4.5" || key != "sk-test-key-12345678" {
		t.Fatalf("active=%+v key=%q ok=%v", profile, key, ok)
	}

	var payload aiSettingsDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.Profiles[0].APIKeySet || !stringsContainsStar(payload.Profiles[0].APIKey) {
		t.Fatalf("expected masked key: %+v", payload.Profiles[0])
	}
}

func stringsContainsStar(s string) bool {
	return len(s) > 0 && (s == "********" || len(s) >= 4 && s[4] == '*')
}
