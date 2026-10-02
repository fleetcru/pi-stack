#!/usr/bin/env bash
# Provision pi-server on an Ubuntu/Debian VPS as a dedicated systemd service.
# Run as root. Provide either PI_SERVER_BINARY_PATH or PI_SERVER_REPO_URL.
set -euo pipefail

# Root is the simple default for a personal trusted VPS. Set
# PI_SERVER_SERVICE_USER to use a dedicated account instead.
SERVICE_USER="${PI_SERVER_SERVICE_USER:-root}"
SERVICE_GROUP="${PI_SERVER_SERVICE_GROUP:-$SERVICE_USER}"
SERVICE_HOME="${PI_SERVER_SERVICE_HOME:-$(if [[ "$SERVICE_USER" == "root" ]]; then echo /root; else echo "/home/$SERVICE_USER"; fi)}"
DATA_DIR="${PI_SERVER_DATA_DIR:-/var/lib/pi-server}"
PROJECT_ROOT="${PI_SERVER_PROJECT_ROOT:-$SERVICE_HOME/projects}"
BINARY_PATH="${PI_SERVER_BINARY_PATH:-}"
REPO_URL="${PI_SERVER_REPO_URL:-}"
REPO_REF="${PI_SERVER_REPO_REF:-main}"
INSTALL_DIR="${PI_SERVER_INSTALL_DIR:-/usr/local/lib/pi-server}"
LISTEN_ADDR="${PI_SERVER_ADDR:-127.0.0.1:3142}"
AUTH_TOKEN="${PI_SERVER_AUTH_TOKEN:-}"
PI_BINARY_OVERRIDE="${PI_SERVER_PI_BINARY:-}"
AGENT_BUNDLE="${PI_SERVER_AGENT_BUNDLE:-}"
TAILSCALE_AUTH_KEY="${PI_SERVER_TAILSCALE_AUTH_KEY:-}"

if [[ "${EUID}" -ne 0 ]]; then
  echo "Run this script as root (for example: sudo bash bootstrap-linux-vps.sh)." >&2
  exit 1
fi

if [[ ! -f /etc/debian_version ]]; then
  echo "This bootstrap script currently supports Ubuntu/Debian only." >&2
  exit 1
fi

[[ -n "$BINARY_PATH" || -n "$REPO_URL" ]] || { echo 'Set PI_SERVER_BINARY_PATH or PI_SERVER_REPO_URL before running.' >&2; exit 1; }
[[ "$AUTH_TOKEN" != *$'\n'* && "$AUTH_TOKEN" != *$'\r'* ]] || { echo 'Auth token contains a line break.' >&2; exit 1; }
for path in "$INSTALL_DIR" "$DATA_DIR" "$PROJECT_ROOT" "$SERVICE_HOME"; do
  [[ "$path" == /* && "$path" != *$'\n'* && "$path" != *$'\r'* && "$path" != *'"'* && "$path" != *'%'* ]] || { echo "Unsupported path: $path" >&2; exit 1; }
done
command -v systemctl >/dev/null && systemctl show-environment >/dev/null || { echo 'A running systemd is required.' >&2; exit 1; }
ENV_FILE='/etc/pi-server.env'
UNIT_FILE='/etc/systemd/system/pi-server.service'
if [[ -f "$UNIT_FILE" ]] && ! grep -Eq '^EnvironmentFile="?/etc/pi-server\.env"?$' "$UNIT_FILE"; then
  echo 'The existing service belongs to another installer. Use that installer to upgrade it.' >&2
  exit 1
fi
apt-get update
apt-get install -y ca-certificates curl git openssl

# Set PI_SERVER_TAILSCALE=1 to install/enroll Tailscale and bind pi-server to
# the machine's Tailnet IPv4. The default remains loopback-only.
if [[ "${PI_SERVER_TAILSCALE:-0}" == "1" ]]; then
  if ! command -v tailscale >/dev/null 2>&1; then
    curl -fsSL --connect-timeout 10 --max-time 120 https://tailscale.com/install.sh | sh
  fi
  if ! tailscale ip -4 >/dev/null 2>&1; then
    if [[ -z "$TAILSCALE_AUTH_KEY" && -t 0 ]]; then
      read -r -s -p "Tailscale auth key (input hidden): " TAILSCALE_AUTH_KEY
      echo
    fi
    if [[ -z "$TAILSCALE_AUTH_KEY" ]]; then
      echo "Tailscale is not enrolled. Enter an auth key when prompted, or set PI_SERVER_TAILSCALE_AUTH_KEY." >&2
      exit 1
    fi
    tailscale up --auth-key="$TAILSCALE_AUTH_KEY"
    unset TAILSCALE_AUTH_KEY
  fi
  TAILSCALE_IP="$(tailscale ip -4 | head -n 1)"
  if [[ -z "$TAILSCALE_IP" ]]; then
    echo "Could not determine the Tailscale IPv4 address after enrollment." >&2
    exit 1
  fi
  LISTEN_ADDR="${TAILSCALE_IP}:${LISTEN_ADDR##*:}"
fi

if ! id "$SERVICE_USER" >/dev/null 2>&1; then
  useradd --system --create-home --home-dir "$SERVICE_HOME" --shell /bin/bash "$SERVICE_USER"
fi
install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0750 "$DATA_DIR" "$PROJECT_ROOT"
install -d -o root -g root -m 0755 "$INSTALL_DIR"
umask 077
exec 9>"$INSTALL_DIR/.bootstrap.lock"
flock -n 9 || { echo 'Another bootstrap is running.' >&2; exit 1; }
STAGE=$(mktemp -d "$INSTALL_DIR/.bootstrap-XXXXXXXX")
BUNDLE_DIR=''
CHANGED=0
WAS_RUNNING=0
KEEP_STAGE=0
systemctl is-active --quiet pi-server && WAS_RUNNING=1
[[ ! -f "$INSTALL_DIR/pi-server" ]] || cp -p "$INSTALL_DIR/pi-server" "$STAGE/previous-binary"
[[ ! -f "$ENV_FILE" ]] || cp -p "$ENV_FILE" "$STAGE/previous-env"
[[ ! -f "$UNIT_FILE" ]] || cp -p "$UNIT_FILE" "$STAGE/previous-unit"
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if (( status != 0 && CHANGED )); then
    systemctl stop pi-server || true
    for pair in "previous-binary:$INSTALL_DIR/pi-server" "previous-env:$ENV_FILE" "previous-unit:$UNIT_FILE"; do
      local saved=${pair%%:*} target=${pair#*:}
      if [[ -f "$STAGE/$saved" ]]; then
        cp -p "$STAGE/$saved" "$target.rollback" && mv -f "$target.rollback" "$target" || KEEP_STAGE=1
      else rm -f "$target" || KEEP_STAGE=1; fi
    done
    systemctl daemon-reload || true
    if (( WAS_RUNNING )); then systemctl start pi-server || true; fi
  fi
  [[ -z "$BUNDLE_DIR" ]] || rm -rf "$BUNDLE_DIR"
  if (( KEEP_STAGE )); then echo "Rollback files retained at $STAGE" >&2; else rm -rf "$STAGE"; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

# Install/copy pi-server. For private repositories, copy a prebuilt binary to the
# VPS and set PI_SERVER_BINARY_PATH=/path/to/pi-server instead of cloning.
if [[ -n "$BINARY_PATH" ]]; then
  if [[ ! -f "$BINARY_PATH" ]]; then
    echo "PI_SERVER_BINARY_PATH does not exist: $BINARY_PATH" >&2
    exit 1
  fi
  install -m 0755 "$BINARY_PATH" "$STAGE/new-binary"
elif [[ -n "$REPO_URL" ]]; then
  command -v go >/dev/null || { echo 'Go is required for a source build. A prebuilt binary does not require Go.' >&2; exit 1; }
  BUILD_DIR="$STAGE/source"
  mkdir "$BUILD_DIR"
  git -C "$BUILD_DIR" init -q
  git -C "$BUILD_DIR" remote add origin "$REPO_URL"
  git -C "$BUILD_DIR" fetch --depth 1 origin "$REPO_REF"
  git -C "$BUILD_DIR" checkout --detach FETCH_HEAD
  if [[ -d "$BUILD_DIR/pi-server-exp/cmd/pi-server" ]]; then BUILD_DIR="$BUILD_DIR/pi-server-exp"; fi
  (cd "$BUILD_DIR" && go build -trimpath -o "$STAGE/new-binary" ./cmd/pi-server)
else
  echo "Set PI_SERVER_BINARY_PATH (recommended) or PI_SERVER_REPO_URL before running." >&2
  exit 1
fi

# Install Pi as the same account that will run pi-server. Pi's official
# installer bundles Node below ~/.local/share/pi-node, so discover both the Pi
# launcher and its private Node runtime rather than assuming system Node exists.
if [[ -n "$PI_BINARY_OVERRIDE" ]]; then
  PI_BINARY="$PI_BINARY_OVERRIDE"
else
  runuser -u "$SERVICE_USER" -- env HOME="$SERVICE_HOME" bash -lc \
    'set -euo pipefail; curl -fsSL --connect-timeout 10 --max-time 120 https://pi.dev/install.sh | sh'
  PI_BINARY="$(runuser -u "$SERVICE_USER" -- env HOME="$SERVICE_HOME" bash -lc 'command -v pi' || true)"
fi
if [[ -z "$PI_BINARY" ]]; then
  for candidate in "$SERVICE_HOME/.local/bin/pi" "$SERVICE_HOME/.npm-global/bin/pi"; do
    if [[ -x "$candidate" ]]; then
      PI_BINARY="$candidate"
      break
    fi
  done
fi
if [[ -z "$PI_BINARY" ]]; then
  PI_BINARY="$(find "$SERVICE_HOME/.local" -type f -name pi -perm -u+x -print -quit 2>/dev/null || true)"
fi
if [[ -z "$PI_BINARY" || ! -x "$PI_BINARY" ]]; then
  echo "Pi was not found after installation. Re-run with PI_SERVER_PI_BINARY=/absolute/path/to/pi." >&2
  exit 1
fi

NODE_BINARY="${PI_SERVER_NODE_BINARY:-}"
if [[ -z "$NODE_BINARY" ]]; then
  NODE_BINARY="$(find "$SERVICE_HOME/.local/share/pi-node" -type f -name node -perm -u+x -print -quit 2>/dev/null || true)"
fi
if [[ -z "$NODE_BINARY" ]]; then
  NODE_BINARY="$(command -v node || true)"
fi
if [[ -z "$NODE_BINARY" || ! -x "$NODE_BINARY" ]]; then
  echo "Node was not found. Set PI_SERVER_NODE_BINARY=/absolute/path/to/node and run again." >&2
  exit 1
fi
NODE_DIR="$(dirname "$NODE_BINARY")"
SERVICE_PATH="$NODE_DIR:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

# Fail provisioning now if Pi cannot use the exact Node/PATH systemd will use.
runuser -u "$SERVICE_USER" -- env HOME="$SERVICE_HOME" PATH="$SERVICE_PATH" "$PI_BINARY" --version

# Optional portable settings/auth/resources bundle produced by
# scripts/package-pi-agent-config.ps1. Import it after Pi installs so the
# caller's selected model/provider settings replace installer defaults.
if [[ -n "$AGENT_BUNDLE" ]]; then
  if [[ ! -f "$AGENT_BUNDLE" ]]; then
    echo "PI_SERVER_AGENT_BUNDLE does not exist: $AGENT_BUNDLE" >&2
    exit 1
  fi
  apt-get install -y unzip
  BUNDLE_DIR="$(mktemp -d)"
  unzip -q "$AGENT_BUNDLE" -d "$BUNDLE_DIR"
  if [[ ! -d "$BUNDLE_DIR/agent" ]]; then
    echo "Invalid Pi agent bundle: missing agent/ directory." >&2
    exit 1
  fi
  install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0700 "$SERVICE_HOME/.pi/agent"
  cp -a "$BUNDLE_DIR/agent/." "$SERVICE_HOME/.pi/agent/"
  chown -R "$SERVICE_USER:$SERVICE_GROUP" "$SERVICE_HOME/.pi"
  chmod 0700 "$SERVICE_HOME/.pi" "$SERVICE_HOME/.pi/agent"
  find "$SERVICE_HOME/.pi/agent" -maxdepth 1 -type f \( -name auth.json -o -name settings.json -o -name trust.json \) -exec chmod 0600 {} +
  if [[ -f "$BUNDLE_DIR/config/mcp/mcp.json" ]]; then
    install -d -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0700 "$SERVICE_HOME/.config/mcp"
    install -o "$SERVICE_USER" -g "$SERVICE_GROUP" -m 0600 "$BUNDLE_DIR/config/mcp/mcp.json" "$SERVICE_HOME/.config/mcp/mcp.json"
    echo "Imported MCP configuration: $SERVICE_HOME/.config/mcp/mcp.json"
  fi
  echo "Imported Pi agent configuration bundle: $AGENT_BUNDLE"
fi

# Preserve all existing settings, not only the bearer token.
if [[ -f "$ENV_FILE" ]]; then
  cp -p "$ENV_FILE" "$STAGE/new-env"
  GENERATED_TOKEN=0
  if [[ ${PI_SERVER_AUTH_TOKEN+x} ]]; then
    grep -v '^PI_SERVER_AUTH_TOKEN=' "$STAGE/new-env" >"$STAGE/env-token" || true
    escaped_token=${AUTH_TOKEN//\\/\\\\}
    escaped_token=${escaped_token//\"/\\\"}
    printf 'PI_SERVER_AUTH_TOKEN="%s"\n' "$escaped_token" >>"$STAGE/env-token"
    mv "$STAGE/env-token" "$STAGE/new-env"
  fi
  echo "Existing settings preserved in $ENV_FILE. Edit it to change the bind address or project roots."
else
if [[ -z "$AUTH_TOKEN" ]]; then
  AUTH_TOKEN="$(openssl rand -hex 32)"
  GENERATED_TOKEN=1
else
  GENERATED_TOKEN=0
fi


umask 077
escaped_token=${AUTH_TOKEN//\\/\\\\}
escaped_token=${escaped_token//\"/\\\"}
cat >"$STAGE/new-env" <<EOF
PI_SERVER_ADDR=$LISTEN_ADDR
PI_SERVER_DATA_DIR=$DATA_DIR
PI_SERVER_PI_BINARY=$PI_BINARY
PI_SERVER_NODE_BINARY=$NODE_BINARY
PI_SERVER_AUTH_TOKEN="$escaped_token"
PI_SERVER_ALLOWED_ROOTS=$PROJECT_ROOT
PI_SERVER_MAX_SESSIONS=${PI_SERVER_MAX_SESSIONS:-4}
EOF
fi
chmod 0600 "$STAGE/new-env"

if [[ -f "$UNIT_FILE" ]]; then
  # Older generated units duplicated the address as a flag, defeating edits
  # to EnvironmentFile. Remove that flag while retaining other unit settings.
  awk '/^ExecStart=/ {sub(/ --addr [^[:space:]]+/, "")} {print}' "$UNIT_FILE" >"$STAGE/new-unit"
else
cat >"$STAGE/new-unit" <<EOF
[Unit]
Description=Pi Server daemon
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory="$PROJECT_ROOT"
Environment="HOME=$SERVICE_HOME"
Environment="PATH=$SERVICE_PATH"
EnvironmentFile="$ENV_FILE"
ExecStart="$INSTALL_DIR/pi-server" --pairing-qr=false
Restart=on-failure
RestartSec=2
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
# Pi keeps its provider configuration/extensions in its service user's home.
ProtectHome=false
ReadWritePaths="$DATA_DIR" "$PROJECT_ROOT" "$SERVICE_HOME"

[Install]
WantedBy=multi-user.target
EOF
fi
chmod 0644 "$STAGE/new-unit"
chmod 0755 "$STAGE/new-binary"
if (( WAS_RUNNING )); then systemctl stop pi-server; fi
CHANGED=1
mv -f "$STAGE/new-binary" "$INSTALL_DIR/pi-server"
mv -f "$STAGE/new-env" "$ENV_FILE"
mv -f "$STAGE/new-unit" "$UNIT_FILE"
systemctl daemon-reload
systemctl enable pi-server
systemctl restart pi-server
sleep 2
systemctl is-active --quiet pi-server || { echo 'Service failed. Check journalctl -u pi-server -n 50.' >&2; exit 1; }
systemctl --no-pager --full status pi-server

echo
echo "pi-server is running on $LISTEN_ADDR"
echo "Pi launcher: $PI_BINARY"
echo "Node runtime: $NODE_BINARY"
echo "Allowed project root: $PROJECT_ROOT"
echo "Service logs: journalctl -u pi-server -f"
if [[ "$GENERATED_TOKEN" == "1" ]]; then
  echo ""
  echo "Generated a credential, stored privately in $ENV_FILE."
  echo "Read it with: sudo grep PI_SERVER_AUTH_TOKEN $ENV_FILE"
fi
