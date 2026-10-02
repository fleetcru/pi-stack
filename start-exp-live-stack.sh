#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/dev-launcher-common.sh"
SERVER_DIR="$SCRIPT_DIR/pi-server-exp"
WEB_DIR="$SCRIPT_DIR/pi-webby-exp"
DATA_DIR="${DATA_DIR:-$SCRIPT_DIR/.data/pi-server}"
SERVER_PORT="${SERVER_PORT:-3142}"
WEB_PORT="${WEB_PORT:-5174}"
AUTH_TOKEN="${AUTH_TOKEN-${PI_SERVER_AUTH_TOKEN:-}}"
HEALTH_TIMEOUT="${HEALTH_TIMEOUT:-30}"
while (( $# )); do
  case "$1" in
    -s|--server-port) dev_need_value "$@"; SERVER_PORT=$2; shift 2 ;;
    -w|--web-port) dev_need_value "$@"; WEB_PORT=$2; shift 2 ;;
    -t|--token) dev_need_value "$@"; AUTH_TOKEN=$2; shift 2 ;;
    -d|--data-dir) dev_need_value "$@"; DATA_DIR=$2; shift 2 ;;
    -h|--help)
      printf '%s\n' 'Usage: start-exp-live-stack.sh [-s SERVER_PORT] [-w WEB_PORT] [-t TOKEN] [-d DATA_DIR]' \
        'Runs server and Webby. Ctrl+C stops both, including their child processes.'
      exit 0 ;;
    *) dev_fail "Unknown option: $1" ;;
  esac
done
dev_port "$SERVER_PORT"
dev_port "$WEB_PORT"
(( 10#$SERVER_PORT != 10#$WEB_PORT )) || dev_fail 'Server and web ports must differ.'
[[ "$HEALTH_TIMEOUT" =~ ^[1-9][0-9]{0,2}$ ]] || dev_fail 'HEALTH_TIMEOUT must be between 1 and 999 seconds.'
for command in go pnpm curl; do dev_require "$command"; done
for dir in "$SERVER_DIR" "$WEB_DIR" "$WEB_DIR/node_modules"; do
  [[ -d "$dir" ]] || dev_fail "Required directory is missing: $dir. Install Webby dependencies first."
done
dev_assert_port_free "$SERVER_PORT"
dev_assert_port_free "$WEB_PORT"
NETWORK_IP=$(dev_network_ip || true)
NETWORK_IP=${NETWORK_IP:-127.0.0.1}
BUILD_DIR=$(mktemp -d)
SERVER_PID=''
WEB_PID=''
# Each background job gets its own process group. This includes pnpm's Vite
# child, which otherwise survives when only the pnpm parent is stopped.
set -m
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  for pid in "$SERVER_PID" "$WEB_PID"; do
    [[ -n "$pid" ]] || continue
    # Signal the dedicated group even if its leader exited. Vite can still
    # be alive after pnpm dies, when jobs -pr no longer lists the parent.
    kill -TERM -- "-$pid" 2>/dev/null || true
  done
  for pid in "$SERVER_PID" "$WEB_PID"; do [[ -z "$pid" ]] || wait "$pid" 2>/dev/null || true; done
  rm -rf "$BUILD_DIR"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
(cd "$SERVER_DIR" && go build -trimpath -o "$BUILD_DIR/pi-server" ./cmd/pi-server)
mkdir -p "$DATA_DIR"
DATA_DIR=$(cd "$DATA_DIR" && pwd)
export PI_SERVER_ADDR="0.0.0.0:$SERVER_PORT"
export PI_SERVER_CWD="$SCRIPT_DIR"
export PI_SERVER_DATA_DIR="$DATA_DIR"
export PI_SERVER_ALLOWED_ROOTS="${PI_SERVER_ALLOWED_ROOTS:-$SCRIPT_DIR}"
export PI_SERVER_PI_EXTENSIONS="$SERVER_DIR/extensions/session-title.ts"
export PI_SERVER_AUTH_TOKEN="$AUTH_TOKEN"
# An explicit CORS allowlist remains in effect.
"$BUILD_DIR/pi-server" --pairing-qr=false &
SERVER_PID=$!
printf 'Waiting for pi-server...\n'
dev_wait_http "http://127.0.0.1:$SERVER_PORT/healthz" "$SERVER_PID" "$HEALTH_TIMEOUT"
(cd "$WEB_DIR" && exec env -u PI_SERVER_AUTH_TOKEN pnpm exec vite --host 0.0.0.0 --port "$WEB_PORT" --strictPort) &
WEB_PID=$!
dev_wait_http "http://127.0.0.1:$WEB_PORT/" "$WEB_PID" "$HEALTH_TIMEOUT"
printf 'Stack is ready.\nWebby: http://127.0.0.1:%s\nServer: http://%s:%s\n' "$WEB_PORT" "$NETWORK_IP" "$SERVER_PORT"
printf 'CORS: %s\n' "${PI_SERVER_ALLOWED_ORIGINS:-automatic private-network policy}"
[[ -n "$AUTH_TOKEN" ]] || printf 'Warning: no authentication. Use only on a trusted private network.\n' >&2
printf 'Press Ctrl+C to stop the stack.\n'
# Polling also works on Bash versions without wait -n. Preserve failure codes.
while kill -0 "$SERVER_PID" 2>/dev/null && kill -0 "$WEB_PID" 2>/dev/null; do sleep 0.25; done
if ! kill -0 "$SERVER_PID" 2>/dev/null; then
  if wait "$SERVER_PID"; then status=0; else status=$?; fi
else
  if wait "$WEB_PID"; then status=0; else status=$?; fi
fi
exit "$status"
