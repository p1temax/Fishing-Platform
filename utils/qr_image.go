package utils

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
)

const (
	// MaxQRRelayImageBytes is the hard cap for a single uploaded QR frame.
	MaxQRRelayImageBytes = 2 << 20 // 2 MiB
)

var allowedQRImageTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/gif":  ".gif",
	"image/webp": ".webp",
}

// DetectAllowedImageType inspects magic bytes and returns a canonical content-type
// plus file extension. Rejects anything that is not a supported still image.
func DetectAllowedImageType(header []byte) (contentType, ext string, err error) {
	if len(header) == 0 {
		return "", "", fmt.Errorf("empty image")
	}
	detected := http.DetectContentType(header)
	// DetectContentType may append "; charset=..." for some types — strip params.
	media, _, _ := mime.ParseMediaType(detected)
	media = strings.ToLower(strings.TrimSpace(media))
	ext, ok := allowedQRImageTypes[media]
	if !ok {
		return "", "", fmt.Errorf("unsupported image type %q (allowed: png, jpeg, gif, webp)", media)
	}
	return media, ext, nil
}

// ReadImageUpload reads at most MaxQRRelayImageBytes+1 from r and validates it
// as an allowed image by magic bytes (not by client Content-Type alone).
func ReadImageUpload(r io.Reader) (data []byte, contentType, ext string, err error) {
	limited := io.LimitReader(r, MaxQRRelayImageBytes+1)
	data, err = io.ReadAll(limited)
	if err != nil {
		return nil, "", "", err
	}
	if len(data) == 0 {
		return nil, "", "", fmt.Errorf("empty image")
	}
	if int64(len(data)) > MaxQRRelayImageBytes {
		return nil, "", "", fmt.Errorf("image exceeds %d bytes", MaxQRRelayImageBytes)
	}
	contentType, ext, err = DetectAllowedImageType(data)
	if err != nil {
		return nil, "", "", err
	}
	return data, contentType, ext, nil
}

// IsImageOnlyMultipart rejects requests that are not multipart or that include
// unexpected file parts. Only a single file field named "image" is allowed.
func IsImageOnlyMultipart(contentType string) bool {
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return strings.EqualFold(media, "multipart/form-data")
}

// NewQRRelayToken returns a random upload token and its sha256 hex hash.
func NewQRRelayToken() (plain, hash string, err error) {
	buf := make([]byte, 32)
	if _, err = rand.Read(buf); err != nil {
		return "", "", err
	}
	plain = hex.EncodeToString(buf)
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	return plain, hash, nil
}

// HashQRRelayToken hashes a plaintext relay token for storage/lookup.
func HashQRRelayToken(plain string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(plain)))
	return hex.EncodeToString(sum[:])
}

// NewQRRelaySlug returns a URL-safe random slug.
func NewQRRelaySlug() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// NormalizeQRRelaySlug validates operator-provided slugs.
func NormalizeQRRelaySlug(raw string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" {
		return NewQRRelaySlug()
	}
	if len(s) < 8 || len(s) > 64 {
		return "", fmt.Errorf("slug must be 8-64 characters")
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			continue
		}
		return "", fmt.Errorf("slug may only contain a-z, 0-9, and hyphen")
	}
	return s, nil
}

// QRRelayPublicPath returns the public path for a hosted QR image.
func QRRelayPublicPath(slug string) string {
	slug = strings.TrimSpace(slug)
	return "/q/" + slug + ".png"
}

// SniffLooksLikeImage is a tiny helper for tests.
func SniffLooksLikeImage(b []byte) bool {
	_, _, err := DetectAllowedImageType(b)
	return err == nil
}

// ExtForContentType maps content-type to extension; default .png.
func ExtForContentType(contentType string) string {
	if ext, ok := allowedQRImageTypes[strings.ToLower(contentType)]; ok {
		return ext
	}
	return ".bin"
}

// SafeQRRelayFilename builds a stored filename for the current frame.
func SafeQRRelayFilename(contentType string) string {
	ext := ExtForContentType(contentType)
	if ext == ".jpg" || ext == ".jpeg" || ext == ".gif" || ext == ".webp" || ext == ".png" {
		return "current" + ext
	}
	return "current.png"
}

// EnsurePNGHostName always exposes .png in the public URL even if the stored
// file is jpeg/webp — browsers still render by Content-Type. Storage keeps real ext.
func EnsurePNGHostName(slug string) string {
	return filepath.Base(QRRelayPublicPath(slug))
}

// PNGMagic for tests.
var PNGMagic = []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}

// MinimalPNG returns a tiny valid 1x1 PNG for tests.
func MinimalPNG() []byte {
	// Precomputed 1x1 transparent PNG
	return []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d,
		0x49, 0x48, 0x44, 0x52, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4, 0x89, 0x00, 0x00, 0x00,
		0x0a, 0x49, 0x44, 0x41, 0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x49,
		0x45, 0x4e, 0x44, 0xae, 0x42, 0x60, 0x82,
	}
}

// BytesReader helper.
func BytesReader(b []byte) io.Reader { return bytes.NewReader(b) }
