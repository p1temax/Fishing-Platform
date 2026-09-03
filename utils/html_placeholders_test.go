package utils

import "testing"

func TestApplyProjectHTMLPlaceholders(t *testing.T) {
	in := `<form action="{{SUBMIT_URL}}"><a href="{{REDIRECT_URL}}">x</a></form>`
	out := ApplyProjectHTMLPlaceholders(in, ProjectHTMLVars{
		SubmitURL:   "/api/submit",
		RedirectURL: "https://origin.example/login",
	})
	for _, want := range []string{
		`action="/api/submit"`,
		`href="https://origin.example/login"`,
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
