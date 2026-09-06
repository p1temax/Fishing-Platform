package utils

import (
	"bytes"
	"strings"
	"testing"
)

func TestDetectAllowedImageTypePNG(t *testing.T) {
	ct, ext, err := DetectAllowedImageType(MinimalPNG())
	if err != nil {
		t.Fatal(err)
	}
	if ct != "image/png" || ext != ".png" {
		t.Fatalf("ct=%s ext=%s", ct, ext)
	}
}

func TestDetectAllowedImageTypeRejectsText(t *testing.T) {
	_, _, err := DetectAllowedImageType([]byte("not an image"))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestReadImageUploadEnforcesSize(t *testing.T) {
	huge := append(MinimalPNG(), bytes.Repeat([]byte{0}, MaxQRRelayImageBytes)...)
	_, _, _, err := ReadImageUpload(bytes.NewReader(huge))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err=%v", err)
	}
}

func TestNormalizeQRRelaySlug(t *testing.T) {
	s, err := NormalizeQRRelaySlug("AbC-12345678")
	if err != nil || s != "abc-12345678" {
		t.Fatalf("got %q err=%v", s, err)
	}
	if _, err := NormalizeQRRelaySlug("bad slug"); err == nil {
		t.Fatal("expected error for space")
	}
}

func TestIsImageOnlyMultipart(t *testing.T) {
	if !IsImageOnlyMultipart("multipart/form-data; boundary=x") {
		t.Fatal("expected true")
	}
	if IsImageOnlyMultipart("application/json") {
		t.Fatal("expected false")
	}
}
