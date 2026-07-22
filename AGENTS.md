# Pi Stack — Agent Guide

This repository contains three projects forming a multi-device Pi coding-agent ecosystem. This document is for AI agents and developers working on the codebase.

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

All communication uses Pi RPC's strict LF-delimited JSONL protocol over stdin/stdout. WebSocket connections use single-use tickets for auth.

## Projects

### pi-server-exp (Go 1.23)

**Path:** `pi-server-exp/`

HTTP/WebSocket daemon — the hub of the stack.

| Area | Files |
|---|---|
| Entry point | `cmd/pi-server/main.go` |
| Server setup / routes | `internal/server/server.go` |
| RPC / process lifecycle | `internal/server/rpc.go` |
| Session registry | `internal/server/session.go`, `session_handlers.go` |
| Session inventory | `internal/server/session_inventory.go` |
| WebSocket handler | `internal/server/ws_handler.go` |
| WS ticket auth | `internal/server/ws_tickets.go` |
| Workers | `internal/server/workers.go`, `worker_heartbeat.go`, `worker_paths.go` |
| Remote sessions | `internal/server/remote_sessions.go`, `remote_proxy.go` |
| External relay | `internal/server/external_sessions.go`, `external_ws.go`, `external_relay_ws.go`, `external_command_store.go` |
| File access | `internal/server/file_content.go`, `file_handlers.go`, `file_watcher.go`, `directory_handler.go` |
| Git handlers | `internal/server/git_handlers.go` |
| Security / CORS | `internal/server/security.go`, `http_util.go` |
| Config | `internal/server/config.go` |
| OpenAPI | `internal/server/openapi.go` |
| Extensions | `extensions/external-session-bridge.ts`, `extensions/session-title.ts` |

```bash
cd pi-server-exp
go test ./... -race
go vet ./...
go build ./cmd/pi-server
```

### pi-webby-exp (React + TypeScript)

**Path:** `pi-webby-exp/` · **Stack:** React 19, Vite 8, TypeScript, shadcn/ui, Tailwind v4

Browser client for pi-server.

| Area | Files |
|---|---|
| API client | `src/api/client.ts` |
| WebSocket | `src/api/session-socket.ts` |
| React hooks | `src/api/hooks.ts` |
| App state | `src/state/app-store.ts` |
| Main layout | `src/components/workspace-shell.tsx` |
| Chat workspace | `src/components/session-workspace.tsx` |
| Inspector panel | `src/components/session-inspector.tsx` |
| Create session | `src/components/create-session-dialog.tsx` |
| Server connections | `src/components/server-connections-dialog.tsx` |
| Generated types | `src/api/types.ts` — do not hand-edit |

```bash
cd pi-webby-exp
pnpm install
pnpm typecheck
pnpm lint
pnpm build
pnpm test
```

### pi-companion-exp (Android)

**Path:** `pi-companion-exp/` · **Stack:** Kotlin, Jetpack Compose, DataStore, OkHttp

Android client for pi-server.

| Area | Files |
|---|---|
| HTTP client | `app/src/main/java/.../data/api/PiServerClient.kt` |
| WebSocket | `app/src/main/java/.../data/websocket/SessionEventSocket.kt` |
| Models | `app/src/main/java/.../data/model/` |
| Repositories | `app/src/main/java/.../data/repository/` |
| Settings | `app/src/main/java/.../data/settings/` |
| Home screen | `app/src/main/java/.../ui/main/` |
| Session detail | `app/src/main/java/.../ui/sessiondetail/` |
| Sessions list | `app/src/main/java/.../ui/sessions/` |
| Workers | `app/src/main/java/.../ui/workers/` |
| Settings UI | `app/src/main/java/.../ui/settings/` |

```bash
cd pi-companion-exp
./gradlew :app:compileDebugKotlin
./gradlew :app:testDebugUnitTest
```

### pi-desktop (Electron)

**Path:** `pi-desktop/` · **Stack:** React, TypeScript, Vite, Electron

Desktop client with native OS integration. Shares API client and components with pi-webby.

```bash
cd pi-desktop
pnpm install
pnpm dev
pnpm build
```

## Key Design Decisions

### Concurrency

- **Lock ordering:** `SessionRegistry.mu` → `PiProcess.mu`. Never reverse. `ListSpecs()` and `ActiveCount()` copy data under RLock, release, then call `Status()` outside.
- **Write serialization:** WebSocket connections use `writeMu` to serialize all writes (events, nacks, ping frames).
- **Subscriber dispatch:** Copies subscriber set under write lock before iterating outside lock.

### Security

- **Auth:** Bearer token via `Authorization` header. Token fingerprint (SHA-256) for WS ticket binding.
- **CORS:** Rejects browser cross-origin requests when no origins configured. Non-browser clients (no Origin header) pass through.
- **File access:** `filepath.EvalSymlinks` + `allowedFilePath` prevents symlink escapes. Git args from fixed whitelist only.
- **Worker proxy:** SSRF mitigation via scheme/host validation and optional allowlist. Only pre-registered URLs contacted.
- **SensitiveString:** Worker tokens redact on `String()`/`GoString()` to prevent log leakage.

### Relay Sessions

External Pi TUI sessions bridge into pi-server via the `external-session-bridge.ts` extension.

- **Lease rotation:** New bridge detaches old relay. Old relay's read goroutine exits via `isCurrentRelay()` check.
- **Command persistence:** Commands saved synchronously under lock before `enqueue()` returns. Atomic rename prevents truncated stores.
- **Event ring:** Dual-bound (200 count + 8MB bytes). Matches PiProcess semantics.
- **Detach/close ordering:** LIFO — close then detach to prevent stale relay from clobbering new one.

### Event Deduplication

- **Server:** Monotonic uint64 event IDs per session. `events_lost` sentinel when cursor predates the ring.
- **Webby:** `seenEventIds` Map with 10K eldest-eviction. Generation guard after reconnect.
- **Companion:** `LinkedHashMap` with 2K eldest-eviction + generation counter for stale callback prevention.

## Scripts

| Script | Purpose |
|---|---|
| `start-exp-server.ps1` / `.sh` | Server only (dev) |
| `start-exp-live-stack.ps1` / `.sh` | Server + Web + Pi TUI (dev) |
| `install-exp-external-bridge.ps1` | Install relay bridge extension |
| `install-server.sh` | Linux VPS install (systemd) |
| `install-server.ps1` | Windows VPS install (admin, scheduled task) |
| `install-server-user.ps1` | Windows personal install (no admin) |

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `PI_SERVER_ADDR` | `127.0.0.1:3141` | Listen address |
| `PI_SERVER_AUTH_TOKEN` | _(none)_ | Bearer token for API auth |
| `PI_SERVER_CWD` | `.` | Default working directory |
| `PI_SERVER_DATA_DIR` | `.data/pi-server` | Persisted data |
| `PI_SERVER_ALLOWED_ROOTS` | `.` | Restrict session CWDs |
| `PI_SERVER_ALLOWED_ORIGINS` | _(none)_ | CORS origins (comma-separated) |
| `PI_SERVER_MAX_SESSIONS` | `8` | Max concurrent sessions (0 = unlimited) |
| `PI_SERVER_ALLOW_INSECURE` | _(empty)_ | `1` to allow non-loopback without auth |
| `PI_SERVER_PI_BINARY` | `pi` | Path to Pi CLI |
| `PI_SERVER_PI_EXTENSIONS` | _(none)_ | Extensions to load |

## Testing

```bash
# Server
cd pi-server-exp && go test ./... -race -count=3

# Web
cd pi-webby-exp && pnpm test

# Android
cd pi-companion-exp && ./gradlew :app:testDebugUnitTest
```

## Common Pitfalls

1. **Don't hold `SessionRegistry.mu` while calling `PiProcess.Status()`** — can deadlock if a callback acquires the registry lock.
2. **Don't write to DataStore inside a `map` transform** — causes deadlock. Extract migration to a suspend function.
3. **Don't use `!!` on nullable DataStore preferences** — crashes on first launch. Use `!= true` instead.
4. **Don't create PiProcess without checking for existing** — use `AttachIfAbsent` to prevent orphaned processes.
5. **Don't forget `response.use {}` in OkHttp** — unclosed responses leak connections.
