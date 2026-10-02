#!/usr/bin/env bash
# All installation, service, network, and Pi operations use temporary fixtures.
set -euo pipefail
ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
TEMP=$(mktemp -d)
OWNED_PIDS=()
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  for pid in "${OWNED_PIDS[@]}"; do
    if [[ -f "/proc/$pid/cmdline" ]] && grep -aq 'PiScriptTestHTTP' "/proc/$pid/cmdline"; then kill -TERM "$pid" 2>/dev/null || true; fi
  done
  rm -rf "$TEMP"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }
copy_script() { tr -d '\r' <"$ROOT/$1" >"$TEMP/$1"; }
for script in install-server.sh dev-launcher-common.sh start-exp-server.sh start-exp-live-stack.sh; do copy_script "$script"; bash -n "$TEMP/$script"; done
for script in pi-server-exp/scripts/install-systemd.sh pi-server-exp/scripts/bootstrap-linux-vps.sh; do bash -n <(tr -d '\r' <"$ROOT/$script"); done
# Bypass only the EUID check in a temporary copy. Every writable path and
# command that could touch the host is redirected below. No sudo is required.
awk 'index($0,"[[ $EUID -eq 0 ]]") == 0' "$TEMP/install-server.sh" >"$TEMP/installer-fixture.sh"
grep -Fq '[[ $EUID -eq 0 ]]' "$ROOT/install-server.sh" || fail 'Production installer lost its root check.'
mkdir -p "$TEMP/bin" "$TEMP/home" "$TEMP/pi-server-exp/extensions" "$TEMP/pi-webby-exp/node_modules"
printf 'fixture extension\n' >"$TEMP/pi-server-exp/extensions/session-title.ts"
cat >"$TEMP/bin/systemctl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$*" >>"$TEST_CASE/systemctl-calls"
case "$1" in
  show-environment|daemon-reload) ;;
  is-active) [[ $(<"$TEST_CASE/state") == active ]] ;;
  is-enabled) [[ $(<"$TEST_CASE/enabled") == enabled ]] ;;
  enable) printf enabled >"$TEST_CASE/enabled" ;;
  disable) printf disabled >"$TEST_CASE/enabled" ;;
  stop) printf inactive >"$TEST_CASE/state" ;;
  restart)
    if [[ ${TEST_FAILURE:-} == startup && $(<"$PI_SERVER_INSTALL_DIR/pi-server") == new-binary ]]; then printf inactive >"$TEST_CASE/state"; exit 1; fi
    printf active >"$TEST_CASE/state" ;;
  start) printf active >"$TEST_CASE/state" ;;
  show) printf '123\n' ;;
  *) echo "Unexpected systemctl command: $*" >&2; exit 99 ;;
esac
MOCK
cat >"$TEMP/bin/getent" <<'MOCK'
#!/usr/bin/env bash
printf '%s:x:%s:%s:fixture:%s:/bin/bash\n' "$PI_SERVER_SERVICE_USER" "$(id -u)" "$(id -g)" "$TEST_HOME"
MOCK
cat >"$TEMP/bin/runuser" <<'MOCK'
#!/usr/bin/env bash
# Never execute Pi or a login shell from installer tests.
exit 0
MOCK
cat >"$TEMP/bin/readlink" <<'MOCK'
#!/usr/bin/env bash
if [[ $1 == /proc/123/exe ]]; then printf '%s/pi-server\n' "$PI_SERVER_INSTALL_DIR"; else exec /usr/bin/readlink "$@"; fi
MOCK
cat >"$TEMP/bin/curl" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
url='' out='' write_out=0
while (( $# )); do
  case "$1" in
    -o|--output) out=$2; shift 2 ;;
    --write-out) write_out=1; shift 2 ;;
    --proto|--proto-redir|--connect-timeout|--max-time|--max-filesize|--retry|--retry-delay|--noproxy) shift 2 ;;
    https://*|http://*) url=$1; shift ;;
    *) shift ;;
  esac
done
if [[ $url == http://* ]]; then
  if [[ ${TEST_INSTALLER:-0} == 1 ]]; then exit 0; fi
  exec /usr/bin/curl --noproxy '*' -fsS --connect-timeout 1 --max-time 2 "$url"
fi
printf '%s\n' "$url" >>"$TEST_CASE/downloads"
case "$url" in
  */releases/tags/server-dev)
    printf '{"draft":false}' >"$out"
    if (( write_out )); then printf '%s' "${TEST_HTTP_STATUS:-200}"; fi ;;
  */releases\?*)
    printf '%s' '[{"tag_name":"v99.0.0","draft":false,"prerelease":false},{"tag_name":"server-v0.9.0","draft":false,"prerelease":false},{"tag_name":"server-v0.10.0","draft":false,"prerelease":false},{"tag_name":"server-v2.0.0","draft":true,"prerelease":false},{"tag_name":"server-v3.0.0-rc.1","draft":false,"prerelease":true}]' >"$out" ;;
  */SHA256SUMS)
    hash=$(printf new-binary | sha256sum | cut -d' ' -f1)
    if [[ ${TEST_FAILURE:-} == checksum ]]; then hash=$(printf '%064d' 0); fi
    printf '%s  pi-server-linux-amd64\n' "$hash" >"$out" ;;
  */pi-server-linux-amd64)
    [[ ${TEST_FAILURE:-} != download ]] || exit 28
    printf new-binary >"$out" ;;
  *) echo "Unexpected network request: $url" >&2; exit 99 ;;
esac
MOCK
cat >"$TEMP/bin/git" <<'MOCK'
#!/usr/bin/env bash
# A failing source checkout must leave the installation alone.
exit 7
MOCK
cat >"$TEMP/bin/go" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf called >>"$TEST_CASE/builds"
[[ ${TEST_FAILURE:-} != build ]] || exit 17
while (( $# )); do
  if [[ $1 == -o ]]; then out=$2; break; fi
  shift
done
cp "$TEST_MOCK_SERVER" "$out"
chmod +x "$out"
MOCK
cat >"$TEMP/mock-server" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$$" >"$TEST_SERVER_PID"
[[ ${TEST_FAILURE:-} != server ]] || exit 21
exec python3 -c '
import http.server, os
class PiScriptTestHTTP(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        self.send_response(200); self.end_headers(); self.wfile.write(b"ready")
    def log_message(self, *args): pass
http.server.HTTPServer(("127.0.0.1", int(os.environ["PI_SERVER_ADDR"].rsplit(":", 1)[1])), PiScriptTestHTTP).serve_forever()
'
MOCK
cat >"$TEMP/bin/pnpm" <<'MOCK'
#!/usr/bin/env bash
set -euo pipefail
printf '%s\n' "$$" >"$TEST_CASE/pnpm-pid"
while (( $# )); do
  if [[ $1 == --port ]]; then port=$2; break; fi
  shift
done
TEST_SERVER_PID="$TEST_CASE/web-child-pid" PI_SERVER_ADDR="127.0.0.1:$port" TEST_FAILURE='' "$TEST_MOCK_SERVER" &
child=$!
printf '%s\n' "$child" >"$TEST_CASE/web-child-pid"
if [[ ${TEST_FAILURE:-} == web-before ]]; then exit 17; fi
if [[ ${TEST_FAILURE:-} == web-after ]]; then sleep 1; exit 17; fi
wait "$child"
MOCK
chmod +x "$TEMP/bin/"* "$TEMP/mock-server"
export PATH="$TEMP/bin:/usr/bin:/bin"
export TEST_HOME="$TEMP/home" TEST_MOCK_SERVER="$TEMP/mock-server"
export PI_SERVER_SERVICE_USER="$(id -un)" PI_SERVER_PI_BINARY="$TEMP/bin/pi"
printf '#!/bin/sh\nexit 0\n' >"$PI_SERVER_PI_BINARY"
chmod +x "$PI_SERVER_PI_BINARY"

for scenario in success stable missing-dev forbidden download checksum startup source overrides; do
  export TEST_CASE="$TEMP/install-$scenario" TEST_INSTALLER=1 TEST_FAILURE='' TEST_HTTP_STATUS=200
  export PI_SERVER_INSTALL_DIR="$TEST_CASE/bin" PI_SERVER_CONFIG_DIR="$TEST_CASE/config" PI_SERVER_SYSTEMD_DIR="$TEST_CASE/systemd" PI_SERVER_DATA_DIR="$TEST_CASE/data"
  unset PI_SERVER_PORT PI_SERVER_AUTH_TOKEN PI_SERVER_CHANNEL PI_SERVER_ALLOW_SOURCE_BUILD PI_SERVER_ADDR
  mkdir -p "$PI_SERVER_INSTALL_DIR" "$PI_SERVER_CONFIG_DIR" "$PI_SERVER_SYSTEMD_DIR"
  printf old-binary >"$PI_SERVER_INSTALL_DIR/pi-server"
  printf 'PI_SERVER_ADDR=0.0.0.0:43123\nPI_SERVER_AUTH_TOKEN=saved-token\nPI_SERVER_MAX_SESSIONS=3\n# keep this\n' >"$PI_SERVER_CONFIG_DIR/pi-server.env"
  printf 'EnvironmentFile=%s/pi-server.env\nExecStart=%s/pi-server\n# custom unit setting\n' "$PI_SERVER_CONFIG_DIR" "$PI_SERVER_INSTALL_DIR" >"$PI_SERVER_SYSTEMD_DIR/pi-server.service"
  cp "$PI_SERVER_CONFIG_DIR/pi-server.env" "$TEST_CASE/original-env"
  cp "$PI_SERVER_SYSTEMD_DIR/pi-server.service" "$TEST_CASE/original-unit"
  printf active >"$TEST_CASE/state"
  printf disabled >"$TEST_CASE/enabled"
  case "$scenario" in
    stable) export PI_SERVER_CHANNEL=stable ;;
    missing-dev) export TEST_HTTP_STATUS=404 ;;
    forbidden) export TEST_HTTP_STATUS=403 ;;
    download|checksum|startup) export TEST_FAILURE="$scenario" ;;
    source) export PI_SERVER_ALLOW_SOURCE_BUILD=1 ;;
    overrides) export PI_SERVER_PORT=43124 PI_SERVER_AUTH_TOKEN='new-token$with\literal' ;;
  esac
  if bash "$TEMP/installer-fixture.sh" >"$TEST_CASE/output" 2>&1; then result=0; else result=$?; fi
  case "$scenario" in
    success|stable|missing-dev|overrides)
      (( result == 0 )) || { /usr/bin/tail -30 "$TEST_CASE/output"; fail "$scenario installation failed."; }
      [[ $(<"$PI_SERVER_INSTALL_DIR/pi-server") == new-binary ]] || fail 'Verified binary was not installed.'
      if [[ $scenario != overrides ]]; then cmp "$TEST_CASE/original-env" "$PI_SERVER_CONFIG_DIR/pi-server.env" || fail 'Existing config was overwritten.'
      else
        grep -Fq '0.0.0.0:43124' "$PI_SERVER_CONFIG_DIR/pi-server.env" || fail 'Explicit port did not apply.'
        grep -Fq 'PI_SERVER_MAX_SESSIONS=3' "$PI_SERVER_CONFIG_DIR/pi-server.env" || fail 'Override deleted unrelated settings.'
        grep -Fq 'new-token$with\\literal' "$PI_SERVER_CONFIG_DIR/pi-server.env" || fail 'Token serialization changed literal characters.'
      fi
      cmp "$TEST_CASE/original-unit" "$PI_SERVER_SYSTEMD_DIR/pi-server.service" || fail 'Custom unit was overwritten.'
      grep -q '^restart ' "$TEST_CASE/systemctl-calls" || fail 'Upgrade did not restart the service.' ;;
    *)
      (( result != 0 )) || fail "$scenario failure was accepted."
      [[ $(<"$PI_SERVER_INSTALL_DIR/pi-server") == old-binary ]] || fail 'Failure replaced the previous binary.'
      cmp "$TEST_CASE/original-env" "$PI_SERVER_CONFIG_DIR/pi-server.env" || fail 'Rollback lost configuration.'
      [[ $(<"$TEST_CASE/state") == active ]] || fail 'Previous service was not restored.'
      [[ $(<"$TEST_CASE/enabled") == disabled ]] || fail 'Rollback changed startup enablement.'
      if [[ $scenario != startup ]]; then ! grep -q '^stop ' "$TEST_CASE/systemctl-calls" || fail 'Unverified download stopped the service.'; fi ;;
  esac
  [[ -z $(find "$PI_SERVER_INSTALL_DIR" -maxdepth 1 -name '.install-*' -print -quit) ]] || fail 'Temporary install files leaked.'
  if [[ $scenario == stable || $scenario == missing-dev ]]; then grep -q '/server-v0.10.0/' "$TEST_CASE/downloads" || fail 'Wrong stable product/version selected.'; fi
  if [[ $scenario == forbidden ]]; then ! grep -q '/releases?' "$TEST_CASE/downloads" || fail '403 changed release channels.'; fi
done

unset PI_SERVER_INSTALL_DIR PI_SERVER_CONFIG_DIR PI_SERVER_SYSTEMD_DIR PI_SERVER_DATA_DIR PI_SERVER_AUTH_TOKEN PI_SERVER_PORT PI_SERVER_CHANNEL PI_SERVER_ALLOW_SOURCE_BUILD PI_SERVER_ADDR
export TEST_INSTALLER=0
free_port() { python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1]); s.close()'; }
alive() { [[ -r /proc/$1/status ]] && ! grep -q '^State:.*Z' "/proc/$1/status"; }
remember_children() {
  for file in "$TEST_CASE/server-pid" "$TEST_CASE/web-child-pid"; do
    [[ ! -f "$file" ]] || OWNED_PIDS+=("$(<"$file")")
  done
}
assert_children_stopped() {
  remember_children
  for file in "$TEST_CASE/server-pid" "$TEST_CASE/pnpm-pid" "$TEST_CASE/web-child-pid"; do
    if [[ -f "$file" ]] && alive "$(<"$file")"; then fail "Orphaned process $(<"$file") after ${TEST_FAILURE:-normal} launch."; fi
  done
}
for scenario in build server web-before web-after; do
  export TEST_CASE="$TEMP/launch-$scenario" TEST_FAILURE="$scenario" DATA_DIR="$TEMP/data-$scenario" TEST_SERVER_PID="$TEMP/launch-$scenario/server-pid" HEALTH_TIMEOUT=3
  mkdir -p "$TEST_CASE"
  server_port=$(free_port); web_port=$(free_port)
  if timeout --kill-after=3 12 bash "$TEMP/start-exp-live-stack.sh" -s "$server_port" -w "$web_port" >"$TEST_CASE/output" 2>&1; then result=0; else result=$?; fi
  remember_children
  (( result != 0 && result != 124 && result != 137 )) || { /usr/bin/tail -30 "$TEST_CASE/output"; fail "$scenario did not fail promptly."; }
  if [[ $scenario == web-after ]]; then (( result == 17 )) || fail 'Web failure exit code was swallowed.'; fi
  sleep 0.25
  assert_children_stopped
done
# Healthy services must also stop together on a termination signal.
export TEST_CASE="$TEMP/launch-healthy" TEST_FAILURE='' DATA_DIR="$TEMP/data-healthy" TEST_SERVER_PID="$TEMP/launch-healthy/server-pid"
mkdir -p "$TEST_CASE"
server_port=$(free_port); web_port=$(free_port)
bash "$TEMP/start-exp-live-stack.sh" -s "$server_port" -w "$web_port" >"$TEST_CASE/output" 2>&1 &
stack_pid=$!
ready=0
for _ in {1..60}; do
  if grep -q 'Stack is ready' "$TEST_CASE/output"; then ready=1; break; fi
  sleep 0.1
done
if (( ! ready )); then kill -TERM "$stack_pid" 2>/dev/null || true; wait "$stack_pid" || true; /usr/bin/tail -30 "$TEST_CASE/output"; remember_children; fail 'Healthy stack never became ready.'; fi
kill -TERM "$stack_pid"
wait "$stack_pid" || true
sleep 0.25
assert_children_stopped
for args in '--server-port' '--server-port 0' '--server-port 43123 --web-port 43123'; do
  if bash "$TEMP/start-exp-live-stack.sh" $args >"$TEMP/invalid-output" 2>&1; then fail 'Invalid launcher arguments accepted.'; fi
  ! grep -q 'unbound variable' "$TEMP/invalid-output" || fail 'Missing argument produced an opaque shell error.'
done
printf 'Shell script tests passed. No real services, installations, or Pi sessions were changed.\n'
