#!/usr/bin/env bash
set -euo pipefail

# ── Defaults ──────────────────────────────────────────────
SERVER_PORT="${SERVER_PORT:-3142}"
WEB_PORT="${WEB_PORT:-5174}"
AUTH_TOKEN="${AUTH_TOKEN:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVER_DIR="$SCRIPT_DIR/pi-server-exp"
WEB_DIR="$SCRIPT_DIR/pi-webby-exp"
DATA_DIR="${DATA_DIR:-$SCRIPT_DIR/.data/pi-server}"

# ── Parse args ────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    -s|--server-port) SERVER_PORT="$2"; shift 2 ;;
    -w|--web-port)    WEB_PORT="$2"; shift 2 ;;
    -t|--token)       AUTH_TOKEN="$2"; shift 2 ;;
    -d|--data-dir)    DATA_DIR="$2"; shift 2 ;;
    -h|--help)
      echo "Usage: start-exp-live-stack.sh [-s SERVER_PORT] [-w WEB_PORT] [-t AUTH_TOKEN] [-d DATA_DIR]"
      exit 0 ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# ── Checks ────────────────────────────────────────────────
for dir in "$SERVER_DIR" "$WEB_DIR"; do
  if [[ ! -d "$dir" ]]; then
    echo "Error: Required directory not found: $dir" >&2
    exit 1
  fi
done

for cmd in go pnpm; do
  if ! command -v "$cmd" &>/dev/null; then
    echo "Error: $cmd is not installed or not in PATH" >&2
    exit 1
  fi
done

# ── Detect Tailscale IP ───────────────────────────────────
TAILSCALE_IP=""
if command -v tailscale &>/dev/null; then
  TAILSCALE_IP=$(tailscale ip -4 2>/dev/null || true)
fi

if [[ -z "$TAILSCALE_IP" ]]; then
  for iface in tailscale0 utun*; do
    TAILSCALE_IP=$(ip -4 addr show "$iface" 2>/dev/null | grep -oP '(?<=inet\s)\d+(\.\d+){3}' | head -1 || true)
    [[ -n "$TAILSCALE_IP" ]] && break
  done
fi

if [[ -z "$TAILSCALE_IP" ]]; then
  TAILSCALE_IP=$(ip -4 addr show 2>/dev/null | grep -oP '(?<=inet\s)(?!127\.|169\.254\.)\d+(\.\d+){3}' | head -1 || true)
fi

if [[ -z "$TAILSCALE_IP" ]]; then
  echo "Warning: Could not detect a network IP. Using localhost."
  TAILSCALE_IP="127.0.0.1"
fi

# ── Setup ─────────────────────────────────────────────────
mkdir -p "$DATA_DIR"

ORIGINS="http://127.0.0.1:${WEB_PORT},http://localhost:${WEB_PORT},http://${TAILSCALE_IP}:${WEB_PORT}"

EXTENSION="$SERVER_DIR/extensions/session-title.ts"

export PI_SERVER_ADDR="0.0.0.0:${SERVER_PORT}"
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
  export PI_SERVER_ALLOW_INSECURE=1
fi

# ── Launch server in background ───────────────────────────
echo ""
echo "  Starting exp live stack"
echo "  ────────────────────────────────────"

cd "$SERVER_DIR"
go run ./cmd/pi-server &
SERVER_PID=$!

# Wait for server to be ready
echo "  Waiting for pi-server..."
for i in $(seq 1 30); do
  if curl -sf "http://127.0.0.1:${SERVER_PORT}/healthz" >/dev/null 2>&1; then
    echo "  pi-server is ready."
    break
  fi
  sleep 1
done

# ── Launch web app in background ──────────────────────────
cd "$WEB_DIR"
pnpm exec vite --host 0.0.0.0 --port "$WEB_PORT" --strictPort &
WEB_PID=$!

# ── Summary ───────────────────────────────────────────────
echo ""
echo "  ────────────────────────────────────"
echo "  Webby:      http://127.0.0.1:${WEB_PORT}  (phone: http://${TAILSCALE_IP}:${WEB_PORT})"
echo "  pi-server:  http://${TAILSCALE_IP}:${SERVER_PORT}"
echo "  Origins:    ${ORIGINS}"
if [[ -n "$AUTH_TOKEN" ]]; then
  echo "  Auth:       configured"
else
  printf "  Auth:       none \033[33m(trusted network only)\033[0m\n"
fi
echo ""
echo "  Press Ctrl+C to stop all services."
echo ""

# ── Cleanup on exit ───────────────────────────────────────
cleanup() {
  echo ""
  echo "  Stopping services..."
  kill "$SERVER_PID" "$WEB_PID" 2>/dev/null || true
  wait "$SERVER_PID" "$WEB_PID" 2>/dev/null || true
  echo "  Done."
}
trap cleanup EXIT INT TERM

# Wait for either process to exit
wait -n "$SERVER_PID" "$WEB_PID" 2>/dev/null || true
