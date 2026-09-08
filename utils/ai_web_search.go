package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"
)

const infoGatherRawMax = 256 << 10 // 256 KiB

// InfoGatherSystemPrompt locks the model to public OSINT + JSON output.
const InfoGatherSystemPrompt = `You are an OSINT assistant for authorized security assessments.
Use web search (and X search when enabled) to find publicly exposed contact information for the given target.

Rules:
1. Only report contacts that appear on public pages or public posts.
2. Every finding MUST include a real source_url you observed. Never invent emails, phones, or URLs.
3. If you cannot verify a contact with a source, omit it.
4. Prefer official sites, press pages, WHOIS/public registries, and public staff directories.
5. Final answer MUST be a single JSON object (no markdown fences) with this shape:
{
  "findings": [
    {
      "kind": "email|phone|url|name|other",
      "value": "string",
      "label": "optional person/role",
      "source_url": "https://...",
      "snippet": "short evidence quote",
      "confidence": "high|medium|low"
    }
  ],
  "notes": "optional short summary"
}`

var (
	phoneLooseRe = regexp.MustCompile(`(?i)^\+?[\d\s\-().]{7,20}$`)
	digitsOnlyRe = regexp.MustCompile(`\d+`)
)

// InfoGatherInput is passed into the AI web-search prompt.
type InfoGatherInput struct {
	Target         string
	Notes          string
	IncludeXSearch bool
}

// InfoGatherFindingDTO is one normalized finding from the model.
type InfoGatherFindingDTO struct {
	Kind       string `json:"kind"`
	Value      string `json:"value"`
	Label      string `json:"label"`
	SourceURL  string `json:"source_url"`
	Snippet    string `json:"snippet"`
	Confidence string `json:"confidence"`
}

// InfoGatherResult is the parsed model output after validation.
type InfoGatherResult struct {
	Findings    []InfoGatherFindingDTO
	Notes       string
	RawResponse string
}

type responsesRequest struct {
	Model        string           `json:"model"`
	Input        any              `json:"input"`
	Instructions string           `json:"instructions,omitempty"`
	Tools        []map[string]any `json:"tools"`
	Temperature  float64          `json:"temperature"`
}

type responsesAPIResponse struct {
	Output json.RawMessage `json:"output"`
	Error  json.RawMessage `json:"error"`
	// Some gateways nest the assistant text here.
	OutputText string `json:"output_text"`
}

// GatherContactsWithAI calls a generic Responses API:
//
//	POST {base_url}/responses
//	Authorization: Bearer {api_key}
//	tools: web_search (+ optional x_search)
//
// Users supply any compatible base_url + api_key in AI Settings.
// The endpoint must support Responses-style web_search; Chat Completions-only URLs will fail.
func GatherContactsWithAI(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration, in InfoGatherInput) (InfoGatherResult, error) {
	var empty InfoGatherResult
	if strings.TrimSpace(apiKey) == "" {
		return empty, fmt.Errorf("AI API key is not configured")
	}
	if strings.TrimSpace(model) == "" {
		model = defaultAIModelFallback()
	}
	baseURL = NormalizeOpenAICompatibleBaseURL(baseURL)
	if baseURL == "" {
		return empty, fmt.Errorf("AI base URL is not configured")
	}
	if timeout <= 0 {
		timeout = 180 * time.Second
	}
	if timeout < 120*time.Second {
		timeout = 120 * time.Second
	}

	target := strings.TrimSpace(in.Target)
	if target == "" {
		return empty, fmt.Errorf("target is required")
	}

	payload := buildInfoGatherResponsesRequest(model, in)
	raw, err := json.Marshal(payload)
	if err != nil {
		return empty, err
	}

	endpoint := baseURL + "/responses"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return empty, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return empty, fmt.Errorf("AI request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil {
		return empty, err
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return empty, fmt.Errorf("AI base_url does not support POST /responses with web_search (HTTP %d at %s). Set base_url to an API root that exposes this endpoint (usually ending in /v1)", resp.StatusCode, endpoint)
	}

	var parsed responsesAPIResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return empty, fmt.Errorf("AI request failed with status %d: %s", resp.StatusCode, truncateForErr(string(body), 300))
		}
		return empty, fmt.Errorf("invalid AI response: %w", err)
	}
	if errMsg := extractAIErrorMessage(parsed.Error); errMsg != "" {
		return empty, fmt.Errorf("AI error: %s", errMsg)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		hint := ""
		bodyLower := strings.ToLower(string(body))
		if strings.Contains(bodyLower, "web_search") || strings.Contains(bodyLower, "tool") || strings.Contains(bodyLower, "x_search") {
			hint = " Check that this model/key enables the web_search tool (and x_search if selected)."
		}
		return empty, fmt.Errorf("AI request failed with status %d: %s%s", resp.StatusCode, truncateForErr(string(body), 300), hint)
	}

	text := strings.TrimSpace(parsed.OutputText)
	if text == "" {
		text = extractResponsesOutputText(parsed.Output)
	}
	if text == "" {
		// Last resort: whole body may be the JSON payload in some proxies.
		text = strings.TrimSpace(string(body))
	}
	if text == "" {
		return empty, fmt.Errorf("AI returned empty content")
	}

	result, err := ParseInfoGatherJSON(text)
	if err != nil {
		result.RawResponse = truncateRaw(text, infoGatherRawMax)
		return result, err
	}
	result.RawResponse = truncateRaw(text, infoGatherRawMax)
	return result, nil
}

// buildInfoGatherResponsesRequest builds one vendor-agnostic Responses payload.
// Shape:
//
//	{ model, instructions, input(string), tools:[{type:web_search}], temperature }
//
// Optional tools: x_search when IncludeXSearch is true (ignored by providers that lack it).
func buildInfoGatherResponsesRequest(model string, in InfoGatherInput) responsesRequest {
	tools := []map[string]any{{"type": "web_search"}}
	if in.IncludeXSearch {
		tools = append(tools, map[string]any{"type": "x_search"})
	}
	return responsesRequest{
		Model:        model,
		Instructions: InfoGatherSystemPrompt,
		Input:        BuildInfoGatherUserPrompt(in),
		Tools:        tools,
		Temperature:  0,
	}
}

// BuildInfoGatherUserPrompt builds the user message for contact gathering.
func BuildInfoGatherUserPrompt(in InfoGatherInput) string {
	var b strings.Builder
	b.WriteString("Target: ")
	b.WriteString(strings.TrimSpace(in.Target))
	b.WriteString("\n")
	if notes := strings.TrimSpace(in.Notes); notes != "" {
		b.WriteString("Operator notes: ")
		b.WriteString(notes)
		b.WriteString("\n")
	}
	if in.IncludeXSearch {
		b.WriteString("Also search public X/Twitter posts when useful.\n")
	}
	b.WriteString("Collect publicly exposed emails, phone numbers, and relevant contact/profile URLs with sources. Return JSON only.")
	return b.String()
}

// ParseInfoGatherJSON extracts and validates findings from model text.
func ParseInfoGatherJSON(text string) (InfoGatherResult, error) {
	var empty InfoGatherResult
	cleaned := normalizeAIHTMLOutput(text) // strips ``` fences if present
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		return empty, fmt.Errorf("empty AI JSON")
	}

	// Try direct object; if that fails, find first {...} block.
	payload := cleaned
	if !json.Valid([]byte(payload)) {
		start := strings.Index(cleaned, "{")
		end := strings.LastIndex(cleaned, "}")
		if start < 0 || end <= start {
			return empty, fmt.Errorf("AI response is not JSON")
		}
		payload = cleaned[start : end+1]
	}

	var raw struct {
		Findings []InfoGatherFindingDTO `json:"findings"`
		Notes    string                 `json:"notes"`
	}
	if err := json.Unmarshal([]byte(payload), &raw); err != nil {
		return empty, fmt.Errorf("invalid findings JSON: %w", err)
	}

	out := InfoGatherResult{Notes: strings.TrimSpace(raw.Notes)}
	seen := map[string]struct{}{}
	for _, f := range raw.Findings {
		norm, ok := NormalizeInfoGatherFinding(f)
		if !ok {
			continue
		}
		key := strings.ToLower(norm.Kind + "|" + norm.Value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out.Findings = append(out.Findings, norm)
	}
	if len(out.Findings) == 0 {
		return out, fmt.Errorf("no valid findings with source_url")
	}
	return out, nil
}

// NormalizeInfoGatherFinding validates and cleans one finding.
func NormalizeInfoGatherFinding(f InfoGatherFindingDTO) (InfoGatherFindingDTO, bool) {
	kind := strings.ToLower(strings.TrimSpace(f.Kind))
	value := strings.TrimSpace(f.Value)
	source := strings.TrimSpace(f.SourceURL)
	if value == "" || source == "" {
		return InfoGatherFindingDTO{}, false
	}
	if !strings.HasPrefix(strings.ToLower(source), "http://") && !strings.HasPrefix(strings.ToLower(source), "https://") {
		return InfoGatherFindingDTO{}, false
	}

	switch kind {
	case "email":
		addr, err := mail.ParseAddress(value)
		if err != nil || addr == nil || !strings.Contains(addr.Address, "@") {
			return InfoGatherFindingDTO{}, false
		}
		value = strings.ToLower(strings.TrimSpace(addr.Address))
	case "phone":
		if !phoneLooseRe.MatchString(value) {
			return InfoGatherFindingDTO{}, false
		}
		digits := strings.Join(digitsOnlyRe.FindAllString(value, -1), "")
		if len(digits) < 7 || len(digits) > 15 {
			return InfoGatherFindingDTO{}, false
		}
	case "url", "name", "other":
		// keep as-is
	default:
		kind = "other"
	}

	conf := strings.ToLower(strings.TrimSpace(f.Confidence))
	switch conf {
	case "high", "medium", "low":
	default:
		conf = "medium"
	}

	label := strings.TrimSpace(f.Label)
	snippet := strings.TrimSpace(f.Snippet)
	if len(value) > 500 {
		value = value[:500]
	}
	if len(source) > 1000 {
		source = source[:1000]
	}
	if len(label) > 255 {
		label = label[:255]
	}
	if len(snippet) > 2000 {
		snippet = snippet[:2000]
	}

	return InfoGatherFindingDTO{
		Kind:       kind,
		Value:      value,
		Label:      label,
		SourceURL:  source,
		Snippet:    snippet,
		Confidence: conf,
	}, true
}

func extractResponsesOutputText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	// output may be a string
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}

	// output may be an array of items
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return ""
	}

	var parts []string
	for _, item := range items {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(item, &obj); err != nil {
			continue
		}
		// Skip tool-call only items when content is empty.
		if content, ok := obj["content"]; ok {
			parts = append(parts, extractContentText(content)...)
		}
		if text, ok := obj["text"]; ok {
			var s string
			if json.Unmarshal(text, &s) == nil && strings.TrimSpace(s) != "" {
				parts = append(parts, strings.TrimSpace(s))
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func extractContentText(raw json.RawMessage) []string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		asString = strings.TrimSpace(asString)
		if asString == "" {
			return nil
		}
		return []string{asString}
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil
	}
	var out []string
	for _, part := range arr {
		if t, ok := part["text"].(string); ok && strings.TrimSpace(t) != "" {
			out = append(out, strings.TrimSpace(t))
			continue
		}
		if t, ok := part["type"].(string); ok && (t == "output_text" || t == "text") {
			if s, ok := part["text"].(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

func truncateRaw(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}
