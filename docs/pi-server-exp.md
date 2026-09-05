# pi-server-exp — file guide

Go 1.23 HTTP/WebSocket daemon. Spawns Pi CLI processes, speaks strict LF-delimited JSONL over their stdin/stdout, and re-exposes everything to browsers, Android, desktop, and remote workers.

## Entry point

- `cmd/pi-server/main.go` — parses env/log flags, builds `server.Config`, starts the HTTP server, wires the external relay bridge extension. `daemon_unix.go` / `daemon_windows.go` provide the `daemonize()` fork-into-background step (Windows uses a detached child process; Unix uses double-fork style daemonization).

## Server core

- `internal/server/server.go` — the `Server` struct and constructor. Owns the HTTP mux, registries (sessions, remote sessions, external relays, workers, devices), admission control, metrics, and route registration. `Shutdown` drains gracefully.
- `config.go` — `Config` struct plus `ConfigFromEnv()`. Maps `PI_SERVER_*` environment variables (address, auth token, data dir, allowed roots/origins, capacity limits, timeouts) into typed values with validation.
- `capabilities.go` — `GET /v1` capability discovery and `/healthz` health check.
- `context.go` — helper wrapping requests in a timeout context.
- `errors.go` — request-ID plumbing and `writeErrorCode`, the uniform JSON error writer used by every handler.
- `http_util.go` — `writeJSON`, CORS middleware (rejects browser cross-origin when no origins configured; non-browser requests pass), loopback-origin equivalence checks.
- `operational.go` — security headers middleware, request body size limit, and the Prometheus `/metrics` endpoint.
- `request_metrics.go` — per-route request counters/latency histograms feeding the Prometheus output.
- `path.go` — URL-path splitting (`/v1/sessions/{id}/...`), session-ID validation, CWD normalization against allowed roots.
- `security.go` — host allowlist checks and `allowedCWD()` enforcement (every session creation and file access goes through this).
- `openapi.go` — hand-maintained OpenAPI document served at `/openapi.json`.

## RPC to Pi processes

- `rpc.go` — the heart of the daemon. `PiProcess` wraps one Pi child process: writes JSONL commands to stdin, reads events from stdout, assigns each event a monotonic uint64 ID, and keeps a ring buffer (count + byte bounded) so late WebSocket subscribers can replay. Also handles request/response correlation for commands that expect a reply.
- `rpc_handlers.go` — HTTP handlers for the generic RPC surface: `POST/GET /v1/sessions/{id}/rpc`, prompt delivery, raw send.
- `rpc_actions.go` — convenience action dispatch (`prompt`, `abort`, model/thinking changes, extension UI responses) mapping REST bodies into RPC commands, plus local-run admission checks.
- `rpc_catalog.go` — publishes the list of supported RPC actions for clients.
- `codec.go` — `EventCodec` interface (JSON today, pluggable later).
- `process_windows.go` / `process_other.go` — platform `applyProcessAttrs`: on Windows sets `CREATE_NO_WINDOW` so child Pi processes never flash a console.

## Sessions

- `session.go` — `SessionSpec` (id, cwd, args, env, metadata, worktree branch) and `SessionRegistry`, the JSON-file-persisted map of sessions with `AttachIfAbsent` semantics to prevent orphaned duplicate processes.
- `session_handlers.go` — create/delete session endpoints. Handles the auto-worktree flow (`createWorktree.enabled` → `<repo>/.pi-worktrees/<title>` + `feature/<title>` branch), mutual exclusion with explicit `worktreePath`, and cleanup on delete including the Windows fallback (clear read-only attrs → `os.RemoveAll` → `worktree prune` → `branch -D`).
- `session_inventory.go` — the unified session list. Merges local sessions, remote worker sessions, machine-discovered sessions, and live relay sessions into one `SessionSummary` shape. Relay sessions win over local processes for the same JSONL file (two Pi processes on one file corrupt history).
- `session_inventory` helpers in `machine_sessions.go` — scans `~/.pi/agent/sessions/*.jsonl` to discover Pi sessions the server didn't spawn itself, with an mtime cache.
- `global_sessions.go` — worker-scoped session addressing (`workerID:sessionID`) so a client can attach to a session on any worker with one ID.
- `session_history.go` — paginated reading of session JSONL history with an in-memory index cache; powers the history replay clients show on open.
- `history_ownership.go` — reserves which server component owns a session's JSONL file so a bridged relay and a managed process never both own it. Lock files in `<DataDir>/history-locks/` record the owner PID; when the lock already exists and its PID is dead (crashed server), the lock is reclaimed automatically instead of failing with "already owned by another server". PID liveness is platform-specific (`pid_windows.go` uses `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)`, `pid_unix.go` uses `Signal(0)`).
- `session_state_cache.go` — short-TTL cache of Pi process state queries to avoid hammering idle processes.
- `session_bridge.go` — maps managed sessions onto the native Pi session directory so inventory discovery can find them.
- `session_transport.go` — abstraction over where a session's events come from (local process vs relay), exposing a uniform `SessionTransport` interface.
- `metadata_handlers.go` — `PATCH`-style metadata updates on sessions (titles, pinned flags, worktree branch).
- `capacity.go` — per-request session-count enforcement against `PI_SERVER_MAX_SESSIONS`.

## Admission and scheduling

- `task_admission.go` — hub-wide run admission: global max, per-session, and per-worker active-run limits with a bounded wait queue (`PI_SERVER_MAX_QUEUED_RUNS`).
- `task_scheduler.go` — priority queue of waiting runs; higher priority dequeues first.
- `distributed_admission.go` — the same accounting for remote/relay runs: leases, per-session run tracking, and cleanup channels.
- `distributed_persistence.go` — persists active distributed runs to disk so a server restart can re-subscribe to still-running remote work instead of losing them.

## Event journaling

- `event_journal.go` — append-only per-session event journal on disk (fsync interval configurable via `PI_SERVER_EVENT_JOURNAL_SYNC_INTERVAL`; default strict per-event durability). Restores the ring buffer after restart.
- `daemon_handlers.go` — daemon status endpoint and cross-session event history queries.

## WebSocket transports

- `ws_handler.go` — the per-session browser WebSocket: ticket auth, `since` cursor replay from the ring buffer, event fan-out with a per-connection write mutex, and nacks.
- `ws_multiplex.go` — multiplexed WebSocket allowing one connection to subscribe to multiple sessions (`subscribe`/`unsubscribe` messages routed to the right Pi process).
- `ws_tickets.go` — single-use, short-TTL-ticket store. Tickets are bound to the SHA-256 fingerprint of the bearer token so a leaked ticket is useless without the token. Browser WebSocket upgrades cannot send an Authorization header, so an upgrade presenting no credential (`anonymous` fingerprint) is accepted; the ticket itself is the credential. A upgrade presenting a *different* token is rejected.

(SSE support described in AGENTS.md is served through the same session WebSocket handlers; there is no separate `sse_handler.go` in the current tree.)

## Workers (remote Pi instances)

- `workers.go` — worker registry: registration, tokens (`SensitiveString` redacts itself in logs and `%v`), persisted to disk.
- `worker_heartbeat.go` — periodic worker health checks; marks workers stale/offline.
- `worker_paths.go` — parsing of worker-scoped URL paths.
- `remote_sessions.go` — registry of sessions that live on workers, persisted locally.
- `remote_session_handlers.go` — create/list/attach remote session endpoints; forwards to the worker.
- `remote_proxy.go` — HTTP proxying of session actions to workers with SSRF mitigation (scheme/host validation, optional URL allowlist).
- `worker_ws_proxy.go` — bidirectional WebSocket proxy between browser and worker, with two pumps and completion cleanup.

## External relay (Pi TUI bridging)

The `external-session-bridge.ts` extension in a user's Pi TUI connects back to the server so their terminal session appears in all clients.

- `external_sessions.go` — `ExternalRegistry` and `ExternalSession` types; tracks connected relays and the commands pending delivery to them.
- `external_command_store.go` — atomic (write-temp + rename) persistence of queued relay commands so they survive a server crash.
- `external_ws.go` — the inbound WebSocket a bridged TUI connects to; handles receipt acks and command delivery.
- `external_relay_ws.go` — the viewer-facing WebSocket: clients subscribe to a relay session's event stream and send prompts, which are queued as commands and only acknowledged after the relay's `message_start` proves delivery.
- `external_history.go` — reads a relayed session's JSONL directly to serve history pages and stats for relay sessions.

## Files and git

- `file_handlers.go` — directory listing and file-tree endpoints for a session's cwd.
- `file_content.go` — file read/write with `allowedFilePath` (symlink-resolution + root confinement) and binary-sniffing so images return as samples.
- `file_watcher.go` — fsnotify watcher per session emitting `file_change` events to connected clients.
- `directory_handler.go` — allowed directory roots listing (drives the directory pickers in clients).
- `git_handlers.go` — git status/branches/worktrees/commit/push endpoints. All git arguments come from a fixed whitelist. Implements the T3-Code-style enriched status (upstream, remote, GitHub repo parse) and worktree create/remove.

## Devices, admin, diagnostics

- `devices.go` — persisted device registry used by QR pairing (Companion scans a code, server records the device).
- `admin.go` / `admin_config.go` — built-in admin page and runtime-adjustable admin settings (capacity limits, durations) that overlay the env config.
- `diagnostics.go` — aggregated health snapshot (sessions, workers, uptime, versions).
- `command_receipts.go` — persisted receipts for delivered commands so clients can reconcile "did my prompt actually arrive" after reconnects.
- `scheduler_handler.go` — scheduler introspection endpoint.
