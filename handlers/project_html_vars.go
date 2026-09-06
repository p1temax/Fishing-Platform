package handlers

import (
	"strconv"
	"strings"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
)

// projectHTMLVars builds placeholder values for a project's landing HTML.
func projectHTMLVars(project *models.Project) utils.ProjectHTMLVars {
	vars := utils.ProjectHTMLVars{
		SubmitURL:   project.ContainerRoute,
		RedirectURL: project.LoginURL,
	}
	if project == nil || project.QrRelayID == nil || *project.QrRelayID == 0 {
		return vars
	}
	var relay models.QrRelay
	if err := config.GetDB().Select("id", "slug", "enabled").First(&relay, *project.QrRelayID).Error; err != nil {
		return vars
	}
	if !relay.Enabled || strings.TrimSpace(relay.Slug) == "" {
		return vars
	}
	path := utils.QRRelayPublicPath(relay.Slug)
	base := strings.TrimRight(EffectivePublicBaseURL(), "/")
	if base != "" {
		vars.QRRelayURL = base + path
	} else {
		vars.QRRelayURL = path
	}
	return vars
}

func parseOptionalUintString(raw string) *uint {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0" {
		return nil
	}
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil || v == 0 {
		return nil
	}
	u := uint(v)
	return &u
}
