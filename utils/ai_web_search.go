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
1. ONLY collect email addresses and phone numbers. Do NOT report urls, names, or other kinds.
2. Only report contacts that appear on public pages or public posts.
3. Every finding MUST include a real source_url you observed. Never invent emails or phones.
4. If you cannot verify a contact with a source, omit it.
5. Prefer official sites, press pages, WHOIS/public registries, and public staff directories.
6. Final answer MUST be a single JSON object (no markdown fences) with this shape:
{
  "findings": [
    {
      "kind": "email|phone",
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

// InfoGatherProgressFunc reports live search stages for UI visualization.
type InfoGatherProgressFunc func(stage string, percent int, message string)

// InfoGatherInput is passed into the AI web-search prompt.
type InfoGatherInput struct {
	Target         string
	Notes          string
	IncludeXSearch bool
	OnProgress     InfoGatherProgressFunc
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

const (
	infoGatherModeResponses   = "responses"
	infoGatherModeGLMSearch   = "glm_web_search"
	infoGatherModeGLMChatTool = "glm_chat_web_search"
)

// GatherContactsWithAI finds public contacts for a target via web search.
//
// Preferred path: POST {base_url}/responses with tools web_search (xAI/OpenAI-style).
// Fallback (Zhipu GLM): POST {base_url}/chat/completions with tools web_search enable=true.
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

	reportInfoGatherProgress(in, "starting", 5, "Starting info gathering")

	// Zhipu GLM roots do not expose /responses; go straight to chat + web_search.
	if prefersGLMWebSearch(baseURL) {
		return gatherContactsViaGLMChatWebSearch(ctx, baseURL, apiKey, model, timeout, in)
	}

	// Try Responses-style first; on missing endpoint fall back to GLM chat web_search.
	reportInfoGatherProgress(in, "searching", 25, "Calling Responses API with web_search")
	result, err := gatherContactsViaResponses(ctx, baseURL, apiKey, model, timeout, in)
	if err == nil {
		reportInfoGatherProgress(in, "parsing", 85, "Parsing email/phone findings")
		return result, nil
	}
	if !isMissingResponsesEndpoint(err) {
		return result, err
	}
	reportInfoGatherProgress(in, "searching", 30, "Falling back to Zhipu web_search")
	return gatherContactsViaGLMChatWebSearch(ctx, baseURL, apiKey, model, timeout, in)
}

func prefersGLMWebSearch(baseURL string) bool {
	lower := strings.ToLower(baseURL)
	return strings.Contains(lower, "bigmodel.cn") || strings.Contains(lower, "open.bigmodel")
}

func isMissingResponsesEndpoint(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "does not support post /responses") ||
		strings.Contains(msg, "post /responses not available") {
		return true
	}
	if !strings.Contains(msg, "/responses") {
		return false
	}
	// GLM and similar roots often close /responses oddly instead of a clean 404.
	return strings.Contains(msg, "http 404") ||
		strings.Contains(msg, "http 405") ||
		strings.Contains(msg, "unexpected eof") ||
		strings.Contains(msg, "connection reset") ||
		strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded")
}

func gatherContactsViaResponses(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration, in InfoGatherInput) (InfoGatherResult, error) {
	var empty InfoGatherResult
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
		return empty, fmt.Errorf("AI base_url does not support POST /responses with web_search (HTTP %d at %s)", resp.StatusCode, endpoint)
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

type glmChatWebSearchRequest struct {
	Model       string                   `json:"model"`
	Messages    []chatMessage            `json:"messages"`
	Tools       []map[string]interface{} `json:"tools"`
	Temperature float64                  `json:"temperature"`
	MaxTokens   int                      `json:"max_tokens,omitempty"`
}

type glmChatWebSearchResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	WebSearch []struct {
		Title   string `json:"title"`
		Link    string `json:"link"`
		Content string `json:"content"`
	} `json:"web_search"`
	Error json.RawMessage `json:"error"`
}

type glmWebSearchHit struct {
	Title   string `json:"title"`
	Link    string `json:"link"`
	Content string `json:"content"`
	Media   string `json:"media"`
}

func gatherContactsViaGLMChatWebSearch(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration, in InfoGatherInput) (InfoGatherResult, error) {
	var empty InfoGatherResult

	// Preferred Zhipu path from docs: standalone /web_search, then chat to structure findings.
	reportInfoGatherProgress(in, "searching", 20, "Searching the web")
	hits, searchErr := fetchGLMWebSearchHits(ctx, baseURL, apiKey, in, timeout)
	if searchErr == nil && len(hits) > 0 {
		reportInfoGatherProgress(in, "searching", 55, fmt.Sprintf("Collected %d search hits", len(hits)))
		reportInfoGatherProgress(in, "synthesizing", 70, "Extracting emails and phones")
		result, err := synthesizeFindingsFromSearchHits(ctx, baseURL, apiKey, model, timeout, in, hits)
		if err == nil {
			reportInfoGatherProgress(in, "parsing", 90, "Validating email/phone findings")
		}
		return result, err
	}

	// Fallback: chat/completions with tools.web_search.enable=true and forced search_query.
	reportInfoGatherProgress(in, "searching", 40, "Searching via chat web_search tool")
	endpoint := baseURL + "/chat/completions"
	query := strings.TrimSpace(in.Target)
	if notes := strings.TrimSpace(in.Notes); notes != "" {
		query = query + " " + notes
	}
	query = truncateRunes(query, 70)
	payload := glmChatWebSearchRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: InfoGatherSystemPrompt},
			{Role: "user", Content: BuildInfoGatherUserPrompt(in)},
		},
		Tools: []map[string]interface{}{
			{
				"type": "web_search",
				"web_search": map[string]interface{}{
					"enable":        true,
					"search_result": true,
					"search_engine": "search_std",
					"search_intent": false,
					"search_query":  query,
					"count":         10,
					"content_size":  "medium",
				},
			},
		},
		Temperature: 0,
		MaxTokens:   4096,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return empty, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return empty, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		if searchErr != nil {
			return empty, fmt.Errorf("GLM web_search failed (%v); chat web_search also failed: %w", searchErr, err)
		}
		return empty, fmt.Errorf("AI request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 12<<20))
	if err != nil {
		return empty, err
	}

	var parsed glmChatWebSearchResponse
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
		return empty, fmt.Errorf("AI request failed with status %d: %s", resp.StatusCode, truncateForErr(string(body), 300))
	}

	text := ""
	if len(parsed.Choices) > 0 {
		text = strings.TrimSpace(parsed.Choices[0].Message.Content)
	}
	if text == "" {
		return empty, fmt.Errorf("AI returned empty content from chat web_search")
	}

	reportInfoGatherProgress(in, "parsing", 85, "Parsing email/phone findings")
	result, err := ParseInfoGatherJSON(text)
	if err != nil {
		result.RawResponse = truncateRaw(text, infoGatherRawMax)
		return result, err
	}
	result.RawResponse = truncateRaw(text, infoGatherRawMax)
	return result, nil
}

func fetchGLMWebSearchHits(ctx context.Context, baseURL, apiKey string, in InfoGatherInput, timeout time.Duration) ([]glmWebSearchHit, error) {
	queries := buildGLMSearchQueries(in)
	client := &http.Client{Timeout: timeout}
	if timeout > 45*time.Second {
		client.Timeout = 45 * time.Second
	}
	var all []glmWebSearchHit
	seen := map[string]struct{}{}
	var lastErr error
	for i, q := range queries {
		percent := 20 + ((i+1)*30)/len(queries)
		if percent > 55 {
			percent = 55
		}
		reportInfoGatherProgress(in, "searching", percent, "Query: "+q)
		payload := map[string]any{
			"search_query":          q,
			"search_engine":         "search_std",
			"search_intent":         false,
			"count":                 10,
			"content_size":          "high",
			"search_recency_filter": "noLimit",
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		endpoint := baseURL + "/web_search"
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			lastErr = fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateForErr(string(body), 200))
			continue
		}
		var parsed struct {
			SearchResult []glmWebSearchHit `json:"search_result"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			lastErr = err
			continue
		}
		for _, hit := range parsed.SearchResult {
			link := strings.TrimSpace(hit.Link)
			if link == "" {
				continue
			}
			if _, ok := seen[link]; ok {
				continue
			}
			seen[link] = struct{}{}
			all = append(all, hit)
		}
	}
	if len(all) == 0 {
		if lastErr != nil {
			return nil, lastErr
		}
		return nil, fmt.Errorf("no web_search hits")
	}
	return all, nil
}

func buildGLMSearchQueries(in InfoGatherInput) []string {
	target := strings.TrimSpace(in.Target)
	notes := strings.TrimSpace(in.Notes)
	candidates := []string{
		truncateRunes(target+" 联系方式 邮箱 电话", 70),
		truncateRunes(target+" 客服 邮箱", 70),
		truncateRunes(target+" official contact email", 70),
	}
	if notes != "" {
		candidates = append([]string{truncateRunes(target+" "+notes, 70)}, candidates...)
	}
	out := make([]string, 0, 3)
	seen := map[string]struct{}{}
	for _, q := range candidates {
		q = strings.TrimSpace(q)
		if q == "" {
			continue
		}
		if _, ok := seen[q]; ok {
			continue
		}
		seen[q] = struct{}{}
		out = append(out, q)
		if len(out) >= 3 {
			break
		}
	}
	return out
}

func synthesizeFindingsFromSearchHits(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration, in InfoGatherInput, hits []glmWebSearchHit) (InfoGatherResult, error) {
	var empty InfoGatherResult
	var b strings.Builder
	b.WriteString(BuildInfoGatherUserPrompt(in))
	b.WriteString("\n\nBelow are web_search results. Extract ONLY emails and phone numbers that appear in these results. Do not output url/name/other. Every finding MUST use a real link from the results as source_url.\n\n")
	limit := len(hits)
	if limit > 15 {
		limit = 15
	}
	for i := 0; i < limit; i++ {
		h := hits[i]
		b.WriteString(fmt.Sprintf("[%d] title: %s\nlink: %s\nsnippet: %s\n\n", i+1, strings.TrimSpace(h.Title), strings.TrimSpace(h.Link), truncateForErr(strings.TrimSpace(h.Content), 400)))
	}

	payload := map[string]any{
		"model": model,
		"messages": []chatMessage{
			{Role: "system", Content: InfoGatherSystemPrompt},
			{Role: "user", Content: b.String()},
		},
		"temperature": 0,
		"max_tokens":  4096,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return empty, err
	}
	endpoint := baseURL + "/chat/completions"
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
	var parsed chatCompletionResponse
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
		return empty, fmt.Errorf("AI request failed with status %d: %s", resp.StatusCode, truncateForErr(string(body), 300))
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return empty, fmt.Errorf("AI returned empty content while synthesizing search hits")
	}
	text := strings.TrimSpace(parsed.Choices[0].Message.Content)
	result, err := ParseInfoGatherJSON(text)
	if err != nil {
		result.RawResponse = truncateRaw(text, infoGatherRawMax)
		return result, err
	}
	result.RawResponse = truncateRaw(text, infoGatherRawMax)
	return result, nil
}

func truncateRunes(s string, max int) string {
	if max <= 0 {
		return ""
	}
	r := []rune(strings.TrimSpace(s))
	if len(r) <= max {
		return string(r)
	}
	return string(r[:max])
}

// ProbeWebSearchResult is returned by ProbeWebSearch / ProbeWebSearchResponses.
type ProbeWebSearchResult struct {
	Endpoint   string `json:"endpoint"`
	LatencyMs  int64  `json:"latency_ms"`
	Model      string `json:"model"`
	StatusCode int    `json:"status_code"`
	Mode       string `json:"mode,omitempty"` // responses | glm_web_search
}

// ProbeWebSearch verifies Info Gathering web_search capability.
// Order: Zhipu chat+web_search (for bigmodel roots) → Responses /responses → standalone /web_search.
func ProbeWebSearch(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration) (ProbeWebSearchResult, error) {
	out := ProbeWebSearchResult{Model: strings.TrimSpace(model)}
	if strings.TrimSpace(apiKey) == "" {
		return out, fmt.Errorf("API key is required")
	}
	if strings.TrimSpace(model) == "" {
		return out, fmt.Errorf("model is required")
	}
	baseURL = NormalizeOpenAICompatibleBaseURL(baseURL)
	if baseURL == "" {
		return out, fmt.Errorf("base URL is required")
	}
	if timeout <= 0 {
		timeout = 45 * time.Second
	}

	if prefersGLMWebSearch(baseURL) {
		glmStand, standErr := probeGLMStandaloneWebSearch(ctx, baseURL, apiKey, model, 20*time.Second)
		if standErr == nil {
			return glmStand, nil
		}
		glmChat, err := probeGLMChatWebSearch(ctx, baseURL, apiKey, model, timeout)
		if err == nil {
			return glmChat, nil
		}
		out = glmStand
		if out.Endpoint == "" {
			out.Endpoint = baseURL + "/web_search"
		}
		return out, fmt.Errorf("%v (also chat web_search: %v)", standErr, err)
	}

	respResult, respErr := probeResponsesWebSearch(ctx, baseURL, apiKey, model, timeout)
	if respErr == nil {
		return respResult, nil
	}

	glmResult, glmErr := probeGLMStandaloneWebSearch(ctx, baseURL, apiKey, model, 20*time.Second)
	if glmErr == nil {
		return glmResult, nil
	}
	glmChat, chatErr := probeGLMChatWebSearch(ctx, baseURL, apiKey, model, timeout)
	if chatErr == nil {
		return glmChat, nil
	}

	if isMissingResponsesEndpoint(respErr) || respResult.StatusCode == http.StatusNotFound || respResult.StatusCode == http.StatusMethodNotAllowed {
		out = glmResult
		if out.Endpoint == "" {
			out.Endpoint = baseURL + "/web_search"
		}
		return out, fmt.Errorf("%s (also tried /responses: %v; chat web_search: %v)", glmErr.Error(), respErr, chatErr)
	}
	out = respResult
	return out, fmt.Errorf("%s (also tried /web_search: %v; chat web_search: %v)", respErr.Error(), glmErr, chatErr)
}

// ProbeWebSearchResponses is kept as an alias for older callers; prefers multi-path ProbeWebSearch.
func ProbeWebSearchResponses(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration) (ProbeWebSearchResult, error) {
	return ProbeWebSearch(ctx, baseURL, apiKey, model, timeout)
}

func probeResponsesWebSearch(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration) (ProbeWebSearchResult, error) {
	out := ProbeWebSearchResult{Model: model, Mode: infoGatherModeResponses}
	endpoint := baseURL + "/responses"
	out.Endpoint = endpoint

	payload := map[string]any{
		"model": model,
		"instructions": "You are a connectivity probe for web_search. Reply in one short sentence confirming whether web search worked.",
		"input":        "Using the web_search tool, what organization operates example.com? Reply in under 20 words.",
		"tools":        []map[string]any{{"type": "web_search"}},
		"temperature":  0,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return out, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	out.LatencyMs = time.Since(started).Milliseconds()
	if err != nil {
		return out, fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()
	out.StatusCode = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, err
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return out, fmt.Errorf("POST /responses not available (HTTP %d)", resp.StatusCode)
	}

	var parsed responsesAPIResponse
	_ = json.Unmarshal(body, &parsed)
	if errMsg := extractAIErrorMessage(parsed.Error); errMsg != "" {
		hint := ""
		lower := strings.ToLower(errMsg)
		if strings.Contains(lower, "web_search") || strings.Contains(lower, "tool") {
			hint = " This model/key may not enable the web_search tool."
		}
		return out, fmt.Errorf("AI error (HTTP %d) %s: %s%s", resp.StatusCode, endpoint, errMsg, hint)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("AI request failed (HTTP %d) %s: %s", resp.StatusCode, endpoint, truncateForErr(string(body), 300))
	}
	return out, nil
}

func probeGLMChatWebSearch(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration) (ProbeWebSearchResult, error) {
	out := ProbeWebSearchResult{Model: model, Mode: infoGatherModeGLMChatTool}
	endpoint := baseURL + "/chat/completions"
	out.Endpoint = endpoint

	payload := glmChatWebSearchRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "user", Content: "Reply with the single word PONG."},
		},
		Tools: []map[string]interface{}{
			{
				"type": "web_search",
				"web_search": map[string]interface{}{
					"enable":        true,
					"search_result": true,
					"search_engine": "search_std",
					"search_intent": false,
					"count":         1,
					"content_size":  "medium",
				},
			},
		},
		Temperature: 0,
		MaxTokens:   16,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return out, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	out.LatencyMs = time.Since(started).Milliseconds()
	if err != nil {
		return out, fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()
	out.StatusCode = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, err
	}

	var parsed glmChatWebSearchResponse
	_ = json.Unmarshal(body, &parsed)
	if errMsg := extractAIErrorMessage(parsed.Error); errMsg != "" {
		lower := strings.ToLower(errMsg)
		hint := ""
		if strings.Contains(lower, "web_search") || strings.Contains(lower, "tool") || strings.Contains(lower, "enable") {
			hint = " Ensure tools web_search.enable=true is allowed for this key/model."
		}
		return out, fmt.Errorf("AI error (HTTP %d) %s: %s%s", resp.StatusCode, endpoint, errMsg, hint)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, fmt.Errorf("AI request failed (HTTP %d) %s: %s", resp.StatusCode, endpoint, truncateForErr(string(body), 300))
	}
	return out, nil
}

func probeGLMStandaloneWebSearch(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration) (ProbeWebSearchResult, error) {
	out := ProbeWebSearchResult{Model: model, Mode: infoGatherModeGLMSearch}
	endpoint := baseURL + "/web_search"
	out.Endpoint = endpoint

	payload := map[string]any{
		"search_query":         "example.com",
		"search_engine":        "search_std",
		"search_intent":        false,
		"count":                1,
		"content_size":         "medium",
		"search_recency_filter": "noLimit",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return out, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return out, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	started := time.Now()
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	out.LatencyMs = time.Since(started).Milliseconds()
	if err != nil {
		return out, fmt.Errorf("request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()
	out.StatusCode = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return out, err
	}

	if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
		return out, fmt.Errorf("POST /web_search not available (HTTP %d). Info Gathering needs Responses web_search or Zhipu /web_search", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var parsed struct {
			Error json.RawMessage `json:"error"`
			Msg   string          `json:"msg"`
			Message string        `json:"message"`
		}
		_ = json.Unmarshal(body, &parsed)
		if errMsg := extractAIErrorMessage(parsed.Error); errMsg != "" {
			return out, fmt.Errorf("AI error (HTTP %d) %s: %s", resp.StatusCode, endpoint, errMsg)
		}
		if msg := strings.TrimSpace(parsed.Msg); msg != "" {
			return out, fmt.Errorf("AI error (HTTP %d) %s: %s", resp.StatusCode, endpoint, msg)
		}
		if msg := strings.TrimSpace(parsed.Message); msg != "" {
			return out, fmt.Errorf("AI error (HTTP %d) %s: %s", resp.StatusCode, endpoint, msg)
		}
		return out, fmt.Errorf("AI request failed (HTTP %d) %s: %s", resp.StatusCode, endpoint, truncateForErr(string(body), 300))
	}
	return out, nil
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
	b.WriteString("Collect ONLY publicly exposed emails and phone numbers with sources. Do not include urls/names/other. Return JSON only.")
	return b.String()
}

func reportInfoGatherProgress(in InfoGatherInput, stage string, percent int, message string) {
	if in.OnProgress == nil {
		return
	}
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	in.OnProgress(stage, percent, message)
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
		return out, fmt.Errorf("no valid email/phone findings with source_url")
	}
	return out, nil
}

// NormalizeInfoGatherFinding validates and cleans one finding.
// Only email and phone are accepted.
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
	default:
		// Drop url/name/other — info gathering only keeps email and phone.
		return InfoGatherFindingDTO{}, false
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
