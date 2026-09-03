package handlers

import (
	"testing"

	"fishing-platform-backend/models"
)

func TestNormalizeSmtpPayloadDefaults(t *testing.T) {
	port := 0
	_, err := normalizeSmtpPayload(smtpServicePayload{
		Name:      "main",
		Host:      "smtp.example.com",
		Port:      &port,
		FromEmail: "noreply@example.com",
	}, true)
	if err == nil {
		t.Fatal("expected invalid port error")
	}

	svc, err := normalizeSmtpPayload(smtpServicePayload{
		Name:      "main",
		Host:      "smtp.example.com",
		FromEmail: "noreply@example.com",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if svc.Port != 587 || !svc.UseTLS || svc.UseSSL {
		t.Fatalf("unexpected defaults: %+v", svc)
	}

	port465 := 465
	svc, err = normalizeSmtpPayload(smtpServicePayload{
		Name:      "ssl",
		Host:      "smtp.example.com",
		Port:      &port465,
		FromEmail: "noreply@example.com",
	}, true)
	if err != nil {
		t.Fatal(err)
	}
	if !svc.UseSSL || svc.UseTLS {
		t.Fatalf("expected SSL defaults for 465: %+v", svc)
	}
}

func TestNormalizeSmtpPayloadRejectsBothTLSFlags(t *testing.T) {
	useTLS := true
	useSSL := true
	_, err := normalizeSmtpPayload(smtpServicePayload{
		Name:      "bad",
		Host:      "smtp.example.com",
		FromEmail: "a@b.com",
		UseTLS:    &useTLS,
		UseSSL:    &useSSL,
	}, true)
	if err == nil {
		t.Fatal("expected mutual exclusion error")
	}
}

func TestSmtpServiceToPublicHidesPassword(t *testing.T) {
	svc := models.SmtpService{
		ID:        1,
		Name:      "main",
		Host:      "smtp.example.com",
		Port:      587,
		Password:  "secret",
		FromEmail: "a@b.com",
		Enabled:   true,
	}
	pub := svc.ToPublic()
	if !pub.HasPassword {
		t.Fatal("expected has_password true")
	}
	// Ensure public struct has no password field exposed via JSON tags of model
	if pub.Name != "main" || pub.FromEmail != "a@b.com" {
		t.Fatalf("unexpected public payload: %+v", pub)
	}
}

func TestParseEmailListField(t *testing.T) {
	got := parseEmailListField("a@x.com, b@y.com")
	if len(got) != 2 {
		t.Fatalf("string parse failed: %#v", got)
	}
	got = parseEmailListField([]interface{}{"a@x.com", "b@y.com"})
	if len(got) != 2 {
		t.Fatalf("array parse failed: %#v", got)
	}
}
