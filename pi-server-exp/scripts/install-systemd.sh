#!/usr/bin/env bash
set -euo pipefail
BIN=${1:-/usr/local/bin/pi-server}
ADDR=${PI_SERVER_ADDR:-127.0.0.1:3141}
cat >/etc/systemd/system/pi-server.service <<UNIT
[Unit]
Description=Pi Server daemon
After=network.target

[Service]
ExecStart=$BIN --addr $ADDR
Restart=on-failure
RestartSec=2
Environment=PI_SERVER_DATA_DIR=/var/lib/pi-server
StateDirectory=pi-server

[Install]
WantedBy=multi-user.target
UNIT
systemctl daemon-reload
systemctl enable pi-server
