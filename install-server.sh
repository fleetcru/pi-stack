#!/usr/bin/env bash
# Install or upgrade a verified server release. Existing settings stay intact.
set -euo pipefail

REPO="fleetcru/pi-stack"
SERVICE_NAME="pi-server"
INSTALL_DIR="${PI_SERVER_INSTALL_DIR:-/opt/pi-server}"
DATA_DIR="${PI_SERVER_DATA_DIR:-/var/lib/pi-server}"
CONFIG_DIR="${PI_SERVER_CONFIG_DIR:-/etc/pi-server}"
SYSTEMD_DIR="${PI_SERVER_SYSTEMD_DIR:-/etc/systemd/system}"
SERVICE_USER="${PI_SERVER_SERVICE_USER:-${SUDO_USER:-root}}"
PORT="${PI_SERVER_PORT:-3142}"
AUTH_TOKEN="${PI_SERVER_AUTH_TOKEN:-}"
CHANNEL="${PI_SERVER_CHANNEL:-dev}"
SOURCE_REVISION="${PI_SERVER_SOURCE_REVISION:-3ef3f52c2b776b2a913122dc473302d06665e7cc}"
BUILD_FROM_SOURCE="${PI_SERVER_ALLOW_SOURCE_BUILD:-0}"

info() { printf '[info] %s\n' "$*"; }
warn() { printf '[warn] %s\n' "$*" >&2; }
fail() { printf '[error] %s\n' "$*" >&2; exit 1; }
for arg in "$@"; do
  case "$arg" in
    --insecure) warn '--insecure is obsolete and does not disable authentication.' ;;
    --help|-h)
      printf '%s\n' 'Usage: sudo bash install-server.sh' \
        'PI_SERVER_CHANNEL=dev|stable selects the release channel.' \
        'Existing configuration and credentials are preserved.' \
        'Set PI_SERVER_PORT or PI_SERVER_AUTH_TOKEN explicitly to change them.' \
        'PI_SERVER_ALLOW_SOURCE_BUILD=1 explicitly builds PI_SERVER_SOURCE_REVISION.'
      exit 0 ;;
    *) fail "Unknown option: $arg" ;;
  esac
done
[[ "$PORT" =~ ^[0-9]{1,5}$ ]] && (( 10#$PORT >= 1 && 10#$PORT <= 65535 )) || fail 'Port must be between 1 and 65535.'
[[ "$CHANNEL" == dev || "$CHANNEL" == stable ]] || fail 'PI_SERVER_CHANNEL must be dev or stable.'
[[ "$BUILD_FROM_SOURCE" == 0 || "$BUILD_FROM_SOURCE" == 1 ]] || fail 'PI_SERVER_ALLOW_SOURCE_BUILD must be 0 or 1.'
[[ "$SOURCE_REVISION" =~ ^[0-9a-fA-F]{40}$ ]] || fail 'Source revision must be an exact 40-character Git commit.'
[[ "$AUTH_TOKEN" != *$'\n'* && "$AUTH_TOKEN" != *$'\r'* ]] || fail 'Auth tokens cannot contain line breaks.'
[[ $EUID -eq 0 ]] || fail 'Run this installer with sudo.'
[[ $(uname -s) == Linux ]] || fail 'This installer supports Linux only.'
case "$(uname -m)" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) fail 'Unsupported architecture.' ;;
esac
for command in systemctl getent runuser curl sha256sum python3 flock; do
  command -v "$command" >/dev/null || fail "Required command is missing: $command"
done
systemctl show-environment >/dev/null || fail 'systemd is not running.'
id "$SERVICE_USER" >/dev/null 2>&1 || fail "Unknown service user: $SERVICE_USER"
SERVICE_HOME=$(getent passwd "$SERVICE_USER" | cut -d: -f6)
SERVICE_GROUP=$(id -gn "$SERVICE_USER")
[[ -d "$SERVICE_HOME" ]] || fail "Service home does not exist: $SERVICE_HOME"
PI_BINARY="${PI_SERVER_PI_BINARY:-$(runuser -u "$SERVICE_USER" -- bash -lc 'command -v pi' 2>/dev/null || true)}"
[[ -n "$PI_BINARY" ]] || fail "Pi CLI is not available for $SERVICE_USER. Install Pi for that account first."
# Preserve the service user's login PATH, including user-installed Node.
LOGIN_PATH=$(runuser -u "$SERVICE_USER" -- env HOME="$SERVICE_HOME" bash -lc 'printf "%s" "$PATH"' 2>/dev/null || true)
SERVICE_PATH="$(dirname "$PI_BINARY"):${LOGIN_PATH:-$PATH}"
runuser -u "$SERVICE_USER" -- env HOME="$SERVICE_HOME" PATH="$SERVICE_PATH" "$PI_BINARY" --version >/dev/null || fail 'Pi cannot run as the service user.'
for path in "$INSTALL_DIR" "$DATA_DIR" "$CONFIG_DIR" "$SYSTEMD_DIR" "$SERVICE_HOME" "$PI_BINARY" "$SERVICE_PATH"; do
  [[ "$path" == /* && "$path" != *$'\n'* && "$path" != *$'\r'* && "$path" != *'"'* && "$path" != *'%'* && "$path" != *$'\\'* ]] || fail "Unsupported path: $path"
done

umask 077
mkdir -p "$INSTALL_DIR" "$CONFIG_DIR" "$SYSTEMD_DIR"
exec 9>"$INSTALL_DIR/.install.lock"
flock -n 9 || fail 'Another pi-server installer is running.'
STAGE=$(mktemp -d "$INSTALL_DIR/.install-XXXXXXXX")
ENV_FILE="$CONFIG_DIR/pi-server.env"
UNIT_FILE="$SYSTEMD_DIR/$SERVICE_NAME.service"
BINARY="$INSTALL_DIR/pi-server"
WAS_RUNNING=0
WAS_ENABLED=0
CHANGED=0
STOPPED=0
KEEP_STAGE=0
systemctl is-active --quiet "$SERVICE_NAME" && WAS_RUNNING=1
systemctl is-enabled --quiet "$SERVICE_NAME" && WAS_ENABLED=1
if [[ -f "$UNIT_FILE" ]] && ! grep -Fxq "EnvironmentFile=$ENV_FILE" "$UNIT_FILE" && ! grep -Fxq "EnvironmentFile=\"$ENV_FILE\"" "$UNIT_FILE"; then
  rm -rf "$STAGE"
  fail 'The existing service uses another configuration file. Upgrade it with its original installer.'
fi
[[ ! -f "$BINARY" ]] || cp -p "$BINARY" "$STAGE/previous-binary"
[[ ! -f "$ENV_FILE" ]] || cp -p "$ENV_FILE" "$STAGE/previous-env"
[[ ! -f "$UNIT_FILE" ]] || cp -p "$UNIT_FILE" "$STAGE/previous-unit"
cleanup() {
  local status=$?
  trap - EXIT INT TERM
  if (( status != 0 && CHANGED )); then
    warn 'Installation failed. Restoring the previous binary and configuration.'
    systemctl stop "$SERVICE_NAME" || true
    for pair in "previous-binary:$BINARY" "previous-env:$ENV_FILE" "previous-unit:$UNIT_FILE"; do
      local saved=${pair%%:*} target=${pair#*:}
      if [[ -f "$STAGE/$saved" ]]; then
        cp -p "$STAGE/$saved" "$target.rollback" && mv -f "$target.rollback" "$target" || KEEP_STAGE=1
      else
        rm -f "$target" || KEEP_STAGE=1
      fi
    done
    systemctl daemon-reload || true
    if (( ! WAS_ENABLED )); then systemctl disable "$SERVICE_NAME" || true; fi
  fi
  if (( status != 0 && STOPPED && WAS_RUNNING )); then systemctl start "$SERVICE_NAME" || warn 'Previous service could not restart.'; fi
  if (( KEEP_STAGE )); then warn "Rollback files retained at $STAGE"; else rm -rf "$STAGE"; fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

fetch() {
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --connect-timeout 10 --max-time 120 --max-filesize 200000000 --retry 3 --retry-delay 1 "$1" -o "$2"
}
select_stable_release() {
  local page count
  for page in {1..10}; do
    fetch "https://api.github.com/repos/$REPO/releases?per_page=100&page=$page" "$STAGE/releases-$page.json"
    count=$(python3 -c 'import json,sys; data=json.load(open(sys.argv[1])); assert isinstance(data,list); print(len(data))' "$STAGE/releases-$page.json")
    (( count == 100 )) || break
    (( page < 10 )) || fail 'Release listing exceeded 1,000 entries. Cannot safely select the newest server release.'
  done
  # Python only parses API JSON. It does not modify scripts or configuration.
  python3 - "$STAGE" <<'PY'
import glob, json, re, sys
candidates = []
for path in glob.glob(sys.argv[1] + '/releases-*.json'):
    for release in json.load(open(path)):
        tag = release.get('tag_name', '')
        match = re.fullmatch(r'server-v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)', tag)
        if match and not release.get('draft') and not release.get('prerelease'):
            candidates.append((tuple(map(int, match.groups())), tag))
if not candidates:
    sys.exit('No stable server-v<version> release exists. Try PI_SERVER_CHANNEL=dev.')
print(max(candidates)[1])
PY
}
if [[ "$BUILD_FROM_SOURCE" == 1 ]]; then
  for command in git go; do command -v "$command" >/dev/null || fail "$command is required for source builds."; done
  info "Building pinned source revision $SOURCE_REVISION..."
  mkdir "$STAGE/source"
  git -C "$STAGE/source" init -q
  git -C "$STAGE/source" remote add origin "https://github.com/$REPO.git"
  git -C "$STAGE/source" fetch -q --depth 1 origin "$SOURCE_REVISION"
  git -C "$STAGE/source" checkout -q --detach FETCH_HEAD
  (cd "$STAGE/source/pi-server-exp" && go build -trimpath -o "$STAGE/new-binary" ./cmd/pi-server)
else
  TAG=server-dev
  if [[ "$CHANNEL" == dev ]]; then
    status=$(curl --silent --show-error --location --proto '=https' --proto-redir '=https' \
      --connect-timeout 10 --max-time 30 --retry 3 --output "$STAGE/dev-release.json" \
      --write-out '%{http_code}' "https://api.github.com/repos/$REPO/releases/tags/server-dev")
    case "$status" in
      200) ;;
      404) warn 'server-dev does not exist. Trying a stable server release.'; TAG=$(select_stable_release) ;;
      *) fail "GitHub release lookup failed with HTTP $status. No fallback attempted." ;;
    esac
  else
    TAG=$(select_stable_release)
  fi
  BASE="https://github.com/$REPO/releases/download/$TAG"
  ASSET="pi-server-linux-$ARCH"
  info "Downloading $TAG for linux/$ARCH..."
  fetch "$BASE/$ASSET" "$STAGE/new-binary"
  fetch "$BASE/SHA256SUMS" "$STAGE/SHA256SUMS"
  expected=$(awk -v name="$ASSET" '$2 == name || $2 == "*" name {print $1}' "$STAGE/SHA256SUMS")
  [[ "$expected" =~ ^[0-9a-fA-F]{64}$ ]] || fail 'Checksum file must contain exactly one valid entry for the server binary.'
  actual=$(sha256sum "$STAGE/new-binary" | cut -d' ' -f1)
  [[ "$actual" == "${expected,,}" ]] || fail 'Downloaded binary checksum mismatch. Existing installation is unchanged.'
fi
chmod 0755 "$STAGE/new-binary"

# Serialize values for systemd EnvironmentFile without evaluating user input.
write_env() {
  local value=${2//\\/\\\\}
  value=${value//\"/\\\"}
  printf '%s="%s"\n' "$1" "$value"
}
if [[ -f "$ENV_FILE" ]]; then
  cp -p "$ENV_FILE" "$STAGE/new-env"
  info 'Preserving existing configuration and credentials.'
  if [[ ${PI_SERVER_ADDR+x} ]]; then
    grep -v '^PI_SERVER_ADDR=' "$STAGE/new-env" >"$STAGE/env-address" || true
    write_env PI_SERVER_ADDR "$PI_SERVER_ADDR" >>"$STAGE/env-address"
    mv "$STAGE/env-address" "$STAGE/new-env"
  fi
  if [[ ${PI_SERVER_PORT+x} ]]; then
    VALUE="$PORT" awk '/^PI_SERVER_ADDR=/ {sub(/:[0-9]+"?$/, ":" ENVIRON["VALUE"] (substr($0,length($0)) == "\"" ? "\"" : ""))} {print}' "$STAGE/new-env" >"$STAGE/env-port"
    mv "$STAGE/env-port" "$STAGE/new-env"
  fi
  if [[ ${PI_SERVER_AUTH_TOKEN+x} ]]; then
    grep -v '^PI_SERVER_AUTH_TOKEN=' "$STAGE/new-env" >"$STAGE/env-token" || true
    write_env PI_SERVER_AUTH_TOKEN "$AUTH_TOKEN" >>"$STAGE/env-token"
    mv "$STAGE/env-token" "$STAGE/new-env"
  fi
else
  if [[ -z "$AUTH_TOKEN" ]]; then AUTH_TOKEN=$(od -An -N32 -tx1 /dev/urandom | tr -d ' \n'); fi
  {
    write_env PI_SERVER_ADDR "${PI_SERVER_ADDR:-0.0.0.0:$PORT}"
    write_env PI_SERVER_CWD "$SERVICE_HOME"
    write_env PI_SERVER_DATA_DIR "$DATA_DIR"
    write_env PI_SERVER_ALLOWED_ROOTS "$SERVICE_HOME"
    write_env PI_SERVER_AUTH_TOKEN "$AUTH_TOKEN"
    write_env PI_SERVER_PI_BINARY "$PI_BINARY"
  } >"$STAGE/new-env"
fi
chmod 0600 "$STAGE/new-env"
PROBE_URL=$(python3 - "$STAGE/new-env" <<'PY'
import re, sys
address = '0.0.0.0:3142'
for line in open(sys.argv[1]):
    if line.startswith('PI_SERVER_ADDR='):
        address = line.partition('=')[2].strip().strip('\"')
match = re.fullmatch(r'(\[[0-9a-fA-F:]+\]|[A-Za-z0-9_.-]+):([0-9]{1,5})', address)
if not match or not 1 <= int(match[2]) <= 65535:
    sys.exit('Invalid configured PI_SERVER_ADDR. Existing server has not been stopped.')
host = {'0.0.0.0': '127.0.0.1', '[::]': '[::1]'}.get(match[1], match[1])
print('http://' + host + ':' + match[2] + '/healthz')
PY
)
if [[ -f "$UNIT_FILE" ]]; then
  cp -p "$UNIT_FILE" "$STAGE/new-unit"
  info 'Preserving the existing systemd unit and custom service settings.'
else
cat >"$STAGE/new-unit" <<EOF
[Unit]
Description=Pi Server coding agent hub
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$SERVICE_USER
Group=$SERVICE_GROUP
WorkingDirectory="$SERVICE_HOME"
Environment="HOME=$SERVICE_HOME"
Environment="PATH=$SERVICE_PATH"
EnvironmentFile="$ENV_FILE"
ExecStart="$BINARY" --pairing-qr=false
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=read-only
PrivateTmp=true
ReadWritePaths="$DATA_DIR" "$SERVICE_HOME"

[Install]
WantedBy=multi-user.target
EOF
fi
chmod 0644 "$STAGE/new-unit"
install -d -m 0700 -o "$SERVICE_USER" -g "$SERVICE_GROUP" "$DATA_DIR"
if (( WAS_RUNNING )); then systemctl stop "$SERVICE_NAME"; STOPPED=1; fi
CHANGED=1
mv -f "$STAGE/new-binary" "$BINARY"
mv -f "$STAGE/new-env" "$ENV_FILE"
mv -f "$STAGE/new-unit" "$UNIT_FILE"
systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
# enable --now does not restart an already-running unit after an upgrade.
systemctl restart "$SERVICE_NAME"
sleep 2
systemctl is-active --quiet "$SERVICE_NAME" || fail "Service failed to start. Check journalctl -u $SERVICE_NAME -n 50."
ready=0
for _ in {1..30}; do
  systemctl is-active --quiet "$SERVICE_NAME" || fail 'Service exited during startup.'
  main_pid=$(systemctl show --property=MainPID --value "$SERVICE_NAME")
  running_binary=$(readlink "/proc/$main_pid/exe" 2>/dev/null || true)
  if [[ "$running_binary" == "$BINARY" ]] && curl --noproxy '*' -fsS --connect-timeout 1 --max-time 2 "$PROBE_URL" >/dev/null 2>&1; then ready=1; break; fi
  sleep 1
done
(( ready )) || fail "Server did not become healthy. Check journalctl -u $SERVICE_NAME -n 50."
info 'pi-server is running in the background and will start at boot.'
printf 'Config: %s\nLogs: journalctl -u %s -f\nRestart: sudo systemctl restart %s\n' "$ENV_FILE" "$SERVICE_NAME" "$SERVICE_NAME"
printf '%s\n' 'Use a private LAN or Tailscale address in Companion. Do not expose the server publicly.'
printf 'Credentials are stored in %s with mode 600.\n' "$ENV_FILE"
