package handlers

import (
	"testing"

	"fishing-platform-backend/models"
)

func TestBuildRobotTestPayloadTelegram(t *testing.T) {
	payload, err := buildRobotTestPayload(models.Robot{RobotType: models.RobotTypeTelegram, Secret: "-100123"}, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if payload["chat_id"] != "-100123" || payload["text"] == "" {
		t.Fatalf("unexpected Telegram payload: %#v", payload)
	}
}

func TestBuildRobotTestPayloadSlack(t *testing.T) {
	payload, err := buildRobotTestPayload(models.Robot{RobotType: models.RobotTypeSlack}, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if payload["text"] == "" || payload["blocks"] == nil {
		t.Fatalf("unexpected Slack payload: %#v", payload)
	}
}

func TestTelegramErrorResponseFails(t *testing.T) {
	ok, message := webhookResponseSucceeded(200, []byte(`{"ok":false,"description":"Bad Request: chat not found"}`))
	if ok || message == "" {
		t.Fatalf("expected Telegram error response to fail, got ok=%v message=%q", ok, message)
	}
}

func TestBuildRobotTestPayloadDingTalk(t *testing.T) {
	payload, err := buildRobotTestPayload(models.Robot{RobotType: models.RobotTypeDingTalk}, "hello")
	if err != nil || payload["msgtype"] != "markdown" {
		t.Fatalf("unexpected DingTalk payload: %#v, err=%v", payload, err)
	}
}

func TestBuildRobotTestPayloadDiscord(t *testing.T) {
	payload, err := buildRobotTestPayload(models.Robot{RobotType: models.RobotTypeDiscord}, "hello")
	if err != nil || payload["content"] == "" || payload["embeds"] == nil {
		t.Fatalf("unexpected Discord payload: %#v, err=%v", payload, err)
	}
}
