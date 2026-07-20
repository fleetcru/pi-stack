# Pi Stack

This folder contains three projects that form a multi-device Pi coding-agent ecosystem. All use Pi RPC's strict LF-delimited JSONL protocol. `pi-server` is the hub; pi-webby and pi-companion are clients.

## Projects

### pi-server (Go)

**Path:** `pi-server/` · **Go:** 1.23

HTTP/WebSocket daemon that supervises `pi --mode rpc` processes, persists session metadata, and proxies registered remote workers.

- Entry: `cmd/pi-server/main.go`
- RPC/session lifecycle: `internal/server/rpc.go`, `session*.go`
- Workers/remote mapping: `internal/server/workers*.go`, `remote_*.go`
- WebSockets/tickets: `internal/server/ws_handler.go`, `ws_tickets.go`
- API/OpenAPI: `internal/server/openapi.go`
- Security: `internal/server/security.go`

Default address: `127.0.0.1:3141`.

```powershell
cd pi-server
go test ./...
go vet ./...
go build ./cmd/pi-server
```

Read `pi-server/AGENTS.md` for server-specific protocol and architecture details.

### pi-webby (React + TypeScript)

**Path:** `pi-webby/` · **Stack:** React 19, Vite 8, TypeScript, shadcn/ui, Tailwind v4

Browser client for pi-server.

- API: `src/api/` (`PiServerClient`, ticketed `SessionSocket`)
- UI: `src/components/`
- Connection/workspace state: Zustand + TanStack Query
- Generated OpenAPI types: `src/api/types.ts` — do not hand-edit
- Multi-server state: `src/state/app-store.ts`

```powershell
cd pi-webby
pnpm typecheck
pnpm lint
pnpm build
```

### pi-companion (Android)

**Path:** `pi-companion/` · **Stack:** Kotlin, Jetpack Compose, DataStore, OkHttp

Android client for pi-server.

- API: `app/src/main/java/com/example/picompanion/data/api/`
- WebSocket: `data/websocket/SessionEventSocket.kt`
- Session UI/state: `ui/sessiondetail/`
- Worker UI/state: `ui/workers/`
- Settings/multi-server config: `data/settings/`
- OpenAPI generation script: `scripts/generate-openapi-models.ps1`; generated sources belong under `data/api/generated/`

```powershell
cd pi-companion
./gradlew :app:compileDebugKotlin :app:testDebugUnitTest
```

## Current Integration Status

### Implemented

- **Secure/reliable streaming:** Android uses pi-server WebSocket tickets and replays from `_daemonEventId` after reconnection.
- **Multi-server web UI:** Webby can add, select, and remove trusted server URLs; tokens remain memory-only.
- **Session controls:** Android supports metadata, compact, model/thinking cycling, auto retry/compact, fork, clone, new/switch/rename/abort-bash actions, and Git status/diff/log requests. Webby has its inspector settings controls.
- **File access:** Webby previews session-scoped files. Android browses server-allowed roots and previews bounded file content.
- **Image prompts:** Android supports gallery attachments and temporary app-cache camera captures, encoding images as Pi prompt payloads.
- **Workers:** Android has add/edit/delete/health controls with optional tokens/tags. Webby requires worker selection for session creation and can check remote-worker health.
- **Extension UI:** Android and web render basic confirmation/text extension requests and send `ui-response`.
- **Offline feedback:** Companion has its existing connection state; Webby shows a visible unavailable-server alert.
- **OpenAPI workflow:** Webby uses generated TypeScript types. Companion has a reproducible, committed-source Kotlin generation workflow; migration from handwritten models is incremental.

### Follow-up / polish

- Render Android image thumbnails and clearer attachment-send status.
- Replace remaining handwritten Android models with generated OpenAPI models after reviewing generator output.
- Improve worker health result presentation and confirm destructive worker deletion.
- Make session switch choose a target Pi session rather than issuing a bare switch command.
- Align exact text/error components across web and Android.
- Add more complete create-session fields on Android (`env`, metadata, session path) and on web (`env`, metadata editor).

## Future Features

`FEATURES.md` is intentionally separate from parity work. It contains designs for:

1. **Global Session Discovery** across local sessions, workers, and peer servers.
2. **Live Session Sharing** with expiring permissions, QR/deep links, and remote proxy attachment.

Do not mix these features into ordinary client parity changes without an explicit feature implementation plan.
