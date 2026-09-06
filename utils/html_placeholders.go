package utils

import (
	"fmt"
	"os"
	"strings"
)

// Landing-page placeholders replaced when HTML is prepared for a project runtime.
const (
	PlaceholderSubmitURL   = "{{SUBMIT_URL}}"
	PlaceholderRedirectURL = "{{REDIRECT_URL}}"
	PlaceholderQRRelayURL  = "{{QR_RELAY_URL}}"
	PlaceholderQRRelayImg  = "{{QR_RELAY_IMG}}"
)

// ProjectHTMLVars are values substituted into uploaded/mirrored landing HTML.
type ProjectHTMLVars struct {
	SubmitURL   string
	RedirectURL string
	QRRelayURL  string // absolute or path URL to live QR image
}

// ApplyProjectHTMLPlaceholders replaces supported tokens in landing HTML.
func ApplyProjectHTMLPlaceholders(html string, vars ProjectHTMLVars) string {
	submit := strings.TrimSpace(vars.SubmitURL)
	if submit == "" {
		submit = "/api/submit"
	}
	redirect := strings.TrimSpace(vars.RedirectURL)
	qrURL := strings.TrimSpace(vars.QRRelayURL)
	qrImg := ""
	if qrURL != "" {
		qrImg = fmt.Sprintf(`<img src="%s" alt="qr" width="240" height="240" />`, qrURL)
	}

	out := html
	out = strings.ReplaceAll(out, PlaceholderSubmitURL, submit)
	out = strings.ReplaceAll(out, PlaceholderRedirectURL, redirect)
	out = strings.ReplaceAll(out, PlaceholderQRRelayURL, qrURL)
	out = strings.ReplaceAll(out, PlaceholderQRRelayImg, qrImg)
	return out
}

// RenderProjectHTMLFile reads a source HTML file and applies project placeholders.
func RenderProjectHTMLFile(srcPath string, vars ProjectHTMLVars) ([]byte, error) {
	raw, err := os.ReadFile(srcPath)
	if err != nil {
		return nil, err
	}
	return []byte(ApplyProjectHTMLPlaceholders(string(raw), vars)), nil
}
