#!/usr/bin/env bash
# Shared helpers. Sourcing this file does not launch or stop processes.
dev_fail() { printf 'Error: %s\n' "$*" >&2; exit 1; }
dev_require() { command -v "$1" >/dev/null 2>&1 || dev_fail "$1 is not installed or is not in PATH."; }
dev_port() {
  [[ "$1" =~ ^[0-9]{1,5}$ ]] && (( 10#$1 >= 1 && 10#$1 <= 65535 )) || dev_fail 'Ports must be between 1 and 65535.'
}
dev_need_value() { (( $# >= 2 )) && [[ "$2" != --* ]] || dev_fail "Missing value for $1"; }
dev_assert_port_free() {
  if (exec 3<>"/dev/tcp/127.0.0.1/$1") 2>/dev/null; then
    dev_fail "Port $1 is already in use. Stop the existing service or choose another port."
  fi
}
dev_wait_http() {
  local url=$1 pid=$2 timeout=$3 deadline=$((SECONDS + $3))
  while (( SECONDS < deadline )); do
    kill -0 "$pid" 2>/dev/null || dev_fail "Process $pid exited before $url became ready."
    if curl --noproxy '*' -fsS --connect-timeout 1 --max-time 2 "$url" >/dev/null 2>&1; then
      sleep 0.25
      kill -0 "$pid" 2>/dev/null || dev_fail "Process $pid exited during startup."
      return 0
    fi
    sleep 0.25
  done
  dev_fail "Timed out waiting for $url after ${timeout}s."
}
dev_network_ip() {
  if command -v tailscale >/dev/null 2>&1; then
    local address
    address=$(tailscale ip -4 2>/dev/null | head -n 1 || true)
    if [[ -n "$address" ]]; then printf '%s\n' "$address"; return; fi
  fi
  if command -v ip >/dev/null 2>&1; then
    ip -4 -o addr show 2>/dev/null | awk '{split($4,a,"/"); if (a[1] ~ /^(10\.|192\.168\.|172\.(1[6-9]|2[0-9]|3[01])\.|100\.(6[4-9]|[7-9][0-9]|1[01][0-9]|12[0-7])\.)/) {print a[1]; exit}}'
  fi
}
