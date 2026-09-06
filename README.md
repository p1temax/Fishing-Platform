<p align="center">
  <img src="README.assets/logo.svg" alt="Fishing Platform" width="96" height="96" />
</p>

<h1 align="center">Fishing Platform</h1>

<p align="center">
  <strong>Authorized phishing simulation &amp; credential-harvesting operations console</strong>
</p>

<p align="center">
  Go + Gin control plane · Next.js operator UI · SQLite · single binary · distributed agents
</p>

<p align="center">
  <a href="#features">Features</a> ·
  <a href="#architecture">Architecture</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#operator-modules">Modules</a> ·
  <a href="#security--compliance">Security</a> ·
  <a href="#license--intended-use">License</a>
</p>

---

## Overview

**Fishing Platform** is a self-hosted console for **authorized** phishing exercises: build lure pages, deploy them on the control plane or remote agents, send tracked email campaigns, capture submitted credentials, and review engagement plus host telemetry—with RBAC and immutable audit logs.

It ships as **one Go binary** that embeds the production frontend (`frontend/dist`) and serves the API and UI on the same port.

> **Authorized use only.** Use this software only on systems and users you are explicitly permitted to assess (red team, purple team, security awareness). Unauthorized phishing or credential theft is illegal and unsupported.

---

## Features

### Implemented today

| Area | What it does |
|------|----------------|
| **Dashboard** | Project / agent / deployment / credential counters; host CPU · memory · disk snapshots every **30s**, retained **24h** |
| **Projects** | Create lure projects; run mode **Docker** or **Local Flask**; upload HTML with `{{SUBMIT_URL}}` / `{{REDIRECT_URL}}`; optional TLS certs; bind **push channels** and **agents** |
| **Deployments** | Build / start / stop / restart on the platform; per-agent isolated deployments with revision tracking |
| **Credential records** | Harvest username / password / captcha (and extras); passwords encrypted at rest; **IP → geo** via embedded QQWry; CSV export; linkable to mail campaigns |
| **Runtime logs** | Platform container/local logs plus centrally collected agent project logs |
| **Page Builder** | Library of HTML pages as cards (live thumbnails); **upload**; **mirror URL + AI rewrite** (requires an enabled AI profile); delete |
| **Info Gathering** | AI **web_search** via generic `POST {base_url}/responses` (user-supplied URL + API key); optional `x_search`; structured contacts with sources; export CSV; hand off to Send Email |
| **QR Phishing** | Live QR relay: local screen capture (ROI picker + heartbeat) uploads image frames only; stable `/q/{slug}.png`; project HTML placeholders `{{QR_RELAY_URL}}` / `{{QR_RELAY_IMG}}`; SSE live preview, frame history, health badges, rate limit, rotate slug/token |
| **Send Email** | Bulk campaigns via configured SMTP; HTML/plain; optional open tracking; click tracking with **per-campaign opaque** `/api/<slug>` paths; HTML noise injection; campaign detail (sent / open / click + event list: status, time, email, IP) |
| **Mail Tracking (System)** | Platform settings: enable tracking, `public_base_url`, HMAC secret, redirect host allow-list (paths are **auto-generated per campaign**, not hand-edited) |
| **Mail Services (SMTP)** | Multiple SMTP profiles (TLS/SSL), test send, encrypted passwords |
| **Push Channels** | Webhooks: Feishu, WeCom, Telegram, Slack, DingTalk, Discord; test push + push logs; bind to projects for credential / runtime alerts |
| **Agents** | Token registration + heartbeat; lease-based tasks; artifact download; credential submit & log upload from the edge |
| **IP Blacklist** | Entries created from captured credentials (not free-form create in UI); enable/disable; applies **host firewall** rules where supported |
| **Users (admin)** | Create operators; reset password; operators cannot access audit logs or admin-only settings |
| **Audit Logs (admin)** | Append-only operator action history (no delete) |
| **AI Settings (admin)** | User-supplied base URL + API key + model profiles; only one enabled; Page Builder uses `/chat/completions`, Info Gathering uses `/responses` + `web_search` |
| **Access control** | JWT sessions; optional **platform Basic Auth** gate; ownership scoping for operators |
| **i18n** | English (default) and Chinese in the UI |



---

## Architecture

```text
┌──────────────────────────────────────────┐
│           Operator browser               │
│     Next.js SPA (EN/ZH), JWT + RBAC      │
└────────────────────┬─────────────────────┘
                     │
┌────────────────────▼─────────────────────┐
│         Control plane (Go / Gin)         │
│  Embedded UI · SQLite · audit middleware │
│  Projects · Mail · SMTP · Agents · AI    │
└─────────┬────────────────────┬───────────┘
          │                    │
          ▼                    ▼
   Local Docker /         Remote Agents
   Local Flask            (multi-project hosts)
          │                    │
          └────────┬───────────┘
                   ▼
            Credential + log ingest
            Mail open/click (/api/:slug)
```

**Stack:** Go 1.24 · Gin · GORM/SQLite · Next.js (static export embedded) · Docker SDK (optional run mode)

---

## Quick start

### Requirements

- Control-plane host: Linux, macOS, or Windows  
- **Docker** only if you use project run mode `docker`  
- Agents must be able to reach the control plane HTTP(S) URL  

### 1. Configure

```bash
cp config.yaml.example config.yaml
```

Replace every placeholder secret before first run:

```yaml
server:
  port: 8000
  gin_mode: release          # use release in production

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

Mail tracking (`public_base_url`, enable flag, secret, redirect allow-list) is managed in **System → Mail Tracking** after boot. YAML entries (if present) are seed / fallback only. Click and open URL paths are **not** configured globally—they are generated per mail campaign.

> Do **not** commit `config.yaml`, `data/`, or TLS private keys. They are listed in `.gitignore`.

### 2. Run a release binary

```bash
./fishing-platform
# or a platform build from ./release/ after build-all.sh
```

### 3. Build from source

```bash
./build-all.sh
```

This builds the frontend, then cross-compiles control-plane and agent binaries into `release/` for linux/windows/darwin (amd64 & arm64), and copies `config.yaml.example` / `agent.yaml.example`.

Local dev (API already running separately):

```bash
cd frontend && npm install && npm run dev   # http://127.0.0.1:8081
```

### 4. Open the console

Browse to `http://localhost:8000` (or your `server.port`).

First boot writes admin credentials to:

```bash
cat data/admin_password.txt
```

```text
Username: admin
Password: <generated>
```

Save the password and delete that file after login.

---

## Operator modules

Mapped to the sidebar as shipped:

| Nav | Status | Notes |
|-----|--------|--------|
| **Dashboard** | Ready | Counters + host resource charts |
| **Projects** | Ready | Overview · Deployments · Logs · Records |
| **Workbench → Send Email** | Ready | Campaigns + open/click funnel |
| **Workbench → Page Builder** | Ready | Upload / AI mirror / manage pages |
| **Workbench → Info Gathering** | Ready | AI web search → structured contacts |
| **Workbench → QR Phishing** | Ready | Live QR relay, SSE preview, frame history, project bind + `/q/{slug}.png` |
| **Push Channels** | Ready | Multi-vendor webhooks |
| **Agents** | Ready | Online capacity & registration |
| **IP Blacklist** | Ready | From captures → host firewall |
| **Mail Services** | Ready | SMTP profiles |
| **Audit Logs** | Admin | Immutable |
| **Users** | Admin | admin / operator |
| **AI Settings** | Admin | Mirror rewrite models |
| **Mail Tracking** | Admin | Public base URL & tracking crypto |

Typical flow: configure SMTP + (optional) agents → build pages → create/deploy project → send campaign → review opens/clicks and credential records → optional IP block / push notify.

Agent packaging, ports, and systemd: **[docs/agent-deployment.md](docs/agent-deployment.md)**. Default agent project port range: `10000–20000/TCP`.

---

## Landing page hooks

Runtimes expect submissions on the project submit route (default `/api/submit`).

**JSON**

```javascript
await fetch('/api/submit', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({ username, password, captchavalue }),
})
```

**Form-urlencoded**

```javascript
const body = new URLSearchParams({ username, password, captchavalue })
await fetch('/api/submit', {
  method: 'POST',
  headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
  body,
})
```

HTML templates may use `{{SUBMIT_URL}}`, `{{REDIRECT_URL}}`, `{{QR_RELAY_URL}}`, and `{{QR_RELAY_IMG}}`.  
Mail bodies may use `{{email}}`, `{{click_url}}`, `{{open_pixel}}`, `{{landing_url}}`, and related tracking placeholders.

Full placeholder guides:

- Chinese: [`docs/placeholders.zh.md`](docs/placeholders.zh.md)
- English: [`docs/placeholders.en.md`](docs/placeholders.en.md)

HTTP access logs are written under `data/access-YYYY-MM-DD.log` (daily rotation). Successful fast `/api/dashboard` polls are omitted from the console but still recorded in the file.

---

## Security & compliance

- Rotate and protect `encryption_key`, `jwt_secret`, `container_secret`, and `agent_registration_token`. Changing `encryption_key` invalidates stored ciphertext (robot secrets, SMTP passwords, credential passwords, mail-tracking secret).
- Prefer `gin_mode: release` and enable `platform_basic_auth_enabled` on any exposed console.
- Treat harvested credentials and campaign recipient lists as highly sensitive.
- Audit logs are designed to be **non-deletable** through the product API.
- Operators are scoped to their own resources; admins manage users, audit, AI, and mail-tracking settings.

---

## Repository layout (high level)

```text
main.go / handlers / models / middleware / utils / config
agent/                 # edge agent binary
frontend/              # Next.js SPA (build → frontend/dist, embedded)
docs/agent-deployment.md
config.yaml.example
agent.yaml.example
build-all.sh
README.assets/logo.svg
```

---

## License & intended use

Provided for **authorized security research, red-team / purple-team exercises, and awareness training**.  

Unauthorized use against third parties is prohibited. Contributors and operators are responsible for complying with local law and engagement rules of engagement (RoE).

## Contributing

Issues and pull requests are welcome. Prefer minimal diffs, clear repro steps, and notes on security impact.

## Support

Open a GitHub Issue for bugs, deployment questions, or feature requests.
