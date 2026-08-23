#!/usr/bin/env bash
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────
PORT="${PORT:-3142}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
ALLOW_INSECURE="${ALLOW_INSECURE:-0}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$SCRIPT_DIR/pi-server-exp"
DATA_DIR="${DATA_DIR:-$SCRIPT_DIR/.data/pi-server}"

# ── Parse args ────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--port)       PORT="$2"; shift 2 ;;
    -t|--token)      AUTH_TOKEN="$2"; shift 2 ;;
    -d|--data-dir)   DATA_DIR="$2"; shift 2 ;;
    --allow-insecure) ALLOW_INSECURE=1; shift ;;
    -h|--help)
      echo "Usage: start-exp-server.sh [-p PORT] [-t AUTH_TOKEN] [-d DATA_DIR]"
      echo "  -p, --port       Server port (default: 3142)"
      echo "  -t, --token      Auth token (omit for no auth)"
      echo "  -d, --data-dir   Data directory (default: .data/pi-server)"
      echo "      --allow-insecure  Bind to the network without a token"
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

if [[ -z "$TAILSCALE_IP" ]]; then
  # Fallback: first non-loopback, non-link-local IP
  TAILSCALE_IP=$(ip -4 addr show 2>/dev/null | grep -oP '(?<=inet\s)(?!127\.|169\.254\.)\d+(\.\d+){3}' | head -1 || true)
fi

if [[ -z "$TAILSCALE_IP" ]]; then
  echo "Warning: Could not detect a network IP. Clients may not reach the server." >&2
  TAILSCALE_IP="127.0.0.1"
fi

# ── Setup ─────────────────────────────────────────────────
mkdir -p "$DATA_DIR"

# Allow origins from Tailscale IP, localhost, and common web dev ports
ORIGINS="http://127.0.0.1:5173,http://localhost:5173,http://127.0.0.1:5174,http://localhost:5174,http://${TAILSCALE_IP}:5173,http://${TAILSCALE_IP}:5174"

EXTENSION="$SERVER_DIR/extensions/session-title.ts"

BIND_HOST="127.0.0.1"
if [[ -n "$AUTH_TOKEN" || "$ALLOW_INSECURE" == "1" ]]; then
  BIND_HOST="0.0.0.0"
fi
export PI_SERVER_ADDR="${BIND_HOST}:${PORT}"
export PI_SERVER_CWD="$SCRIPT_DIR"
export PI_SERVER_DATA_DIR="$DATA_DIR"
export PI_SERVER_ALLOWED_ROOTS="$SCRIPT_DIR"
export PI_SERVER_ALLOWED_ORIGINS="$ORIGINS"

if [[ -f "$EXTENSION" ]]; then
  export PI_SERVER_PI_EXTENSIONS="$EXTENSION"
else
  unset PI_SERVER_PI_EXTENSIONS 2>/dev/null || true
fi

if [[ -n "$AUTH_TOKEN" ]]; then
  export PI_SERVER_AUTH_TOKEN="$AUTH_TOKEN"
  unset PI_SERVER_ALLOW_INSECURE 2>/dev/null || true
else
  unset PI_SERVER_AUTH_TOKEN 2>/dev/null || true
  if [[ "$ALLOW_INSECURE" == "1" ]]; then
    export PI_SERVER_ALLOW_INSECURE=1
  else
    unset PI_SERVER_ALLOW_INSECURE 2>/dev/null || true
  fi
fi

# ── Launch ────────────────────────────────────────────────
echo ""
echo "  pi-server-exp"
echo "  ────────────────────────────────────"
echo "  Bind:      ${BIND_HOST}:${PORT}"
echo "  Tailscale: http://${TAILSCALE_IP}:${PORT}"
echo "  Data:      ${DATA_DIR}"
echo "  Origins:   ${ORIGINS}"
if [[ -n "$AUTH_TOKEN" ]]; then
  echo "  Auth:      configured"
else
  printf "  Auth:      none \033[33m(trusted network only)\033[0m\n"
fi
echo ""

cd "$SERVER_DIR"
exec go run ./cmd/pi-server
