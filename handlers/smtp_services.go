package handlers

import (
	"errors"
	"net/http"
	"net/mail"
	"strings"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"fishing-platform-backend/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type smtpServicePayload struct {
	Name        string `json:"name"`
	Host        string `json:"host"`
	Port        *int   `json:"port"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	FromEmail   string `json:"from_email"`
	FromName    string `json:"from_name"`
	UseTLS      *bool  `json:"use_tls"`
	UseSSL      *bool  `json:"use_ssl"`
	Enabled     *bool  `json:"enabled"`
	Description string `json:"description"`
}

type smtpSendPayload struct {
	To      interface{} `json:"to"`
	Cc      interface{} `json:"cc"`
	Bcc     interface{} `json:"bcc"`
	Subject string      `json:"subject"`
	Body    string      `json:"body"`
	HTML    bool        `json:"html"`
}

type smtpTestPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

func parseEmailListField(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		return utils.SplitAddressList(v)
	case []interface{}:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return utils.NormalizeEmailList(parts)
	case []string:
		return utils.NormalizeEmailList(v)
	default:
		return nil
	}
}

func validateEmailAddresses(addrs []string) error {
	for _, addr := range addrs {
		if _, err := mail.ParseAddress(addr); err != nil {
			return errors.New("invalid email address: " + addr)
		}
	}
	return nil
}

func normalizeSmtpPayload(payload smtpServicePayload, creating bool) (models.SmtpService, error) {
	name := strings.TrimSpace(payload.Name)
	host := strings.TrimSpace(payload.Host)
	fromEmail := strings.TrimSpace(payload.FromEmail)
	if name == "" {
		return models.SmtpService{}, errors.New("name is required")
	}
	if host == "" {
		return models.SmtpService{}, errors.New("host is required")
	}
	if fromEmail == "" {
		return models.SmtpService{}, errors.New("from_email is required")
	}
	if _, err := mail.ParseAddress(fromEmail); err != nil {
		return models.SmtpService{}, errors.New("from_email is invalid")
	}

	port := 587
	if payload.Port != nil {
		port = *payload.Port
	}
	if port <= 0 || port > 65535 {
		return models.SmtpService{}, errors.New("port must be between 1 and 65535")
	}

	useTLS := true
	if payload.UseTLS != nil {
		useTLS = *payload.UseTLS
	}
	useSSL := false
	if payload.UseSSL != nil {
		useSSL = *payload.UseSSL
	}
	if useTLS && useSSL {
		return models.SmtpService{}, errors.New("use_tls and use_ssl cannot both be true")
	}
	if creating && payload.UseTLS == nil && payload.UseSSL == nil {
		if port == 465 {
			useTLS = false
			useSSL = true
		} else {
			useTLS = true
			useSSL = false
		}
	}

	enabled := true
	if payload.Enabled != nil {
		enabled = *payload.Enabled
	}

	return models.SmtpService{
		Name:        name,
		Host:        host,
		Port:        port,
		Username:    strings.TrimSpace(payload.Username),
		Password:    payload.Password,
		FromEmail:   fromEmail,
		FromName:    strings.TrimSpace(payload.FromName),
		UseTLS:      useTLS,
		UseSSL:      useSSL,
		Enabled:     enabled,
		Description: strings.TrimSpace(payload.Description),
	}, nil
}

func smtpConfigFromModel(service models.SmtpService) utils.SmtpSendConfig {
	return utils.SmtpSendConfig{
		Host:      service.Host,
		Port:      service.Port,
		Username:  service.Username,
		Password:  service.Password,
		FromEmail: service.FromEmail,
		FromName:  service.FromName,
		UseTLS:    service.UseTLS,
		UseSSL:    service.UseSSL,
	}
}

func GetSmtpServices(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var services []models.SmtpService
	if err := scopeByOwner(db, user).Order("created_at DESC").Find(&services).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve SMTP services"})
		return
	}
	out := make([]models.SmtpServicePublic, 0, len(services))
	for _, s := range services {
		out = append(out, s.ToPublic())
	}
	c.JSON(http.StatusOK, out)
}

func GetSmtpService(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var service models.SmtpService
	if err := scopeByOwner(db, user).First(&service, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMTP service not found"})
		return
	}
	c.JSON(http.StatusOK, service.ToPublic())
}

func CreateSmtpService(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	var payload smtpServicePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	service, err := normalizeSmtpPayload(payload, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	service.CreatedBy = user.ID

	db := config.GetDB()
	if err := db.Create(&service).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create SMTP service"})
		return
	}
	c.JSON(http.StatusCreated, service.ToPublic())
}

func UpdateSmtpService(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var existing models.SmtpService
	if err := scopeByOwner(db, user).First(&existing, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMTP service not found"})
		return
	}

	var payload smtpServicePayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	updated, err := normalizeSmtpPayload(payload, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	existing.Name = updated.Name
	existing.Host = updated.Host
	existing.Port = updated.Port
	existing.Username = updated.Username
	existing.FromEmail = updated.FromEmail
	existing.FromName = updated.FromName
	existing.UseTLS = updated.UseTLS
	existing.UseSSL = updated.UseSSL
	existing.Enabled = updated.Enabled
	existing.Description = updated.Description
	if strings.TrimSpace(payload.Password) != "" {
		existing.Password = payload.Password
	}

	if err := db.Save(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update SMTP service"})
		return
	}
	c.JSON(http.StatusOK, existing.ToPublic())
}

func DeleteSmtpService(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var service models.SmtpService
	if err := scopeByOwner(db, user).First(&service, c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "SMTP service not found"})
		return
	}
	if err := db.Delete(&service).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete SMTP service"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "SMTP service deleted"})
}

func loadSmtpServiceOrFail(c *gin.Context) (*models.SmtpService, bool) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return nil, false
	}
	db := config.GetDB()
	var service models.SmtpService
	if err := scopeByOwner(db, user).First(&service, c.Param("id")).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "SMTP service not found"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load SMTP service"})
		}
		return nil, false
	}
	return &service, true
}

func TestSmtpService(c *gin.Context) {
	service, ok := loadSmtpServiceOrFail(c)
	if !ok {
		return
	}
	if !service.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SMTP service is disabled"})
		return
	}

	var payload smtpTestPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	to := utils.SplitAddressList(payload.To)
	if len(to) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to is required"})
		return
	}
	if err := validateEmailAddresses(to); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	subject := strings.TrimSpace(payload.Subject)
	if subject == "" {
		subject = "Fishing Platform SMTP test"
	}
	body := payload.Body
	if strings.TrimSpace(body) == "" {
		body = "This is a test email from Fishing Platform SMTP service \"" + service.Name + "\"."
	}

	err := utils.SendSMTPMail(smtpConfigFromModel(*service), utils.SmtpMessage{
		To:      to,
		Subject: subject,
		Body:    body,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Test email sent"})
}

func SendSmtpServiceMail(c *gin.Context) {
	service, ok := loadSmtpServiceOrFail(c)
	if !ok {
		return
	}
	if !service.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "SMTP service is disabled"})
		return
	}

	var payload smtpSendPayload
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	to := parseEmailListField(payload.To)
	cc := parseEmailListField(payload.Cc)
	bcc := parseEmailListField(payload.Bcc)
	if len(to) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "to is required"})
		return
	}
	if err := validateEmailAddresses(append(append(to, cc...), bcc...)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	subject := strings.TrimSpace(payload.Subject)
	if subject == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "subject is required"})
		return
	}
	if strings.TrimSpace(payload.Body) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body is required"})
		return
	}

	err := utils.SendSMTPMail(smtpConfigFromModel(*service), utils.SmtpMessage{
		To:      to,
		Cc:      cc,
		Bcc:     bcc,
		Subject: subject,
		Body:    payload.Body,
		HTML:    payload.HTML,
	})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Email sent"})
}
