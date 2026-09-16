<p align="right">
  <a href="README.md">English</a> · <b>中文</b>
</p>

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="README.assets/logo-dark.svg" />
    <img src="README.assets/logo.svg" alt="钓鱼台" width="96" height="96" />
  </picture>
</p>

<h1 align="center">钓鱼台</h1>

<p align="center">
  <strong>开源钓鱼套件，红队自己部署自己用</strong>
</p>

<p align="center">
  Go + Gin · Next.js · SQLite · 一个二进制 · 可选远程 Agent
</p>

<p align="center">
  <a href="#这是什么">这是什么</a> ·
  <a href="#能干什么">能干什么</a> ·
  <a href="#架构">架构</a> ·
  <a href="#跑起来">跑起来</a> ·
  <a href="#agents">Agent</a> ·
  <a href="#控制台">控制台</a> ·
  <a href="#文档">文档</a>
</p>

---

## 这是什么

**钓鱼台**是给红队用的自托管钓鱼套件。落地页、发信、看打开/点击，有人交了账密就推你一声。页面可以跑在本机，也可以丢到 Agent 上。

用过 Gophish 的话：那个偏活动/模板。这个是作业台——页面、Agent、实时 QR、AI 搜集、Webhook、黑名单。

一个 Go 二进制把前端（`frontend/dist`）打进去，API 和后台同一端口。

<p align="center">
  <img src="README.assets/login.png" alt="登录页" width="880" />
</p>

> 只打你有授权的目标。没授权就别用。

---

## 能干什么

- **项目** — 上传 HTML（或在页面制作里扒页）。Docker 或本机 Flask。可选 TLS、Agent、QR 中继。
- **邮件** — 多套 SMTP，群发，每个活动单独的打开/点击追踪（`/api/<slug>`）。需要的话给 HTML 加点噪声。
- **上钩通知** — 飞书 / 企微 / Telegram / Slack / 钉钉 / Discord。通道绑到项目上，有人提交就推「鱼儿上钩」。打开、点击不走这条。
- **QR** — 稳定地址 `/q/{slug}.png` 出实时二维码。本机脚本只传屏幕帧，页面里放 `{{QR_RELAY_URL}}` / `{{QR_RELAY_IMG}}`。
- **信息搜集** — 用你自己的 AI 接口（`/responses` + `web_search`）捞公开邮箱/电话，导出 CSV，接着去发信。
- **Agent** — 另一台机器上的小程序。一个项目可以铺多个点，一台机器也能挂多个项目。日志和提交回主控。
- **杂项** — 仪表盘和主机曲线、密码落盘加密、QQWry 定位、CSV、IP 黑名单（有人批量提交就拉黑，防蓝队反制）、JWT + 可选 Basic Auth、管理员/运营、只追加的审计、中英界面。

常见走法：SMTP + Webhook → 做页 → 建项目（绑通道）→ 发信 → 等上钩 → 看记录。有人开始刷提交，就把 IP 拉黑。

---

## 架构

```text
  浏览器（中/英，JWT）
           │
           ▼
  主控  (Go / Gin，SQLite，内嵌 UI)
           │
     ┌─────┴──────┐
     ▼            ▼
  Docker /     远程 Agent
  本机 Flask
     │            │
     └─────┬──────┘
           ▼
  提交 · 日志 · 邮件打开/点击 · 实时 QR
```

Go 1.24、Gin、GORM/SQLite、Next.js 静态导出，Docker SDK 可选。

---

## 跑起来

Linux / macOS / Windows。

- **直接跑发行包：** 二进制 + `config.yaml` 就行。
- **从源码编：** Go **1.24+**、Node.js（前端）。
- **Docker 运行模式：** 要有 Docker Engine。只用本机 Flask / Agent 可以不装。
- **本机 Flask：** `python3`，带 Flask（`flask`、`flask_cors`、`requests`）。
- **QR 截屏：** Python 3，从 **工作台 → QR 钓鱼** 下 zip（或看 [`scripts/qr-relay`](scripts/qr-relay/README.md)）。macOS 解码需要 `zbar`。
- Agent 要能访问主控的 HTTP(S)。

| 用途 | 默认 |
|------|------|
| 控制台 / API | `:8000` |
| 前端开发 | `:8081`，把 `/api` 转到 `:8000` |
| Agent 上的落地页 | 该主机 `10000–20000/TCP` |

### 1. 配置

```bash
cp config.yaml.example config.yaml
```

真要用之前把占位密钥换掉：

```yaml
server:
  port: 8000
  gin_mode: release

database:
  path: ./data/fishing.db

security:
  encryption_key: replace-with-a-generated-key
  jwt_secret: replace-with-a-long-random-secret
  container_secret: replace-with-another-long-random-secret
  agent_registration_token: replace-with-agent-registration-token
  platform_basic_auth_enabled: true

container:
  ssl_cert_path: /app/certificates/cert.pem
  ssl_key_path: /app/certificates/key.pem
```

加密密钥可以用 `cmd/generate-encryption-key`。邮件追踪（公网地址、HMAC、跳转白名单）启动后在 **系统 → 邮件追踪** 里改。YAML 只是种子。打开/点击路径按每个活动生成，不是全局配的。

`config.yaml`、`data/`、私钥别提交（已 gitignore）。

### 2. 运行

```bash
./fishing-platform
# 或者 ./build-all.sh 之后用 ./release/ 里的包
```

启动嫌密钥是空的/默认的，实验室里点一下继续就行。真上场先换密钥。

### 3. 从源码编

```bash
./build-all.sh
```

先编前端，再出主控和 Agent（linux / windows / darwin，amd64 & arm64），并把两个 example 拷到 `release/`。

只跑界面（API 已经在）：

```bash
cd frontend && npm install && npm run dev   # http://127.0.0.1:8081
```

### 4. 登录

打开 `http://localhost:8000`（或你配的端口）。第一次启动的管理员密码在：

```bash
cat data/admin_password.txt
```

存好就删这个文件。以后重置用 `cmd/reset-password`。

---

## Agents

不想全堆在主控上，就把 **fishing-agent** 丢到别的机器。一个项目可以铺多个点，一台机器也能挂多个项目。每个绑定自己的端口、版本、日志。提交和日志回主控；主控闪一下，Agent 会先攒着再补传。

项目端口默认 **`10000–20000/TCP`**。Agent **主动连出**到主控。访问者打 Agent（反代后面就把对外地址写到 `advertise_host`，见完整手册）。

**令牌** — 主控 `security.agent_registration_token` 单独设（别拿 JWT 密钥凑），重启。第一次注册用这个共享令牌，之后 `{work_dir}/state.json` 里是这台 Agent 自己的 id/token（Linux/macOS 上 `0600`）。

**启动** — `./build-all.sh` 会把二进制和 `agent.yaml.example` 放到 `release/`：

```bash
cp agent.yaml.example agent.yaml
```

```yaml
server_url: http://control-plane.example.com:8000   # 不要末尾 /
registration_token: replace-with-a-long-random-agent-token
```

```bash
chmod +x ./fishing-agent
./fishing-agent -config ./agent.yaml
```

```powershell
.\fishing-agent-windows-amd64.exe -config .\agent.yaml
```

日志里看到 `agent … connected to http://…` 就对了。

**后台里** — **系统 → Agent** 等到在线（大约 90 秒没心跳算离线）。**项目 → 新建**，勾 Agent，上传 HTML。每个 Agent 是独立实例（`pending` → `deploying` → `running`）。日志和记录在项目上汇总。改项目会升版本；加 Agent 就部署；取消勾选会下发删除。

systemd、Windows 服务、反代：**[docs/agent-deployment.md](docs/agent-deployment.md)**。

---

## 控制台

| | |
|---|---|
| **仪表盘** | 计数、主机 CPU/内存 |
| **项目** | 概览、部署、日志、记录 |
| **审计日志** | 管理员，删不了 |
| **发送邮件** | 活动、打开/点击 |
| **页面制作** | 上传 / AI 镜像 |
| **信息搜集** | AI 搜索 → 联系人 |
| **QR 钓鱼** | 实时二维码中继 |
| **推送通道** | 有人提交就 Webhook |
| **Agent** | 在线 / 最后心跳 |
| **IP 黑名单** | 批量提交就封 IP，防蓝队反制灌数据 |
| **邮件服务** | SMTP |
| **用户 / AI / 邮件追踪** | 管理员 |

页面默认 POST 到 `/api/submit`。HTML：`{{SUBMIT_URL}}`、`{{REDIRECT_URL}}`、`{{QR_RELAY_URL}}`、`{{QR_RELAY_IMG}}`。邮件：`{{email}}`、`{{click_url}}`、`{{open_pixel}}`、`{{landing_url}}` …  
说明：[中文](docs/placeholders.zh.md) · [英文](docs/placeholders.en.md)

访问日志：`data/access-YYYY-MM-DD.log`。仪表盘高频轮询不刷控制台，文件里还在。

---

## 文档

| | |
|---|---|
| 占位符（页面 + 邮件） | [中文](docs/placeholders.zh.md) · [英文](docs/placeholders.en.md) |
| Agent（systemd、Windows、反代） | [docs/agent-deployment.md](docs/agent-deployment.md) |
| QR 截屏脚本 | [scripts/qr-relay](scripts/qr-relay/README.md) — 或从 QR 页下 zip |

QR：控制台建中继，复制上传 token（只显示一次），写进脚本配置。脚本只传图片帧和心跳。对外地址是 `/q/{slug}.png`。

---

## 安全

- `encryption_key`、`jwt_secret`、`container_secret`、`agent_registration_token` 要轮换。换 `encryption_key` 等于以前加密的东西全废（SMTP、机器人密钥、收到的密码、邮件追踪密钥）。
- 对外暴露就 `gin_mode: release` + Basic Auth。
- 收到的账密、收件人列表、Webhook、AI Key 当活弹药。
- 审计日志产品里删不掉。运营只能看自己的东西。

---

## 卡住了？

| | |
|---|---|
| 打不开界面 | 看 `server.port`。开了 Basic Auth 的话浏览器会弹两次（外围门禁 + 登录）。 |
| 没有管理员密码 | 第一次在 `data/admin_password.txt`。以后用 `cmd/reset-password`。 |
| Agent 一直离线 | 必须能 **连出** 到 `server_url`。大约 90 秒没心跳算离线。第一次注册的 token 要和主控 `agent_registration_token` 对上。 |
| 邮件打开/点击是空的 | **系统 → 邮件追踪**：打开开关，`public_base_url` 填受害者真能访问的地址。 |
| 本机 Flask 起不来 | PATH 里要有 `python3`，`pip install flask flask_cors requests`。 |
| Docker 模式被跳过 | Engine 连不上（启动检查默认 `127.0.0.1:2375`）。改用本机 Flask 或 Agent。 |
| QR 健康是 stale | 截屏脚本没跑，或 token/地址不对。 |

---

## 目录

```text
main.go / handlers / models / middleware / utils / config
agent/                 # 边缘二进制
frontend/              # Next.js → frontend/dist（内嵌）
cmd/                   # generate-encryption-key、reset-password
docs/agent-deployment.md
docs/placeholders.en.md
docs/placeholders.zh.md
config.yaml.example
agent.yaml.example
build-all.sh
README.md / README.zh.md
README.assets/
```

---

## 免责

给红队在**有授权**的情况下钓鱼用。没授权去打别人，后果自己扛，项目不支持这种用法。

代码使用 [MIT License](LICENSE)。

Issue、PR 随便提。坏了就开 GitHub Issue。
