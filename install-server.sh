#!/usr/bin/env bash
set -euo pipefail

# ── Config ────────────────────────────────────────────────
REPO="fleetcru/pi-stack"
SERVICE_NAME="pi-server"
INSTALL_DIR="/opt/pi-server"
DATA_DIR="/var/lib/pi-server"
CONFIG_DIR="/etc/pi-server"
USER="pi-server"
PORT="${PI_SERVER_PORT:-3142}"
AUTH_TOKEN="${PI_SERVER_AUTH_TOKEN:-}"

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

# ── Create user and directories ───────────────────────────
info "Creating service user and directories..."

if ! id "$USER" &>/dev/null; then
  useradd --system --no-create-home --shell /usr/sbin/nologin "$USER"
  ok "Created user: $USER"
fi

mkdir -p "$INSTALL_DIR" "$DATA_DIR" "$CONFIG_DIR"

# ── Download or build binary ──────────────────────────────
info "Downloading pi-server for ${OS}/${ARCH}..."

BINARY_URL="https://github.com/${REPO}/releases/latest/download/pi-server-${OS}-${ARCH}"

if curl -sfSL --head "$BINARY_URL" &>/dev/null; then
  curl -sfSL "$BINARY_URL" -o "${INSTALL_DIR}/pi-server"
  chmod +x "${INSTALL_DIR}/pi-server"
  ok "Downloaded pre-built binary"
else
  warn "No pre-built binary found. Building from source..."
  
  # Install Go if needed
  if ! command -v go &>/dev/null; then
    info "Installing Go..."
    GO_VERSION="1.23.4"
    GO_URL="https://go.dev/dl/go${GO_VERSION}.${OS}-${ARCH}.tar.gz"
    curl -sfSL "$GO_URL" | tar -C /usr/local -xzf -
    export PATH="/usr/local/go/bin:$PATH"
    ok "Installed Go ${GO_VERSION}"
  fi

  # Clone and build
  TMPDIR=$(mktemp -d)
  info "Cloning repository..."
  git clone --depth 1 "https://github.com/${REPO}.git" "$TMPDIR/pi-stack"
  
  info "Building pi-server..."
  cd "$TMPDIR/pi-stack/pi-server-exp"
  go build -o "${INSTALL_DIR}/pi-server" ./cmd/pi-server
  cd /
  rm -rf "$TMPDIR"
  ok "Built from source"
fi

# ── Write config ──────────────────────────────────────────
info "Writing configuration..."

cat > "${CONFIG_DIR}/pi-server.env" <<EOF
# pi-server configuration
# Edit this file, then run: systemctl restart pi-server

PI_SERVER_ADDR=0.0.0.0:${PORT}
PI_SERVER_DATA_DIR=${DATA_DIR}
PI_SERVER_ALLOWED_ROOTS=${DATA_DIR}
PI_SERVER_ALLOW_INSECURE=1

# Set a token to require authentication:
# PI_SERVER_AUTH_TOKEN=your-secret-token
EOF

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
User=${USER}
Group=${USER}
EnvironmentFile=${CONFIG_DIR}/pi-server.env
ExecStart=${INSTALL_DIR}/pi-server
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
ReadWritePaths=${DATA_DIR}

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
ok "Service created: ${SERVICE_NAME}.service"

# ── Start service ─────────────────────────────────────────
info "Starting pi-server..."

chown -R "${USER}:${USER}" "$DATA_DIR"
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
echo -e "${YELLOW}  Set PI_SERVER_AUTH_TOKEN in ${CONFIG_DIR}/pi-server.env${NC}"
echo -e "${YELLOW}  before exposing this to the internet!${NC}"
echo -e "${GREEN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
