package utils

import (
	"strings"
	"testing"
)

func TestSplitAddressList(t *testing.T) {
	got := SplitAddressList(" a@x.com, b@y.com;c@z.com ,, ")
	if len(got) != 3 {
		t.Fatalf("expected 3 addresses, got %#v", got)
	}
}

func TestNormalizeEmailListDedupes(t *testing.T) {
	got := NormalizeEmailList([]string{"A@x.com", "a@x.com, b@y.com"})
	if len(got) != 2 {
		t.Fatalf("expected 2 unique addresses, got %#v", got)
	}
}

func TestBuildMimeMessage(t *testing.T) {
	raw, err := buildMimeMessage(SmtpSendConfig{
		FromEmail: "from@example.com",
		FromName:  "Sender",
	}, SmtpMessage{
		To:      []string{"to@example.com"},
		Subject: "Hello\nWorld",
		Body:    "body text",
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "To: to@example.com") {
		t.Fatalf("missing To header: %s", text)
	}
	if strings.Contains(text, "Hello\nWorld") {
		t.Fatalf("subject should be sanitized: %s", text)
	}
	if !strings.Contains(text, "Subject: HelloWorld") {
		t.Fatalf("unexpected subject: %s", text)
	}
	if !strings.Contains(text, "body text") {
		t.Fatalf("missing body: %s", text)
	}
}

func TestBuildMimeMessageRequiresRecipient(t *testing.T) {
	_, err := buildMimeMessage(SmtpSendConfig{FromEmail: "from@example.com"}, SmtpMessage{})
	if err == nil {
		t.Fatal("expected error")
	}
}
