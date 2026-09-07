#!/usr/bin/env bash
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────
PORT="${PORT:-3142}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
OPEN_ADMIN="${OPEN_ADMIN:-0}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$SCRIPT_DIR/pi-server-exp"
DATA_DIR="${DATA_DIR:-$SCRIPT_DIR/.data/pi-server}"

# ── Parse args ────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--port)       PORT="$2"; shift 2 ;;
    -t|--token)      AUTH_TOKEN="$2"; shift 2 ;;
    -d|--data-dir)   DATA_DIR="$2"; shift 2 ;;
    --open-admin) OPEN_ADMIN=1; shift ;;
    -h|--help)
      echo "Usage: start-exp-server.sh [-p PORT] [-t AUTH_TOKEN] [-d DATA_DIR]"
      echo "  -p, --port       Server port (default: 3142)"
      echo "  -t, --token      Auth token (omit for no auth)"
      echo "  -d, --data-dir   Data directory (default: .data/pi-server)"
      echo "      --open-admin      Open Admin after the server is ready"
      exit 0 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

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

# ── Setup ─────────────────────────────────────────────────
mkdir -p "$DATA_DIR"

# With no explicit allowlist, pi-server's built-in CORS policy accepts
# loopback, private home-LAN, and Tailscale browser origins.
EXTENSION="$SERVER_DIR/extensions/session-title.ts"

BIND_HOST="0.0.0.0"
export PI_SERVER_ADDR="${BIND_HOST}:${PORT}"
export PI_SERVER_CWD="$SCRIPT_DIR"
export PI_SERVER_DATA_DIR="$DATA_DIR"
export PI_SERVER_ALLOWED_ROOTS="$SCRIPT_DIR"
unset PI_SERVER_ALLOWED_ORIGINS 2>/dev/null || true

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
echo "  Browser:   localhost, home LAN, and Tailscale origins allowed"
if [[ -n "$AUTH_TOKEN" ]]; then
  echo "  Auth:      configured"
else
  printf "  Auth:      none \033[33m(trusting home LAN/Tailscale)\033[0m\n"
fi
echo ""

if [[ "$OPEN_ADMIN" == "1" ]]; then
  ADMIN_URL="http://127.0.0.1:${PORT}/admin/"
  (
    for _ in $(seq 1 60); do
      if curl -fsS "http://127.0.0.1:${PORT}/healthz" >/dev/null 2>&1; then
        if command -v xdg-open >/dev/null 2>&1; then xdg-open "$ADMIN_URL" >/dev/null 2>&1
        elif command -v open >/dev/null 2>&1; then open "$ADMIN_URL" >/dev/null 2>&1
        else echo "  Admin:      $ADMIN_URL"; fi
        exit 0
      fi
      sleep 0.5
    done
  ) &
  echo "  Pairing:    Create a trusted device in Admin, then scan its QR from Companion."
fi

cd "$SERVER_DIR"
exec go run ./cmd/pi-server
