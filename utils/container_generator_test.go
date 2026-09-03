package utils

import (
	"strings"
	"testing"
)

func TestGenerateRobotConfigsCleansAndQuotesWebhook(t *testing.T) {
	got := generateRobotConfigs([]RobotConfig{
		{
			ID:      7,
			Webhook: " 'https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=00000000-0000-4000-8000-000000000000' ",
			Type:    " wecom ",
			Secret:  " a'b ",
		},
	})

	want := `[{"id": 7, "webhook": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=00000000-0000-4000-8000-000000000000", "type": "wecom", "secret": "a'b"}]`
	if got != want {
		t.Fatalf("generateRobotConfigs() = %q, want %q", got, want)
	}
}

func TestGenerateContainerCodeRecordsPushLogs(t *testing.T) {
	code := GenerateContainerCode("test", 1, "/", "/api/submit", "https://example.com/login", "secret", []RobotConfig{
		{ID: 12, Webhook: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test", Type: "wecom"},
	})

	for _, want := range []string{
		`{"id": 12, "webhook": "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test", "type": "wecom", "secret": ""}`,
		"@app.route('/_fp_health', methods=['GET'])",
		"def record_push_log(",
		"/api/robot_push_logs/",
		"webhook_response_ok(response)",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("generated container code is missing %q", want)
		}
	}
}

func TestGenerateContainerCodeUsesRuntimeNewlinesInWecomMarkdown(t *testing.T) {
	code := GenerateContainerCode("test", 1, "/", "/api/submit", "https://example.com/login", "secret", []RobotConfig{
		{Webhook: "https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=test", Type: "wecom"},
	})

	if strings.Contains(code, "快抄网\\\\n```") {
		t.Fatalf("wecom markdown contains literal backslash-n text: %q", code)
	}

	want := "快抄网\\n```eg：用户名/密码\\n{username}/{password}\\n```\\n---\\n[点击登陆]({login_url})\\n\\n"
	if !strings.Contains(code, want) {
		t.Fatalf("wecom markdown newline escapes were not generated as expected")
	}
}

func TestGenerateContainerCodeSubmitsClientIP(t *testing.T) {
	code := GenerateContainerCode("test", 1, "/", "/api/submit", "https://example.com/login", "secret", nil)

	for _, want := range []string{
		"def get_client_ip():",
		"forwarded_for = request.headers.get('X-Forwarded-For', '')",
		"return request.remote_addr or ''",
		"'ip_address': client_ip",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("generated container code is missing %q", want)
		}
	}
}

func TestGenerateContainerCodeSupportsTelegramAndSlack(t *testing.T) {
	code := GenerateContainerCode("test", 1, "/", "/api/submit", "https://example.com/login", "secret", []RobotConfig{
		{ID: 1, Webhook: "https://api.telegram.org/botTOKEN/sendMessage", Type: "telegram", Secret: "-100123"},
		{ID: 2, Webhook: "https://hooks.slack.com/services/T/B/X", Type: "slack"},
	})

	for _, want := range []string{
		"def create_telegram_message(",
		"message['chat_id'] = robot['secret']",
		"robot['type'] == 'telegram'",
		"def create_slack_message(",
		"robot['type'] == 'slack'",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("generated container code is missing %q", want)
		}
	}
}

func TestGenerateContainerCodeSupportsDingTalkAndDiscord(t *testing.T) {
	code := GenerateContainerCode("test", 1, "/", "/api/submit", "https://example.com/login", "secret", []RobotConfig{
		{ID: 3, Webhook: "https://oapi.dingtalk.com/robot/send?access_token=x", Type: "dingtalk", Secret: "sign-secret"},
		{ID: 4, Webhook: "https://discord.com/api/webhooks/id/token", Type: "discord"},
	})
	for _, want := range []string{
		"def sign_dingtalk(", "def create_dingtalk_message(", "robot['type'] == 'dingtalk'",
		"def create_discord_message(", "robot['type'] == 'discord'",
	} {
		if !strings.Contains(code, want) {
			t.Fatalf("generated container code is missing %q", want)
		}
	}
}
