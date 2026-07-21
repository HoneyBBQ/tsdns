<div align="center">

<img src="https://raw.githubusercontent.com/HoneyBBQ/tsdns/main/assets/banner.png" alt="tsdns banner" width="800">

# tsdns

Enhanced TeamSpeak TSDNS server with regex matching, multiple storage backends, and a management API.

<br>

[![GitHub](https://img.shields.io/badge/GitHub-Repo-181717?logo=github&logoColor=white)](https://github.com/honeybbq/tsdns)
[![Docker Hub](https://img.shields.io/badge/Docker_Hub-Image-2496ED?logo=docker&logoColor=white)](https://hub.docker.com/r/honeybbq/tsdns)
[![GHCR](https://img.shields.io/badge/GHCR-Image-444444?logo=github&logoColor=white)](https://github.com/honeybbq/tsdns/pkgs/container/tsdns)
[![Go Reference](https://pkg.go.dev/badge/github.com/honeybbq/tsdns/core.svg)](https://pkg.go.dev/github.com/honeybbq/tsdns/core)

[![CI](https://github.com/honeybbq/tsdns/actions/workflows/ci.yml/badge.svg)](https://github.com/honeybbq/tsdns/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/honeybbq/tsdns)](https://goreportcard.com/report/github.com/honeybbq/tsdns)
[![Codecov](https://codecov.io/gh/HoneyBBQ/tsdns/graph/badge.svg)](https://codecov.io/gh/HoneyBBQ/tsdns)
[![Release](https://img.shields.io/github/v/release/honeybbq/tsdns.svg)](https://github.com/honeybbq/tsdns/releases)
[![License](https://img.shields.io/github/license/honeybbq/tsdns.svg)](https://github.com/honeybbq/tsdns/blob/main/LICENSE)
[![Docker Pulls](https://img.shields.io/docker/pulls/honeybbq/tsdns.svg)](https://hub.docker.com/r/honeybbq/tsdns)

<br>

[中文文档](https://github.com/honeybbq/tsdns/blob/main/README_CN.md)

</div>

---

> This project is a protocol-compatible implementation and is not affiliated with TeamSpeak Systems GmbH.

---

## Features

- **Protocol Compatibility**: Implements the TSDNS protocol (TCP port 41144).
- **Advanced Matching**: Supports regex matching and flexible wildcard patterns.
- **Multi-Target Support**: A single domain can resolve to multiple IP:Port combinations.
- **Management API**: HTTP REST API for managing records without server restarts.
- **Storage Backends**: Supports SQLite, PostgreSQL, MySQL, and Redis.
- **Pure Go SQLite**: Uses a no-CGO SQLite driver for cross-platform compatibility.
- **Caching**: In-memory cache with configurable background refresh.
- **Zero-Config Local Management**: Unix Domain Socket enabled by default in Docker and Linux packages for seamless CLI usage without tokens.
- **TSDNS Rules**:
  - Exact and wildcard domain matching (`*` and `*.domain`).
  - Regex matching (via `reg:` prefix).
  - Advanced globs (e.g., `ts-*-server.com`).
  - `$PORT` placeholder substitution (via port `0`).
  - `NORESPONSE` support (empty target list).

---

## Installation

### 1. Quick Install (Script)

The easiest way to install the latest `tsdns` binary on Linux, macOS, or FreeBSD:

```bash
curl -fsSL https://raw.githubusercontent.com/HoneyBBQ/tsdns/main/scripts/install.sh | sh
```

### 2. Native Packages (Recommended for Linux)

Download the `.deb`, `.rpm`, or `.apk` package from the [Releases](https://github.com/honeybbq/tsdns/releases) page.

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

> **Note:** Native packages automatically create a `tsdns` user, set up a systemd service, and generate a random API token in `/etc/tsdns/config.yaml`.

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

### 4. From Source

Requires Go 1.25 or later.

```bash
go install github.com/honeybbq/tsdns/cmd/tsdns@latest
```

---

## Library

The protocol server is published as a separate module with no third-party dependencies. Applications that provide their own `RecordRepository` can use the core without downloading the CLI, HTTP API, metrics, or storage backend dependencies.

```bash
go get github.com/honeybbq/tsdns/core@v0.1.1
```

```go
import tsdns "github.com/honeybbq/tsdns/core"

server, err := tsdns.NewServer("0.0.0.0").
	WithRepository(repository).
	Build()
```

`repository` must implement `tsdns.RecordRepository`. Logging and metrics use the standard library `slog.Logger` and the small `tsdns.Metrics` interface.

---

## Usage

### 1. Configuration
Configure via `config.yaml` or environment variables.

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

### 2. Management
Records can be managed via the CLI or the REST API.

**CLI examples:**
```bash
# Add a simple record
tsdns records add --domain ts.example.com --target 1.2.3.4:9987 --token my-token

# Add multiple targets with $PORT (port 0)
tsdns records add --domain *.example.com --target "1.2.3.4:0 [2001:db8::1]:9987" --token my-token

# Add a regex record
tsdns records add --domain "reg:^ts\d+\.example\.com$" --target "1.2.3.4:9987" --token my-token
```

**API example:**
```bash
curl -X POST http://localhost:8080/api/v1/records \
  -H "Authorization: Bearer my-token" \
  -d '{
    "domain": "ts.example.com",
    "targets": ["1.2.3.4:9987", "5.6.7.8:0"]
  }'
```

### 3. Query Verification
```bash
printf "ts.example.com\n" | nc localhost 41144
```

---

## Configuration

Configurable via environment variables:

| Variable | Default | Description |
| :--- | :--- | :--- |
| `TSDNS_LISTEN` | `0.0.0.0:41144` | TSDNS TCP listen address |
| `TSDNS_CACHE_REFRESH_INTERVAL` | `30s` | Background cache refresh interval |
| `TSDNS_API_LISTEN` | `127.0.0.1:8080` | Admin API TCP listen address |
| `TSDNS_API_SOCKET` | `/tmp/tsdns.sock` (Docker) | Unix domain socket path for local management |
| `TSDNS_API_TOKEN` | empty | Shared secret for API authentication |
| `TSDNS_STORAGE_DSN` | `sqlite:./tsdns.sqlite` | Storage backend connection string |

---

## Storage Options

Set the backend via `TSDNS_STORAGE_DSN`:

| Backend | DSN Format |
| :--- | :--- |
| **SQLite** | `sqlite:<path>` |
| **PostgreSQL** | `postgres://<user>:<pass>@<host>:<port>/<db>` |
| **MySQL** | `mysql://<user>:<pass>@<host>:<port>/<db>` |
| **Redis** | `redis://:<pass>@<host>:<port>/<db>` |


---

## Admin API

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| `GET` | `/healthz` | Health check |
| `GET` | `/api/v1/records` | List records |
| `POST` | `/api/v1/records` | Create/Update record |
| `GET` | `/api/v1/records/{domain}` | Get record |
| `DELETE` | `/api/v1/records/{domain}` | Delete record |

---

## Project Structure

- `cmd/tsdns/`: Application entry point.
- `internal/`: Core implementation details (API, CLI, config, storage).
- `repository/`: Database-specific driver implementations.
- `record.go`: Core domain model and repository interface.

---

## License

MIT License. See [LICENSE](https://github.com/honeybbq/tsdns/blob/main/LICENSE).
