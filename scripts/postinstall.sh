#!/bin/sh
set -e

# Create tsdns user/group if they don't exist
# debian/ubuntu/centos/fedora
if command -v groupadd >/dev/null 2>&1; then
    getent group tsdns >/dev/null || groupadd --system tsdns
    getent passwd tsdns >/dev/null || useradd --system --gid tsdns --no-create-home --shell /usr/sbin/nologin tsdns
elif command -v addgroup >/dev/null 2>&1; then
    # alpine
    getent group tsdns >/dev/null || addgroup -S tsdns
    getent passwd tsdns >/dev/null || adduser -S -G tsdns -H -s /sbin/nologin tsdns
fi

# Create directories and set permissions
mkdir -p /etc/tsdns
mkdir -p /var/lib/tsdns
mkdir -p /run/tsdns
chown -R tsdns:tsdns /etc/tsdns /var/lib/tsdns /run/tsdns
chmod 750 /etc/tsdns
chmod 640 /etc/tsdns/config.yaml

# Generate random API token if it's empty in config
CONFIG_FILE="/etc/tsdns/config.yaml"
if [ -f "$CONFIG_FILE" ]; then
    if grep -q 'token: ""' "$CONFIG_FILE"; then
        # Generate 32-character random token using /dev/urandom
        RANDOM_TOKEN=$(head /dev/urandom | tr -dc A-Za-z0-9 | head -c 32)
        sed -i "s/token: \"\"/token: \"$RANDOM_TOKEN\"/" "$CONFIG_FILE"
        
        echo "================================================================="
        echo "  TSDNS Installation Successful!"
        echo "  A random API token has been generated for you:"
        echo "  Token: $RANDOM_TOKEN"
        echo "  Config: $CONFIG_FILE"
        echo "================================================================="
    fi
fi

# Reload systemd if present
if [ -d /run/systemd/system ]; then
    systemctl daemon-reload
    echo ""
    echo "To start TSDNS and enable it on boot, run:"
    echo "  systemctl enable --now tsdns"
    echo ""
    echo "To check the status:"
    echo "  systemctl status tsdns"
elif [ -f /etc/init.d/functions ]; then
    # SysVinit / OpenRC fallback hint
    echo ""
    echo "To start TSDNS:"
    echo "  tsdns serve --config /etc/tsdns/config.yaml &"
fi
echo "================================================================="