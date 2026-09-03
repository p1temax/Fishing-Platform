package utils

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

// SmtpSendConfig is the dial/auth profile used to send mail.
type SmtpSendConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
	FromName  string
	UseTLS    bool
	UseSSL    bool
}

// SmtpMessage is an outbound message.
type SmtpMessage struct {
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
	Body    string
	HTML    bool
}

func formatAddress(name, email string) string {
	email = strings.TrimSpace(email)
	name = strings.TrimSpace(name)
	if email == "" {
		return ""
	}
	if name == "" {
		return email
	}
	addr := mail.Address{Name: name, Address: email}
	return addr.String()
}

func buildMimeMessage(cfg SmtpSendConfig, msg SmtpMessage) ([]byte, error) {
	from := formatAddress(cfg.FromName, cfg.FromEmail)
	if from == "" {
		return nil, fmt.Errorf("from email is required")
	}
	if len(msg.To) == 0 {
		return nil, fmt.Errorf("at least one recipient is required")
	}

	headers := []string{
		"From: " + from,
		"To: " + strings.Join(msg.To, ", "),
		"Subject: " + sanitizeHeader(msg.Subject),
		"MIME-Version: 1.0",
		fmt.Sprintf("Date: %s", time.Now().Format(time.RFC1123Z)),
	}
	if len(msg.Cc) > 0 {
		headers = append(headers, "Cc: "+strings.Join(msg.Cc, ", "))
	}
	contentType := "text/plain; charset=UTF-8"
	if msg.HTML {
		contentType = "text/html; charset=UTF-8"
	}
	headers = append(headers, "Content-Type: "+contentType)

	var b strings.Builder
	b.WriteString(strings.Join(headers, "\r\n"))
	b.WriteString("\r\n\r\n")
	b.WriteString(msg.Body)
	return []byte(b.String()), nil
}

func sanitizeHeader(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "\r", ""), "\n", "")
}

func allRecipients(msg SmtpMessage) []string {
	out := make([]string, 0, len(msg.To)+len(msg.Cc)+len(msg.Bcc))
	out = append(out, msg.To...)
	out = append(out, msg.Cc...)
	out = append(out, msg.Bcc...)
	return out
}

// SendSMTPMail sends a message using the provided SMTP profile.
func SendSMTPMail(cfg SmtpSendConfig, msg SmtpMessage) error {
	if strings.TrimSpace(cfg.Host) == "" {
		return fmt.Errorf("SMTP host is required")
	}
	if cfg.Port <= 0 {
		cfg.Port = 587
	}

	raw, err := buildMimeMessage(cfg, msg)
	if err != nil {
		return err
	}
	recipients := allRecipients(msg)
	if len(recipients) == 0 {
		return fmt.Errorf("at least one recipient is required")
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	var auth smtp.Auth
	if strings.TrimSpace(cfg.Username) != "" {
		auth = smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)
	}

	tlsConfig := &tls.Config{
		ServerName: cfg.Host,
		MinVersion: tls.VersionTLS12,
	}

	if cfg.UseSSL || cfg.Port == 465 {
		conn, err := tls.Dial("tcp", addr, tlsConfig)
		if err != nil {
			return fmt.Errorf("SMTP SSL connect failed: %w", err)
		}
		defer conn.Close()
		client, err := smtp.NewClient(conn, cfg.Host)
		if err != nil {
			return fmt.Errorf("SMTP client failed: %w", err)
		}
		defer client.Close()
		return smtpClientSend(client, auth, cfg.FromEmail, recipients, raw)
	}

	conn, err := net.DialTimeout("tcp", addr, 15*time.Second)
	if err != nil {
		return fmt.Errorf("SMTP connect failed: %w", err)
	}
	client, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		_ = conn.Close()
		return fmt.Errorf("SMTP client failed: %w", err)
	}
	defer client.Close()

	if cfg.UseTLS || cfg.Port == 587 {
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return fmt.Errorf("SMTP STARTTLS failed: %w", err)
			}
		} else if cfg.UseTLS {
			return fmt.Errorf("SMTP server does not support STARTTLS")
		}
	}

	return smtpClientSend(client, auth, cfg.FromEmail, recipients, raw)
}

func smtpClientSend(client *smtp.Client, auth smtp.Auth, from string, to []string, raw []byte) error {
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return fmt.Errorf("SMTP auth failed: %w", err)
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return fmt.Errorf("SMTP MAIL FROM failed: %w", err)
	}
	for _, rcpt := range to {
		if err := client.Rcpt(rcpt); err != nil {
			return fmt.Errorf("SMTP RCPT TO %s failed: %w", rcpt, err)
		}
	}
	w, err := client.Data()
	if err != nil {
		return fmt.Errorf("SMTP DATA failed: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		_ = w.Close()
		return fmt.Errorf("SMTP write body failed: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("SMTP close body failed: %w", err)
	}
	return client.Quit()
}

// SplitAddressList splits comma/semicolon separated emails.
func SplitAddressList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// NormalizeEmailList accepts string or []string-like values already as []string.
func NormalizeEmailList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, v := range values {
		for _, addr := range SplitAddressList(v) {
			key := strings.ToLower(addr)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, addr)
		}
	}
	return out
}
