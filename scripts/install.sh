#!/bin/sh
set -e

# tsdns installation script
# Inspired by goreleaser, hugo, and other go-based projects.

OWNER="HoneyBBQ"
REPO="tsdns"
BINARY="tsdns"

# --- helper functions ---
info() { echo "info: $*"; }
warn() { echo "warn: $*" >&2; }
error() { echo "error: $*" >&2; exit 1; }

# --- detect system ---
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64) ARCH="amd64" ;;
    aarch64|arm64) ARCH="arm64" ;;
    i386|i686) ARCH="386" ;;
    armv7l) ARCH="armv7" ;;
    *) error "Unsupported architecture: $ARCH" ;;
esac

case "$OS" in
    linux) OS="linux" ;;
    darwin) OS="darwin" ;;
    freebsd) OS="freebsd" ;;
    *) error "Unsupported OS: $OS" ;;
esac

# --- get latest version ---
if [ -z "$VERSION" ]; then
    VERSION=$(curl -sI https://github.com/$OWNER/$REPO/releases/latest | grep -i "^location:" | grep -oE "v[0-9.]+" || true)
    if [ -z "$VERSION" ]; then
        # Fallback to API if location header fails
        VERSION=$(curl -s https://api.github.com/repos/$OWNER/$REPO/releases/latest | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
    fi
fi

if [ -z "$VERSION" ]; then
    error "Could not detect latest version"
fi

info "Downloading tsdns $VERSION for $OS/$ARCH..."

# --- download and install ---
EXTENSION="tar.gz"
FILENAME="${BINARY}_${VERSION#v}_${OS}_${ARCH}.${EXTENSION}"
URL="https://github.com/$OWNER/$REPO/releases/download/$VERSION/$FILENAME"

TMP_DIR=$(mktemp -d)
trap 'rm -rf "$TMP_DIR"' EXIT

curl -sSL "$URL" -o "$TMP_DIR/$FILENAME"
tar -xzf "$TMP_DIR/$FILENAME" -C "$TMP_DIR"

INSTALL_DIR="/usr/local/bin"
if [ ! -w "$INSTALL_DIR" ]; then
    warn "No write access to $INSTALL_DIR, trying to use sudo..."
    sudo mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
else
    mv "$TMP_DIR/$BINARY" "$INSTALL_DIR/$BINARY"
fi

chmod +x "$INSTALL_DIR/$BINARY"
info "Successfully installed $BINARY to $INSTALL_DIR"
$BINARY version
