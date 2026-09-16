package utils

import (
	"strings"
	"testing"
)

func TestParseInfoGatherJSONValid(t *testing.T) {
	raw := `{
  "findings": [
    {"kind":"email","value":"Alice@Example.com","label":"Alice","source_url":"https://example.com/team","snippet":"contact","confidence":"high"},
    {"kind":"phone","value":"+1 (555) 123-4567","source_url":"https://example.com/contact","confidence":"medium"},
    {"kind":"email","value":"alice@example.com","source_url":"https://dup.example","confidence":"low"}
  ],
  "notes":"ok"
}`
	got, err := ParseInfoGatherJSON(raw)
	if err != nil {
		t.Fatalf("ParseInfoGatherJSON: %v", err)
	}
	if got.Notes != "ok" {
		t.Fatalf("notes=%q", got.Notes)
	}
	if len(got.Findings) != 2 {
		t.Fatalf("findings=%d want 2 (deduped)", len(got.Findings))
	}
	if got.Findings[0].Value != "alice@example.com" {
		t.Fatalf("email normalized=%q", got.Findings[0].Value)
	}
}

func TestParseInfoGatherJSONRejectsNoSource(t *testing.T) {
	raw := `{"findings":[{"kind":"email","value":"a@b.com","source_url":"","confidence":"high"}]}`
	_, err := ParseInfoGatherJSON(raw)
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}

func TestParseInfoGatherJSONFenced(t *testing.T) {
	raw := "```json\n{\"findings\":[{\"kind\":\"email\",\"value\":\"ops@example.com\",\"source_url\":\"https://example.com\",\"confidence\":\"high\"}]}\n```"
	got, err := ParseInfoGatherJSON(raw)
	if err != nil {
		t.Fatalf("ParseInfoGatherJSON: %v", err)
	}
	if len(got.Findings) != 1 || got.Findings[0].Kind != "email" {
		t.Fatalf("unexpected %+v", got.Findings)
	}
}

func TestNormalizeInfoGatherFindingRejectsURLAndName(t *testing.T) {
	_, ok := NormalizeInfoGatherFinding(InfoGatherFindingDTO{
		Kind: "url", Value: "https://example.com", SourceURL: "https://example.com",
	})
	if ok {
		t.Fatal("url findings must be rejected")
	}
	_, ok = NormalizeInfoGatherFinding(InfoGatherFindingDTO{
		Kind: "name", Value: "Alice", SourceURL: "https://example.com",
	})
	if ok {
		t.Fatal("name findings must be rejected")
	}
}

func TestNormalizeInfoGatherFindingPhone(t *testing.T) {
	_, ok := NormalizeInfoGatherFinding(InfoGatherFindingDTO{
		Kind: "phone", Value: "123", SourceURL: "https://x.test",
	})
	if ok {
		t.Fatal("short phone should fail")
	}
	got, ok := NormalizeInfoGatherFinding(InfoGatherFindingDTO{
		Kind: "phone", Value: "13800138000", SourceURL: "https://x.test", Confidence: "HIGH",
	})
	if !ok || got.Confidence != "high" {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
}

func TestBuildInfoGatherUserPrompt(t *testing.T) {
	s := BuildInfoGatherUserPrompt(InfoGatherInput{Target: "Acme", Notes: "VPN", IncludeXSearch: true})
	for _, want := range []string{"Acme", "VPN", "X/Twitter"} {
		if !strings.Contains(s, want) {
			t.Fatalf("prompt missing %q: %s", want, s)
		}
	}
}

func TestBuildInfoGatherResponsesRequestGeneric(t *testing.T) {
	in := InfoGatherInput{Target: "Acme", Notes: "VPN", IncludeXSearch: true}
	req := buildInfoGatherResponsesRequest("any-model", in)
	if req.Model != "any-model" {
		t.Fatalf("model=%q", req.Model)
	}
	if req.Instructions == "" {
		t.Fatal("instructions required")
	}
	input, ok := req.Input.(string)
	if !ok || !strings.Contains(input, "Acme") {
		t.Fatalf("input=%v", req.Input)
	}
	if len(req.Tools) != 2 {
		t.Fatalf("tools=%v want web_search+x_search", req.Tools)
	}
	if req.Tools[0]["type"] != "web_search" || req.Tools[1]["type"] != "x_search" {
		t.Fatalf("tools=%v", req.Tools)
	}

	req2 := buildInfoGatherResponsesRequest("m", InfoGatherInput{Target: "T"})
	if len(req2.Tools) != 1 || req2.Tools[0]["type"] != "web_search" {
		t.Fatalf("default tools=%v", req2.Tools)
	}
}
