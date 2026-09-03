# Fishing Platform Agent 使用手册

## 1. 功能说明

Fishing Platform Agent 是部署在远程节点上的页面运行程序，由主控端统一注册、调度和管理。

它支持：

- 一个项目同时部署到多个 Agent。
- 一个 Agent 同时承载多个项目。
- 不同项目使用独立部署目录、端口、版本和日志序列。
- 主控端统一下发部署、启动、停止、重新部署和删除任务。
- 页面提交请求通过 Agent 的认证通道回传主控端。
- 任务日志和访问日志按项目集中存储在主控端。
- 主控暂时不可达时在 Agent 本地缓冲日志，恢复后自动续传。
- Agent 重启后通过心跳对账自动恢复应运行的项目。

## 2. 运行关系

```text
管理后台
   │
   ├── 新建/编辑项目并选择多个 Agent
   │
主控端 ── 项目、部署状态、任务、日志、提交数据
   │
   ├── Agent A ── 项目 1、项目 2
   └── Agent B ── 项目 1、项目 3
```

项目和 Agent 是多对多关系。主控端使用 `deployment_id` 唯一区分“某项目在某 Agent 上的部署实例”。

## 3. 环境要求

### 主控端

- 主控端服务已经启动。
- `config.yaml` 已配置 `security.agent_registration_token`。
- Agent 可以访问主控端的 HTTP 或 HTTPS 地址。

### Agent 节点

- Linux、Windows 或 macOS。
- Agent 节点可以主动访问主控端。
- 防火墙允许项目端口范围对访问者开放。
- Agent 工作目录具有读写权限。

默认项目端口范围为：

```text
10000-20000/TCP
```

如果 Agent 位于反向代理后方，应把 `advertise_host` 配置为访问者实际可达的域名或地址。

## 4. 配置主控端

编辑主控端 `config.yaml`：

```yaml
security:
  agent_registration_token: replace-with-a-long-random-agent-token
```

修改配置后重启主控端。所有首次注册的 Agent 使用同一个注册令牌，注册成功后每个 Agent 会获得独立的 Agent Token。

主控端涉及的接口包括：

```text
POST /api/agents/register/                 首次注册
POST /api/agents/heartbeat/                心跳与部署对账
GET  /api/agent/tasks/lease/               领取任务
POST /api/agent/tasks/{taskID}/start/      开始任务
POST /api/agent/tasks/{taskID}/renew/      续租任务
POST /api/agent/tasks/{taskID}/complete/   回传结果
GET  /api/agent/deployments/{id}/artifact/ 下载项目制品
POST /api/agent/deployments/{id}/submit/   回传页面提交
POST /api/agent/deployments/{id}/logs/     上传项目日志
```

## 5. 获取 Agent

### 5.1 使用构建脚本

在项目根目录执行：

```bash
./build-all.sh
```

`release/` 目录会同时生成主控端和 Agent 的多平台文件，例如：

```text
fishing-agent-linux-amd64
fishing-agent-linux-arm64
fishing-agent-windows-amd64.exe
fishing-agent-windows-arm64.exe
fishing-agent-macos-amd64
fishing-agent-macos-arm64
agent.yaml.example
```

### 5.2 单独构建当前平台

```bash
go build -o fishing-agent ./agent
```

### 5.3 Linux AMD64 交叉构建

```bash
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
  go build -ldflags="-s -w" -o fishing-agent-linux-amd64 ./agent
```

## 6. Agent 配置文件

复制示例：

```bash
cp agent.yaml.example agent.yaml
```

完整示例：

```yaml
server_url: https://platform.example.com
registration_token: replace-with-a-long-random-agent-token
```

### 参数说明

| 参数 | 必填 | 默认值 | 说明 |
|---|---:|---|---|
| `server_url` | 是 | 无 | 主控端地址，不要以 `/` 结尾 |
| `registration_token` | 首次注册必填 | 无 | 对应主控端 `agent_registration_token` |

Agent 配置文件仅保留上述两个必填项。项目运行端口由主控端从 `10000-20000` 范围内按 Agent 分配，并随部署任务下发，Agent 不再配置或自行选择端口。

## 7. 首次启动

Linux/macOS：

```bash
chmod +x ./fishing-agent
./fishing-agent -config ./agent.yaml
```

Windows PowerShell：

```powershell
.\fishing-agent-windows-amd64.exe -config .\agent.yaml
```

正常启动日志类似：

```text
agent edge-agent-01 (agent_xxx) connected to https://platform.example.com
```

注册成功后会生成：

```text
{work_dir}/state.json
```

示例：

```json
{
  "agent_id": "agent_xxx",
  "token": "agent_token_xxx"
}
```

该文件权限在 Linux/macOS 上为 `0600`。后续启动直接使用其中的独立 Agent Token，不再执行首次注册。

## 8. 在主控端使用 Agent

### 8.1 检查 Agent 状态

进入管理后台的 **Agent** 页面，确认节点状态为“在线”。页面会显示：

- 节点名称和 Agent ID
- 主机名和 IP
- 操作系统与架构
- Agent 版本
- capabilities 和 tags
- 当前承载项目数
- 最后心跳时间

超过约 90 秒未收到心跳时，主控端会把节点显示为离线。

### 8.2 新建项目并选择 Agent

1. 打开 **项目** 页面。
2. 点击 **新建项目**。
3. 填写项目名称、页面路由、提交路由等配置。
4. 在 **部署节点** 中选择一个或多个在线 Agent。
5. 上传页面文件并保存。

主控端会为每个选中的 Agent 创建独立部署实例和部署任务。

例如选择 Agent A、Agent B 后：

```text
项目 12 / Agent A / deployment 101
项目 12 / Agent B / deployment 102
```

两个实例分别拥有独立状态、端口、运行地址和日志来源。

### 8.3 编辑项目

编辑并保存项目后，主控端递增项目部署版本，并向所有仍被选中的 Agent 下发重新部署任务。

如果修改 Agent 选择：

- 新增 Agent：向新节点下发完整部署任务。
- 取消 Agent：向被取消节点下发删除任务。
- 保留 Agent：同步最新项目版本。

### 8.4 查看部署状态

进入项目详情页的 **Agent 部署实例** 区域，可以查看：

- Agent 名称
- 实际部署状态
- 已部署版本
- 独立运行地址
- 重新部署、启动、停止和删除操作

常见状态：

| 状态 | 含义 |
|---|---|
| `pending` | 等待 Agent 领取任务 |
| `deploying` | 正在部署 |
| `running` | 页面服务运行中 |
| `stopped` | 页面服务已停止 |
| `failed` | 部署或操作失败 |
| `removed` | 已从 Agent 清理 |

### 8.5 查看项目日志

项目详情页的 **项目日志** 会聚合该项目在所有 Agent 上的日志。

每条日志保留：

- `project_id`：所属项目
- `agent_id`：来源 Agent
- `deployment_id`：来源部署实例
- `task_id`：关联任务
- `stream`：`system` 或 `access`
- `level`：日志级别
- `sequence`：部署实例内日志序列
- `logged_at`：产生时间

日志实际存储在主控端数据库中，而不是只保留在 Agent 上。

## 9. Agent 本地目录

典型目录结构：

```text
/var/lib/fishing-agent/
├── state.json
├── runtime-state.json
├── pending-logs.json
└── projects/
    ├── deployment-101/
    │   └── revisions/
    │       ├── 1/
    │       │   └── index.html
    │       └── 2/
    │           └── index.html
    └── deployment-105/
        └── revisions/
            └── 1/
                └── index.html
```

文件说明：

| 文件 | 说明 |
|---|---|
| `state.json` | Agent ID 和独立 Token |
| `runtime-state.json` | 部署实例与端口映射，用于重新部署时复用端口 |
| `pending-logs.json` | 主控不可达时的持久化日志缓冲 |
| `projects/deployment-{id}` | 每个部署实例的独立制品目录 |

## 10. Linux systemd 部署

### 10.1 创建用户和目录

```bash
sudo useradd --system --home /var/lib/fishing-agent --shell /usr/sbin/nologin fishing-agent
sudo mkdir -p /opt/fishing-agent /etc/fishing-agent /var/lib/fishing-agent
sudo chown -R fishing-agent:fishing-agent /var/lib/fishing-agent
```

### 10.2 安装文件

```bash
sudo install -m 0755 fishing-agent-linux-amd64 /opt/fishing-agent/fishing-agent
sudo install -m 0600 agent.yaml /etc/fishing-agent/agent.yaml
```

确保配置中的工作目录为：

```yaml
work_dir: /var/lib/fishing-agent
```

### 10.3 创建服务

保存为 `/etc/systemd/system/fishing-agent.service`：

```ini
[Unit]
Description=Fishing Platform Agent
Wants=network-online.target
After=network-online.target

[Service]
Type=simple
User=fishing-agent
Group=fishing-agent
WorkingDirectory=/var/lib/fishing-agent
ExecStart=/opt/fishing-agent/fishing-agent -config /etc/fishing-agent/agent.yaml
Restart=always
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
```

### 10.4 启动与查看日志

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now fishing-agent
sudo systemctl status fishing-agent
sudo journalctl -u fishing-agent -f
```

## 11. Windows 启动

前台运行：

```powershell
.\fishing-agent-windows-amd64.exe -config C:\fishing-agent\agent.yaml
```

可以使用 Windows Task Scheduler 或服务管理工具将该命令配置为开机启动。Agent 工作目录建议使用固定路径：

```yaml
work_dir: C:\fishing-agent\data
```

## 12. 防火墙配置

Agent 必须能主动访问主控端：

```text
Agent -> 主控端 HTTP/HTTPS 端口
```

访问者必须能连接 Agent 的项目端口范围：

```text
访问者 -> Agent 10000-20000/TCP
```

Ubuntu UFW 示例：

```bash
sudo ufw allow 10000:20000/tcp
```

firewalld 示例：

```bash
sudo firewall-cmd --permanent --add-port=10000-20000/tcp
sudo firewall-cmd --reload
```

如果只允许指定网络访问，可在防火墙规则中限制来源网段。

## 13. 运行机制

1. Agent 首次启动向主控端注册并取得独立 Token。
2. Agent 周期性发送心跳，并报告本机正在运行的 `deployment_id`。
3. Agent 主动轮询任务，不要求主控端反向连接 Agent。
4. Agent 领取任务后获得 60 秒租约，执行期间每 20 秒续租。
5. Agent 下载页面制品并校验 `SHA-256`。
6. Agent 把制品写入对应部署实例的版本目录。
7. 主控端按 Agent 分配独立端口并随任务下发，Agent 只监听主控端指定端口。
8. 页面提交请求由 Agent 附带自身 Token 转发到主控端。
9. 主控端根据 Agent Token 和 `deployment_id` 确定真实项目，不信任页面传入的项目 ID。
10. Agent 把任务日志和访问日志写入本地缓冲并批量上传。
11. 主控端按项目保存日志，同时记录 Agent 和部署实例来源。
12. Agent 重启后，心跳对账会重新下发主控期望运行但本机缺失的部署。

## 14. 升级 Agent

推荐步骤：

```bash
sudo systemctl stop fishing-agent
sudo cp /opt/fishing-agent/fishing-agent /opt/fishing-agent/fishing-agent.bak
sudo install -m 0755 ./fishing-agent-linux-amd64 /opt/fishing-agent/fishing-agent
sudo systemctl start fishing-agent
sudo systemctl status fishing-agent
```

升级时不要删除：

```text
state.json
runtime-state.json
pending-logs.json
projects/
```

保留这些文件可以维持 Agent 身份、端口映射和断网日志。

## 15. 重置或重新注册 Agent

普通重启不需要重新注册。

需要生成新 Agent 身份时：

1. 停止 Agent。
2. 备份工作目录。
3. 删除 `state.json`。
4. 确认 `agent.yaml` 中存在有效 `registration_token`。
5. 重新启动 Agent。

如果配置了固定 `agent_id`，重新注册会复用该 Agent ID 并轮换 Agent Token；保持 `agent_id: ""` 时会生成新的节点身份。

## 16. 停止与卸载

停止服务：

```bash
sudo systemctl stop fishing-agent
```

禁止开机启动：

```bash
sudo systemctl disable fishing-agent
```

完整卸载前，先在主控端取消该 Agent 上的项目部署，等待部署状态变为 `removed`，再删除程序和数据目录。

## 17. 故障排查

### 17.1 Agent 未出现在管理后台

检查：

```bash
./fishing-agent -config ./agent.yaml
```

重点确认：

- `server_url` 可以从 Agent 节点访问。
- `registration_token` 与主控端一致。
- 主控端已重启并加载最新配置。
- Agent 工作目录可写。

### 17.2 显示离线

检查 Agent 进程和网络：

```bash
sudo systemctl status fishing-agent
sudo journalctl -u fishing-agent -n 200
curl -I https://platform.example.com
```

心跳间隔默认为 30 秒，超过约 90 秒无心跳会显示离线。

### 17.3 项目一直处于 pending

检查：

- Agent 是否在线。
- Agent 日志中是否存在 `lease tasks failed`。
- Agent 是否可以访问主控端 `/api/agent/tasks/lease/`。
- Agent Token 是否仍有效。

### 17.4 页面无法访问

检查部署状态和监听端口：

```bash
ss -lntp | grep fishing-agent
```

同时确认：

- Agent 主机名对访问者可解析、可连接。
- 项目端口位于主控端分配的 `10000-20000` 范围中。
- 防火墙已放行端口范围。
- 项目详情页显示的运行地址正确。

### 17.5 页面能打开但提交失败

检查：

- Agent 可以访问主控端。
- 项目配置中的提交路由与页面请求路径一致。
- Agent Token 有效。
- 主控端数据库可写。
- Agent 日志中是否出现 `upstream unavailable`。

### 17.6 日志没有及时出现在主控端

日志通常每 2 秒尝试批量上传。检查：

```text
{work_dir}/pending-logs.json
```

如果该文件持续增大，说明 Agent 无法访问主控端日志接口。网络恢复后 Agent 会自动续传。

### 17.7 端口耗尽

主控端出现以下错误时：

```text
no project port available for agent
```

处理方式：

- 删除已经不需要的部署实例以释放主控端分配记录。
- 检查是否有其他进程占用端口。
- 重启 Agent 使主控端重新对账。

## 18. 常用命令速查

```bash
# 构建 Agent
go build -o fishing-agent ./agent

# 查看参数
./fishing-agent -h

# 前台启动
./fishing-agent -config ./agent.yaml

# systemd 启动
sudo systemctl start fishing-agent

# systemd 停止
sudo systemctl stop fishing-agent

# 查看状态
sudo systemctl status fishing-agent

# 实时日志
sudo journalctl -u fishing-agent -f

# 查看监听端口
ss -lntp | grep fishing-agent
```
