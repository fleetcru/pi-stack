#!/usr/bin/env bash
set -euo pipefail

# ── Config ────────────────────────────────────────────────
REPO="fleetcru/pi-stack"
SERVICE_NAME="pi-server"
INSTALL_DIR="/opt/pi-server"
DATA_DIR="/var/lib/pi-server"
CONFIG_DIR="/etc/pi-server"
SERVICE_USER="${PI_SERVER_SERVICE_USER:-${SUDO_USER:-root}}"
PORT="${PI_SERVER_PORT:-3142}"
AUTH_TOKEN="${PI_SERVER_AUTH_TOKEN:-}"
ALLOW_INSECURE="${PI_SERVER_ALLOW_INSECURE:-}"
ALLOW_SOURCE_BUILD="${PI_SERVER_ALLOW_SOURCE_BUILD:-}"
SOURCE_REVISION="${PI_SERVER_SOURCE_REVISION:-098d635625f0bdb1edbb2e84f148d093afcfe8da}"

# ── Colors ────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[info]${NC} $*"; }
ok()    { echo -e "${GREEN}[ok]${NC} $*"; }
warn()  { echo -e "${YELLOW}[warn]${NC} $*"; }
fail()  { echo -e "${RED}[error]${NC} $*" >&2; exit 1; }

# ── Parse flags ──────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --insecure) ALLOW_INSECURE=1 ;;
  esac
done

# ── Checks ────────────────────────────────────────────────
[[ $EUID -eq 0 ]] || fail "Run as root: curl -sSL ... | sudo bash"

ARCH=$(uname -m)
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) fail "Unsupported architecture: $ARCH" ;;
esac

OS=$(uname -s | tr '[:upper:]' '[:lower:]')
[[ "$OS" == "linux" ]] || fail "This installer supports Linux only. For Windows/macOS, see the README."

command -v systemctl &>/dev/null || fail "systemd is required but not found."
command -v getent &>/dev/null || fail "getent is required but not found."
id "$SERVICE_USER" &>/dev/null || fail "Service user does not exist: $SERVICE_USER"
SERVICE_HOME=$(getent passwd "$SERVICE_USER" | cut -d: -f6)
[[ -n "$SERVICE_HOME" && -d "$SERVICE_HOME" ]] || fail "Could not resolve home directory for $SERVICE_USER"

# ── Install dependencies ──────────────────────────────────
info "Checking dependencies..."

if ! command -v curl &>/dev/null; then
  if command -v apt-get &>/dev/null; then
    apt-get update -qq && apt-get install -y -qq curl
  elif command -v yum &>/dev/null; then
    yum install -y -q curl
  else
    fail "curl is required. Install it manually."
  fi
fi

# ── Create directories ────────────────────────────────────
info "Creating service directories..."
umask 077
mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$CONFIG_DIR"

# ── Download or build binary ──────────────────────────────
info "Downloading pi-server for ${OS}/${ARCH}..."

BINARY_URL="https://github.com/${REPO}/releases/latest/download/pi-server-${OS}-${ARCH}"

CHECKSUM_URL="https://github.com/${REPO}/releases/latest/download/SHA256SUMS"
if curl -sfSL --head "$BINARY_URL" &>/dev/null; then
  tmp_binary=$(mktemp)
  tmp_checksums=$(mktemp)
  curl -sfSL "$BINARY_URL" -o "$tmp_binary"
  curl -sfSL "$CHECKSUM_URL" -o "$tmp_checksums" || fail "Release checksum file is unavailable"
  expected=$(awk -v name="pi-server-${OS}-${ARCH}" '$2 == name || $2 == "*" name {print $1; exit}' "$tmp_checksums")
  [[ -n "$expected" ]] || fail "Release checksum does not contain pi-server-${OS}-${ARCH}"
  actual=$(sha256sum "$tmp_binary" | awk '{print $1}')
  [[ "$actual" == "$expected" ]] || fail "Downloaded binary checksum mismatch"
  install -m 0755 "$tmp_binary" "${INSTALL_DIR}/pi-server"
  rm -f "$tmp_binary" "$tmp_checksums"
  ok "Downloaded and verified pre-built binary"
elif [[ "$ALLOW_SOURCE_BUILD" == "1" ]]; then
  warn "No pre-built binary found. Building the current default branch because PI_SERVER_ALLOW_SOURCE_BUILD=1."
  
  # Install Go if needed
  if ! command -v go &>/dev/null; then
    info "Installing Go..."
    GO_VERSION="1.23.4"
    GO_URL="https://go.dev/dl/go${GO_VERSION}.${OS}-${ARCH}.tar.gz"
    curl -sfSL "$GO_URL" | tar -C /usr/local -xzf -
    export PATH="/usr/local/go/bin:$PATH"
    ok "Installed Go ${GO_VERSION}"
  fi

  command -v git &>/dev/null || fail "git is required for source builds"
  # Fetch and build the pinned revision.
  TMPDIR=$(mktemp -d)
  info "Fetching pinned source revision ${SOURCE_REVISION}..."
  git -C "$TMPDIR" init -q
  git -C "$TMPDIR" remote add origin "https://github.com/${REPO}.git"
  git -C "$TMPDIR" fetch -q --depth 1 origin "$SOURCE_REVISION"
  git -C "$TMPDIR" checkout -q --detach FETCH_HEAD

  info "Building pi-server..."
  cd "$TMPDIR/pi-server-exp"
  go build -o "${INSTALL_DIR}/pi-server" ./cmd/pi-server
  cd /
  rm -rf "$TMPDIR"
  ok "Built from source"
else
  fail "No verified release binary found. Set PI_SERVER_ALLOW_SOURCE_BUILD=1 to explicitly permit a source build."
fi

# ── Generate auth token if needed ────────────────────────
if [[ -z "$AUTH_TOKEN" ]]; then
  AUTH_TOKEN=$(head -c 32 /dev/urandom | base64 | tr -d '/+=' | head -c 32)
  info "Generated an auth token"
fi

# ── Write config ──────────────────────────────────────────
info "Writing configuration..."
PI_BINARY=$(runuser -u "$SERVICE_USER" -- bash -lc 'command -v pi' 2>/dev/null || true)
[[ -n "$PI_BINARY" ]] || fail "Pi CLI is not available for service user $SERVICE_USER"

cat > "${CONFIG_DIR}/pi-server.env" <<EOF
# pi-server configuration
# Edit this file, then run: systemctl restart pi-server

PI_SERVER_ADDR=0.0.0.0:${PORT}
PI_SERVER_DATA_DIR=${DATA_DIR}
PI_SERVER_ALLOWED_ROOTS=${SERVICE_HOME}
PI_SERVER_AUTH_TOKEN=${AUTH_TOKEN}
PI_SERVER_PI_BINARY=${PI_BINARY}
EOF

# Only add ALLOW_INSECURE if explicitly requested
if [[ "$ALLOW_INSECURE" == "1" ]]; then
  echo "PI_SERVER_ALLOW_INSECURE=1" >> "${CONFIG_DIR}/pi-server.env"
  warn "Running in INSECURE mode — auth token will not be enforced"
fi

chmod 0600 "${CONFIG_DIR}/pi-server.env"
ok "Config written to ${CONFIG_DIR}/pi-server.env"

# ── Write systemd service ────────────────────────────────
info "Creating systemd service..."

cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=pi-server — Pi coding agent hub
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=${SERVICE_USER}
Group=$(id -gn "$SERVICE_USER")
WorkingDirectory=${SERVICE_HOME}
EnvironmentFile=${CONFIG_DIR}/pi-server.env
ExecStart=${INSTALL_DIR}/pi-server
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=true
ReadWritePaths=${DATA_DIR} ${SERVICE_HOME}

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
ok "Service created: ${SERVICE_NAME}.service"

# ── Start service ─────────────────────────────────────────
info "Starting pi-server..."

chown -R "${SERVICE_USER}:$(id -gn "$SERVICE_USER")" "$DATA_DIR"
systemctl enable --now "$SERVICE_NAME"

# Wait for startup
sleep 2
if systemctl is-active --quiet "$SERVICE_NAME"; then
  ok "pi-server is running"
else
  warn "pi-server may have failed to start. Check: journalctl -u ${SERVICE_NAME} -n 20"
fi

# ── Summary ───────────────────────────────────────────────
SERVER_IP=$(curl -sf https://ifconfig.me 2>/dev/null || hostname -I | awk '{print $1}')

echo ""
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "  pi-server installed and running!"
echo ""
echo -e "  URL:      ${CYAN}http://${SERVER_IP}:${PORT}${NC}"
echo -e "  Config:   ${CONFIG_DIR}/pi-server.env"
echo -e "  Data:     ${DATA_DIR}"
echo -e "  Logs:     ${CYAN}journalctl -u ${SERVICE_NAME} -f${NC}"
echo ""
echo -e "  Commands:"
echo -e "    ${CYAN}systemctl restart ${SERVICE_NAME}${NC}   Restart"
echo -e "    ${CYAN}systemctl stop ${SERVICE_NAME}${NC}      Stop"
echo -e "    ${CYAN}systemctl status ${SERVICE_NAME}${NC}    Status"
echo ""
echo -e "  Auth token: stored in ${CONFIG_DIR}/pi-server.env (mode 600)"
echo -e "${YELLOW}  Read it with: sudo grep PI_SERVER_AUTH_TOKEN ${CONFIG_DIR}/pi-server.env${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
