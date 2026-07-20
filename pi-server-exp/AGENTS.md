# Pi Server

Go daemon that supervises local `pi --mode rpc` subprocesses and coordinates remote `pi-server` workers over HTTP/WebSocket. It is intended for personal, trusted-machine use, including Tailscale networks.

## Pi protocol references

The implementation mirrors Pi RPC mode rather than reimplementing agent behavior.

- Pi RPC protocol: `C:\Users\basin\AppData\Local\pnpm\store\v11\links\@earendil-works\pi-coding-agent\0.80.7\97ed7555a5dad30946bcddcce7884b9ddb6fcbb6d184186767a8c79921a189ea\node_modules\@earendil-works\pi-coding-agent\docs\rpc.md`
- Pi SDK reference: `C:\Users\basin\AppData\Local\pnpm\store\v11\links\@earendil-works\pi-coding-agent\0.80.7\97ed7555a5dad30946bcddcce7884b9ddb6fcbb6d184186767a8c79921a189ea\node_modules\@earendil-works\pi-coding-agent\docs\sdk.md`
- Pi README / modes overview: `C:\Users\basin\AppData\Local\pnpm\store\v11\links\@earendil-works\pi-coding-agent\0.80.7\97ed7555a5dad30946bcddcce7884b9ddb6fcbb6d184186767a8c79921a189ea\node_modules\@earendil-works\pi-coding-agent\README.md`

Pi RPC is strict LF-delimited JSONL. Preserve that rule: do not replace the stdout parser with a generic Unicode-aware line reader.

## Architecture

- `cmd/pi-server/main.go`: CLI entrypoint and graceful OS-signal shutdown.
- `internal/server/rpc.go`: one `PiProcess` per local session; JSONL command correlation, process lifecycle, event buffer, restart/backoff.
- `internal/server/session*.go`: persisted local session specs and HTTP session lifecycle.
- `internal/server/rpc_*.go`: Pi convenience endpoint mapping plus raw RPC passthrough.
- `internal/server/ws_handler.go`: local bidirectional session WebSocket stream; exactly one goroutine writes each WebSocket.
- `internal/server/workers*.go`: remote worker registry, heartbeats, HTTP/WebSocket proxying.
- `internal/server/remote_*.go`: global coordinator session ID -> worker ID -> remote Pi session ID mapping.
- `internal/server/http_util.go`, `security.go`: auth, CORS, safe cwd roots, error/response helpers.
- `internal/server/openapi.go`: OpenAPI 3.1 document.

## Implemented features

### Local Pi sessions

- Per-session `cwd`, CLI args, optional environment, old Pi `sessionPath`, restart policy.
- Persistent local registry: `PI_SERVER_DATA_DIR/sessions.json`.
- Pi RPC commands exposed through raw `/command` and `/send`, direct session WebSocket, and convenience endpoints for prompts, queues, models, thinking, retry, compaction, bash, session tree, fork/clone, export, and extension UI responses.
- Daemon status, bounded event history, `_daemonEventId` stream cursor, HTTP/WS replay via `since`.
- App metadata: `project`, `title`, `taskType`, `owner`, `labels`, `metadata`; update with `POST /v1/sessions/{id}/metadata`.
  - Metadata belongs to the daemon session record, not Pi's JSONL session.
  - Pi `new_session`, `fork`, `clone`, and `switch_session` change Pi runtime/session state but do not automatically copy/reset daemon metadata. Apps should explicitly update metadata after those operations when desired.

### Multi-machine workers

- Persistent worker registry: `PI_SERVER_DATA_DIR/workers.json`.
- HTTP and WebSocket proxying to remote workers.
- Persistent global remote-session mappings: `PI_SERVER_DATA_DIR/remote-sessions.json`.
- Global remote session IDs work through `/v1/sessions/{id}/...` and are resolved/proxied by the coordinator.
- Worker heartbeat/capacity information and local active-session limit.

### Safety and operations

- Optional bearer auth: `PI_SERVER_AUTH_TOKEN`.
- CORS allowlist: `PI_SERVER_ALLOWED_ORIGINS`; localhost/127.0.0.1/::1 are equivalent when scheme/port match.
- CWD/file root allowlist: `PI_SERVER_ALLOWED_ROOTS`.
- Worker host allowlist: `PI_SERVER_ALLOWED_WORKER_HOSTS`.
- Limits: `PI_SERVER_MAX_SESSIONS`, `PI_SERVER_RESTART_MAX`, `PI_SERVER_RESTART_BACKOFF`.
- Basic OpenAPI: `GET /openapi.json`.
- Systemd, Windows service, and cross-platform build scripts in `scripts/`.

## Development

Run from repository root:

```powershell
gofmt -w .
go test ./...
go vet ./...
go build ./cmd/pi-server
```

The race detector needs CGO in this Windows environment. If CGO is available, also run:

```powershell
$env:CGO_ENABLED = "1"
go test -race ./...
```

Keep files focused. Add tests for behavior changes, especially process lifecycle, RPC mappings, replay cursors, worker proxying, auth/CORS, persistence, and capacity limits.

## Local test UI

Expanded manual test client:

```text
C:\Users\basin\Desktop\pi-server-full-test
```

Serve it instead of opening it as `file://`:

```powershell
cd C:\Users\basin\Desktop\pi-server-full-test
python -m http.server 8080
```

Start the daemon with a matching allowlist, for example:

```powershell
$env:PI_SERVER_ALLOWED_ORIGINS = "http://127.0.0.1:8080"
$env:PI_SERVER_ALLOWED_ROOTS = "C:\Users\basin\pi-server,C:\Users\basin\Desktop"
go run ./cmd/pi-server --addr 127.0.0.1:3141
```

`http://localhost:8080` is also accepted for that same loopback port.
