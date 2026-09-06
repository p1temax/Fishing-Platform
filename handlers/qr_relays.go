package handlers

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const qrRelayUploadDir = "./data/qr_relays"

type createQrRelayRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type qrRelayDTO struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	Slug         string     `json:"slug"`
	Enabled      bool       `json:"enabled"`
	ContentType  string     `json:"content_type"`
	ImageBytes   int64      `json:"image_bytes"`
	Payload      string     `json:"payload"`
	HasImage     bool       `json:"has_image"`
	PublicURL    string     `json:"public_url"`
	PublicPath   string     `json:"public_path"`
	PlaceholderURL string   `json:"placeholder_url"` // {{QR_RELAY_URL}} value
	PlaceholderImg string   `json:"placeholder_img"` // {{QR_RELAY_IMG}} value
	LastUploadAt *time.Time `json:"last_upload_at"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	UploadCount  int64      `json:"upload_count"`
	Health       string     `json:"health"` // online|stale|paused|empty
	CreatedBy    uint       `json:"created_by"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	// UploadToken is only set on create / rotate responses.
	UploadToken string `json:"upload_token,omitempty"`
}

func qrRelayHealth(row models.QrRelay) string {
	if !row.Enabled {
		return "paused"
	}
	if strings.TrimSpace(row.ImagePath) == "" {
		return "empty"
	}
	seen := row.LastSeenAt
	if seen == nil {
		seen = row.LastUploadAt
	}
	if seen == nil {
		return "empty"
	}
	if time.Since(*seen) <= 10*time.Second {
		return "online"
	}
	if time.Since(*seen) > 60*time.Second {
		return "stale"
	}
	return "online"
}

func toQrRelayDTO(row models.QrRelay, uploadToken string) qrRelayDTO {
	path := utils.QRRelayPublicPath(row.Slug)
	base := EffectivePublicBaseURL()
	publicURL := path
	if base != "" {
		publicURL = strings.TrimRight(base, "/") + path
	}
	img := ""
	if publicURL != "" {
		img = `<img src="` + publicURL + `" alt="qr" width="240" height="240" />`
	}
	return qrRelayDTO{
		ID:             row.ID,
		Name:           row.Name,
		Slug:           row.Slug,
		Enabled:        row.Enabled,
		ContentType:    row.ContentType,
		ImageBytes:     row.ImageBytes,
		Payload:        row.Payload,
		HasImage:       strings.TrimSpace(row.ImagePath) != "",
		PublicURL:      publicURL,
		PublicPath:     path,
		PlaceholderURL: publicURL,
		PlaceholderImg: img,
		LastUploadAt:   row.LastUploadAt,
		LastSeenAt:     row.LastSeenAt,
		UploadCount:    row.UploadCount,
		Health:         qrRelayHealth(row),
		CreatedBy:      row.CreatedBy,
		CreatedAt:      row.CreatedAt,
		UpdatedAt:      row.UpdatedAt,
		UploadToken:    uploadToken,
	}
}

// GetQrRelays lists relay channels for the current user.
func GetQrRelays(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var rows []models.QrRelay
	if err := scopeByOwner(db.Model(&models.QrRelay{}), user).Order("id DESC").Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list QR relays"})
		return
	}
	out := make([]qrRelayDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, toQrRelayDTO(row, ""))
	}
	c.JSON(http.StatusOK, out)
}

// GetQrRelay returns one relay.
func GetQrRelay(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	c.JSON(http.StatusOK, toQrRelayDTO(row, ""))
}

// CreateQrRelay creates a relay and returns the upload token once.
func CreateQrRelay(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	var req createQrRelayRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	slug, err := utils.NormalizeQRRelaySlug(req.Slug)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	plain, hash, err := utils.NewQRRelayToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate upload token"})
		return
	}
	row := models.QrRelay{
		Name:      name,
		Slug:      slug,
		TokenHash: hash,
		Enabled:   true,
		CreatedBy: user.ID,
	}
	db := config.GetDB()
	if err := db.Create(&row).Error; err != nil {
		if isUniqueViolation(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "slug already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create QR relay"})
		return
	}
	c.JSON(http.StatusCreated, toQrRelayDTO(row, plain))
}

// RotateQrRelayToken issues a new upload token (previous becomes invalid).
func RotateQrRelayToken(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	plain, hash, err := utils.NewQRRelayToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate upload token"})
		return
	}
	db := config.GetDB()
	if err := db.Model(&row).Update("token_hash", hash).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to rotate token"})
		return
	}
	row.TokenHash = hash
	c.JSON(http.StatusOK, toQrRelayDTO(row, plain))
}

// SetQrRelayEnabled pauses or resumes public hosting / uploads.
func SetQrRelayEnabled(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	var payload struct {
		Enabled *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&payload); err != nil || payload.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled boolean is required"})
		return
	}
	db := config.GetDB()
	if err := db.Model(&row).Update("enabled", *payload.Enabled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update relay"})
		return
	}
	row.Enabled = *payload.Enabled
	c.JSON(http.StatusOK, toQrRelayDTO(row, ""))
}

// DeleteQrRelay removes a relay and its stored image.
func DeleteQrRelay(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	db := config.GetDB()
	if err := db.Delete(&row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete QR relay"})
		return
	}
	_ = os.RemoveAll(filepath.Join(qrRelayUploadDir, strconv.FormatUint(uint64(row.ID), 10)))
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// UploadQrRelayFrame accepts ONLY a multipart image upload authenticated by relay token.
// Security rules:
//   - Content-Type must be multipart/form-data
//   - Only file field "image" is accepted (extra file parts are rejected)
//   - Magic-byte sniffing must resolve to png/jpeg/gif/webp
//   - Size capped at utils.MaxQRRelayImageBytes
//   - Optional text field "payload" (decoded QR text) is allowed; no other files
func UploadQrRelayFrame(c *gin.Context) {
	token := extractQrRelayToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing relay upload token"})
		return
	}
	if !utils.IsImageOnlyMultipart(c.GetHeader("Content-Type")) {
		c.JSON(http.StatusUnsupportedMediaType, gin.H{"error": "only multipart/form-data image uploads are allowed"})
		return
	}

	// Cap form memory; reject oversized bodies early.
	if err := c.Request.ParseMultipartForm(utils.MaxQRRelayImageBytes + (1 << 20)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid multipart form or image too large"})
		return
	}
	form := c.Request.MultipartForm
	if form == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing multipart form"})
		return
	}
	// Reject any unexpected file fields — image uploads only.
	for field := range form.File {
		if field != "image" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "only the \"image\" file field is allowed"})
			return
		}
	}
	files := form.File["image"]
	if len(files) != 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "exactly one \"image\" file is required"})
		return
	}
	header := files[0]
	if header.Size > utils.MaxQRRelayImageBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("image exceeds %d bytes", utils.MaxQRRelayImageBytes)})
		return
	}
	src, err := header.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read image"})
		return
	}
	defer src.Close()

	data, contentType, ext, err := utils.ReadImageUpload(src)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image: " + err.Error()})
		return
	}

	db := config.GetDB()
	var row models.QrRelay
	hash := utils.HashQRRelayToken(token)
	if err := db.Where("token_hash = ?", hash).First(&row).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid relay upload token"})
		return
	}
	if !row.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "relay is paused"})
		return
	}
	if !allowQrUpload(row.ID) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": "upload rate limit exceeded"})
		return
	}

	dir := filepath.Join(qrRelayUploadDir, strconv.FormatUint(uint64(row.ID), 10))
	histDir := filepath.Join(dir, "history")
	if err := os.MkdirAll(histDir, 0o750); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare storage"})
		return
	}
	filename := "current" + ext
	fullPath := filepath.Join(dir, filename)
	tmpPath := fullPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0o640); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store image"})
		return
	}
	if err := os.Rename(tmpPath, fullPath); err != nil {
		_ = os.Remove(tmpPath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to finalize image"})
		return
	}
	// Remove previous current.* with other extensions.
	entries, _ := os.ReadDir(dir)
	for _, ent := range entries {
		if ent.IsDir() {
			continue
		}
		name := ent.Name()
		if name != filename && strings.HasPrefix(name, "current.") {
			_ = os.Remove(filepath.Join(dir, name))
		}
	}

	now := time.Now()
	payload := strings.TrimSpace(c.PostForm("payload"))
	payloadHash := ""
	if payload != "" {
		payloadHash = utils.HashQRRelayToken(payload)
	}
	payloadChanged := payloadHash != "" && payloadHash != strings.TrimSpace(row.PayloadHash)

	histName := fmt.Sprintf("%d%s", now.UnixNano(), ext)
	histPath := filepath.Join(histDir, histName)
	_ = os.WriteFile(histPath, data, 0o640)
	frame := models.QrRelayFrame{
		RelayID:     row.ID,
		ImagePath:   histPath,
		ContentType: contentType,
		ImageBytes:  int64(len(data)),
		Payload:     payload,
		PayloadHash: payloadHash,
		CreatedAt:   now,
	}
	_ = db.Create(&frame).Error
	pruneQrRelayFrames(db, row.ID, 20)

	updates := map[string]any{
		"image_path":     fullPath,
		"content_type":   contentType,
		"image_bytes":    int64(len(data)),
		"payload":        payload,
		"payload_hash":   payloadHash,
		"last_upload_at": now,
		"last_seen_at":   now,
		"upload_count":   gorm.Expr("upload_count + 1"),
		"updated_at":     now,
	}
	if err := db.Model(&row).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update relay metadata"})
		return
	}

	var uploadCount int64
	_ = db.Model(&models.QrRelay{}).Where("id = ?", row.ID).Select("upload_count").Scan(&uploadCount).Error
	qrEvents.publish(row.ID, qrRelayEvent{
		Type:           "frame",
		UploadCount:    uploadCount,
		At:             now.UTC().Format(time.RFC3339),
		PayloadHash:    payloadHash,
		PayloadChanged: payloadChanged,
		FrameID:        frame.ID,
	})

	c.JSON(http.StatusOK, gin.H{
		"ok":           true,
		"bytes":        len(data),
		"content_type": contentType,
		"public_path":  utils.QRRelayPublicPath(row.Slug),
		"frame_id":     frame.ID,
	})
}

// ServeQrRelayImage publicly hosts the latest QR image at /q/:slug.png
func ServeQrRelayImage(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	slug = strings.TrimSuffix(slug, ".png")
	slug = strings.ToLower(slug)
	if slug == "" {
		c.Status(http.StatusNotFound)
		return
	}
	db := config.GetDB()
	var row models.QrRelay
	if err := db.Where("slug = ? AND enabled = ?", slug, true).First(&row).Error; err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	if strings.TrimSpace(row.ImagePath) == "" {
		c.Status(http.StatusNotFound)
		return
	}
	data, err := os.ReadFile(row.ImagePath)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	ct := row.ContentType
	if ct == "" {
		ct = "image/png"
	}
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	c.Header("Pragma", "no-cache")
	c.Data(http.StatusOK, ct, data)
}

func loadQrRelayOwned(c *gin.Context) (models.QrRelay, bool) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return models.QrRelay{}, false
	}
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid relay id"})
		return models.QrRelay{}, false
	}
	db := config.GetDB()
	var row models.QrRelay
	if err := db.First(&row, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "QR relay not found"})
		return models.QrRelay{}, false
	}
	if !canAccessOwned(user, row.CreatedBy) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return models.QrRelay{}, false
	}
	return row, true
}

func extractQrRelayToken(c *gin.Context) string {
	if v := strings.TrimSpace(c.GetHeader("X-QR-Relay-Token")); v != "" {
		return v
	}
	auth := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(auth) >= 7 && strings.EqualFold(auth[:7], "Bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return strings.TrimSpace(c.Query("token"))
}

func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "unique") || strings.Contains(msg, "duplicate")
}

// ConstantTimeTokenCompare exported for tests if needed.
func constantTimeTokenMatch(a, b string) bool {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

const qrRelayHistoryLimit = 20

func pruneQrRelayFrames(db *gorm.DB, relayID uint, keep int) {
	if keep < 1 {
		keep = qrRelayHistoryLimit
	}
	var frames []models.QrRelayFrame
	if err := db.Where("relay_id = ?", relayID).Order("id DESC").Offset(keep).Find(&frames).Error; err != nil {
		return
	}
	for _, f := range frames {
		_ = os.Remove(f.ImagePath)
		_ = db.Delete(&f).Error
	}
}

// HeartbeatQrRelay updates last_seen without uploading an image.
func HeartbeatQrRelay(c *gin.Context) {
	token := extractQrRelayToken(c)
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing relay upload token"})
		return
	}
	db := config.GetDB()
	var row models.QrRelay
	if err := db.Where("token_hash = ?", utils.HashQRRelayToken(token)).First(&row).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid relay upload token"})
		return
	}
	if !row.Enabled {
		c.JSON(http.StatusForbidden, gin.H{"error": "relay is paused"})
		return
	}
	now := time.Now()
	_ = db.Model(&row).Updates(map[string]any{
		"last_seen_at": now,
		"updated_at":   now,
	}).Error
	c.JSON(http.StatusOK, gin.H{"ok": true, "at": now.UTC().Format(time.RFC3339)})
}

type qrRelayFrameDTO struct {
	ID          uint      `json:"id"`
	RelayID     uint      `json:"relay_id"`
	ContentType string    `json:"content_type"`
	ImageBytes  int64     `json:"image_bytes"`
	Payload     string    `json:"payload"`
	PayloadHash string    `json:"payload_hash"`
	CreatedAt   time.Time `json:"created_at"`
	ImageURL    string    `json:"image_url"`
}

// ListQrRelayFrames returns recent historical frames (JWT).
func ListQrRelayFrames(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var frames []models.QrRelayFrame
	if err := db.Where("relay_id = ?", row.ID).Order("id DESC").Limit(qrRelayHistoryLimit).Find(&frames).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list frames"})
		return
	}
	out := make([]qrRelayFrameDTO, 0, len(frames))
	for _, f := range frames {
		out = append(out, qrRelayFrameDTO{
			ID:          f.ID,
			RelayID:     f.RelayID,
			ContentType: f.ContentType,
			ImageBytes:  f.ImageBytes,
			Payload:     f.Payload,
			PayloadHash: f.PayloadHash,
			CreatedAt:   f.CreatedAt,
			ImageURL:    fmt.Sprintf("/api/qr-relays/%d/frames/%d/", row.ID, f.ID),
		})
	}
	c.JSON(http.StatusOK, out)
}

// ServeQrRelayFrameImage serves a historical frame to the owner (JWT).
func ServeQrRelayFrameImage(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	fid, err := strconv.ParseUint(c.Param("fid"), 10, 64)
	if err != nil || fid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid frame id"})
		return
	}
	db := config.GetDB()
	var frame models.QrRelayFrame
	if err := db.Where("id = ? AND relay_id = ?", fid, row.ID).First(&frame).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Frame not found"})
		return
	}
	data, err := os.ReadFile(frame.ImagePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Frame file missing"})
		return
	}
	ct := frame.ContentType
	if ct == "" {
		ct = "image/png"
	}
	c.Header("Cache-Control", "private, max-age=60")
	c.Data(http.StatusOK, ct, data)
}

// PromoteQrRelayFrame copies a history frame back to current.
func PromoteQrRelayFrame(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	fid, err := strconv.ParseUint(c.Param("fid"), 10, 64)
	if err != nil || fid == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid frame id"})
		return
	}
	db := config.GetDB()
	var frame models.QrRelayFrame
	if err := db.Where("id = ? AND relay_id = ?", fid, row.ID).First(&frame).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Frame not found"})
		return
	}
	data, err := os.ReadFile(frame.ImagePath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Frame file missing"})
		return
	}
	ext := utils.ExtForContentType(frame.ContentType)
	dir := filepath.Join(qrRelayUploadDir, strconv.FormatUint(uint64(row.ID), 10))
	_ = os.MkdirAll(dir, 0o750)
	fullPath := filepath.Join(dir, "current"+ext)
	if err := os.WriteFile(fullPath, data, 0o640); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to restore frame"})
		return
	}
	now := time.Now()
	_ = db.Model(&row).Updates(map[string]any{
		"image_path":     fullPath,
		"content_type":   frame.ContentType,
		"image_bytes":    int64(len(data)),
		"payload":        frame.Payload,
		"payload_hash":   frame.PayloadHash,
		"last_upload_at": now,
		"last_seen_at":   now,
		"updated_at":     now,
	}).Error
	_ = db.First(&row, row.ID)
	qrEvents.publish(row.ID, qrRelayEvent{
		Type:        "frame",
		UploadCount: row.UploadCount,
		At:          now.UTC().Format(time.RFC3339),
		PayloadHash: frame.PayloadHash,
		FrameID:     frame.ID,
	})
	c.JSON(http.StatusOK, toQrRelayDTO(row, ""))
}

// RotateQrRelaySlug issues a new public slug; old /q/{slug}.png becomes 404.
func RotateQrRelaySlug(c *gin.Context) {
	row, ok := loadQrRelayOwned(c)
	if !ok {
		return
	}
	slug, err := utils.NewQRRelaySlug()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate slug"})
		return
	}
	db := config.GetDB()
	if err := db.Model(&row).Update("slug", slug).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to rotate slug"})
		return
	}
	row.Slug = slug
	c.JSON(http.StatusOK, toQrRelayDTO(row, ""))
}

func ensureOwnedQrRelay(c *gin.Context, db *gorm.DB, user models.User, relayID *uint) bool {
	if relayID == nil || *relayID == 0 {
		return true
	}
	var relay models.QrRelay
	if err := db.First(&relay, *relayID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "QR relay not found"})
		return false
	}
	if !canAccessOwned(user, relay.CreatedBy) {
		c.JSON(http.StatusForbidden, gin.H{"error": "QR relay is not accessible"})
		return false
	}
	return true
}
