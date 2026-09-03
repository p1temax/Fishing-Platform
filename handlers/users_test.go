package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	"fishing-platform-backend/config"
	"fishing-platform-backend/middleware"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func setupUsersTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "users.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	config.DB = db
	middleware.InitJWT()
	return db
}

func createTestUser(t *testing.T, db *gorm.DB, username, password string, role models.UserRole) models.User {
	t.Helper()
	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	user := models.User{Username: username, Password: string(hashed), Role: role}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func TestOperatorCannotAccessAuditLogs(t *testing.T) {
	db := setupUsersTestDB(t)
	admin := createTestUser(t, db, "admin", "password123", models.UserRoleAdmin)
	operator := createTestUser(t, db, "ops", "password123", models.UserRoleOperator)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api")
	protected.Use(middleware.AuthMiddleware())
	adminOnly := protected.Group("/")
	adminOnly.Use(middleware.RequireAdmin())
	adminOnly.GET("/audit-logs/", ListAuditLogs)

	token, err := middleware.GenerateToken(operator)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/audit-logs/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("operator status = %d, body=%s", rec.Code, rec.Body.String())
	}

	adminToken, err := middleware.GenerateToken(admin)
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/audit-logs/", nil)
	req.Header.Set("Authorization", "Bearer "+adminToken)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin status = %d, body=%s", rec.Code, rec.Body.String())
	}
}

func TestCannotResetAdminPassword(t *testing.T) {
	db := setupUsersTestDB(t)
	admin := createTestUser(t, db, "admin", "password123", models.UserRoleAdmin)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api")
	protected.Use(middleware.AuthMiddleware(), middleware.RequireAdmin())
	protected.POST("/users/:id/reset-password/", ResetUserPassword)

	token, err := middleware.GenerateToken(admin)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/users/"+itoa(admin.ID)+"/reset-password/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestResetOperatorPasswordGeneratesSecret(t *testing.T) {
	db := setupUsersTestDB(t)
	admin := createTestUser(t, db, "admin", "password123", models.UserRoleAdmin)
	ops := createTestUser(t, db, "ops", "password123", models.UserRoleOperator)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api")
	protected.Use(middleware.AuthMiddleware(), middleware.RequireAdmin())
	protected.POST("/users/:id/reset-password/", ResetUserPassword)

	token, err := middleware.GenerateToken(admin)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/users/"+itoa(ops.ID)+"/reset-password/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Password string `json:"password"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Password) < 12 {
		t.Fatalf("generated password too short: %q", payload.Password)
	}
}

func TestCreateOperatorUser(t *testing.T) {
	db := setupUsersTestDB(t)
	admin := createTestUser(t, db, "admin", "password123", models.UserRoleAdmin)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api")
	protected.Use(middleware.AuthMiddleware(), middleware.RequireAdmin())
	protected.POST("/users/", CreateUser)

	token, err := middleware.GenerateToken(admin)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"username":"alice","password":"secret12"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/users/", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var created userPublic
	if err := json.Unmarshal(rec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Role != models.UserRoleOperator {
		t.Fatalf("role = %s", created.Role)
	}
}

func TestOperatorCannotDeleteProject(t *testing.T) {
	db := setupUsersTestDB(t)
	operator := createTestUser(t, db, "ops", "password123", models.UserRoleOperator)
	project := models.Project{Name: "p1"}
	if err := db.Create(&project).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	protected := router.Group("/api")
	protected.Use(middleware.AuthMiddleware())
	adminOnly := protected.Group("/")
	adminOnly.Use(middleware.RequireAdmin())
	adminOnly.DELETE("/projects/:id/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	token, err := middleware.GenerateToken(operator)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/projects/"+itoa(project.ID)+"/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func itoa(v uint) string {
	return strconv.FormatUint(uint64(v), 10)
}
