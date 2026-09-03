package handlers

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fishing-platform-backend/config"
	"fishing-platform-backend/models"
	"github.com/gin-gonic/gin"
)

// CreateRobot creates a new robot
func CreateRobot(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	var robot models.Robot
	if err := c.ShouldBindJSON(&robot); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if err := validateRobot(robot); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	robot.CreatedBy = user.ID

	db := config.GetDB()
	if err := db.Create(&robot).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create robot"})
		return
	}

	c.JSON(http.StatusCreated, robot)
}

// GetRobots retrieves all robots
func GetRobots(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	db := config.GetDB()
	var robots []models.Robot

	if err := scopeByOwner(db, user).Preload("Projects").Find(&robots).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve robots"})
		return
	}

	c.JSON(http.StatusOK, robots)
}

// GetRobot retrieves a single robot by ID
func GetRobot(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()
	var robot models.Robot

	if err := scopeByOwner(db, user).Preload("Projects").First(&robot, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}

	c.JSON(http.StatusOK, robot)
}

// UpdateRobot updates an existing robot
func UpdateRobot(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()

	var robot models.Robot
	if err := scopeByOwner(db, user).First(&robot, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}
	ownerID := robot.CreatedBy

	if err := c.ShouldBindJSON(&robot); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}
	if err := validateRobot(robot); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	robot.CreatedBy = ownerID

	if err := db.Save(&robot).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update robot"})
		return
	}

	c.JSON(http.StatusOK, robot)
}

// DeleteRobot deletes a robot
func DeleteRobot(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()

	var robot models.Robot
	if err := scopeByOwner(db, user).First(&robot, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}

	if err := db.Delete(&robot).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete robot"})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// TestRobot tests a robot's webhook
func TestRobot(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()

	var robot models.Robot
	if err := scopeByOwner(db, user).First(&robot, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}

	var req struct {
		Message string `json:"message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	result := sendRobotTestMessage(robot, req.Message)
	logEntry := saveRobotPushLog(robot, "test", result)

	statusMessage := "Push channel test sent successfully"
	if result.Status == models.RobotPushStatusFailed {
		statusMessage = "Push channel test failed"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  result.Status,
		"message": statusMessage,
		"log":     logEntry,
	})
}

// GetRobotPushLogs retrieves push logs for a robot.
func GetRobotPushLogs(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	limit := parsePushLogLimit(c.Query("limit"))

	db := config.GetDB()
	var robot models.Robot
	if err := scopeByOwner(db, user).First(&robot, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}
	var logs []models.RobotPushLog
	if err := db.Where("robot_id = ?", robot.ID).Order("created_at DESC").Limit(limit).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve push logs"})
		return
	}

	c.JSON(http.StatusOK, logs)
}

// CreateRobotPushLog records push logs sent by generated Flask runtimes.
func CreateRobotPushLog(c *gin.Context) {
	var req struct {
		RobotID        uint   `json:"robot_id" binding:"required"`
		Trigger        string `json:"trigger"`
		Status         string `json:"status" binding:"required"`
		Message        string `json:"message"`
		RequestPayload string `json:"request_payload"`
		ResponseStatus int    `json:"response_status"`
		ResponseBody   string `json:"response_body"`
		ErrorMessage   string `json:"error_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request data"})
		return
	}

	db := config.GetDB()
	var robot models.Robot
	if err := db.First(&robot, req.RobotID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}

	status := models.RobotPushStatusFailed
	if req.Status == string(models.RobotPushStatusSuccess) {
		status = models.RobotPushStatusSuccess
	}
	trigger := strings.TrimSpace(req.Trigger)
	if trigger == "" {
		trigger = "runtime"
	}

	logEntry := models.RobotPushLog{
		RobotID:        robot.ID,
		RobotName:      robot.Name,
		RobotType:      robot.RobotType,
		Trigger:        trigger,
		Status:         status,
		Message:        limitString(req.Message, 2000),
		RequestPayload: limitString(req.RequestPayload, 4000),
		ResponseStatus: req.ResponseStatus,
		ResponseBody:   limitString(req.ResponseBody, 4000),
		ErrorMessage:   limitString(req.ErrorMessage, 2000),
	}

	if err := db.Create(&logEntry).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create push log"})
		return
	}

	c.JSON(http.StatusCreated, logEntry)
}

// GetRobotMessages retrieves messages for a robot
func GetRobotMessages(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()

	var robot models.Robot
	if err := scopeByOwner(db, user).Preload("Projects.Messages").First(&robot, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}

	// Collect all messages from all projects
	var messages []models.Message
	for _, project := range robot.Projects {
		messages = append(messages, project.Messages...)
	}

	c.JSON(http.StatusOK, messages)
}

// StartRobot sets robot status to online
func StartRobot(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()

	var robot models.Robot
	if err := scopeByOwner(db, user).First(&robot, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}

	robot.Status = "online"
	if err := db.Save(&robot).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to start robot"})
		return
	}

	c.JSON(http.StatusOK, robot)
}

// StopRobot sets robot status to offline
func StopRobot(c *gin.Context) {
	user, ok := currentUserOrAbort(c)
	if !ok {
		return
	}
	id := c.Param("id")
	db := config.GetDB()

	var robot models.Robot
	if err := scopeByOwner(db, user).First(&robot, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Robot not found"})
		return
	}

	robot.Status = "offline"
	if err := db.Save(&robot).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to stop robot"})
		return
	}

	c.JSON(http.StatusOK, robot)
}

type robotPushResult struct {
	Status         models.RobotPushStatus
	Message        string
	RequestPayload string
	ResponseStatus int
	ResponseBody   string
	ErrorMessage   string
}

func sendRobotTestMessage(robot models.Robot, customMessage string) robotPushResult {
	message := strings.TrimSpace(customMessage)
	if message == "" {
		message = "Fishing Platform push channel test"
	}

	payload, err := buildRobotTestPayload(robot, message)
	if err != nil {
		return robotPushResult{
			Status:       models.RobotPushStatusFailed,
			Message:      message,
			ErrorMessage: err.Error(),
		}
	}

	webhookURL := cleanRobotWebhook(robot.Webhook)
	if (robot.RobotType == models.RobotTypeWecom || robot.RobotType == models.RobotTypeDingTalk) && strings.TrimSpace(robot.Secret) != "" {
		webhookURL, err = signedWecomWebhookURL(webhookURL, robot.Secret)
		if err != nil {
			return robotPushResult{
				Status:         models.RobotPushStatusFailed,
				Message:        message,
				RequestPayload: stringifyPayload(payload),
				ErrorMessage:   err.Error(),
			}
		}
	}

	return postRobotWebhook(webhookURL, payload, message)
}

func buildRobotTestPayload(robot models.Robot, message string) (map[string]interface{}, error) {
	now := time.Now().Format("2006-01-02 15:04:05")
	switch robot.RobotType {
	case models.RobotTypeWecom:
		return map[string]interface{}{
			"msgtype": "markdown_v2",
			"markdown_v2": map[string]interface{}{
				"content": fmt.Sprintf("## Fishing Platform push channel test\n```message\n%s\n```\n---\n%s", message, now),
			},
		}, nil
	case models.RobotTypeFeishu:
		return map[string]interface{}{
			"msg_type": "interactive",
			"card": map[string]interface{}{
				"config": map[string]interface{}{
					"wide_screen_mode": true,
				},
				"header": map[string]interface{}{
					"template": "blue",
					"title": map[string]interface{}{
						"tag":     "plain_text",
						"content": "Fishing Platform push channel test",
					},
				},
				"elements": []map[string]interface{}{
					{
						"tag": "div",
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**Message**\n%s", message),
						},
					},
					{
						"tag": "div",
						"text": map[string]interface{}{
							"tag":     "lark_md",
							"content": fmt.Sprintf("**Time**\n%s", now),
						},
					},
				},
			},
		}, nil
	case models.RobotTypeTelegram:
		chatID := strings.TrimSpace(robot.Secret)
		if chatID == "" {
			return nil, errors.New("telegram chat ID is required in the secret field")
		}
		return map[string]interface{}{
			"chat_id":                  chatID,
			"text":                     fmt.Sprintf("Fishing Platform push channel test\n\n%s\n\n%s", message, now),
			"disable_web_page_preview": true,
		}, nil
	case models.RobotTypeSlack:
		return map[string]interface{}{
			"text": fmt.Sprintf("Fishing Platform push channel test\n%s\n%s", message, now),
			"blocks": []map[string]interface{}{
				{"type": "header", "text": map[string]interface{}{"type": "plain_text", "text": "Fishing Platform push channel test"}},
				{"type": "section", "text": map[string]interface{}{"type": "mrkdwn", "text": fmt.Sprintf("*Message*\n%s", message)}},
				{"type": "context", "elements": []map[string]interface{}{{"type": "mrkdwn", "text": now}}},
			},
		}, nil
	case models.RobotTypeDingTalk:
		return map[string]interface{}{
			"msgtype": "markdown",
			"markdown": map[string]interface{}{
				"title": "Fishing Platform push channel test",
				"text":  fmt.Sprintf("### Fishing Platform push channel test\n\n%s\n\n---\n\n%s", message, now),
			},
		}, nil
	case models.RobotTypeDiscord:
		return map[string]interface{}{
			"content": "Fishing Platform push channel test",
			"embeds": []map[string]interface{}{
				{
					"title":       "Fishing Platform push channel test",
					"description": message,
					"color":       3447003,
					"footer":      map[string]interface{}{"text": now},
				},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported robot type: %s", robot.RobotType)
	}
}

func postRobotWebhook(webhookURL string, payload map[string]interface{}, message string) robotPushResult {
	requestPayload := stringifyPayload(payload)
	body, err := json.Marshal(payload)
	if err != nil {
		return robotPushResult{
			Status:         models.RobotPushStatusFailed,
			Message:        message,
			RequestPayload: requestPayload,
			ErrorMessage:   "failed to encode payload: " + err.Error(),
		}
	}

	req, err := http.NewRequest(http.MethodPost, webhookURL, bytes.NewReader(body))
	if err != nil {
		return robotPushResult{
			Status:         models.RobotPushStatusFailed,
			Message:        message,
			RequestPayload: requestPayload,
			ErrorMessage:   "failed to create request: " + err.Error(),
		}
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return robotPushResult{
			Status:         models.RobotPushStatusFailed,
			Message:        message,
			RequestPayload: requestPayload,
			ErrorMessage:   "webhook request failed: " + err.Error(),
		}
	}
	defer resp.Body.Close()

	responseBodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	responseBody := string(responseBodyBytes)
	if ok, errorMessage := webhookResponseSucceeded(resp.StatusCode, responseBodyBytes); !ok {
		return robotPushResult{
			Status:         models.RobotPushStatusFailed,
			Message:        message,
			RequestPayload: requestPayload,
			ResponseStatus: resp.StatusCode,
			ResponseBody:   limitString(responseBody, 4000),
			ErrorMessage:   errorMessage,
		}
	}

	return robotPushResult{
		Status:         models.RobotPushStatusSuccess,
		Message:        message,
		RequestPayload: requestPayload,
		ResponseStatus: resp.StatusCode,
		ResponseBody:   limitString(responseBody, 4000),
	}
}

func webhookResponseSucceeded(statusCode int, body []byte) (bool, string) {
	if statusCode < 200 || statusCode >= 300 {
		return false, fmt.Sprintf("webhook returned HTTP %d", statusCode)
	}

	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil || len(data) == 0 {
		return true, ""
	}
	if raw, ok := data["ok"]; ok {
		if success, boolean := raw.(bool); boolean && !success {
			return false, platformErrorMessage(data, "ok", raw)
		}
	}

	for _, key := range []string{"errcode", "code", "StatusCode"} {
		if raw, ok := data[key]; ok && !isZeroWebhookCode(raw) {
			return false, platformErrorMessage(data, key, raw)
		}
	}

	return true, ""
}

func validateRobot(robot models.Robot) error {
	switch robot.RobotType {
	case models.RobotTypeFeishu, models.RobotTypeWecom, models.RobotTypeSlack, models.RobotTypeDingTalk, models.RobotTypeDiscord:
	case models.RobotTypeTelegram:
		if strings.TrimSpace(robot.Secret) == "" {
			return errors.New("telegram chat ID is required")
		}
	default:
		return fmt.Errorf("unsupported robot type: %s", robot.RobotType)
	}
	if strings.TrimSpace(cleanRobotWebhook(robot.Webhook)) == "" {
		return errors.New("webhook URL is required")
	}
	return nil
}

func isZeroWebhookCode(value interface{}) bool {
	switch typed := value.(type) {
	case float64:
		return typed == 0
	case int:
		return typed == 0
	case string:
		typed = strings.TrimSpace(typed)
		return typed == "" || typed == "0" || strings.EqualFold(typed, "ok")
	default:
		return value == nil
	}
}

func platformErrorMessage(data map[string]interface{}, codeKey string, code interface{}) string {
	for _, key := range []string{"errmsg", "msg", "message", "StatusMessage"} {
		if raw, ok := data[key]; ok && strings.TrimSpace(fmt.Sprint(raw)) != "" {
			return fmt.Sprintf("platform returned %s=%v: %v", codeKey, code, raw)
		}
	}
	return fmt.Sprintf("platform returned %s=%v", codeKey, code)
}

func signedWecomWebhookURL(webhook string, secret string) (string, error) {
	parsed, err := url.Parse(webhook)
	if err != nil {
		return "", fmt.Errorf("invalid webhook URL: %w", err)
	}

	timestamp := strconv.FormatInt(time.Now().UnixMilli(), 10)
	stringToSign := timestamp + "\n" + secret
	hmacCode := hmac.New(sha256.New, []byte(secret))
	if _, err := hmacCode.Write([]byte(stringToSign)); err != nil {
		return "", fmt.Errorf("failed to sign webhook request: %w", err)
	}
	sign := base64.StdEncoding.EncodeToString(hmacCode.Sum(nil))

	query := parsed.Query()
	query.Set("timestamp", timestamp)
	query.Set("sign", sign)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func saveRobotPushLog(robot models.Robot, trigger string, result robotPushResult) models.RobotPushLog {
	logEntry := models.RobotPushLog{
		RobotID:        robot.ID,
		RobotName:      robot.Name,
		RobotType:      robot.RobotType,
		Trigger:        trigger,
		Status:         result.Status,
		Message:        limitString(result.Message, 2000),
		RequestPayload: limitString(result.RequestPayload, 4000),
		ResponseStatus: result.ResponseStatus,
		ResponseBody:   limitString(result.ResponseBody, 4000),
		ErrorMessage:   limitString(result.ErrorMessage, 2000),
	}

	if err := config.GetDB().Create(&logEntry).Error; err != nil {
		logEntry.ErrorMessage = strings.TrimSpace(logEntry.ErrorMessage + "; failed to save push log: " + err.Error())
	}
	return logEntry
}

func parsePushLogLimit(value string) int {
	limit, err := strconv.Atoi(value)
	if err != nil || limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func stringifyPayload(payload map[string]interface{}) string {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(encoded)
}

func cleanRobotWebhook(value string) string {
	return strings.Trim(strings.TrimSpace(value), `'"`)
}

func limitString(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	return value[:max]
}
