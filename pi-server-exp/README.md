# pi-server

A small, robust Go daemon that supervises `pi --mode rpc` child processes and exposes them over HTTP + WebSocket.

## Why Go?

Go is the least-headache choice here: simple single-binary deployment, excellent HTTP tooling, straightforward concurrency, and good process supervision primitives.

## Run

```bash
go run ./cmd/pi-server --addr 127.0.0.1:3141
```

Environment variables:

- `PI_SERVER_ADDR` default `127.0.0.1:3141`
- `PI_SERVER_PI_BINARY` default `pi`
- `PI_SERVER_CWD` default current directory
- `PI_SERVER_DATA_DIR` default `~/.pi/server`
- `PI_SERVER_REQUEST_TIMEOUT` default `30s`
- `PI_SERVER_READ_TIMEOUT` default `30s`; `PI_SERVER_WRITE_TIMEOUT` default `60s`; `PI_SERVER_IDLE_TIMEOUT` default `120s`
- `PI_SERVER_SHUTDOWN_TIMEOUT` default `10s`
- `PI_SERVER_MAX_SESSIONS` default `2` active Pi child processes
- `PI_SERVER_EVENT_HISTORY_MAX` default `100` retained events per local session
- `PI_SERVER_EVENT_HISTORY_BYTES` default `2097152` (2 MiB) retained event payload budget per local session
- `PI_SERVER_MAX_WATCHES` default `2048` directories watched per session; dependency, VCS, cache, and generated-output directories are skipped
- `PI_SERVER_DEBUG=1` enables debug logging
- `PI_SERVER_AUTH_TOKEN` enables bearer-token authentication
- `PI_SERVER_ALLOWED_ORIGINS` comma-separated browser origin allowlist
- `PI_SERVER_ALLOWED_ROOTS` comma-separated cwd/file API root allowlist
- `PI_SERVER_ALLOWED_WORKER_HOSTS` comma-separated remote worker host allowlist

For any network-accessible deployment, set an auth token and all three allowlists. The browser test page should be served from an allowed HTTP origin rather than opened as `file://`.

## API

### Health

```bash
curl http://127.0.0.1:3141/healthz
```

### Create a Pi RPC session

```bash
curl -X POST http://127.0.0.1:3141/v1/sessions \
  -H 'content-type: application/json' \
  -d '{"cwd":"/path/to/project","args":["--no-session"],"start":true}'
```

Create a session for an existing/old Pi session, preserving the cwd Pi needs for context/resource discovery:

```bash
curl -X POST http://127.0.0.1:3141/v1/sessions \
  -H 'content-type: application/json' \
  -d '{"cwd":"/path/to/original/project","sessionPath":"/path/to/session.jsonl","start":true}'
```

`sessionPath` is passed to Pi as `--session <path|id>`, so it can be a full JSONL path or a Pi session id/partial id that Pi can resolve.

### Discover supported wrapper endpoints

```bash
curl http://127.0.0.1:3141/v1/rpc/commands
```

### Convenience Pi RPC endpoints

The daemon now exposes common Pi RPC commands as small HTTP endpoints:

```text
GET  /v1/sessions/<id>/state
GET  /v1/sessions/<id>/messages
GET  /v1/sessions/<id>/stats
GET  /v1/sessions/<id>/models
GET  /v1/sessions/<id>/commands
GET  /v1/sessions/<id>/entries?since=<entry-id>
GET  /v1/sessions/<id>/tree
GET  /v1/sessions/<id>/last-assistant-text
GET  /v1/sessions/<id>/fork-messages
POST /v1/sessions/<id>/prompt
POST /v1/sessions/<id>/steer
POST /v1/sessions/<id>/follow-up
POST /v1/sessions/<id>/abort
POST /v1/sessions/<id>/compact
POST /v1/sessions/<id>/bash
POST /v1/sessions/<id>/ui-response
```

Example:

```bash
curl -X POST http://127.0.0.1:3141/v1/sessions/<id>/prompt \
  -H 'content-type: application/json' \
  -d '{"message":"Hello"}'
```

### Raw command and wait for its Pi RPC response

```bash
curl -X POST http://127.0.0.1:3141/v1/sessions/<id>/command \
  -H 'content-type: application/json' \
  -d '{"type":"get_state"}'
```

### Stream events and send commands over WebSocket

Connect to:

```text
ws://127.0.0.1:3141/v1/sessions/<id>/ws
```

Add `?watch=files` to opt into workspace file-change events. It is disabled by default so a chat-only client does not allocate a recursive filesystem watcher.

Send Pi RPC JSON objects, for example:

```json
{"type":"prompt","message":"Hello from the daemon"}
```

The socket streams Pi RPC events/responses back as JSON objects. Every daemon-streamed event includes `_daemonEventId`, a monotonic per-process cursor. Reconnect with `?since=<last-event-id>` to replay buffered events after that cursor:

```text
ws://127.0.0.1:3141/v1/sessions/<id>/ws?since=42
```

HTTP history supports the same cursor:

```text
GET /v1/sessions/<id>/events?since=42&limit=200
```

### Fire-and-forget command

```bash
curl -X POST http://127.0.0.1:3141/v1/sessions/<id>/send \
  -H 'content-type: application/json' \
  -d '{"type":"abort"}'
```

### Delete a session

```bash
curl -X DELETE http://127.0.0.1:3141/v1/sessions/<id>
```

## Notes

- The daemon preserves Pi's strict JSONL semantics internally and never uses generic Unicode line splitting.
- Each daemon session owns a `pi --mode rpc` child process.
- Each session stores its own `cwd`, args, optional env, and optional old Pi session path/id.
- Session metadata is persisted in `PI_SERVER_DATA_DIR/sessions.json`, so old-session cwd mappings survive daemon restarts.
- REST `/command` injects a correlation id and waits for matching `type=response`.
- WebSocket is bidirectional and suitable for UIs: send commands, receive streaming events.
