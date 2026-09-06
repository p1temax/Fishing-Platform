package handlers

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type mailTrackingSettingsDTO struct {
	PublicBaseURL      string   `json:"public_base_url"`
	Enabled            bool     `json:"enabled"`
	Secret             string   `json:"secret"`
	SecretSet          bool     `json:"secret_set"`
	AllowRedirectHosts []string `json:"allow_redirect_hosts"`
}

type mailTrackingSettingsPayload struct {
	PublicBaseURL      string   `json:"public_base_url"`
	Enabled            *bool    `json:"enabled"`
	Secret             string   `json:"secret"`
	AllowRedirectHosts []string `json:"allow_redirect_hosts"`
}

// GetMailTrackingSettings returns platform-managed mail tracking config (admin).
func GetMailTrackingSettings(c *gin.Context) {
	row, err := ensureMailTrackingSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load mail tracking settings"})
		return
	}
	c.JSON(http.StatusOK, toMailTrackingDTO(row))
}

// UpdateMailTrackingSettings updates the singleton mail tracking config (admin).
func UpdateMailTrackingSettings(c *gin.Context) {
	var payload mailTrackingSettingsPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	row, err := ensureMailTrackingSetting()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load mail tracking settings"})
		return
	}

	base := strings.TrimRight(strings.TrimSpace(payload.PublicBaseURL), "/")
	if base != "" {
		u, err := url.Parse(base)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "public_base_url must be an http(s) URL"})
			return
		}
		base = strings.TrimRight(u.String(), "/")
	}

	hosts := make([]string, 0, len(payload.AllowRedirectHosts))
	for _, h := range payload.AllowRedirectHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if h != "" {
			hosts = append(hosts, h)
		}
	}

	row.PublicBaseURL = base
	if payload.Enabled != nil {
		row.Enabled = *payload.Enabled
	}
	row.AllowRedirectHosts = strings.Join(hosts, "\n")
	// Click/open paths are generated per mail campaign, not configured globally.

	secretPlain := strings.TrimSpace(payload.Secret)
	if secretPlain != "" && !looksMaskedSecret(secretPlain) {
		enc, err := utils.Encrypt(secretPlain)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encrypt secret"})
			return
		}
		row.Secret = enc
	}

	db := config.GetDB()
	if err := db.Save(row).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save mail tracking settings"})
		return
	}
	c.JSON(http.StatusOK, toMailTrackingDTO(row))
}

// EnsureMailTrackingSettingForBoot seeds DB settings from YAML when missing.
func EnsureMailTrackingSettingForBoot() (*models.MailTrackingSetting, error) {
	return ensureMailTrackingSetting()
}

func ensureMailTrackingSetting() (*models.MailTrackingSetting, error) {
	db := config.GetDB()
	var row models.MailTrackingSetting
	err := db.First(&row, 1).Error
	if err == nil {
		return &row, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	enabled := config.MailTrackingEnabled()
	secretPlain := strings.TrimSpace(config.MailTrackingSecret())
	enc := ""
	if secretPlain != "" {
		if e, eerr := utils.Encrypt(secretPlain); eerr == nil {
			enc = e
		}
	}
	hosts := config.MailTrackingAllowRedirectHosts()
	row = models.MailTrackingSetting{
		ID:                 1,
		PublicBaseURL:      config.PublicBaseURL(),
		Enabled:            enabled,
		Secret:             enc,
		AllowRedirectHosts: strings.Join(hosts, "\n"),
		ClickPath:          "",
		OpenPath:           "",
	}
	if err := db.Create(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func toMailTrackingDTO(row *models.MailTrackingSetting) mailTrackingSettingsDTO {
	plain := decryptMailTrackingSecret(row.Secret)
	hosts := splitHostLines(row.AllowRedirectHosts)
	return mailTrackingSettingsDTO{
		PublicBaseURL:      row.PublicBaseURL,
		Enabled:            row.Enabled,
		Secret:             maskSecret(plain),
		SecretSet:          plain != "",
		AllowRedirectHosts: hosts,
	}
}

func decryptMailTrackingSecret(enc string) string {
	enc = strings.TrimSpace(enc)
	if enc == "" {
		return ""
	}
	if dec, err := utils.Decrypt(enc); err == nil && dec != "" {
		return dec
	}
	return enc
}

func splitHostLines(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r' || r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.ToLower(strings.TrimSpace(p))
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}

func loadMailTrackingSetting() *models.MailTrackingSetting {
	row, err := ensureMailTrackingSetting()
	if err != nil {
		return nil
	}
	return row
}

// EffectivePublicBaseURL prefers DB setting, then YAML, then empty.
func EffectivePublicBaseURL() string {
	if row := loadMailTrackingSetting(); row != nil {
		if base := strings.TrimRight(strings.TrimSpace(row.PublicBaseURL), "/"); base != "" {
			return base
		}
	}
	return config.PublicBaseURL()
}

// EffectiveMailTrackingEnabled prefers DB.
func EffectiveMailTrackingEnabled() bool {
	if row := loadMailTrackingSetting(); row != nil {
		return row.Enabled
	}
	return config.MailTrackingEnabled()
}

// EffectiveMailTrackingSecret prefers DB encrypted secret, then YAML/jwt fallback.
func EffectiveMailTrackingSecret() string {
	if row := loadMailTrackingSetting(); row != nil {
		if plain := decryptMailTrackingSecret(row.Secret); plain != "" {
			return plain
		}
	}
	return config.MailTrackingSecret()
}

// EffectiveMailTrackingAllowRedirectHosts merges DB hosts with YAML hosts.
func EffectiveMailTrackingAllowRedirectHosts() []string {
	seen := map[string]bool{}
	out := make([]string, 0)
	add := func(list []string) {
		for _, h := range list {
			h = strings.ToLower(strings.TrimSpace(h))
			if h == "" || seen[h] {
				continue
			}
			seen[h] = true
			out = append(out, h)
		}
	}
	if row := loadMailTrackingSetting(); row != nil {
		add(splitHostLines(row.AllowRedirectHosts))
	}
	add(config.MailTrackingAllowRedirectHosts())
	return out
}

// DispatchMailTracking routes GET /api/:slug to open/click handlers when the
// slug matches a per-campaign tracking path.
func isReservedAPISlug(slug string) bool {
	s := strings.ToLower(strings.Trim(strings.TrimSpace(slug), "/"))
	switch s {
	case "auth", "agents", "agent", "users", "projects", "robots", "messages",
		"credentials", "ip-blacklist", "smtp-services", "mail-campaigns",
		"phishing-pages", "dashboard", "audit-logs", "ai-settings",
		"mail-tracking-settings", "info-gather-jobs", "qr-relays", "qr-relay",
		"robot_push_logs":
		return true
	default:
		return false
	}
}

func DispatchMailTracking(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
		return
	}
	// Never let the catch-all /api/:slug shadow real platform API prefixes.
	if isReservedAPISlug(slug) {
		c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
		return
	}
	reqPath := config.NormalizeMailTrackingPath("/api/" + slug)
	db := config.GetDB()

	var openHit int64
	_ = db.Model(&models.MailCampaign{}).Where("open_path = ?", reqPath).Limit(1).Count(&openHit).Error
	if openHit > 0 {
		TrackMailOpen(c)
		return
	}
	var clickHit int64
	_ = db.Model(&models.MailCampaign{}).Where("click_path = ?", reqPath).Limit(1).Count(&clickHit).Error
	if clickHit > 0 {
		TrackMailClick(c)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "API endpoint not found"})
}
