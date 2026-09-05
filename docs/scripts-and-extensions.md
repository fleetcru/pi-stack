# Extensions, tray, and root scripts — file guide

## Pi extensions (`pi-server-exp/extensions/`)

These TypeScript files are loaded into Pi processes (via `PI_SERVER_PI_EXTENSIONS` or install scripts), not into pi-server itself.

- `external-session-bridge.ts` — bridges a user's local Pi TUI session into pi-server as an external relay. Connects a WebSocket back to the hub, streams events, receives queued prompts, and emits `ask:requested`/`ask:closed`/`ask:remote-response` on Pi's event bus so the local `ask_user` overlay closes when a phone answers remotely. This is the counterpart to the server's `external_*.go` files. Tokens are sent only in `Sec-WebSocket-Protocol`: URL-safe tokens as `pi-relay.<token>`, others as `pi-relay-b64.<base64url(token)>` (the server decodes both). The bridge re-reads `~/.pi/agent/bridge-config.json` on every registration attempt, so a server restart with a new relay URL/token is picked up on reconnect without `/reload` or `/bridge-reconnect`.
- `session-title.ts` — sets a useful title immediately on session start, then replaces it with a concise task-oriented title once the agent understands the work.

## pi-server-tray

Small Go program (separate module) for Windows desktops running pi-server personally.

- `main.go` — tray icon lifecycle, server start/stop/restart, menu.
- `download.go` — fetches/updates server binaries (with `download_test.go`).
- `process_windows.go` / `process_unix.go` — platform process control.
- `icon_windows.go` / `icon_unix.go` — embedded tray icon per platform.
- `open_windows.go` / `open_darwin.go` / `open_linux.go` — open-URL-in-browser per OS.
- `replace_windows.go` / `replace_unix.go` — self-update file replacement logic.
- `install.ps1` / `uninstall.ps1` — install/uninstall the tray app.

## Root scripts

Startup:
- `start-exp-server.ps1` / `.cmd` / `.sh` — build/run pi-server only (dev).
- `start-exp-live-stack.ps1` / `.cmd` / `.sh` — full dev stack: pi-server + webby dev server + a Pi TUI with the relay bridge.
- `fix-pi-server-node-path.sh` — repairs the Node path so the server can spawn Pi on Linux/macOS.

Install:
- `install-server.sh` — Linux VPS install (systemd unit).
- `install-server.ps1` — Windows install requiring admin (scheduled task).
- `install-server-user.ps1` — per-user Windows install, no admin needed.
- `install-exp-external-bridge.ps1` / `.cmd` — copies the relay-bridge extension into the user's Pi extensions directory.
- `windows-installer-common.ps1` / `test-windows-installer.ps1` — shared installer logic and its test harness.

Maintenance:
- `sync-components.ps1` / `.sh` — copies shared React components between pi-webby-exp and pi-desktop-app to keep the mirrors identical.
- `build-pi-server.ps1` — Windows build of the Go server.
- `chunk.ps1` — utility for splitting large files into chunks.
