package utils

import "testing"

func TestBuildMirrorUserPromptIncludesPlatformFields(t *testing.T) {
	prompt := BuildMirrorUserPrompt(MirrorRewriteInput{
		RawHTML:      "<html></html>",
		SubmitURL:    "/api/submit",
		RedirectURL:  "https://example.com",
		OriginalHost: "login.example.com",
	})
	for _, want := range []string{"SUBMIT_URL: /api/submit", "username", "password", "captchavalue", "login.example.com"} {
		if !contains(prompt, want) {
			t.Fatalf("prompt missing %q", want)
		}
	}
}

func TestValidateRewrittenHTML(t *testing.T) {
	okHTML := `<form action="/api/submit" method="post"><input name="username"><input name="password" type="password"></form>`
	if err := ValidateRewrittenHTML(okHTML, "/api/submit"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateRewrittenHTML(`<form action="/x"><input name="user"></form>`, "/api/submit"); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestStripHeavyBase64DataURIs(t *testing.T) {
	heavy := `src="data:image/png;base64,` + string(make([]byte, 600)) + `"`
	// make valid base64-ish chars
	buf := make([]byte, 600)
	for i := range buf {
		buf[i] = 'A'
	}
	heavy = `src="data:image/png;base64,` + string(buf) + `"`
	out := StripHeavyBase64DataURIs(`<img ` + heavy + `>`)
	if !contains(out, "[STRIPPED_FOR_AI]") {
		t.Fatalf("not stripped: %s", out)
	}
}

func TestNormalizeAIHTMLOutput(t *testing.T) {
	got := normalizeAIHTMLOutput("```html\n<html>ok</html>\n```")
	if got != "<html>ok</html>" {
		t.Fatalf("got %q", got)
	}
}

func TestNormalizeOpenAICompatibleBaseURL(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"https://api.x.ai/v1", "https://api.x.ai/v1"},
		{"https://api.x.ai/v1/", "https://api.x.ai/v1"},
		{"https://open.bigmodel.cn/api/paas/v4/chat/completions", "https://open.bigmodel.cn/api/paas/v4"},
		{"https://open.bigmodel.cn/api/paas/v4/chat/completions/", "https://open.bigmodel.cn/api/paas/v4"},
		{"https://api.openai.com/v1/responses", "https://api.openai.com/v1"},
		{"https://api.openai.com/v1/completions", "https://api.openai.com/v1"},
		{"  https://api.x.ai/v1/CHAT/COMPLETIONS  ", "https://api.x.ai/v1"},
		{"https://open.bigmodel.cn/api/paas/v4/web_search", "https://open.bigmodel.cn/api/paas/v4"},
	}
	for _, tc := range cases {
		if got := NormalizeOpenAICompatibleBaseURL(tc.in); got != tc.want {
			t.Fatalf("NormalizeOpenAICompatibleBaseURL(%q)=%q want %q", tc.in, got, tc.want)
		}
	}
}

func TestExtractAIErrorMessage(t *testing.T) {
	if got := extractAIErrorMessage([]byte(`"bad key"`)); got != "bad key" {
		t.Fatalf("string error: %q", got)
	}
	if got := extractAIErrorMessage([]byte(`{"message":"no credits"}`)); got != "no credits" {
		t.Fatalf("object error: %q", got)
	}
	if got := extractAIErrorMessage([]byte(`null`)); got != "" {
		t.Fatalf("null error: %q", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
