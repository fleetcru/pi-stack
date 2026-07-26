#!/usr/bin/env bash
set -euo pipefail

NODE_DIR="$(env HOME=/root BASH_ENV=/root/.bash_env bash -lc 'dirname "$(command -v node)"')"

mkdir -p /etc/systemd/system/pi-server.service.d

cat > /etc/systemd/system/pi-server.service.d/10-node-path.conf <<PATH
[Service]
Environment="PATH=$NODE_DIR:/root/.local/bin:/root/.cargo/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
PATH

systemctl daemon-reload
systemctl restart pi-server
systemctl status pi-server --no-pager
