# pi-stack

A multi-device coding agent ecosystem for [Pi](https://github.com/earendil-works/pi). Run Pi sessions from your terminal, control them from a web browser, desktop app, or Android phone, and bridge existing TUI sessions into the stack.

## What's in the box

| Project | Stack | What it does |
|---|---|---|
| **pi-server** | Go | HTTP/WebSocket hub that supervises Pi processes, proxies workers, and relays TUI sessions |
| **pi-webby** | React + TypeScript + Vite | Browser client for creating, monitoring, and chatting with Pi sessions |
| **pi-desktop** | Tauri v2 + React + TypeScript | Desktop app with native OS integration, image attachments, and offline support |
| **pi-companion** | Kotlin + Jetpack Compose | Android client with camera attachments, mobile UX, and real-time session status |

## Architecture

```
┌─────────────┐     ┌─────────────┐     ┌──────────────┐     ┌──────────────┐
│   Webby     │────▶│             │◀────│   Desktop    │     │  Companion   │
│  (browser)  │ WS  │             │ WS  │   (Electron) │     │  (Android)   │
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

## One-liner install (production)

Deploy pi-server to a remote VPS or your personal machine with a single command.

### Linux VPS (DigitalOcean, Hetzner, etc.)

```bash
curl -sSL linux.fleetcru.dev | sudo bash
```

- Installs to `/opt/pi-server`
- Creates a systemd service (auto-start on boot)
- Runs as dedicated `pi-server` user
- Config at `/etc/pi-server/pi-server.env`

### Windows VPS (Admin)

```powershell
irm https://windows.fleetcru.dev | iex
```

- Installs to `C:\pi-server`
- Creates a scheduled task (runs at startup as SYSTEM)
- Config at `C:\pi-server\config\pi-server.env`

### Personal machine (no admin)

**Linux/macOS:**
```bash
curl -sSL linux.fleetcru.dev | bash
```

**Windows:**
```powershell
irm https://get.fleetcru.dev | iex
```

- Installs to `%LOCALAPPDATA%\pi-server` (Windows) or `~/.local/share/pi-server` (Linux)
- Runs at logon under your user account
- No admin/root required

### With authentication

```bash
# Linux
PI_SERVER_AUTH_TOKEN="my-secret" curl -sSL linux.fleetcru.dev | sudo bash

# Windows
irm https://windows.fleetcru.dev -OutFile install.ps1
.\install.ps1 -AuthToken "my-secret"
```

## Quick start (development)

### Prerequisites

- [Go 1.23+](https://go.dev/dl/)
- [Node.js 20+](https://nodejs.org/) with [pnpm](https://pnpm.io/)
- [Pi CLI](https://github.com/earendil-works/pi) (`pi` in your PATH)

### Run the full stack (Windows)

```powershell
.\start-exp-live-stack.ps1
```

Opens three windows: pi-server, Webby (Vite dev server), and a Pi TUI terminal.

### Run server only (Windows)

```powershell
.\start-exp-server.ps1
```

### Run on Linux / macOS

```bash
chmod +x start-exp-server.sh
./start-exp-server.sh
```

### Run with authentication

```powershell
.\start-exp-server.ps1 -AuthToken "your-secret-token"
```

### Connect from your phone

The scripts auto-detect your LAN or Tailscale IP. Open the printed URL on your phone:

```
Webby: http://192.168.1.100:5174
pi-server: http://192.168.1.100:3142
```

## Configuration

All configuration is via environment variables (or CLI flags for the server):

| Variable | Default | Description |
|---|---|---|
| `PI_SERVER_ADDR` | `127.0.0.1:3141` | Listen address |
| `PI_SERVER_AUTH_TOKEN` | _(none)_ | Bearer token for API auth |
| `PI_SERVER_CWD` | `.` | Default working directory for new sessions |
| `PI_SERVER_DATA_DIR` | `.data/pi-server` | Persisted session registry and relay commands |
| `PI_SERVER_ALLOWED_ROOTS` | `.` | Restrict session CWDs to these paths |
| `PI_SERVER_ALLOWED_ORIGINS` | _(none)_ | CORS allowed origins (comma-separated) |
| `PI_SERVER_MAX_SESSIONS` | `8` | Max concurrent Pi sessions (0 = unlimited) |
| `PI_SERVER_ALLOW_INSECURE` | _(empty)_ | Set to `1` to allow non-loopback binding without auth |
| `PI_SERVER_PI_BINARY` | `pi` | Path to the Pi CLI executable |

## Building

### Server

```bash
cd pi-server
go build ./cmd/pi-server
go test ./... -race
```

### Web app

```bash
cd pi-webby
pnpm install
pnpm typecheck
pnpm build
```

### Android app

```bash
cd pi-companion
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
| `POST` | `/v1/sessions/{id}/prompt` | Send a prompt |
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
├── pi-server/          # Go HTTP/WebSocket daemon
│   ├── cmd/pi-server/  # Entry point
│   ├── internal/server/ # All server logic
│   └── extensions/     # Pi extensions (relay bridge, session title)
├── pi-webby/           # React + TypeScript browser client
│   ├── src/api/        # Server client, WebSocket, hooks
│   ├── src/components/ # UI components
│   └── src/state/      # Zustand store
├── pi-desktop-app/     # Tauri v2 desktop app
│   ├── src/            # React frontend (shared components with pi-webby)
│   └── src-tauri/      # Rust backend for native OS integration
├── pi-companion/       # Android Kotlin/Compose client
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

## License

[MIT](LICENSE)
