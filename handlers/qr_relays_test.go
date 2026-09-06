package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestQrRelayUploadAcceptsImageOnlyAndServesPublicURL(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "qr.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir()) // isolate data/qr_relays writes
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	config.DB = db

	admin := models.User{Username: "admin", Password: "x", Role: models.UserRoleAdmin}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/qr-relays", func(c *gin.Context) {
		c.Set("user", admin)
		CreateQrRelay(c)
	})
	router.POST("/api/qr-relay/frames/", UploadQrRelayFrame)
	router.GET("/q/:slug", ServeQrRelayImage)

	// Create relay
	createBody := bytes.NewBufferString(`{"name":"login-relay"}`)
	createRes := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/qr-relays", createBody)
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(createRes, req)
	if createRes.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createRes.Code, createRes.Body.String())
	}
	var created struct {
		ID          uint   `json:"id"`
		Slug        string `json:"slug"`
		UploadToken string `json:"upload_token"`
		PublicPath  string `json:"public_path"`
	}
	if err := json.Unmarshal(createRes.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.UploadToken == "" || created.Slug == "" {
		t.Fatalf("missing token/slug: %+v", created)
	}

	// Reject non-multipart
	bad := httptest.NewRecorder()
	badReq := httptest.NewRequest(http.MethodPost, "/api/qr-relay/frames/", bytes.NewBufferString(`{"x":1}`))
	badReq.Header.Set("Content-Type", "application/json")
	badReq.Header.Set("X-QR-Relay-Token", created.UploadToken)
	router.ServeHTTP(bad, badReq)
	if bad.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("json upload status=%d want 415", bad.Code)
	}

	// Reject non-image multipart
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	part, _ := w.CreateFormFile("image", "x.txt")
	_, _ = part.Write([]byte("hello world not image"))
	_ = w.Close()
	rej := httptest.NewRecorder()
	rejReq := httptest.NewRequest(http.MethodPost, "/api/qr-relay/frames/", &buf)
	rejReq.Header.Set("Content-Type", w.FormDataContentType())
	rejReq.Header.Set("X-QR-Relay-Token", created.UploadToken)
	router.ServeHTTP(rej, rejReq)
	if rej.Code != http.StatusBadRequest {
		t.Fatalf("text upload status=%d body=%s", rej.Code, rej.Body.String())
	}

	// Accept PNG
	var okBuf bytes.Buffer
	okW := multipart.NewWriter(&okBuf)
	imgPart, _ := okW.CreateFormFile("image", "qr.png")
	_, _ = imgPart.Write(utils.MinimalPNG())
	_ = okW.WriteField("payload", "https://example.com/login")
	_ = okW.Close()
	okRes := httptest.NewRecorder()
	okReq := httptest.NewRequest(http.MethodPost, "/api/qr-relay/frames/", &okBuf)
	okReq.Header.Set("Content-Type", okW.FormDataContentType())
	okReq.Header.Set("Authorization", "Bearer "+created.UploadToken)
	router.ServeHTTP(okRes, okReq)
	if okRes.Code != http.StatusOK {
		t.Fatalf("png upload status=%d body=%s", okRes.Code, okRes.Body.String())
	}

	// Public host
	pub := httptest.NewRecorder()
	pubReq := httptest.NewRequest(http.MethodGet, created.PublicPath, nil)
	router.ServeHTTP(pub, pubReq)
	if pub.Code != http.StatusOK {
		t.Fatalf("public status=%d", pub.Code)
	}
	if ct := pub.Header().Get("Content-Type"); ct != "image/png" {
		t.Fatalf("content-type=%s", ct)
	}
	if pub.Header().Get("Cache-Control") == "" {
		t.Fatal("expected no-store cache header")
	}

	// Heartbeat updates last_seen
	hb := httptest.NewRecorder()
	hbReq := httptest.NewRequest(http.MethodPost, "/api/qr-relay/heartbeat/", nil)
	hbReq.Header.Set("X-QR-Relay-Token", created.UploadToken)
	router.POST("/api/qr-relay/heartbeat/", HeartbeatQrRelay)
	router.ServeHTTP(hb, hbReq)
	if hb.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", hb.Code, hb.Body.String())
	}
	var row models.QrRelay
	if err := db.First(&row, created.ID).Error; err != nil {
		t.Fatal(err)
	}
	if row.LastSeenAt == nil {
		t.Fatal("expected last_seen_at after heartbeat")
	}
	if qrRelayHealth(row) != "online" {
		t.Fatalf("health=%s want online", qrRelayHealth(row))
	}

	// History frame was recorded
	var frames []models.QrRelayFrame
	if err := db.Where("relay_id = ?", created.ID).Find(&frames).Error; err != nil {
		t.Fatal(err)
	}
	if len(frames) != 1 {
		t.Fatalf("frames=%d want 1", len(frames))
	}
}
