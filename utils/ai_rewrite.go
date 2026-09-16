package utils

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const DefaultMirrorSubmitURL = "/api/submit"

var (
	base64DataURIRe = regexp.MustCompile(`(?i)(src|href)=["'](data:image\/[a-z0-9.+-]+;base64,)[A-Za-z0-9+/=\s]{512,}["']`)
	htmlFenceRe     = regexp.MustCompile("(?s)```(?:html)?\\s*(.*?)```")
)

// MirrorRewriteInput is passed into the AI rewrite prompt.
type MirrorRewriteInput struct {
	RawHTML      string
	SubmitURL    string
	RedirectURL  string
	OriginalHost string
}

// MirrorSystemPrompt is the locked system instruction for login-page rewrite.
const MirrorSystemPrompt = `你是钓鱼演练平台的 HTML 改写助手。
你的唯一任务：在尽量保持页面外观与文案不变的前提下，
把登录相关提交改写为发送到指定的我方接收地址，并统一使用平台接收字段。

硬性规则：
1. 保留原有布局、样式、文案、图片引用，不要重写整页。
2. 将所有登录相关 <form> 的 action 改为 SUBMIT_URL；method 使用 POST（若原本是 GET 登录也改为 POST）。
3. 平台接收字段（必须遵守）：
   - username：账号/用户名/手机号/邮箱等登录标识，统一使用 name="username"
   - password：密码，统一使用 name="password"
   - captchavalue：验证码（若页面有验证码输入框），统一使用 name="captchavalue"
4. 字段改写：
   - 把原站账号类输入框的 name 改为 username（无论原来叫 user/email/account/loginName 等）
   - 把原站密码输入框的 name 改为 password
   - 若存在验证码输入框，name 改为 captchavalue；没有验证码则不要凭空新增
   - 如果原站用 JS 组装 JSON 提交，JSON 字段名也必须改成 username/password/captchavalue
5. 能明确识别为登录提交的 fetch/axios/XMLHttpRequest，改为请求 SUBMIT_URL。
6. 删除或改写会把凭据提交到原站域名的登录逻辑；不要保留可用的原站登录 API 调用。
7. 提交成功后的跳转改为 REDIRECT_URL（若为空则停留当前页并显示简单成功提示）。
8. 不要把 project_id、ip_address 做成用户可见输入框；这些由平台侧处理。
9. 不要加入明显的“这是钓鱼页”文案；不要添加与任务无关的脚本。
10. 只输出完整 HTML 文档，不要 Markdown，不要代码围栏，不要解释。`

// BuildMirrorUserPrompt builds the user message for rewrite.
func BuildMirrorUserPrompt(in MirrorRewriteInput) string {
	submit := strings.TrimSpace(in.SubmitURL)
	if submit == "" {
		submit = DefaultMirrorSubmitURL
	}
	var b strings.Builder
	b.WriteString("SUBMIT_URL: ")
	b.WriteString(submit)
	b.WriteString("\nREDIRECT_URL: ")
	b.WriteString(strings.TrimSpace(in.RedirectURL))
	b.WriteString("\nORIGINAL_HOST: ")
	b.WriteString(strings.TrimSpace(in.OriginalHost))
	b.WriteString("\n\n平台字段：\n- username\n- password\n- captchavalue（可选）\n\n")
	b.WriteString("请将页面登录相关输入字段统一改成上述 name，并保证提交到 SUBMIT_URL。\n")
	b.WriteString("下面是抓取到的原始 HTML。请按系统规则改写后返回完整 HTML：\n\n")
	b.WriteString(in.RawHTML)
	return b.String()
}

// StripHeavyBase64DataURIs replaces oversized inline images with placeholders to reduce tokens.
func StripHeavyBase64DataURIs(html string) string {
	return base64DataURIRe.ReplaceAllString(html, `$1="$2[STRIPPED_FOR_AI]"`)
}

// FetchURLHTML downloads a page with a browser-like UA and returns HTML + final host.
func FetchURLHTML(ctx context.Context, rawURL string, timeout time.Duration) (html string, finalURL *url.URL, err error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", nil, fmt.Errorf("url is required")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", nil, fmt.Errorf("invalid url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", nil, fmt.Errorf("only http/https urls are supported")
	}
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FishingPlatformMirror/1.0)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8MB cap
	if err != nil {
		return "", nil, err
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if ct != "" && !strings.Contains(ct, "html") && !strings.Contains(ct, "text/plain") {
		return "", nil, fmt.Errorf("upstream content-type is not html: %s", ct)
	}
	final := resp.Request.URL
	return string(body), final, nil
}

type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	// Some providers return error as object; others as a plain string.
	Error json.RawMessage `json:"error"`
}

// NormalizeOpenAICompatibleBaseURL strips trailing slash and common endpoint
// suffixes (/chat/completions, /completions, /responses) so callers can append
// the path they need. Accepts both API roots (…/v1, …/v4) and full endpoint URLs.
func NormalizeOpenAICompatibleBaseURL(baseURL string) string {
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	for u != "" {
		lower := strings.ToLower(u)
		var stripped bool
		for _, suffix := range []string{"/chat/completions", "/completions", "/responses", "/web_search"} {
			if strings.HasSuffix(lower, suffix) {
				u = strings.TrimRight(u[:len(u)-len(suffix)], "/")
				stripped = true
				break
			}
		}
		if !stripped {
			break
		}
	}
	return u
}

// ProbeChatCompletionsResult is returned by ProbeChatCompletions.
type ProbeChatCompletionsResult struct {
	Endpoint   string `json:"endpoint"`
	LatencyMs  int64  `json:"latency_ms"`
	Model      string `json:"model"`
	StatusCode int    `json:"status_code"`
}

// ProbeChatCompletions sends a minimal chat/completions request to verify
// base_url + api_key + model connectivity for AI Settings.
func ProbeChatCompletions(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration) (ProbeChatCompletionsResult, error) {
	out := ProbeChatCompletionsResult{Model: strings.TrimSpace(model)}
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
		timeout = 30 * time.Second
	}
	endpoint := baseURL + "/chat/completions"
	out.Endpoint = endpoint

	payload := map[string]interface{}{
		"model": model,
		"messages": []chatMessage{
			{Role: "user", Content: "ping"},
		},
		"max_tokens":  8,
		"temperature": 0,
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

	var parsed chatCompletionResponse
	_ = json.Unmarshal(body, &parsed)
	if errMsg := extractAIErrorMessage(parsed.Error); errMsg != "" {
		return out, formatAIHTTPError(resp.StatusCode, endpoint, errMsg)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return out, formatAIHTTPError(resp.StatusCode, endpoint, truncateForErr(string(body), 300))
	}
	return out, nil
}

// RewriteHTMLWithAI calls an OpenAI-compatible chat completions endpoint.
func RewriteHTMLWithAI(ctx context.Context, baseURL, apiKey, model string, timeout time.Duration, in MirrorRewriteInput) (string, error) {
	if strings.TrimSpace(apiKey) == "" {
		return "", fmt.Errorf("AI API key is not configured")
	}
	if strings.TrimSpace(model) == "" {
		model = defaultAIModelFallback()
	}
	baseURL = NormalizeOpenAICompatibleBaseURL(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("AI base URL is not configured")
	}
	if timeout <= 0 {
		timeout = 120 * time.Second
	}

	endpoint := baseURL + "/chat/completions"
	payload := chatCompletionRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: MirrorSystemPrompt},
			{Role: "user", Content: BuildMirrorUserPrompt(in)},
		},
		Temperature: 0.1,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI request to %s failed: %w", endpoint, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return "", err
	}
	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return "", fmt.Errorf("AI request failed (HTTP %d) %s: %s", resp.StatusCode, endpoint, truncateForErr(string(body), 300))
		}
		return "", fmt.Errorf("invalid AI response from %s: %w", endpoint, err)
	}
	if errMsg := extractAIErrorMessage(parsed.Error); errMsg != "" {
		return "", formatAIHTTPError(resp.StatusCode, endpoint, errMsg)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", formatAIHTTPError(resp.StatusCode, endpoint, truncateForErr(string(body), 300))
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return "", fmt.Errorf("AI returned empty content from %s", endpoint)
	}
	return normalizeAIHTMLOutput(parsed.Choices[0].Message.Content), nil
}

func formatAIHTTPError(status int, endpoint, detail string) error {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		detail = "unknown error"
	}
	hint := ""
	if status == http.StatusNotFound {
		hint = " Hint: set base_url to the API root (e.g. https://open.bigmodel.cn/api/paas/v4 or https://api.x.ai/v1), not the full /chat/completions path."
	}
	if status > 0 {
		return fmt.Errorf("AI error (HTTP %d) %s: %s%s", status, endpoint, detail, hint)
	}
	return fmt.Errorf("AI error %s: %s%s", endpoint, detail, hint)
}

func extractAIErrorMessage(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return strings.TrimSpace(asString)
	}
	var asObj struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	}
	if err := json.Unmarshal(raw, &asObj); err == nil {
		if msg := strings.TrimSpace(asObj.Message); msg != "" {
			return msg
		}
		if code := strings.TrimSpace(asObj.Code); code != "" {
			return code
		}
		if typ := strings.TrimSpace(asObj.Type); typ != "" {
			return typ
		}
	}
	return truncateForErr(string(raw), 200)
}

// ValidateRewrittenHTML performs lightweight acceptance checks.
func ValidateRewrittenHTML(html, submitURL string) error {
	htmlLower := strings.ToLower(html)
	submit := strings.TrimSpace(submitURL)
	if submit == "" {
		submit = DefaultMirrorSubmitURL
	}
	if !strings.Contains(html, submit) && !strings.Contains(htmlLower, strings.ToLower(submit)) {
		return fmt.Errorf("rewritten HTML missing SUBMIT_URL")
	}
	if !strings.Contains(htmlLower, `name="username"`) && !strings.Contains(htmlLower, `name='username'`) {
		return fmt.Errorf("rewritten HTML missing username field")
	}
	if !strings.Contains(htmlLower, `name="password"`) && !strings.Contains(htmlLower, `name='password'`) {
		return fmt.Errorf("rewritten HTML missing password field")
	}
	return nil
}

func normalizeAIHTMLOutput(content string) string {
	content = strings.TrimSpace(content)
	if m := htmlFenceRe.FindStringSubmatch(content); len(m) == 2 {
		content = strings.TrimSpace(m[1])
	}
	content = strings.TrimPrefix(content, "```html")
	content = strings.TrimPrefix(content, "```HTML")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(content, "```")
	return strings.TrimSpace(content)
}

func truncateForErr(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func defaultAIModelFallback() string {
	return "grok-4.5"
}
