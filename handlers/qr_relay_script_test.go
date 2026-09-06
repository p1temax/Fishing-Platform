package handlers

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestDownloadQrRelayScriptZip(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "qr-script.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := models.AutoMigrate(db); err != nil {
		t.Fatal(err)
	}
	config.DB = db
	admin := models.User{Username: "admin", Password: "x", Role: models.UserRoleAdmin}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatal(err)
	}

	RegisterQrRelayScriptFS(fstest.MapFS{
		"scripts/qr-relay/qr_relay.py":         &fstest.MapFile{Data: []byte("print('ok')\n")},
		"scripts/qr-relay/requirements.txt":    &fstest.MapFile{Data: []byte("requests\n")},
		"scripts/qr-relay/README.md":           &fstest.MapFile{Data: []byte("# relay\n")},
		"scripts/qr-relay/config.example.yaml": &fstest.MapFile{Data: []byte("platform_url: x\n")},
	})

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/script.zip", func(c *gin.Context) {
		c.Set("user", admin)
		DownloadQrRelayScript(c)
	})
	res := httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/script.zip", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", res.Code, res.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(res.Body.Bytes()), int64(res.Body.Len()))
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		if f.Name == "qr-relay/config.yaml" {
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			_ = rc.Close()
			if !bytes.Contains(b, []byte("platform_url:")) {
				t.Fatalf("config missing platform_url: %s", b)
			}
		}
	}
	for _, need := range []string{
		"qr-relay/qr_relay.py",
		"qr-relay/requirements.txt",
		"qr-relay/README.md",
		"qr-relay/config.example.yaml",
		"qr-relay/config.yaml",
	} {
		if !names[need] {
			t.Fatalf("zip missing %s", need)
		}
	}
}
