package utils

import (
	"strings"
	"testing"
)

func TestMailTrackingSignRoundTrip(t *testing.T) {
	sig := MailTrackingSign("secret", 7, "Alice@Example.com")
	if sig == "" {
		t.Fatal("empty signature")
	}
	if !MailTrackingSignValid("secret", 7, "alice@example.com", sig) {
		t.Fatal("expected valid signature")
	}
	if MailTrackingSignValid("secret", 7, "bob@example.com", sig) {
		t.Fatal("signature must not validate for other email")
	}
}

func TestRenderMailTemplateHTMLInsertsPixel(t *testing.T) {
	body := `<p>Hi {{email}}</p><a href="{{click_url}}">Go</a>`
	out := RenderMailTemplate(body, map[string]string{
		"email":       "a@x.com",
		"sig":         "abc",
		"v":           "tok_1YWJ4LmNvbQ",
		"click_url":   "http://p/api/clickslug?x=1",
		"open_pixel":  "http://p/api/openslug?x=1",
		"landing_url": "https://land.example/login",
	}, true, true)
	for _, part := range []string{"a@x.com", "http://p/api/clickslug?x=1", `src="http://p/api/openslug?x=1"`} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in %s", part, out)
		}
	}
}

func TestRenderMailTemplatePlainAppendsClick(t *testing.T) {
	out := RenderMailTemplate("Hello", map[string]string{
		"email":       "a@x.com",
		"sig":         "abc",
		"click_url":   "http://p/api/clickslug?x=1",
		"open_pixel":  "http://p/api/openslug?x=1",
		"landing_url": "https://land.example/login",
	}, false, false)
	if out != "Hello\nhttp://p/api/clickslug?x=1" {
		t.Fatalf("unexpected plain render: %q", out)
	}
}

func TestHostAllowedForMailRedirect(t *testing.T) {
	if !HostAllowedForMailRedirect("https://phish.example/login", []string{"phish.example"}) {
		t.Fatal("expected allow")
	}
	if HostAllowedForMailRedirect("https://evil.example/", []string{"phish.example"}) {
		t.Fatal("expected deny")
	}
	if HostAllowedForMailRedirect("javascript:alert(1)", []string{"phish.example"}) {
		t.Fatal("expected deny javascript")
	}
}

func TestMailTrackingVRoundTrip(t *testing.T) {
	secret := "secret"
	email := "user@example.com"
	v := BuildMailTrackingV(secret, 42, email)
	token, parsedEmail, ok := ParseMailTrackingV(v)
	if !ok {
		t.Fatalf("parse failed for %q", v)
	}
	if parsedEmail != email {
		t.Fatalf("email=%q", parsedEmail)
	}
	if !MailTrackingTokenMatches(secret, 42, email, token) {
		t.Fatal("token mismatch")
	}
	if MailTrackingTokenMatches(secret, 43, email, token) {
		t.Fatal("token must not match other campaign")
	}
	out := AppendMailTrackingV("https://land.example/login?x=1", v)
	if !strings.Contains(out, "v=") || !strings.Contains(out, "x=1") {
		t.Fatalf("append failed: %s", out)
	}
	click := BuildMailClickURL("http://p", "/api/clickslug", secret, 42, email, "https://land.example/login")
	if !strings.Contains(click, "v=") || !strings.Contains(click, "/api/clickslug?") {
		t.Fatalf("click url missing v: %s", click)
	}
	open := BuildMailOpenPixelURL("http://p", "/api/openslug", secret, 42, email)
	if !strings.Contains(open, "action=xtrackd") || !strings.Contains(open, "v=") {
		t.Fatalf("open pixel missing v: %s", open)
	}
	rendered := RenderMailTemplate(`<body><a href="{{click_url}}">{{v}}</a></body>`, map[string]string{
		"v":         v,
		"click_url": click,
	}, true, false)
	if !strings.Contains(rendered, v) {
		t.Fatalf("mail html missing v: %s", rendered)
	}
	if !strings.Contains(rendered, `font-size:0px`) {
		t.Fatalf("mail html missing noise: %s", rendered)
	}
}
