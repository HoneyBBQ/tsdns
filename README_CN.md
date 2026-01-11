# tsdns

[English Documentation](https://github.com/honeybbq/tsdns/blob/main/README.md) | [中文说明](https://github.com/honeybbq/tsdns/blob/main/README_CN.md)

[![GitHub](https://img.shields.io/badge/GitHub-Repo-181717?logo=github&logoColor=white)](https://github.com/honeybbq/tsdns)
[![Docker Hub](https://img.shields.io/badge/Docker_Hub-Image-2496ED?logo=docker&logoColor=white)](https://hub.docker.com/r/honeybbq/tsdns)
[![GHCR](https://img.shields.io/badge/GHCR-Image-444444?logo=github&logoColor=white)](https://github.com/honeybbq/tsdns/pkgs/container/tsdns)
[![Build Status](https://github.com/honeybbq/tsdns/actions/workflows/docker-publish.yml/badge.svg)](https://github.com/honeybbq/tsdns/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/honeybbq/tsdns)](https://goreportcard.com/report/github.com/honeybbq/tsdns)
[![Go Reference](https://pkg.go.dev/badge/github.com/honeybbq/tsdns.svg)](https://pkg.go.dev/github.com/honeybbq/tsdns)
[![Docker Pulls](https://img.shields.io/docker/pulls/honeybbq/tsdns.svg)](https://hub.docker.com/r/honeybbq/tsdns)
[![License](https://img.shields.io/github/license/honeybbq/tsdns.svg)](https://github.com/honeybbq/tsdns/blob/main/LICENSE)
[![GitHub release](https://img.shields.io/github/v/release/honeybbq/tsdns.svg)](https://github.com/honeybbq/tsdns/releases)

`tsdns` 是一款增强型 TeamSpeak TSDNS 服务端，支持正则表达式匹配与四种主流存储后端（SQLite、PostgreSQL、MySQL 和 Redis）。同时提供管理 API 和命令行工具，方便高效地进行记录管理。

> 本项目为独立协议实现，不隶属于 TeamSpeak Systems GmbH。

---

## 特性

- **协议兼容**: 实现 TSDNS 协议（TCP 端口 41144）。
- **高级匹配**: 支持正则表达式匹配和灵活的通配符模式。
- **多目标支持**: 单个域名可以解析为多个 IP:Port 组合。
- **管理 API**: 提供 HTTP REST API，支持无需重启服务器的记录更新。
- **存储后端**: 支持 SQLite, PostgreSQL, MySQL 和 Redis。
- **纯 Go SQLite**: 使用无 CGO 的 SQLite 驱动，支持跨平台部署。
- **缓存**: 内存缓存支持可配置的后台刷新。
- **本地零配置管理**: Docker 模式和 Linux 安装包默认开启 Unix Domain Socket，无需 Token 即可直接管理。
- **TSDNS 规则**:
  - 支持精确匹配和通配符域名（`*` 和 `*.domain`）。
  - 支持正则表达式（通过 `reg:` 前缀）。
  - 支持高级通配符（如 `ts-*-server.com`）。
  - 支持 `$PORT` 占位符替换（通过端口 `0` 实现）。
  - 支持 `NORESPONSE`（通过空目标列表实现）。

---

## 安装说明

### 1. 快速安装 (脚本)

在 Linux, macOS 或 FreeBSD 上安装最新版 `tsdns` 二进制文件的最快方式：

```bash
curl -fsSL https://raw.githubusercontent.com/HoneyBBQ/tsdns/main/scripts/install.sh | sh
```

### 2. 原生安装包 (Linux 推荐)

从 [Releases](https://github.com/honeybbq/tsdns/releases) 页面下载 `.deb`, `.rpm` 或 `.apk` 安装包。

**Debian / Ubuntu:**
```bash
sudo dpkg -i tsdns_*.deb
sudo systemctl enable --now tsdns
```

**CentOS / RHEL / Fedora:**
```bash
sudo rpm -ivh tsdns_*.rpm
sudo systemctl enable --now tsdns
```

**Alpine:**
```bash
sudo apk add --allow-untrusted tsdns_*.apk
```

> **注意：** 原生安装包会自动创建 `tsdns` 系统用户、配置 systemd 服务，并在 `/etc/tsdns/config.yaml` 中自动生成随机 API Token。

### 3. Docker

```bash
docker run -d --name tsdns \
  -p 41144:41144 \
  -p 8080:8080 \
  -e TSDNS_API_TOKEN=your-secret-token \
  -e TSDNS_STORAGE_DSN=sqlite:/data/tsdns.sqlite \
  -v tsdns-data:/data \
  honeybbq/tsdns:latest
```

### 4. 源码安装

需要 Go 1.25 或更高版本。

```bash
go install github.com/honeybbq/tsdns/cmd/tsdns@latest
```

---

## 使用说明

### 1. 配置
通过 `config.yaml` 或环境变量进行配置。

```yaml
tsdns:
  listen: "0.0.0.0:41144"
  cache_refresh_interval: "30s"
api:
  listen: "0.0.0.0:8080"
  token: "my-token"
storage:
  dsn: "sqlite:./tsdns.sqlite"
```

### 2. 管理记录
可以通过 CLI 或 REST API 管理记录。

**CLI 示例:**
```bash
# 添加简单记录
tsdns records add --domain ts.example.com --target 1.2.3.4:9987 --token my-token

# 添加带 $PORT 的多值记录 (端口 0 代表 $PORT)
tsdns records add --domain *.example.com --target "1.2.3.4:0 [2001:db8::1]:9987" --token my-token

# 添加正则表达式记录
tsdns records add --domain "reg:^ts\d+\.example\.com$" --target "1.2.3.4:9987" --token my-token
```

**API 示例:**
```bash
curl -X POST http://localhost:8080/api/v1/records \
  -H "Authorization: Bearer my-token" \
  -d '{
    "domain": "ts.example.com",
    "targets": ["1.2.3.4:9987", "5.6.7.8:0"]
  }'
```

### 3. 验证
```bash
printf "ts.example.com\n" | nc localhost 41144
```

---

## 配置说明

可以通过以下环境变量进行配置：

| 环境变量 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `TSDNS_LISTEN` | `0.0.0.0:41144` | TSDNS TCP 监听地址 |
| `TSDNS_CACHE_REFRESH_INTERVAL` | `30s` | 后台缓存刷新频率 |
| `TSDNS_API_LISTEN` | `127.0.0.1:8080` | 管理 API TCP 监听地址 |
| `TSDNS_API_SOCKET` | `/tmp/tsdns.sock` (Docker) | 本地管理的 Unix domain socket 路径 |
| `TSDNS_API_TOKEN` | 空 | API 鉴权使用的共享令牌 |
| `TSDNS_STORAGE_DSN` | `sqlite:./tsdns.sqlite` | 存储后端连接字符串 |

---

## 存储后端

通过 `TSDNS_STORAGE_DSN` 设置后端：

| 后端 | DSN 格式 |
| :--- | :--- |
| **SQLite** | `sqlite:<path>` |
| **PostgreSQL** | `postgres://<user>:<pass>@<host>:<port>/<db>` |
| **MySQL** | `mysql://<user>:<pass>@<host>:<port>/<db>` |
| **Redis** | `redis://:<pass>@<host>:<port>/<db>` |


---

## 管理 API

| 方法 | 接口路径 | 说明 |
| :--- | :--- | :--- |
| `GET` | `/healthz` | 健康检查 |
| `GET` | `/api/v1/records` | 列出记录 |
| `POST` | `/api/v1/records` | 创建/更新记录 |
| `GET` | `/api/v1/records/{domain}` | 获取记录 |
| `DELETE` | `/api/v1/records/{domain}` | 删除记录 |

---

## 项目结构

- `cmd/tsdns/`: 程序入口。
- `internal/`: 核心实现细节（API, CLI, 配置, 存储）。
- `repository/`: 数据库特定的驱动实现。
- `record.go`: 核心领域模型与存储接口定义。

---

## 许可证

MIT License. 详见 [LICENSE](https://github.com/honeybbq/tsdns/blob/main/LICENSE)。
