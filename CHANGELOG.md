# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [v0.1.3] - 2026-01-12

### Added

- **Quick Install Script**: Added a one-line installation script (`scripts/install.sh`) for Linux, macOS, and FreeBSD.
- **Enhanced Documentation**: Comprehensive update to README (English and Chinese) with detailed installation guides for all supported platforms.

### Improved

- **Documentation Formatting**: Better table formatting and bilingual consistency in README files.

## [v0.1.2] - 2026-01-12

### Added

- **Native Packaging**: Added support for `.deb`, `.rpm`, and `.apk` Linux packages via NFPM.
- **Windows Zip Support**: Changed Windows distribution format from `.tar.gz` to `.zip` for better user experience.
- **FreeBSD Support**: Added build targets for FreeBSD (amd64, arm64).
- **Systemd Integration**: Included a systemd service unit in Linux packages.
- **Automatic Token Generation**: Linux packages now automatically generate a secure random API token during post-installation if none is provided.

### Changed

- **Archive Naming**: Refined archive name templates for better clarity across platforms.

### Security

- **Strict Permissions**: Implemented strict 640/750 permissions for configuration files and directories in Linux packages.

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

