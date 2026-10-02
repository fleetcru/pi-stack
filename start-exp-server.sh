#!/usr/bin/env bash
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────
PORT="${PORT:-3142}"
AUTH_TOKEN="${AUTH_TOKEN-${PI_SERVER_AUTH_TOKEN:-}}"
OPEN_ADMIN="${OPEN_ADMIN:-0}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/dev-launcher-common.sh"
SERVER_DIR="$SCRIPT_DIR/pi-server-exp"
DATA_DIR="${DATA_DIR:-$SCRIPT_DIR/.data/pi-server}"

# ── Parse args ────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--port)       dev_need_value "$@"; PORT="$2"; shift 2 ;;
    -t|--token)      dev_need_value "$@"; AUTH_TOKEN="$2"; shift 2 ;;
    -d|--data-dir)   dev_need_value "$@"; DATA_DIR="$2"; shift 2 ;;
    --open-admin) OPEN_ADMIN=1; shift ;;
    -h|--help)
      echo "Usage: start-exp-server.sh [-p PORT] [-t AUTH_TOKEN] [-d DATA_DIR]"
      echo "  -p, --port       Server port (default: 3142)"
      echo "  -t, --token      Auth token (omit for no auth)"
      echo "  -d, --data-dir   Data directory (default: .data/pi-server)"
      echo "      --open-admin Deprecated. Use the terminal pairing QR or Webby/Desktop."
      exit 0 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

dev_port "$PORT"
dev_assert_port_free "$PORT"
# ── Checks ────────────────────────────────────────────────
if [[ ! -d "$SERVER_DIR" ]]; then
  echo "Error: pi-server-exp not found at $SERVER_DIR" >&2
  exit 1
fi

if ! command -v go &>/dev/null; then
  echo "Error: Go is not installed or not in PATH" >&2
  exit 1
fi

# ── Detect Tailscale IP ───────────────────────────────────
TAILSCALE_IP=""
if command -v tailscale &>/dev/null; then
  TAILSCALE_IP=$(tailscale ip -4 2>/dev/null || true)
fi

if [[ -z "$TAILSCALE_IP" ]]; then
  # Fallback: try common Tailscale interface names
  for iface in tailscale0 utun*; do
    TAILSCALE_IP=$(ip -4 addr show "$iface" 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || true)
    [[ -n "$TAILSCALE_IP" ]] && break
  done
fi

HOME_LAN_IP=$(ip -4 addr show 2>/dev/null | grep -oP '(?<=inet\s)(?:10\.\d+\.\d+\.\d+|192\.168\.\d+\.\d+|172\.(?:1[6-9]|2[0-9]|3[0-1])\.\d+\.\d+)' | head -1 || true)

if [[ -z "$TAILSCALE_IP" && -z "$HOME_LAN_IP" ]]; then
  echo "Warning: Could not detect a home-LAN or Tailscale address. Clients may not reach the server." >&2
fi

# Build first and execute the server directly, not a go run parent process.
BUILD_DIR=$(mktemp -d)
SERVER_PID=''
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  [[ -z "$SERVER_PID" ]] || kill -TERM "$SERVER_PID" 2>/dev/null || true
  [[ -z "$SERVER_PID" ]] || wait "$SERVER_PID" 2>/dev/null || true
  rm -rf "$BUILD_DIR"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
(cd "$SERVER_DIR" && go build -trimpath -o "$BUILD_DIR/pi-server" ./cmd/pi-server)
# ── Setup ─────────────────────────────────────────────────
mkdir -p "$DATA_DIR"
DATA_DIR=$(cd "$DATA_DIR" && pwd)

# With no explicit allowlist, pi-server's built-in CORS policy accepts
# loopback, private home-LAN, and Tailscale browser origins.
EXTENSION="$SERVER_DIR/extensions/session-title.ts"

BIND_HOST="0.0.0.0"
export PI_SERVER_ADDR="${BIND_HOST}:${PORT}"
export PI_SERVER_CWD="$SCRIPT_DIR"
export PI_SERVER_DATA_DIR="$DATA_DIR"
export PI_SERVER_ALLOWED_ROOTS="${PI_SERVER_ALLOWED_ROOTS:-$SCRIPT_DIR}"
# Preserve an explicitly configured CORS allowlist.

if [[ -f "$EXTENSION" ]]; then
  export PI_SERVER_PI_EXTENSIONS="$EXTENSION"
else
  unset PI_SERVER_PI_EXTENSIONS 2>/dev/null || true
fi

if [[ -n "$AUTH_TOKEN" ]]; then
  export PI_SERVER_AUTH_TOKEN="$AUTH_TOKEN"
else
  unset PI_SERVER_AUTH_TOKEN 2>/dev/null || true
fi
unset PI_SERVER_ALLOW_INSECURE 2>/dev/null || true

# ── Launch ────────────────────────────────────────────────
echo ""
echo "  pi-server-exp"
echo "  ────────────────────────────────────"
echo "  Bind:      ${BIND_HOST}:${PORT}"
[[ -n "$HOME_LAN_IP" ]] && echo "  Home LAN:  http://${HOME_LAN_IP}:${PORT}"
[[ -n "$TAILSCALE_IP" ]] && echo "  Tailscale: http://${TAILSCALE_IP}:${PORT}"
echo "  Local:     http://127.0.0.1:${PORT}"
echo "  Data:      ${DATA_DIR}"
echo "  Browser:   ${PI_SERVER_ALLOWED_ORIGINS:-automatic private-network policy}"
if [[ -n "$AUTH_TOKEN" ]]; then
  echo "  Auth:      configured"
else
  printf "  Auth:      none \033[33m(trusting home LAN/Tailscale)\033[0m\n"
fi
echo ""

if [[ "$OPEN_ADMIN" == 1 ]]; then
  printf '%s\n' 'Warning: --open-admin is obsolete. Use Webby/Desktop for administration, or scan the terminal pairing QR in Companion.' >&2
fi

"$BUILD_DIR/pi-server" &
SERVER_PID=$!
if wait "$SERVER_PID"; then status=0; else status=$?; fi
SERVER_PID=''
exit "$status"
