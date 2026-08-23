#!/usr/bin/env bash
set -euo pipefail

[[ $EUID -eq 0 ]] || { echo "Run this script with sudo." >&2; exit 1; }

BIN=${1:-/usr/local/bin/pi-server}
ADDR=${PI_SERVER_ADDR:-127.0.0.1:3141}
SERVICE_USER=${PI_SERVER_SERVICE_USER:-${SUDO_USER:-root}}
AUTH_TOKEN=${PI_SERVER_AUTH_TOKEN:-}

[[ -x "$BIN" ]] || { echo "pi-server binary is not executable: $BIN" >&2; exit 1; }
id "$SERVICE_USER" >/dev/null 2>&1 || { echo "Unknown service user: $SERVICE_USER" >&2; exit 1; }
[[ -n "$AUTH_TOKEN" ]] || { echo "Set PI_SERVER_AUTH_TOKEN before installing the service." >&2; exit 1; }

SERVICE_HOME=$(getent passwd "$SERVICE_USER" | cut -d: -f6)
SERVICE_GROUP=$(id -gn "$SERVICE_USER")
PI_BINARY=$(runuser -u "$SERVICE_USER" -- bash -lc 'command -v pi' 2>/dev/null || true)
[[ -n "$PI_BINARY" ]] || { echo "Pi CLI is not available for $SERVICE_USER." >&2; exit 1; }

install -d -m 0700 -o "$SERVICE_USER" -g "$SERVICE_GROUP" /var/lib/pi-server
install -d -m 0700 /etc/pi-server
umask 077
cat >/etc/pi-server/pi-server.env <<ENV
PI_SERVER_ADDR=$ADDR
PI_SERVER_DATA_DIR=/var/lib/pi-server
PI_SERVER_ALLOWED_ROOTS=$SERVICE_HOME
PI_SERVER_AUTH_TOKEN=$AUTH_TOKEN
PI_SERVER_PI_BINARY=$PI_BINARY
ENV
chmod 0600 /etc/pi-server/pi-server.env

cat >/etc/systemd/system/pi-server.service <<UNIT
[Unit]
Description=Pi Server daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory=$SERVICE_HOME
EnvironmentFile=/etc/pi-server/pi-server.env
ExecStart=$BIN
Restart=on-failure
RestartSec=2
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/var/lib/pi-server $SERVICE_HOME

[Install]
WantedBy=multi-user.target
UNIT

systemctl daemon-reload
systemctl enable --now pi-server
