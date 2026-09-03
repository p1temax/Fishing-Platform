package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
)

// 1x1 transparent GIF
var trackingPixelGIF = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00, 0xff, 0xff, 0xff,
	0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00,
	0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
}

func parseTrackingCampaignID(raw string) (uint, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}

func loadTrackedRecipient(campaignID uint, email string) (*models.MailCampaign, *models.MailRecipient, bool) {
	db := config.GetDB()
	var campaign models.MailCampaign
	if err := db.First(&campaign, campaignID).Error; err != nil {
		return nil, nil, false
	}
	var recipient models.MailRecipient
	if err := db.Where("campaign_id = ? AND email = ?", campaignID, strings.ToLower(strings.TrimSpace(email))).First(&recipient).Error; err != nil {
		return nil, nil, false
	}
	return &campaign, &recipient, true
}

// TrackMailOpen records an open event and returns a 1x1 GIF. No robot push.
func TrackMailOpen(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Header("Pragma", "no-cache")

	writePixel := func() {
		c.Data(http.StatusOK, "image/gif", trackingPixelGIF)
	}

	if !EffectiveMailTrackingEnabled() {
		writePixel()
		return
	}

	var campaign *models.MailCampaign
	var recipient *models.MailRecipient
	ok := false
	if v := strings.TrimSpace(c.Query("v")); v != "" {
		campaign, recipient, ok = resolveMailTrackingV(v)
	}
	if !ok {
		cid, cidOK := parseTrackingCampaignID(c.Query("cid"))
		email := strings.TrimSpace(c.Query("e"))
		sig := strings.TrimSpace(c.Query("s"))
		if !cidOK || email == "" || sig == "" || !utils.MailTrackingSignValid(EffectiveMailTrackingSecret(), cid, email, sig) {
			writePixel()
			return
		}
		campaign, recipient, ok = loadTrackedRecipient(cid, email)
	}
	if !ok || campaign == nil || recipient == nil || !campaign.TrackOpens {
		writePixel()
		return
	}

	db := config.GetDB()
	now := time.Now()
	ip := c.ClientIP()
	ua := c.Request.UserAgent()
	if len(ua) > 500 {
		ua = ua[:500]
	}

	firstOpen := recipient.OpenedAt == nil
	_ = db.Exec(
		"UPDATE mail_recipients SET open_count = open_count + 1, last_open_ip = ?, opened_at = COALESCE(opened_at, ?), updated_at = ? WHERE id = ?",
		ip, now, now, recipient.ID,
	).Error
	if firstOpen {
		_ = db.Exec("UPDATE mail_campaigns SET open_count = open_count + 1, updated_at = ? WHERE id = ?", now, campaign.ID).Error
	}

	_ = db.Create(&models.MailEvent{
		CampaignID:  campaign.ID,
		RecipientID: recipient.ID,
		Kind:        models.MailEventOpen,
		IP:          ip,
		UserAgent:   ua,
	}).Error

	writePixel()
}

// TrackMailClick records a click and redirects to the landing URL. No robot push.
func TrackMailClick(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")

	target := strings.TrimSpace(c.Query("u"))
	vParam := strings.TrimSpace(c.Query("v"))
	secret := EffectiveMailTrackingSecret()

	var campaign *models.MailCampaign
	var recipient *models.MailRecipient
	ok := false

	if vParam != "" {
		campaign, recipient, ok = resolveMailTrackingV(vParam)
	}
	if !ok {
		cid, cidOK := parseTrackingCampaignID(c.Query("cid"))
		email := strings.TrimSpace(c.Query("e"))
		sig := strings.TrimSpace(c.Query("s"))
		if !cidOK || email == "" || sig == "" || target == "" {
			c.String(http.StatusBadRequest, "invalid tracking link")
			return
		}
		if !utils.MailTrackingSignValid(secret, cid, email, sig) {
			c.String(http.StatusForbidden, "invalid signature")
			return
		}
		campaign, recipient, ok = loadTrackedRecipient(cid, email)
	}
	if !ok || campaign == nil || recipient == nil {
		c.String(http.StatusNotFound, "unknown recipient")
		return
	}
	if target == "" {
		target = strings.TrimSpace(campaign.LandingURL)
	}
	if target == "" {
		c.String(http.StatusBadRequest, "missing landing url")
		return
	}

	allowed := mailAllowedHosts(campaign.LandingURL, EffectiveMailTrackingAllowRedirectHosts())
	if !utils.HostAllowedForMailRedirect(target, allowed) {
		c.String(http.StatusBadRequest, "redirect target not allowed")
		return
	}

	if campaign.TrackClicks {
		db := config.GetDB()
		now := time.Now()
		ip := c.ClientIP()
		ua := c.Request.UserAgent()
		if len(ua) > 500 {
			ua = ua[:500]
		}

		firstClick := recipient.ClickedAt == nil
		_ = db.Exec(
			"UPDATE mail_recipients SET click_count = click_count + 1, last_click_ip = ?, clicked_at = COALESCE(clicked_at, ?), updated_at = ? WHERE id = ?",
			ip, now, now, recipient.ID,
		).Error
		if firstClick {
			_ = db.Exec("UPDATE mail_campaigns SET click_count = click_count + 1, updated_at = ? WHERE id = ?", now, campaign.ID).Error
		}
		_ = db.Create(&models.MailEvent{
			CampaignID:  campaign.ID,
			RecipientID: recipient.ID,
			Kind:        models.MailEventClick,
			IP:          ip,
			UserAgent:   ua,
			TargetURL:   target,
		}).Error
	}

	if vParam == "" {
		vParam = utils.BuildMailTrackingV(secret, campaign.ID, recipient.Email)
	}
	c.Redirect(http.StatusFound, utils.AppendMailTrackingV(target, vParam))
}

// resolveMailTrackingV finds campaign/recipient for a compact v= token in mail links.
func resolveMailTrackingV(v string) (*models.MailCampaign, *models.MailRecipient, bool) {
	token, email, ok := utils.ParseMailTrackingV(v)
	if !ok || !EffectiveMailTrackingEnabled() {
		return nil, nil, false
	}
	db := config.GetDB()
	var recipients []models.MailRecipient
	if err := db.Where("email = ?", email).Order("id DESC").Limit(50).Find(&recipients).Error; err != nil {
		return nil, nil, false
	}
	secret := EffectiveMailTrackingSecret()
	for i := range recipients {
		rec := &recipients[i]
		if !utils.MailTrackingTokenMatches(secret, rec.CampaignID, email, token) {
			continue
		}
		var campaign models.MailCampaign
		if err := db.First(&campaign, rec.CampaignID).Error; err != nil {
			continue
		}
		return &campaign, rec, true
	}
	return nil, nil, false
}
