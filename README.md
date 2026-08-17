# Homelab Dashboard

自托管的服务器监控面板，面向 homelab 场景。单二进制、零数据库、配置驱动、实时推送。

## 功能特性

- **系统指标**：CPU（每核+总体）、内存、磁盘（按挂载点）、网络速率、Load
- **服务探针**：HTTP / TCP / UDP，支持按 `interval` 独立轮询，协议可编译期扩展
- **DSL 提取**：基于 expr-lang，从 HTTP 响应（JSON/文本）中用表达式/JSONPath/正则提取数据
- **容器指标**：连接 Docker Engine API 兼容 socket，采集容器 CPU/内存/网络指标，并可关联到具体服务
- **配置热重载**：fsnotify 监听配置文件，保存即生效，无重启
- **实时推送**：WebSocket 每周期推送聚合快照，前端 ECharts 展示，断线自动重连
- **零数据库**：历史数据由前端环形缓冲维护
- **单二进制部署**：前端通过 `go:embed` 嵌入后端

## 快速开始

### 直接运行（二进制）

```bash
# 无配置文件也能跑: 按解析顺序查找, 找不到时在当前目录生成样板
./dashboard

# 指定配置文件
./dashboard -config /path/to/dashboard.yaml

# 其他参数
./dashboard -addr :8080 -dev    # -dev 用磁盘上的前端 (开发用)
```

### Docker Compose

```bash
docker compose up -d --build
```

首次启动 `./config` 目录为空时，程序会在挂载卷内自动生成 `dashboard.yaml`，宿主直接可见可改，改后热重载。

### 直接拉取镜像

```bash
docker pull laxtiz/homelab-dashboard:latest
```

镜像经 GitHub Actions 构建发布，支持 `linux/amd64` 与 `linux/arm64`：git tag 对应 `latest` + 版本号 tag，分支 push 对应同名分支 tag（`main` 镜像用于测试）。

## 配置文件解析顺序

程序按以下顺序查找配置文件，命中即用：

1. **命令行参数**：`-config path`（显式指定则必须存在，否则报错退出）
2. **当前目录**：`./dashboard.yaml`
3. **用户级配置目录**：`$XDG_CONFIG_HOME/dashboard/dashboard.yaml`（未设时回退 `~/.config`）

以上都不存在时，在当前工作目录生成一份带注释的样板配置。模板文件为仓库内的 [`config/dashboard.yaml`](config/dashboard.yaml)，经 `go:embed` 嵌入二进制（已加入 `.gitignore` 的是根目录自动生成的 `dashboard.yaml`）。

## 配置参考

```yaml
server:
  addr: ":8080"
  interval: 5s            # 全局采集/推送周期

system:
  enabled: true
  disks: ["/", "/data"]   # 监控挂载点, 留空 = 全部
  netInterfaces: []       # 留空 = 全部网卡

container:
  enabled: true
  endpoint: ""            # 解析顺序: 配置 > $DOCKER_HOST > unix:///var/run/docker.sock
  containers:             # 留空 = 监控全部运行中容器
    - name: "traefik"
    - label: "monitor=true"

services:
  - name: "Home Assistant"
    type: http
    url: "http://192.168.1.10:8123"
    timeout: 5s
    interval: 10s
    extract:
      temp: "Jsonpath(json, '$.sensors.cpu_temp')"
      latency_ms: "latency_ms"
    container: {name: "ha", enabled: true}   # 服务关联容器

  - name: "Redis"
    type: tcp
    address: "127.0.0.1:6379"
    timeout: 2s
    payload: "INFO server\r\n"
    expect: "redis_version:"                  # 响应不含该串则判 down
    extract:
      version: "Regex(body, 'redis_version:([0-9.]+)')"

  - name: "DNS"
    type: udp
    address: "192.168.1.1:53"
    payloadBase64: "AAABAAABAAAAAA..."
    expect: "/[\x81\x80\x00\x01]/"            # 以 / 包裹 = 正则
```

### 探针字段

| 字段 | 说明 |
|---|---|
| `type` | `http` / `tcp` / `udp` |
| `url` / `address` | 目标地址 |
| `method` / `headers` | HTTP 专用 |
| `payload` / `payloadBase64` | 连接后发送的数据（文本 / base64 二进制） |
| `expect` | 响应需包含的内容，缺失判 down；`/re/` 形式为正则 |
| `extract` | DSL 表达式，键为指标名 |
| `container` | 关联容器：`{name, enabled}`，采集该容器指标合并进服务 |

### DSL 环境变量与函数

**变量**：`status`（HTTP 状态码）、`latency_ms`、`response_size`、`body`（原始响应）、`headers`、`json`（解析后的 JSON，非 JSON 为 nil）

**函数**：`Jsonpath(v, "$.a.b[0]")`、`Regex(s, "pattern")`（返回捕获组 1）、`Match(s, "re")`、`Int(x)`、`Float(x)`、`Str(x)`、`Round(x, n)`

> 注意：函数名首字母大写。正则里含 `\r\n` 时用双引号字符串（如 `"used_memory_human:([^\\r\\n]+)"`），单引号是 expr 原始字符串。

## 部署

### Docker Compose

见 [docker-compose.yml](docker-compose.yml)。`network_mode: host` + `pid: host` + `privileged: true` 用于获取宿主真实系统指标；socket 挂载到默认路径。

## Podman 用户

程序兼容任何暴露 Docker Engine API 的 socket（docker、podman compat socket 等）。连接优先级：`container.endpoint` 配置 > `$DOCKER_HOST` 环境变量 > 默认 `unix:///var/run/docker.sock`。

### 裸二进制（推荐）

以运行 podman 的同一用户运行，设置 `DOCKER_HOST` 指向 rootless socket：

```bash
export DOCKER_HOST=unix:///run/user/$(id -u)/podman/podman.sock
./dashboard
```

或在配置中直接指定：

```yaml
container:
  endpoint: "unix:///run/user/1000/podman/podman.sock"
```

### 容器化部署

把默认的 docker.sock 挂载换成你自己的 socket，并设置 `DOCKER_HOST`：

```yaml
services:
  dashboard:
    # ... 其余同默认 compose
    volumes:
      - ./config:/config
      - /run/user/1000/podman/podman.sock:/run/user/1000/podman/podman.sock
    environment:
      DOCKER_HOST: unix:///run/user/1000/podman/podman.sock
```

> 注意：rootless podman socket 是用户级权限，容器内进程需以对应用户身份运行才能访问（或改用 `--userns=keep-id` 等方式）。

## 开发

### 环境搭建

```bash
mise run web:install     # 安装前端依赖
```

> 不需要预装 mise 时，用仓库自带的引导脚本（本地化数据在 `.mise/`，已 gitignore）：
>
> ```bash
> ./bin/mise install        # 安装 mise + mise.toml 中声明的工具
> ./bin/mise run test       # 通过 bootstrap 运行任务 (自动激活 Go/Node)
> ```
>
> 升级 mise 版本时，重新生成引导脚本：`mise generate bootstrap -l -w`。

### 启动开发环境

需要同时运行后端和前端两个服务：

```bash
# 终端 1: 启动后端 (Go 热重载, 监听 :8080)
mise run dev

# 终端 2: 启动前端 Vite dev server (监听 :5173, 代理 API 到 :8080)
mise run web:dev
```

启动后访问 **http://localhost:5173**，前端支持 HMR 热更新，修改代码后浏览器自动刷新，无需手动重建。

> 注意：Vite dev server 会自动将 `/api` 和 `/ws` 请求代理到后端 `:8080`，确保后端先启动。

### 构建与测试

```bash
mise run build           # 构建前端 + 单二进制
mise run test            # go vet + go test + vue-tsc
mise run run             # 运行 bin/dashboard (配置自动解析)
```

> 开发时在项目根目录运行（`mise run run` / `mise run dev`），配置解析会命中自动生成的 `dashboard.yaml`（已被 `.gitignore` 忽略），也可以直接改 `config/dashboard.yaml` 模板后删除根目录文件重建。

## 项目结构

```
cmd/dashboard/       入口
internal/config/     配置模型 + 解析 + fsnotify 热重载
internal/system/     gopsutil 系统指标
internal/container/  容器指标 (Docker API 兼容 socket)
internal/probe/      探针引擎 (http/tcp/udp, 可注册扩展)
internal/dsl/        expr-lang DSL 封装
internal/collector/  聚合器
internal/ws/         WebSocket hub
internal/server/     echo 路由 + 前端 embed
web/                 Vue 3 + TS + Vite + ECharts 前端
config/dashboard.yaml 样板配置模板 (go:embed 嵌入二进制)
Dockerfile           multi-stage 构建 (前端 + 后端 → 单镜像)
docker-compose.yml   compose 部署示例
```