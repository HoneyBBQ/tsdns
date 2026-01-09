# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.0] - 2026-01-10

### Added
- **Protocol Compatibility**: Full implementation of the TeamSpeak TSDNS protocol (TCP port 41144).
- **Multiple Storage Backends**: 
  - SQLite (Pure Go, no CGO required)
  - PostgreSQL
  - MySQL
  - Redis
- **Administrative API**: RESTful API for managing records at runtime.
- **CLI Interface**: Powerful command-line tool for record management, migrations, and server operations.
- **Advanced Matching**:
  - Exact domain matching
  - Wildcard support (`*.domain.com`)
  - Regex support (`reg:^ts\d+\.example\.com$`)
  - Advanced glob matching
- **Docker Support**: Multi-architecture images (amd64, arm64, arm/v7) with distroless security.
- **Caching System**: In-memory cache with background refresh for high performance.
- **Prometheus Metrics**: Built-in metrics for monitoring server health and performance.
- **Security Features**: API token authentication and Unix Domain Socket support for local management.
