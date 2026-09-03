package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"fishing-platform-backend/config"
	"fishing-platform-backend/middleware"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func TestOperatorOnlySeesOwnProjects(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "own.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	config.DB = db
	middleware.InitJWT()

	hashed, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.DefaultCost)
	admin := models.User{Username: "admin", Password: string(hashed), Role: models.UserRoleAdmin}
	ops := models.User{Username: "ops", Password: string(hashed), Role: models.UserRoleOperator}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ops).Error; err != nil {
		t.Fatal(err)
	}
	adminProject := models.Project{Name: "admin-p", CreatedBy: admin.ID}
	opsProject := models.Project{Name: "ops-p", CreatedBy: ops.ID}
	if err := db.Create(&adminProject).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&opsProject).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.AuthMiddleware())
	router.GET("/projects/", GetProjects)
	router.GET("/projects/:id/", GetProject)

	token, err := middleware.GenerateToken(ops)
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/projects/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}
	var listed []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0]["name"] != "ops-p" {
		t.Fatalf("listed=%v", listed)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/projects/1/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("foreign project status=%d body=%s", rec.Code, rec.Body.String())
	}
}
