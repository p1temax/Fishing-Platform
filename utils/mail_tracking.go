package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"strings"
)

// Mail tracking "v" param shape (similar to common phishing platforms):
//
//	v={token}_{version}{rawStdBase64(email)}
//
// where token is the first 24 hex chars of MailTrackingSign(secret, cid, email)
// and version is currently "1".
const (
	mailTrackingVVersion   = "1"
	mailTrackingTokenChars = 24
)

// MailTrackingSign returns a hex HMAC for campaign recipient identity.
func MailTrackingSign(secret string, campaignID uint, email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(fmt.Sprintf("%d|%s", campaignID, email)))
	sum := hex.EncodeToString(mac.Sum(nil))
	if len(sum) > 32 {
		return sum[:32]
	}
	return sum
}

// MailTrackingSignValid checks a recipient signature.
func MailTrackingSignValid(secret string, campaignID uint, email, sig string) bool {
	expected := MailTrackingSign(secret, campaignID, email)
	return hmac.Equal([]byte(strings.ToLower(strings.TrimSpace(sig))), []byte(expected))
}

// BuildMailOpenPixelURL builds the open-tracking pixel URL (sample-style: action + v=).
func BuildMailOpenPixelURL(publicBase, openPath, secret string, campaignID uint, email string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	path := strings.TrimSpace(openPath)
	if path == "" {
		path = "/api/mail-open"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	v := BuildMailTrackingV(secret, campaignID, email)
	q := url.Values{}
	q.Set("action", "xtrackd")
	q.Set("v", v)
	return base + path + "?" + q.Encode()
}

// BuildMailClickURL builds the click-tracking redirect URL (sample-style opaque /api path + v=).
func BuildMailClickURL(publicBase, clickPath, secret string, campaignID uint, email, landingURL string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	path := strings.TrimSpace(clickPath)
	if path == "" {
		path = "/api/mail-click"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	email = strings.ToLower(strings.TrimSpace(email))
	v := BuildMailTrackingV(secret, campaignID, email)
	q := url.Values{}
	q.Set("v", v)
	q.Set("u", strings.TrimSpace(landingURL))
	return base + path + "?" + q.Encode()
}

// InjectMailHTMLNoise inserts invisible marker tags (sample-style fingerprint noise).
func InjectMailHTMLNoise(html string) string {
	html = strings.TrimSpace(html)
	if html == "" {
		return html
	}
	noise := func() string {
		name := randomMailNoiseToken(8)
		comment := randomMailNoiseToken(36)
		family := randomMailNoiseToken(6)
		return fmt.Sprintf(
			`<i name="%s" style="font-size:0px;white-space:nowrap;font-family:%s;"><!-- %s --></i>`,
			name, family, comment,
		)
	}
	// Sprinkle a few markers near the start and before </body>.
	out := html
	if strings.Contains(strings.ToLower(out), "<body") {
		lower := strings.ToLower(out)
		idx := strings.Index(lower, "<body")
		if idx >= 0 {
			end := strings.Index(out[idx:], ">")
			if end >= 0 {
				at := idx + end + 1
				out = out[:at] + noise() + out[at:]
			}
		}
	} else {
		out = noise() + out
	}
	if i := strings.LastIndex(strings.ToLower(out), "</body>"); i >= 0 {
		out = out[:i] + noise() + noise() + out[i:]
	} else {
		out += noise()
	}
	return out
}

// BuildMailTrackingV builds the compact landing attribution value.
func BuildMailTrackingV(secret string, campaignID uint, email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	token := MailTrackingSign(secret, campaignID, email)
	if len(token) > mailTrackingTokenChars {
		token = token[:mailTrackingTokenChars]
	}
	encoded := base64.RawStdEncoding.EncodeToString([]byte(email))
	return token + "_" + mailTrackingVVersion + encoded
}

// ParseMailTrackingV extracts token version and email from a v= value.
// Campaign ID must be resolved by verifying the token against candidate campaigns.
func ParseMailTrackingV(raw string) (token, email string, ok bool) {
	raw = strings.TrimSpace(raw)
	idx := strings.IndexByte(raw, '_')
	if idx <= 0 || idx+2 > len(raw) {
		return "", "", false
	}
	token = strings.ToLower(raw[:idx])
	rest := raw[idx+1:]
	if !strings.HasPrefix(rest, mailTrackingVVersion) {
		return "", "", false
	}
	payload := rest[len(mailTrackingVVersion):]
	decoded, err := base64.RawStdEncoding.DecodeString(payload)
	if err != nil {
		// Accept padded standard base64 too.
		decoded, err = base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return "", "", false
		}
	}
	email = strings.ToLower(strings.TrimSpace(string(decoded)))
	if token == "" || email == "" || !strings.Contains(email, "@") {
		return "", "", false
	}
	return token, email, true
}

// MailTrackingTokenMatches reports whether token is the truncated sign for cid+email.
func MailTrackingTokenMatches(secret string, campaignID uint, email, token string) bool {
	expected := MailTrackingSign(secret, campaignID, email)
	token = strings.ToLower(strings.TrimSpace(token))
	if len(expected) > mailTrackingTokenChars {
		expected = expected[:mailTrackingTokenChars]
	}
	return hmac.Equal([]byte(expected), []byte(token))
}

// AppendMailTrackingV merges v into a landing URL query string.
func AppendMailTrackingV(landingURL, v string) string {
	landingURL = strings.TrimSpace(landingURL)
	v = strings.TrimSpace(v)
	if landingURL == "" || v == "" {
		return landingURL
	}
	parsed, err := url.Parse(landingURL)
	if err != nil {
		return landingURL
	}
	q := parsed.Query()
	q.Set("v", v)
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

func randomMailNoiseToken(n int) string {
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	if n <= 0 {
		n = 8
	}
	buf := make([]byte, n)
	_, _ = rand.Read(buf)
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}

// RenderMailTemplate replaces tracking placeholders for one recipient.
func RenderMailTemplate(body string, vars map[string]string, isHTML, trackOpens bool) string {
	out := body
	replacements := []struct {
		key string
		val string
	}{
		{"{{email}}", vars["email"]},
		{"{{sig}}", vars["sig"]},
		{"{{v}}", vars["v"]},
		{"{{click_url}}", vars["click_url"]},
		{"{{open_pixel}}", vars["open_pixel"]},
		{"{{landing_url}}", vars["landing_url"]},
	}
	for _, item := range replacements {
		out = strings.ReplaceAll(out, item.key, item.val)
	}

	if isHTML {
		out = InjectMailHTMLNoise(out)
	}

	if isHTML && trackOpens {
		pixel := strings.TrimSpace(vars["open_pixel"])
		if pixel != "" && !strings.Contains(out, pixel) {
			out += fmt.Sprintf(
				`<img src="%s" alt="" style="display:none;width:0;height:0" />`,
				pixel,
			)
		}
	}

	if !isHTML {
		clickURL := strings.TrimSpace(vars["click_url"])
		if clickURL != "" && !strings.Contains(out, clickURL) && strings.TrimSpace(vars["landing_url"]) != "" {
			if strings.TrimSpace(out) != "" && !strings.HasSuffix(out, "\n") {
				out += "\n"
			}
			out += clickURL
		}
	}

	return out
}

// HostAllowedForMailRedirect checks whether targetURL host is permitted.
func HostAllowedForMailRedirect(targetURL string, allowedHosts []string) bool {
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return false
	}
	for _, allowed := range allowedHosts {
		allowed = strings.ToLower(strings.TrimSpace(allowed))
		if allowed == "" {
			continue
		}
		if host == allowed || strings.HasSuffix(host, "."+allowed) {
			return true
		}
	}
	return false
}

// ExtractURLHost returns the lowercase hostname of a URL, or empty.
func ExtractURLHost(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}
