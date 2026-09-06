package utils

import "testing"

func TestApplyProjectHTMLPlaceholders(t *testing.T) {
	in := `<form action="{{SUBMIT_URL}}"><a href="{{REDIRECT_URL}}">x</a>{{QR_RELAY_IMG}}</form>`
	out := ApplyProjectHTMLPlaceholders(in, ProjectHTMLVars{
		SubmitURL:   "/api/submit",
		RedirectURL: "https://origin.example/login",
		QRRelayURL:  "/q/abc.png",
	})
	for _, want := range []string{
		`action="/api/submit"`,
		`href="https://origin.example/login"`,
		`src="/q/abc.png"`,
	} {
		if !contains(out, want) {
			t.Fatalf("missing %q in %s", want, out)
		}
	}
	if contains(out, "{{") {
		t.Fatalf("placeholders left: %s", out)
	}
}

func TestApplyProjectHTMLPlaceholdersDefaultSubmit(t *testing.T) {
	out := ApplyProjectHTMLPlaceholders(`{{SUBMIT_URL}}`, ProjectHTMLVars{})
	if out != "/api/submit" {
		t.Fatalf("got %q", out)
	}
}
