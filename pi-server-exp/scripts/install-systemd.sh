#!/usr/bin/env bash
set -euo pipefail

[[ $EUID -eq 0 ]] || { echo 'Run this script with sudo.' >&2; exit 1; }
command -v systemctl >/dev/null || { echo 'systemd is required.' >&2; exit 1; }
systemctl show-environment >/dev/null || { echo 'systemd is not running.' >&2; exit 1; }

BIN=${1:-/usr/local/bin/pi-server}
ADDR=${PI_SERVER_ADDR:-127.0.0.1:3142}
SERVICE_USER=${PI_SERVER_SERVICE_USER:-${SUDO_USER:-root}}
AUTH_TOKEN=${PI_SERVER_AUTH_TOKEN:-}

[[ -x "$BIN" ]] || { echo "pi-server binary is not executable: $BIN" >&2; exit 1; }
id "$SERVICE_USER" >/dev/null 2>&1 || { echo "Unknown service user: $SERVICE_USER" >&2; exit 1; }
[[ -f /etc/pi-server/pi-server.env || -n "$AUTH_TOKEN" ]] || { echo 'Set PI_SERVER_AUTH_TOKEN for the first installation.' >&2; exit 1; }
[[ "$AUTH_TOKEN" != *$'\n'* && "$AUTH_TOKEN" != *$'\r'* ]] || { echo 'Auth token contains a line break.' >&2; exit 1; }

SERVICE_HOME=$(getent passwd "$SERVICE_USER" | cut -d: -f6)
SERVICE_GROUP=$(id -gn "$SERVICE_USER")
PI_BINARY=$(runuser -u "$SERVICE_USER" -- bash -lc 'command -v pi' 2>/dev/null || true)
[[ -n "$PI_BINARY" ]] || { echo "Pi CLI is not available for $SERVICE_USER." >&2; exit 1; }
SERVICE_PATH="$(dirname "$PI_BINARY"):$PATH"
runuser -u "$SERVICE_USER" -- env HOME="$SERVICE_HOME" PATH="$SERVICE_PATH" "$PI_BINARY" --version >/dev/null
for path in "$BIN" "$SERVICE_HOME" "$SERVICE_PATH"; do
  [[ "$path" != *$'\n'* && "$path" != *$'\r'* && "$path" != *'"'* && "$path" != *'%'* ]] || { echo "Unsupported path: $path" >&2; exit 1; }
done

install -d -m 0700 -o "$SERVICE_USER" -g "$SERVICE_GROUP" /var/lib/pi-server
install -d -m 0700 /etc/pi-server
umask 077
if [[ -f /etc/pi-server/pi-server.env ]]; then
  echo 'Existing configuration and credentials preserved. Edit /etc/pi-server/pi-server.env to change settings.'
else
escaped_token=${AUTH_TOKEN//\\/\\\\}
escaped_token=${escaped_token//\"/\\\"}
cat >/etc/pi-server/pi-server.env <<ENV
PI_SERVER_ADDR=$ADDR
PI_SERVER_DATA_DIR=/var/lib/pi-server
PI_SERVER_ALLOWED_ROOTS=$SERVICE_HOME
PI_SERVER_AUTH_TOKEN="$escaped_token"
PI_SERVER_PI_BINARY=$PI_BINARY
ENV
fi
chmod 0600 /etc/pi-server/pi-server.env

if [[ -f /etc/systemd/system/pi-server.service ]]; then
  echo 'Existing service unit preserved.'
else
cat >/etc/systemd/system/pi-server.service <<UNIT
[Unit]
Description=Pi Server daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory="$SERVICE_HOME"
Environment="HOME=$SERVICE_HOME"
Environment="PATH=$SERVICE_PATH"
EnvironmentFile=/etc/pi-server/pi-server.env
ExecStart="$BIN" --pairing-qr=false
Restart=on-failure
RestartSec=2
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=read-only
ReadWritePaths=/var/lib/pi-server "$SERVICE_HOME"

[Install]
WantedBy=multi-user.target
UNIT
fi

systemctl daemon-reload
systemctl enable pi-server
systemctl restart pi-server
sleep 2
systemctl is-active --quiet pi-server || { echo 'Service failed. Check journalctl -u pi-server -n 50.' >&2; exit 1; }
echo 'pi-server is running. Logs: journalctl -u pi-server -f'
