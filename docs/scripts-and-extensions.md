# Extensions, tray, and root scripts — file guide

## Pi extensions (`pi-server-exp/extensions/`)

These TypeScript files are loaded into Pi processes (via `PI_SERVER_PI_EXTENSIONS` or install scripts), not into pi-server itself.

- `external-session-bridge.ts` — bridges a user's local Pi TUI session into pi-server as an external relay. Connects a WebSocket back to the hub, streams events, receives queued prompts, and emits `ask:requested`/`ask:closed`/`ask:remote-response` on Pi's event bus so the local `ask_user` overlay closes when a phone answers remotely. This is the counterpart to the server's `external_*.go` files. Tokens are sent only in `Sec-WebSocket-Protocol`: URL-safe tokens as `pi-relay.<token>`, others as `pi-relay-b64.<base64url(token)>` (the server decodes both). A watcher polls `~/.pi/agent/bridge-config.json` every second and hot-reconnects when the server rewrites it, so a running TUI follows a relay URL, port, or token rotation immediately without `/reload` or `/bridge-reconnect`. At registration the extension writes a random local proof under `~/.pi/agent/bridge-processes/` with its PID, approximate startup timestamp, and history path. The server accepts this evidence only from a local peer and verifies the kernel process creation time. Shutdown removes the proof file, while the server retains the verified identity. `POST /v1/external-sessions/{id}/continue` offers explicit Continue on server after the TUI disconnects and exits. A network disconnect alone is insufficient. Older bridges, remote-host bridges, and unsupported platforms fail closed. The server retains the session ID and Pi history and rejects takeover while relay commands are queued.
- `session-title.ts` — sets a useful title immediately on session start, then replaces it with a concise task-oriented title once the agent understands the work.

## pi-server-tray

Small Go program (separate module) for Windows desktops running pi-server personally.

- `main.go` — tray icon lifecycle, server start/stop/restart, menu.
- `download.go` — fetches/updates server binaries (with `download_test.go`).
- `process_windows.go` / `process_unix.go` — platform process control.
- `icon_windows.go` / `icon_unix.go` — embedded tray icon per platform.
- `open_windows.go` / `open_darwin.go` / `open_linux.go` — open-URL-in-browser per OS.
- `replace_windows.go` / `replace_unix.go` — self-update file replacement logic.
- `install.ps1` / `uninstall.ps1` — build/install or remove the tray app. Installation stages the binary, retains recovery files if rollback fails, and removes an existing startup shortcut with `-NoStartup`. Uninstall removes only its executable rather than recursively deleting the install directory. `-RemoveData` rejects protected home/root directories and a data-directory junction. `-WhatIf` previews removal.

## Root scripts

Startup:
- `start-exp-server.ps1` / `.cmd` / `.sh` build a temporary server executable and run it directly. They validate ports, preserve inherited auth and explicit CORS settings, and clean up the build. PowerShell restores the caller's environment. The deprecated OpenAdmin option prints a hint because `/admin/` no longer exists.
- `start-exp-live-stack.ps1` / `.cmd` start the server, Webby, and optionally a Pi TUI. `-Background` starts only server and Webby. Both services must pass bounded readiness checks. Commands use encoded PowerShell arguments to preserve paths with quotes and spaces; tokens travel through environment variables, not command-line text.
- `stop-exp-live-stack.ps1` reads `<DataDir>/dev-stack.json`, verifies PID creation times, and stops recorded wrappers and descendants. Supports `-WhatIf`. Startup logs remain under `<DataDir>/.launchers/<buildId>/`.
- `start-exp-live-stack.sh` starts server and Webby, not a TUI. Cleanup traps run on startup errors and signals. Dedicated process groups stop Vite even if its pnpm parent has already exited. Child failures propagate a nonzero exit code.
- `dev-launcher-common.ps1` / `.sh` hold port, readiness, build, quoting, and process helpers. PowerShell process records store timestamp ticks as strings to survive PS 5.1 and PS 7 JSON round trips.
- `fix-pi-server-node-path.sh` — repairs the Node path so the server can spawn Pi on Linux/macOS.

Install:
- `install-server.sh` installs a Linux systemd unit. Requires Pi, curl, Python 3, flock, and a running systemd. Downloads and verifies in a private staging directory, serializes installers with flock, preserves configuration and custom units, and restarts the service on upgrade. Checks the main executable and HTTP readiness, with binary/config/unit rollback on failure. `PI_SERVER_PORT` and `PI_SERVER_AUTH_TOKEN` explicitly update settings. Source builds require `PI_SERVER_ALLOW_SOURCE_BUILD=1` and a pinned `PI_SERVER_SOURCE_REVISION`; no automatic Go installation.
- `install-server.ps1` installs a SYSTEM startup task; `install-server-user.ps1` installs a current-user logon task. Both remain self-contained for download-and-execute one-liners. They stage verified binaries, lock concurrent installs, check task ownership and occupied ports, preserve existing settings, and attempt rollback after failed startup. `-Port` and `-AuthToken` explicitly override saved values. `-BuildFromSource` opts into a pinned source build. Task wrappers return native exit codes for restart-on-failure. The user installer initially binds loopback; the admin installer binds all interfaces and generates a token on first install.
- All release installers default to `server-dev`. Stable selection paginates GitHub releases and compares numeric `server-v<major>.<minor>.<patch>` versions, excluding Companion, tray, drafts, and prereleases. Only a 404 for the dev release triggers stable fallback. Download and checksum failures leave the old server untouched.
- `install-exp-external-bridge.ps1` / `.cmd` copy the extension into Pi's automatically discovered user extensions directory, without duplicate package registration. Existing JSON fields and credentials survive reinstall. Changing the relay URL clears a saved credential unless a new token is supplied. Writes private, atomic UTF-8 JSON without a BOM, including on PowerShell 5.1, and removes the legacy persistent token environment variable.
- `windows-installer-common.ps1` contains checksum helpers. `test-windows-installer.ps1` extracts the actual self-contained installer helpers and runs mocked upgrades, download/checksum/startup/ACL failures, bridge JSON, tray removal, and real harmless child-process cleanup tests under PS 5.1 and PS 7.
- `test-shell-scripts.sh` uses temporary directories and mocked service/network commands to test Linux upgrades, rollback, settings preservation, release selection, startup failures, and process-group cleanup. It does not require root or change installed services. CI runs both test suites for installer and launcher changes. `.gitattributes` keeps shell scripts LF-terminated.

Maintenance:
- `sync-components.ps1` / `.sh` — copies shared React components between pi-webby-exp and pi-desktop-app to keep the mirrors identical.
- `build-pi-server.ps1` — Windows build of the Go server.
- `chunk.ps1` — utility for splitting large files into chunks.
