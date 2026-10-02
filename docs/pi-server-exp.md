# pi-server-exp — file guide

Go 1.23 HTTP/WebSocket daemon. Spawns Pi CLI processes, speaks strict LF-delimited JSONL over their stdin/stdout, and re-exposes everything to browsers, Android, desktop, and remote workers.

## Entry point

- `cmd/pi-server/main.go` — parses env/log flags, builds `server.Config`, starts the HTTP server, and wires the external relay bridge extension. Foreground logs write to `<data-dir>/pi-server.log` by default, leaving an interactive terminal clear for the Companion pairing QR. Interactive foreground starts create a revocable device credential and print a QR for the preferred Tailscale or home-LAN URL; `--pairing-qr=false` disables it. `--pair-then-background` waits for Enter after the QR scan, then relaunches detached without minting another pairing credential. `--log-file` overrides the default path. `daemon_unix.go` / `daemon_windows.go` provide background mode.

## Installation scripts

- `scripts/install-windows-service.ps1` retains its legacy filename but installs a current-user logon task. The console binary cannot run as an SCM service, so this no longer uses `sc.exe create`. Checks task ownership, rejects an existing legacy service, and supports `-WhatIf`. Use the root installers for verified downloads and managed configuration.
- `scripts/install-systemd.sh` installs an existing executable. Preserves existing environment and unit files, includes the service user's HOME/PATH on first setup, restarts on rerun, and reports startup failure.
- `scripts/bootstrap-linux-vps.sh` provisions Debian/Ubuntu with a local binary or Git source. Stages builds in unique directories, supports this monorepo and standalone server repositories, preserves existing settings and units, and restores previous files on failed startup. Refuses to overwrite a service owned by another installer. Credentials stay in the private environment file rather than terminal output. Optional Pi/Tailscale installation and agent-bundle import remain provisioning operations, not part of server-file rollback.

See [the script guide](scripts-and-extensions.md) for root installers, development launchers, and sandbox tests.

## Server core

- `internal/server/server.go` — the `Server` struct and constructor. Owns the HTTP mux, registries (sessions, remote sessions, external relays, workers, devices), admission control, metrics, and route registration. `Shutdown` drains gracefully.
- `config.go` — `Config` struct plus `ConfigFromEnv()`. Maps `PI_SERVER_*` environment variables into typed values with validation. The standalone default is `0.0.0.0:3142`, ready for trusted home-LAN and Tailscale clients.
- `capabilities.go` — `GET /v1` capability discovery and `/healthz` health check.
- `context.go` — helper wrapping requests in a timeout context.
- `errors.go` — request-ID plumbing and `writeErrorCode`, the uniform JSON error writer used by every handler.
- `http_util.go` — `writeJSON` and CORS middleware. With no explicit allowlist, browser origins on loopback, RFC1918 LAN addresses, Tailscale `100.64.0.0/10`, and the server's own `*.ts.net` hostname pass. Public origins fail. `PI_SERVER_ALLOWED_ORIGINS` replaces these defaults with a strict list.
- `operational.go` — security headers middleware, request body size limit, and the Prometheus `/metrics` endpoint.
- `request_metrics.go` — per-route request counters/latency histograms feeding the Prometheus output.
- `performance.go` — `GET /v1/performance`: in-memory performance history sampler. Collects fixed bounded rings (default 15s interval, 240 samples; `PI_SERVER_PERFORMANCE_INTERVAL` / `PI_SERVER_PERFORMANCE_HISTORY_MAX`) for pi-server itself and each managed local Pi child process. The collector clamps the interval to at least 1 second and retention to at most 5,760 samples. CPU percent is normalized across logical CPUs to 0..100 and is null for the first sample of a process identity (PID reuse resets the baseline). Samples include RSS, heap, goroutines, request/error rates, average latency, and admission active/queued counts; the requests array summarizes per-route count/errors/average/max latency. Windows reads stats via `x/sys/windows` (`K32GetProcessMemoryInfo` + `GetProcessTimes`), Linux via `/proc/<pid>/stat`; unsupported platforms return null CPU/RSS values. The sampler goroutine selects on `stopHeartbeat`, so `Shutdown` stops it; session histories are pruned when sessions are deleted. Process PID/running state is read via the minimal `PiProcess.ProcessTarget()` after releasing `SessionRegistry.mu`, preserving lock ordering.
- `path.go` — URL-path splitting (`/v1/sessions/{id}/...`), session-ID validation, CWD normalization against allowed roots.
- `security.go` — host allowlist checks and `allowedCWD()` enforcement (every session creation and file access goes through this).
- `network.go` — `PreferredAddresses()` ranks adapter addresses for client advertising. Virtual and container adapters (WSL, Hyper-V, Docker, VMware, VirtualBox, Bluetooth, Tailscale) are excluded, wireless is preferred over wired, and link-local and carrier-grade NAT addresses are rejected. Both the startup log/QR and the Admin pairing list use this single source of truth so they cannot disagree on which host is reachable.
- `openapi.go` — hand-maintained OpenAPI document served at `/openapi.json`.

## RPC to Pi processes

- `config.go` — `Config`, `ConfigFromEnv`, admin overrides, and the shared `DefaultEventHistoryMax` (100) / `DefaultEventHistoryBytes` (2 MiB) constants. Those constants are the single source of truth for the event-ring bounds: `ConfigFromEnv` seeds them into `Config`, and `NewPiProcess` falls back to them for a hand-built `Config`, so the two paths cannot drift.
- `rpc.go` — the heart of the daemon. `PiProcess` wraps one Pi child process: writes JSONL commands to stdin, reads events from stdout, assigns each event a monotonic uint64 ID, and keeps a ring buffer (count + byte bounded) so late WebSocket subscribers can replay. Also handles request/response correlation for commands that expect a reply. `Close` is terminal (sets `closed`, closes the journal); `Stop` halts the child but leaves the process reusable for the idle reaper.
- `rpc_handlers.go` — HTTP handlers for the generic RPC surface: `POST/GET /v1/sessions/{id}/rpc`, prompt delivery, raw send.
- `rpc_actions.go` — convenience action dispatch (`prompt`, `abort`, model/thinking changes, extension UI responses) mapping REST bodies into RPC commands, plus local-run admission checks.
- `rpc_catalog.go` — publishes the list of supported RPC actions for clients.
- `available_models.go` — serves `GET /v1/models` by running `pi --list-models` in the configured server directory and parsing its provider/model table. New-session composers can load providers and models before a session exists.
- `codec.go` — `EventCodec` interface (JSON today, pluggable later).
- `process_windows.go` / `process_other.go` — platform `applyProcessAttrs`: on Windows sets `CREATE_NO_WINDOW` so child Pi processes never flash a console.

## Sessions

- `session.go` — `SessionSpec` (id, cwd, args, env, metadata, worktree branch) and `SessionRegistry`, the JSON-file-persisted map of sessions with `AttachIfAbsent` semantics to prevent orphaned duplicate processes. Inventory status prefers agent runtime (`idle`/`working`) over process liveness.
- `session_handlers.go` — create/delete session endpoints. Handles the auto-worktree flow (`createWorktree.enabled` → `<repo>/.pi-worktrees/<title>` + `feature/<title>` branch), mutual exclusion with explicit `worktreePath`, and cleanup on delete including the Windows fallback (clear read-only attrs → `os.RemoveAll` → `worktree prune` → `branch -D`).
- `session_inventory.go` — the unified session list. Merges local sessions, remote worker sessions, machine-discovered sessions, and live relay sessions into one `SessionSummary` shape. Relay sessions win over local processes for the same JSONL file (two Pi processes on one file corrupt history).
- `session_inventory` helpers in `machine_sessions.go` — scans `~/.pi/agent/sessions/*.jsonl` to discover Pi sessions the server didn't spawn itself, with an mtime cache.
- `global_sessions.go` — worker-scoped session addressing (`workerID:sessionID`) so a client can attach to a session on any worker with one ID.
- `session_history.go` — paginated reading of session JSONL history with an in-memory index cache; powers the history replay clients show on open. Managed sessions merge all transcript files in their session directory so restarts do not hide earlier messages.
- `history_ownership.go` — reserves which server component owns a session's JSONL file so a bridged relay and a managed process never both own it. Lock files in `<DataDir>/history-locks/` record the owner PID; when the lock already exists and its PID is dead (crashed server), the lock is reclaimed automatically instead of failing with "already owned by another server". PID liveness is platform-specific (`pid_windows.go` uses `OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION)`, `pid_unix.go` uses `Signal(0)`). Release closes stored claims by session owner ID, even when deletion prevents expanding a Windows 8.3 history path back to its original lock key. Continuation tests compare canonical paths and include short-name temporary directories.
- `session_state_cache.go` — short-TTL cache of Pi process state queries to avoid hammering idle processes.
- `session_bridge.go` — maps managed sessions onto the native Pi session directory so inventory discovery can find them.
- `session_transport.go` — abstraction over where a session's events come from (local process vs relay), exposing a uniform `SessionTransport` interface.
- `metadata_handlers.go` — `PATCH`-style metadata updates on sessions (titles, pinned flags, worktree branch).
- `capacity.go` — per-request session-count enforcement against `PI_SERVER_MAX_SESSIONS`. When the limit is reached it evicts the longest-idle eligible process via the idle reaper before failing, so a new session displaces a dormant one instead of being rejected.
- `idle_reaper.go` — reclaims memory from dormant sessions. An idle Pi RPC child holds roughly 200 MB RSS whether or not it is working, so a hub with many open sessions pays for every one of them. The reaper stops children idle longer than `PI_SERVER_IDLE_PROCESS_TIMEOUT` (default 20m, non-positive disables) on a 30s sweep, keeping the `SessionSpec` and the durable event journal so the next prompt transparently relaunches Pi from the same spec.

  `PiProcess.Stop()` is deliberately distinct from `PiProcess.Close()`. `Close` is terminal: it sets `closed`, fails every later `Start()` with "session closed", and closes the journal. `Stop` clears only the OS child, so the session stays usable. A `reaping` flag makes `wait()` skip the restart path, otherwise a session with `Restart=true` would respawn the moment it was reclaimed; `Start()` resets that flag and the restart budget.

  A process is eligible only when it is running, not closed, not already reaping, in runtime state `idle`, the idle duration has elapsed, `wsSubscribers == 0`, `pendingExtensionUiRequest == nil`, and `admissionHeld == false`. Each guard prevents losing something live: a streaming client, an unanswered extension dialog, or a checked-out run slot. Under capacity pressure `reapOldestIdle` may evict before the full timeout, but never inside a 60s grace period, so an in-flight prompt is not the victim.

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

- `workers.go` — worker registry: registration, tokens (`SensitiveString` redacts itself in logs and `%v`), persisted to disk. Worker-scoped `GET /v1/workers/{id}/directories` and `/models` proxy each worker's allowed roots and model catalog so new-session controls reflect the selected machine.
- `worker_heartbeat.go` — periodic worker health checks; marks workers stale/offline.
- `worker_paths.go` — parsing of worker-scoped URL paths.
- `remote_sessions.go` — registry of sessions that live on workers, persisted locally.
- `remote_session_handlers.go` — create/list/attach remote session endpoints; forwards to the worker.
- `remote_proxy.go` — HTTP proxying of session actions to workers with SSRF mitigation (scheme/host validation, optional URL allowlist).
- `worker_ws_proxy.go` — bidirectional WebSocket proxy between browser and worker, with two pumps and completion cleanup.

## External relay (Pi TUI bridging)

The `external-session-bridge.ts` extension in a user's Pi TUI connects back to the server so their terminal session appears in all clients.

To continue a terminal session in Companion, restart the rebuilt server, load the updated bridge in the TUI, and then quit the TUI. Open that same session in Companion and tap "Continue on server". The server resumes its saved history through RPC without a live bridge. Old sessions without verified process identity must be opened once with the updated bridge before this action is available. The server and TUI must run on the same Windows or Linux machine; a relay network interruption does not authorize takeover. Once RPC owns the history, do not open a second TUI against that file while the server is using it.

- `external_sessions.go` — `ExternalRegistry` and `ExternalSession` types; tracks connected relays and queued commands. Local registrations verify a random on-disk bridge proof and capture the TUI's kernel process-creation identity. `POST /v1/external-sessions/{id}/continue` rejects connected relays, queued work, unverified process exit, invalid history/CWD, conflicting owners, and capacity exhaustion. Successful handoff preserves the session ID and advances the RPC event cursor beyond the relay cursor. Verified identities persist in dedicated relay-spec fields, separate from app-editable metadata, so an exited TUI remains continuable after a server restart.
- `external_transfer.go` / `continue_session.go` — fences relay registration, command delivery, and event publication while ownership transfers. Validates a JSONL path under the native or managed Pi history roots, checks the header ID/CWD, and waits for an RPC state response confirming the exact session file and ID before publishing ownership. Like machine-session resume, the verified local history's CWD can be outside the server launch directory; file APIs still enforce AllowedRoots. Failed startup leaves the relay record retryable. Windows and Linux verify PID plus kernel creation identity; other platforms and remote-host bridges refuse takeover. Existing relay viewers disconnect and reconnect to RPC. Machine discovery keeps disconnected relays as the owner until explicit continuation.
- `external_command_store.go` — atomic (write-temp + rename) persistence of queued relay commands so they survive a server crash.
- `external_ws.go` — the inbound WebSocket a bridged TUI connects to; handles receipt acks and command delivery.
- `external_relay_ws.go` — the viewer-facing WebSocket: clients subscribe to a relay session's event stream and send prompts, which are queued as commands and only acknowledged after the relay's `message_start` proves delivery.
- Slash-command relay messages are routed through `AgentSession.prompt` with command expansion enabled, so extension, skill, and prompt-template commands execute locally instead of reaching the LLM.
- `external_history.go` — reads a relayed session's JSONL directly to serve history pages and stats for relay sessions.

## Files and git

- `file_handlers.go` — directory listing and file-tree endpoints for a session's cwd.
- `file_content.go` — file read/write with `allowedFilePath` (symlink-resolution + root confinement) and binary-sniffing so images return as samples.
- `file_watcher.go` — fsnotify watcher per session emitting `file_change` events to connected clients; skips the server-generated `.data` tree to prevent event-journal writes from appearing as user file changes.
- `directory_handler.go` — allowed directory roots listing (drives the directory pickers in clients).
- `git_handlers.go` — git status/branches/worktrees/commit/push endpoints. All git arguments come from a fixed whitelist. Implements the T3-Code-style enriched status (upstream, remote, GitHub repo parse) and worktree create/remove.

## Devices, admin, diagnostics

- `devices.go` — persisted device registry used by QR pairing (Companion scans a code, server records the device). `DELETE /v1/devices/{id}/purge` permanently removes a device record. Device administration requires the bootstrap bearer token when authentication is enabled, works without a token when server authentication is disabled, and never accepts a paired-device token as administrator credentials.
- `admin.go` / `admin_config.go` — runtime-adjustable admin settings (capacity limits, durations) that overlay the env config. `applyRuntimeSettings` hot-applies the safe subset, and a 2-second poller on `admin-config.json` re-applies it live when the file changes. `PersistAdminConfig` rewrites the effective configuration back to disk after CLI parsing so a stale persisted `addr` cannot pin a previous listen address across restarts. Structural settings (addr, cwd, dataDir, piBinary, server timeouts) still require a restart. The embedded HTML dashboard and its cookie/CSRF session machinery have been removed; administration is now done through the bearer-token JSON API used by Webby and Desktop: `GET /v1/admin/state` and `PUT /v1/admin/settings`. These require the configured bootstrap bearer token when auth is enabled (`requireBearerAdmin`), are open when auth is disabled, and never accept paired-device tokens. The settings PUT returns `restartRequired` for structural changes.
- `diagnostics.go` — aggregated health snapshot (sessions, workers, uptime, versions).
- `command_receipts.go` — persisted receipts for delivered commands so clients can reconcile "did my prompt actually arrive" after reconnects.
- `scheduler_handler.go` — scheduler introspection endpoint.

WebSocket events are deep-cloned before they enter replay/subscriber buffers.
This prevents nested Pi message content from being mutated while a client is
encoding it, and the JSON WebSocket writer converts unexpected encoder panics
into connection errors instead of crashing the server.

### Runtime-state durability

Synthetic `runtime_state` events are now appended to the per-session event journal and compacted with the normal event ring. Reconnecting WebSocket/SSE clients can therefore recover the same status transitions used by mobile and desktop notification logic.
