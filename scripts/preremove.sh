#!/bin/sh
set -e

if [ -d /run/systemd/system ]; then
    systemctl stop tsdns || true
    systemctl disable tsdns || true
fi
