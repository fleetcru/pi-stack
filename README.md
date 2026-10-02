# pi-stack

A multi-device coding agent ecosystem for [Pi](https://github.com/earendil-works/pi). Run Pi sessions from your terminal, control them from a web browser, desktop app, or Android phone, and bridge existing TUI sessions into the stack.

## What's in the box

| Project | Stack | What it does |
|---|---|---|
| **pi-server** | Go | HTTP/WebSocket hub that supervises Pi processes, proxies workers, and relays TUI sessions |
| **pi-webby** | React + TypeScript + Vite | Browser client for creating, monitoring, and chatting with Pi sessions |
| **pi-desktop** | Tauri v2 + React + TypeScript | Desktop app with native OS integration, image attachments, and offline support |
| **pi-companion** | Kotlin + Jetpack Compose | Android client with camera attachments, mobile UX, and real-time session status |
| **pi-webby-shared** | TypeScript | Shared API, state, socket, and timeline logic used by Webby and Desktop |
| **pi-server-tray** | Go | Desktop tray controller and verified pi-server updater |

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│   Webby     │────▶│             │◀────│   Desktop    │     │  Companion   │
│  (browser)  │ WS  │             │ WS  │   (Tauri)   │     │  (Android)   │
└─────────────┘     │             │     └──────────────┘     └──────────────┘
                    │   ┌─────┐   │                             ▲
                    │   │ Pi  │   │◀──── Pi TUI (via relay)─────┘
                    │   └─────┘   │
                    └──────┬──────┘
                           │
                    ┌──────▼──────┐
                    │   Workers   │  (remote Pi instances)
                    └─────────────┘
```

- **Sessions** — Each Pi process runs as an isolated RPC session with its own working directory, history, and metadata.
- **Workers** — Remote Pi instances registered by URL. pi-server proxies requests and aggregates their session inventories.
- **Relay** — Existing Pi TUI sessions can bridge into pi-server via the external-session extension, making them visible and controllable from Webby, Desktop, and Companion.
- **WebSocket tickets** — Browser clients authenticate via single-use, time-limited tickets instead of exposing bearer tokens over WebSocket.
- **Real-time status** — Sessions show granular runtime state (working, waiting for input, reconnecting) with pulsing indicators and detail labels.
- **Image attachments** — Send images from web and Android clients as multimodal prompts.
- **Session search** — Filter sessions by title, project, worker, or session ID across the sidebar.
- **Pin sessions** — Star important sessions to keep them at the top of their project group.
- **Server administration** — Bearer-authenticated `/v1/admin/*` APIs power in-app server administration in Webby and Desktop: overview, health warnings, settings with live vs restart-required labels, worker management, and trusted-device pairing.
- **Bounded task admission** — Global, per-session, and per-worker run limits with a bounded queue and scheduler-pressure visibility.
- **Durable distributed runs** — Remote and relay reservations survive hub restarts and reconcile from pushed lifecycle events.
- **Indexed history paging** — Large JSONL transcripts use validated sidecar indexes and direct message-boundary seeks.
- **Operational metrics** — Authenticated Prometheus-compatible metrics at `/metrics` expose request, scheduler, replay, and runtime signals.
- **Fast mobile switching** — Companion caches five recent timelines, prefetches likely sessions, and restores cached content while refreshing in the background.

## Releases

- [pi-server releases](https://github.com/fleetcru/pi-stack/releases?q=server-v) — Linux and Windows AMD64 binaries
- [Pi Companion releases](https://github.com/fleetcru/pi-stack/releases?q=v) — directly installable Android APKs

Use the release pages above for current version numbers.

## Deployment boundary

> [!WARNING]
> pi-server is designed for a trusted private network: loopback, a controlled LAN, or an authenticated Tailscale network. It is **not** an internet-facing service. Do not expose it through a public IP address, port-forward, or publicly reachable reverse proxy.
>
> The default setup trusts every device on the home LAN and Tailscale network. Set `PI_SERVER_AUTH_TOKEN` if that network contains devices that should not control Pi.

## Production installation

The installers verify release checksums before installing the server binary.

### Quick setup

Review a script before running it if you are unsure what it changes. These commands install the latest pi-server release and start it automatically after login or boot.

| Platform | Command | Script |
| --- | --- | --- |
| Windows, current user | `irm https://winuser.fleetcru.dev \| iex` | [`install-server-user.ps1`](install-server-user.ps1) |
| Windows, all users | Run PowerShell as Administrator, then `irm https://windows.fleetcru.dev \| iex` | [`install-server.ps1`](install-server.ps1) |
| Linux with systemd | `curl -fsSL https://linux.fleetcru.dev \| sudo bash` | [`install-server.sh`](install-server.sh) |

The current-user Windows installer is the right choice when Pi is installed only for your Windows account. It creates a per-user scheduled task and does not require Administrator access. Its initial configuration listens on loopback. For Companion access, edit `%LOCALAPPDATA%\pi-server\config\pi-server.env` to bind `PI_SERVER_ADDR=0.0.0.0:3142`, set an auth token, and restart the task. Use only a private LAN or Tailscale network.

Rerun the installer to upgrade. Existing settings and credentials stay intact unless you explicitly supply `-Port` or `-AuthToken`, or their Linux environment-variable equivalents. Installers verify downloads before stopping the old server and attempt to restore the previous binary and configuration if startup fails. Upgrades restart the server, so finish active runs first.

The default channel is the rolling `server-dev` release. Use `-Channel stable` on Windows or `PI_SERVER_CHANNEL=stable` on Linux to select the highest stable `server-v<version>` release. A missing development release falls back to stable; network errors and checksum mismatches do not. Source builds require an explicit `-BuildFromSource` or `PI_SERVER_ALLOW_SOURCE_BUILD=1` and an exact pinned revision.

### Linux with systemd

```bash
git clone https://github.com/fleetcru/pi-stack.git
cd pi-stack
sudo PI_SERVER_AUTH_TOKEN="your-secret" ./install-server.sh
```

Install Pi for the service account first. The installer requires a running systemd, curl, Python 3, and flock. It does not install language toolchains automatically.

The service runs as the user who invoked `sudo`, which keeps access to that user's Pi installation and projects. It allows session roots under that user's home directory. Configuration is stored at `/etc/pi-server/pi-server.env` with mode `0600`.

### Windows VPS

```powershell
git clone https://github.com/fleetcru/pi-stack.git
cd pi-stack
.\install-server.ps1 -AuthToken "your-secret"
```

Run PowerShell as Administrator. This installer creates a `SYSTEM` startup task and records the current Pi executable path. A Pi installation that depends on user-only Node configuration may still be inaccessible to `SYSTEM`; use `install-server-user.ps1` in that case.

The standalone server and development launchers trust the home LAN or Tailscale network by default. Set an auth token if other devices on that network should not have Pi access.

## Quick start (development)

The standalone server and development launchers bind pi-server to port **3142**. Webby uses **5174**. Running `pi-server.exe` with no configuration listens on the home LAN and Tailscale interfaces.

### Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/) with [pnpm](https://pnpm.io/)
- [Pi CLI](https://github.com/earendil-works/pi) (`pi` in your PATH)

### Run the full stack (Windows)

```powershell
.\start-exp-live-stack.ps1
```

Opens three windows: pi-server, Webby, and a Pi TUI terminal. Install Webby's dependencies first. Startup fails if either port is already occupied or a service fails its readiness check.

For hidden server and Webby processes, without a hidden Pi TUI:

```powershell
.\start-exp-live-stack.ps1 -Background
.\stop-exp-live-stack.ps1
```

The stop script checks recorded process creation times before stopping the stack. Pass the same `-DataDir` to both scripts if you chose a custom directory. Startup logs live below `<DataDir>\.launchers\`.

### Run server only (Windows)

```powershell
.\start-exp-server.ps1
```

### Run on Linux

```bash
chmod +x start-exp-server.sh
./start-exp-server.sh
```

To run the server and Webby together, install Webby's dependencies and use:

```bash
bash start-exp-live-stack.sh
```

Ctrl+C stops both services and their child processes. The shell stack does not launch a Pi TUI. Both launchers preserve an existing `PI_SERVER_AUTH_TOKEN` and explicit CORS allowlist.

### Run with authentication

```powershell
.\start-exp-server.ps1 -AuthToken "your-secret-token"
```

### Connect from your phone

The executable and scripts auto-detect your LAN or Tailscale IP. Interactive server starts print a Companion pairing QR containing a new revocable device credential, while normal logs and access URLs go to `~/.pi/server/pi-server.log` by default. Scan the QR from Companion Settings, or read the URLs from that log file.

```
Webby: http://192.168.1.100:5174
pi-server: http://192.168.1.100:3142
```

In Companion Settings, tap **Scan pairing QR** to scan an interactive server's terminal QR. To scan and then detach a manually launched executable, use `pi-server --pair-then-background` and press Enter after scanning. Do not use that flag under Task Scheduler or systemd.

Use Webby or Desktop for server administration. The standalone `/admin/` page no longer exists; the old `-OpenAdmin` and `--open-admin` launcher options now print a migration hint.

## Configuration

All configuration is via environment variables (or CLI flags for the server):

| Variable | Default | Description |
|---|---|---|
| `PI_SERVER_ADDR` | `0.0.0.0:3142` | Listen address; reachable over the home LAN and Tailscale |
| `PI_SERVER_AUTH_TOKEN` | _(none)_ | Bearer token for API auth |
| `PI_SERVER_CWD` | `.` | Default working directory for new sessions |
| `PI_SERVER_DATA_DIR` | `.data/pi-server` | Persisted session registry and relay commands |
| `PI_SERVER_ALLOWED_ROOTS` | `.` | Restrict session CWDs to these paths |
| `PI_SERVER_ALLOWED_ORIGINS` | _(automatic private-network policy)_ | Optional strict CORS override. By default, browser apps on localhost, private LAN IPs, Tailscale IPs, and the server's own `*.ts.net` host are accepted. |
| `PI_SERVER_MAX_SESSIONS` | `8` | Max concurrent Pi sessions (0 = unlimited) |
| `PI_SERVER_MAX_ACTIVE_RUNS` | `8` | Hub-wide active local, remote, and relay runs (0 = unlimited) |
| `PI_SERVER_MAX_RUNS_PER_SESSION` | `1` | Concurrent runs permitted for one session |
| `PI_SERVER_MAX_RUNS_PER_WORKER` | `4` | Concurrent runs admitted to one worker |
| `PI_SERVER_MAX_QUEUED_RUNS` | `32` | Bounded admission queue (0 = reject immediately when busy) |
| `PI_SERVER_DISTRIBUTED_RUN_TIMEOUT` | `2h` | Fallback lease expiry when distributed lifecycle delivery is lost |
| `PI_SERVER_PI_BINARY` | `pi` | Path to the Pi CLI executable |
| `PI_SERVER_EVENT_JOURNAL_SYNC_INTERVAL` | `0` | Event-journal fsync interval; `0` keeps strict per-event durability |

## Building

Run the checks for the component you changed before opening a pull request. The full validation matrix and code-ownership guidance live in [`CONTRIBUTING.md`](CONTRIBUTING.md).

### Server

```bash
cd pi-server-exp
go build ./cmd/pi-server
go test ./...
```

### Web app

```bash
cd pi-webby-exp
pnpm install
pnpm typecheck
pnpm build
```

### Android app

```bash
cd pi-companion-exp
./gradlew :app:assembleDebug
```

## API

The server exposes an OpenAPI spec at `GET /openapi.json`. Key endpoints:

| Method | Path | Description |
|---|---|---|
| `GET` | `/healthz` | Health check + capacity info |
| `GET` | `/v1/sessions` | List sessions (local, remote, or all) |
| `POST` | `/v1/sessions` | Create a new session |
| `DELETE` | `/v1/sessions/{id}` | Delete a session |
| `POST` | `/v1/sessions/{id}/prompt` | Send an admitted local, remote, or relay prompt |
| `GET` | `/v1/sessions/{id}/messages` | Page persisted history from newest to oldest |
| `GET` | `/v1/scheduler` | Inspect active runs, queue depth, limits, and worker pressure |
| `GET` | `/metrics` | Prometheus-compatible request, runtime, and scheduler metrics |
| `GET` | `/v1/diagnostics` | Authenticated JSON operational diagnostics |
| `GET` | `/v1/sessions/{id}/ws` | WebSocket for live events |
| `POST` | `/v1/ws-tickets` | Issue a single-use WS auth ticket |
| `GET` | `/v1/workers` | List registered workers |
| `POST` | `/v1/workers` | Register a worker |
| `PATCH` | `/v1/capacity` | Update max session limit |
| `GET` | `/v1/machine-sessions` | List persisted Pi sessions on this machine |

## Relay (bridging existing TUI sessions)

To control an existing Pi TUI session from Webby or Companion:

1. Install the bridge extension:
   ```powershell
   .\install-exp-external-bridge.ps1
   ```

2. Open a new terminal and run `pi` — the bridge auto-connects to pi-server.

3. The TUI session appears in Webby/Companion under "Live TUI bridge" in the sidebar.

## Project structure

```
pi-stack/
├── pi-server-exp/      # Go HTTP/WebSocket daemon
│   ├── cmd/pi-server/  # Entry point
│   ├── internal/server/ # All server logic
│   └── extensions/     # Pi extensions (relay bridge, session title)
├── pi-webby-exp/       # React + TypeScript browser client
│   ├── src/api/        # Server client, WebSocket, hooks
│   ├── src/components/ # UI components
│   └── src/state/      # Zustand store
├── pi-desktop-app/     # Tauri v2 desktop app
│   ├── src/            # React frontend (shared components with pi-webby)
│   └── src-tauri/      # Rust backend for native OS integration
├── pi-companion-exp/   # Android Kotlin/Compose client
│   ├── docs/archive/   # Historical implementation plans
│   └── app/src/main/java/
│       ├── data/api/       # HTTP client
│       ├── data/websocket/ # WebSocket listener
│       ├── ui/sessiondetail/ # Chat UI
│       └── ui/main/        # Home screen
├── start-exp-server.*  # Server-only scripts
├── start-exp-live-stack.* # Full stack scripts
├── install-server.*    # VPS install scripts
└── install-exp-external-bridge.* # Relay bridge installer
```

## Maintenance and audit records

Useful repository-level references:

- [`CONTRIBUTING.md`](CONTRIBUTING.md) — setup, validation, review expectations, and safe change patterns
- [`AGENTS.md`](AGENTS.md) — architecture invariants and subsystem-specific pitfalls
- [`FEATURES.md`](FEATURES.md) — planned feature proposals, not necessarily implemented
- [`REMOTE-SESSION-RECOVERY-PLAN.md`](REMOTE-SESSION-RECOVERY-PLAN.md) — recovery design notes
- [`findings.md`](findings.md) — comprehensive audit history and investigation notes
- [`REMAINING-WORK.md`](REMAINING-WORK.md) — maintained audit backlog and current source of truth

`pi-companion-exp/docs/archive/` contains historical plans. Use the current code, tests, OpenAPI document, and `AGENTS.md` for implemented behavior.

When documentation and behavior disagree, verify the launcher or package manifest first, then update both the README and the relevant detailed guide in the same change.

## License

[MIT](LICENSE)
