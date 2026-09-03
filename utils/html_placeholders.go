package utils

import (
	"os"
	"strings"
)

// Landing-page placeholders replaced when HTML is prepared for a project runtime.
const (
	PlaceholderSubmitURL   = "{{SUBMIT_URL}}"
	PlaceholderRedirectURL = "{{REDIRECT_URL}}"
)

// ProjectHTMLVars are values substituted into uploaded/mirrored landing HTML.
type ProjectHTMLVars struct {
	SubmitURL   string
	RedirectURL string
}

// ApplyProjectHTMLPlaceholders replaces supported tokens in landing HTML.
func ApplyProjectHTMLPlaceholders(html string, vars ProjectHTMLVars) string {
	submit := strings.TrimSpace(vars.SubmitURL)
	if submit == "" {
		submit = "/api/submit"
	}
	redirect := strings.TrimSpace(vars.RedirectURL)

	out := html
	out = strings.ReplaceAll(out, PlaceholderSubmitURL, submit)
	out = strings.ReplaceAll(out, PlaceholderRedirectURL, redirect)
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
