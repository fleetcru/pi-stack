# Pi Stack — Agent Guide

This repository contains three projects forming a multi-device Pi coding-agent ecosystem. This document is for AI agents and developers working on the codebase.

> **Read `docs/` first** — it contains per-file guides for every project (start with `docs/README.md`). After making changes to any project, update the corresponding doc in `docs/` so it stays accurate.

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

All communication uses Pi RPC's strict LF-delimited JSONL protocol over stdin/stdout. WebSocket connections use single-use tickets for auth.

## GitHub authentication

Before pushing to `https://github.com/fleetcru/pi-stack.git`, switch GitHub CLI authentication to an account with write access to `fleetcru`, then push the requested branch:

```bash
gh auth switch --user <authorized-account>
git push origin <branch>
```

If `git push` returns HTTP 403, do not retry with the same account. Check the active account with `gh auth status`, switch or sign in to the authorized account, then rerun the push command.

## CircleCI Android builds and releases

CircleCI builds the signed Companion APK. The GitHub Actions workflow remains available and is not replaced by CircleCI.

### Configuration and triggers

- `.circleci/config.yml` is the dynamic setup configuration. It uses `circleci/path-filtering@3.0.0`.
- `.circleci/continue_config.yml` contains the Android build and GitHub release jobs.
- Commits run the Android workflow only when `pi-companion-exp/**` or `.circleci/**` changed. Other commits run only the small path-check job.
- Tags matching `v*` always run the Android workflow, regardless of changed paths. The setup `path-filtering/filter` job must explicitly allow `v*` tags; without that filter CircleCI accepts the tag pipeline but compiles it to `jobs: {}`.
- The Android job uses `cimg/android:2026.08.1` with the `large` Docker resource class.
- Main-branch builds store `pi-companion-<version>.apk` as a CircleCI artifact but do not create a GitHub Release.
- CircleCI is the only automatic publisher for `v*` tags. `.github/workflows/build-android.yml` remains a manual `workflow_dispatch` fallback and must not add a `v*` push trigger, because two publishers can race to create the same release. Manual runs validate a SemVer `version_name` and always create a uniquely numbered prerelease.

Validate both configurations after changing either file:

```bash
circleci config validate .circleci/config.yml
circleci config validate .circleci/continue_config.yml
```

### Signing and GitHub credentials

The workflow uses the `pi-stack-signing` CircleCI context in organization `8d10a558-3b1b-47a7-9577-4c5fb77bc796`. It requires:

- `KEYSTORE_BASE64`
- `KEYSTORE_PASSWORD`
- `KEY_ALIAS`
- `KEY_PASSWORD`
- `GH_TOKEN`, a fine-grained GitHub token restricted to `fleetcru/pi-stack` with `Contents: read and write`

Store secret values without trailing CR or LF characters. A trailing line break corrupts Android aliases and passwords. The build validates the decoded keystore with `keytool` before compilation.

### Versioning policy

Use Semantic Versioning for each independently released product:

| Product | Stable tag | Prerelease tag |
|---|---|---|
| Companion | `v1.2.3` | `v1.2.3-beta.1` |
| Server | `server-v1.2.3` | `server-v1.2.3-rc.1` |
| Server tray | `tray-v1.2.3` | `tray-v1.2.3-beta.1` |

Increment versions as follows:

- `PATCH` for backward-compatible fixes, such as `v0.1.0` to `v0.1.1`.
- `MINOR` for backward-compatible features, such as `v0.1.1` to `v0.2.0`.
- `MAJOR` for breaking compatibility or a declared stable milestone, such as `v0.9.0` to `v1.0.0`.
- Use `-alpha.N`, `-beta.N`, or `-rc.N` for prereleases. Increment `N` for each candidate.

Versions below `1.0.0` mean the product is still evolving and may introduce breaking changes in a minor release. Start at `v0.1.0` for an initial experimental/public Companion release, or at `v1.0.0` only when declaring its interfaces and update behavior stable. Do not use CI build numbers in stable tags. CircleCI generates Android `versionCode` independently from the build timestamp.

Tags are immutable release identifiers. Never move, overwrite, or reuse a published tag. Ask the user for the exact product and version before creating one. Companion tags are reserved for `v*`; server and tray tags use their product prefixes so they do not trigger the Companion workflow.

### Creating a GitHub Release

A Companion version tag triggers the full release path. Choose a new version and make sure `HEAD` is the commit to release:

```bash
git status --short
git rev-parse HEAD
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin v0.1.0
```

Do not create or push a release tag unless the user explicitly requests that exact version. Do not move or overwrite an existing release tag.

For a `v0.1.0` tag, CircleCI:

1. Builds and signs the release APK.
2. Verifies its APK signature.
3. Stores `pi-companion-0.1.0.apk` as a CircleCI artifact and workspace file.
4. Starts the tag-only `release` job.
5. Uses `circleci/github-cli@3.0.0` to install and authenticate `gh` with `GH_TOKEN`.
6. Creates GitHub Release `v0.1.0` and uploads the APK.

Tags containing a suffix, such as `v0.1.0-beta.1`, create GitHub prereleases. If the release job is rerun, `gh release upload --clobber` replaces the existing APK asset. The Companion updater discovers assets named `pi-companion-*.apk` from `fleetcru/pi-stack` releases.

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
| Session history | `internal/server/session_history.go` |
| Extensions | `extensions/external-session-bridge.ts`, `extensions/session-title.ts` |

```bash
cd pi-server-exp
go test ./... -race
go vet ./...
go build ./cmd/pi-server
```

**Server rebuild requirement:** whenever an agent changes any file under `pi-server-exp/`, it must rebuild `pi-server-exp/pi-server.exe` before reporting the server change complete. Source changes do not affect an already-running executable.

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
| WebSocket / SSE | `app/src/main/java/.../data/websocket/SessionEventSocket.kt` |
| Event dedup | `app/src/main/java/.../data/websocket/EventSequenceTracker.kt` |
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
./gradlew :app:assembleDebug
```

### pi-desktop (Tauri)

**Path:** `pi-desktop-app/` · **Stack:** Tauri v2, React, TypeScript, Vite

Desktop client with native OS integration via Tauri. Shares API client and components with pi-webby.

```bash
cd pi-desktop-app
pnpm install
pnpm dev
pnpm build
```

## Pi Event Formats (Critical Knowledge)

**Read this before modifying any event handling code.**

### JSONL History Format (from `~/.pi/agent/sessions/*.jsonl`)

Pi stores conversation history in JSONL files. Each line is a JSON object with a `type` field:

| `type` | Description |
|---|---|
| `session` | Session metadata (version, id, cwd) |
| `model_change` | Model switch (provider, modelId) |
| `thinking_level_change` | Thinking level change |
| `message` | User/assistant/toolResult message |

#### Message format

```json
{
  "type": "message",
  "id": "abc123",
  "parentId": "parent_id",
  "timestamp": "ISO8601",
  "message": {
    "role": "user" | "assistant" | "toolResult",
    "content": [...]
  }
}
```

#### Tool calls (inside assistant `content` array)

**Pi uses OpenAI format, NOT Anthropic format:**

```json
{
  "role": "assistant",
  "content": [
    {"type": "text", "text": "Let me check that."},
    {"type": "toolCall", "id": "call_abc123", "name": "bash", "arguments": {"command": "ls"}}
  ]
}
```

| Field | Pi format | Anthropic format | Note |
|---|---|---|---|
| Content block type | `toolCall` | `tool_use` | Different string! |
| Arguments field | `arguments` | `input` | Different field name! |
| Content block id | `id` | `id` | Same |

#### Tool results (separate message entry)

```json
{
  "role": "toolResult",
  "toolCallId": "call_abc123",
  "toolName": "bash",
  "content": [{"type": "text", "text": "file1.txt\nfile2.txt"}]
}
```

| Field | Pi format | Anthropic format |
|---|---|---|
| Role | `toolResult` | `tool` |
| ID field | `toolCallId` | `tool_use_id` |

#### Parser requirements (companion app)

The `loadHistory` parser in `SessionDetailViewModel.kt` must handle **all** of these:
- `role: "toolResult"` entries → collect output keyed by `toolCallId`
- `role: "tool"` entries → collect output keyed by `tool_use_id`
- `content` arrays with `type: "toolCall"` blocks → extract tool items
- `content` arrays with `type: "tool_use"` blocks → extract tool items (fallback)
- `_historyType: "tool_use"` entries → standalone tool format
- `_historyType: "tool_result"` entries → standalone result format

### Live Streaming Events (WebSocket/SSE)

These events flow over WebSocket or SSE during active sessions:

| Event type | Direction | Description |
|---|---|---|
| `message_start` | Server→Client | Assistant or user message begins |
| `message_update` | Server→Client | `text_delta`, `text_start`, `text_end` |
| `message_end` | Server→Client | Message complete |
| `tool_execution_start` | Server→Client | Tool begins (has `toolName`, `toolCallId`) |
| `tool_execution_update` | Server→Client | Tool progress (`partialResult`) |
| `tool_execution_end` | Server→Client | Tool complete (`result`, `isError`) |
| `agent_start` | Server→Client | Agent turn begins |
| `agent_end` / `agent_settled` | Server→Client | Agent turn complete |
| `runtime_state` | Server→Client | State: `working`, `idle`, `starting`, etc. |
| `bridge_receipt` | Server→Client | Command acknowledged |
| `response` | Server→Client | Prompt accepted/rejected |
| `file_change` | Server→Client | File modified |
| `events_lost` | Server→Client | Gap in event sequence |

### Event Flow: Server → Client

```
Pi process (stdout JSONL)
  → rpc.go dispatch() (monotonic event ID, ring buffer)
  → ws_handler.go (WebSocket) OR sse_handler.go (SSE)
  → Client: EventSequenceTracker (dedup by 10K window)
  → Client: SessionDetailViewModel.handleEvent()
  → Client: _items StateFlow → Compose LazyColumn
```

### Event Flow: Client → Server

```
Companion/Webby
  → POST /v1/sessions/{id}/prompt (REST)
  → Server: PiProcess.Request() or Send()
  → Pi process (stdin JSONL)
```

### WebSocket vs SSE

- **WebSocket** (`/v1/sessions/{id}/ws?ticket=...`): Bidirectional, ticket auth, used by Webby and as fallback
- **SSE** (`/v1/sessions/{id}/events/stream?since=N`): Read-only, auto-reconnect via Last-Event-ID, used by Companion
- **Both coexist** — clients choose their transport. Server supports both.

## Key Design Decisions

### Concurrency

- **Lock ordering:** `SessionRegistry.mu` → `PiProcess.mu`. Never reverse. `ListSpecs()` and `ActiveCount()` copy data under RLock, release, then call `Status()` outside.
- **Write serialization:** WebSocket connections use `writeMu` to serialize all writes (events, nacks, ping frames).
- **Subscriber dispatch:** Copies subscriber set under write lock before iterating outside lock.

### Security

- **Auth:** Bearer token via `Authorization` header. Token fingerprint (SHA-256) for WS ticket binding.
- **CORS:** With no explicit allowlist, accepts browser origins on loopback, RFC1918 home networks, Tailscale `100.64.0.0/10`, and the server's own `*.ts.net` host. Public origins are rejected. `PI_SERVER_ALLOWED_ORIGINS` replaces this with a strict explicit allowlist. Non-browser clients have no Origin header and pass through.
- **File access:** `filepath.EvalSymlinks` + `allowedFilePath` prevents symlink escapes. Git args from fixed whitelist only.
- **Worker proxy:** SSRF mitigation via scheme/host validation and optional allowlist. Only pre-registered URLs contacted.
- **SensitiveString:** Worker tokens redact on `String()`/`GoString()` to prevent log leakage.

### Relay Sessions

External Pi TUI sessions bridge into pi-server via the `external-session-bridge.ts` extension.

When changing `pi-server-exp/extensions/external-session-bridge.ts`, copy the updated file to `C:\Users\basin\.pi\agent\extensions\external-session-bridge.ts` for the active local Pi installation. Relay slash commands must be routed through `AgentSession.prompt()` with command expansion enabled; do not use `sendUserMessage()` for them because that API disables slash-command handling and can send the command text to the LLM.

- **Single process ownership:** Never run an RPC Pi process and a bridged TUI against the same JSONL file. Inventory and Machine Session Discovery must prefer a live relay, because separate Pi processes do not synchronize live state and can corrupt history.
- **Lifecycle forwarding:** The bridge forwards `agent_start`, `agent_end`, and `agent_settled`; relay admission is released only on `agent_settled`, not the earlier `agent_end`.
- **Delivery confirmation:** A normal idle relay prompt is acknowledged only after its user `message_start` appears. Pi's extension `sendUserMessage()` is fire-and-forget and does not synchronously throw on an idle/working race.
- **Lease rotation:** New bridge detaches old relay. Old relay's read goroutine exits via `isCurrentRelay()` check, and WebSocket acknowledgements are bound to the owning relay generation.
- **Command persistence:** Commands saved synchronously under lock before `enqueue()` returns. Atomic rename prevents truncated stores.
- **Event ring:** Dual-bound (200 count + 8MB bytes). Matches PiProcess semantics.
- **Detach/close ordering:** LIFO — close then detach to prevent stale relay from clobbering new one.

### Interactive Extension UI / `ask_user`

- **Local RPC:** Pi emits blocking `extension_ui_request` events for `select`, `confirm`, `input`, and `editor`; clients answer with `extension_ui_response` through `POST /v1/sessions/{id}/ui-response`.
- **Relayed TUI:** The patched `ask_user` extension emits `ask:requested` / `ask:closed` on Pi's shared event bus. `external-session-bridge.ts` forwards these as `extension_ui_request(method=ask_user)` / `extension_ui_closed` events.
- **Mobile response:** Companion posts `id`, `cancelled`, `value`, `confirmed`, `selections`, `comment`, and `responseKind`. The server persists an `extension_ui_response` relay command; its command `id` is for acknowledgement and `requestId` identifies the waiting question.
- **Bridge completion:** The bridge emits `ask:remote-response` back onto Pi's event bus, allowing `ask_user` to close its local overlay and complete the tool call.
- **Late viewers:** Both local and relay state expose `pendingExtensionUiRequest`, because fresh Companion views intentionally skip old event replay.

### Git Workflows (worktrees & branches)

Per-session isolated git worktrees and a guided commit/push flow, inspired by T3 Code.

- **Auto-worktree on create:** `createSessionRequest.createWorktree.enabled` makes the server create `<repoRoot>/.pi-worktrees/<title>` (sanitized) with a fresh `feature/<title>` branch (numeric `-2`, `-3`… de-conflict) based on the repo default branch. The session `cwd` is set to the worktree and the branch name is recorded in `Metadata["worktreeBranch"]`. Cannot be combined with an explicit `worktreePath` — the server rejects that.
- **Cleanup on delete:** `deleteSession` calls `removeOwnedWorktree(spec)`, which only removes worktrees genuinely registered with the session's repo. On Git for Windows it falls back to clearing read-only attributes + `os.RemoveAll` + `worktree prune` + `branch -D` because `git worktree remove` fails with "Permission denied" even on a clean checkout.
- **Branch naming:** `sanitizeFeatureBranchName` preserves an existing `feature/…` prefix (no double `feature/feature/`), keeps slash-namespaces (`docs/readme` → `feature/docs/readme`), falls back to `feature/update`, and `resolveAutoFeatureBranchName` de-conflicts against existing branches (numeric `-2`, `-3`…). Kept local `feature/agent` fallback rather than t3code's `feature/update`.
- **GitHub open/compare link:** `gitStatus` resolves `origin` via `parseGitHubRepositoryNameWithOwner` / `normalizeGitRemoteURL` (ported from T3 Code, MIT) into `githubRepo`; the inspector renders an "Open compare / PR" link (`/compare/<default>...<branch>`).
- **Enriched status:** `GET .../git/status?format=json` now returns `hasUpstream`, `hasRemote`, `isDefault`, `isWorktree`, `worktreePath`, `defaultBranch`, `remoteUrl`, `githubRepo`. Clients gate the commit/push flow on these.
- **Gated stacked flow:** The inspector's `resolveGitQuickAction` computes one primary action (Commit → Commit & push → Push / Push+set-upstream → blocked-with-hint) from branch state. It disables auto-push on default branches and when `origin` is missing or the branch has diverged/fallen behind.
- **Push with upstream:** `POST .../git/push` accepts `setUpstream` to run `git push -u origin <branch>` in one step for fresh feature branches.

> **Parsing gotcha:** `git for-each-ref --format=%(HEAD)\t%(refname:short)\t%(upstream:short)` emits a literal tab only with `%09`, and pads non-current refs with a leading SPACE. A whole-blob `strings.TrimSpace` (or `TrimSpace` per line) strips that leading space+tab and silently drops the first branch's name. Use `%09` separators and trim only the trailing `\r`/empty fields, not leading whitespace.

### Event Deduplication

- **Server:** Monotonic uint64 event IDs per session. `events_lost` sentinel when cursor predates the ring.
- **Webby:** `seenEventIds` Map with 10K eldest-eviction. Generation guard after reconnect.
- **Companion:** `LinkedHashMap` with 2K eldest-eviction + generation counter for stale callback prevention.

### Companion Event Pipeline

```
OkHttp callback / SSE EventSource
  → Channel<SocketEvent>(2000) — bounded buffer
  → EventSequenceTracker.process() — dedup + gap detection
  → ViewModel socket.events.collect
  → handleEvent() — type-specific processing
  → appendItem() / appendAssistantDelta() / updateTool()
  → _items StateFlow → Compose LazyColumn
```

Key invariants:
- `_items.update` must be atomic (CAS via MutableStateFlow)
- `assistantMutex` serializes assistant bubble state transitions
- `historyGeneration` (volatile) prevents stale HTTP history overwrites items
- `toolExecutionActive` flag prevents runtime_state(idle) from resetting spinner during tool use
- `turnCompleteGeneration` prevents stale runtime_state from re-enabling spinner after turn completes
- LazyColumn keys must be unique — `itemKeys` uses seen-set with index fallback

### Companion self-updater

- Stable users receive only stable GitHub Releases. Prereleases require an explicit future opt-in.
- A release must contain exactly one `pi-companion-<semver>.apk` asset, and `<semver>` must match the `v<semver>` release tag.
- Select releases by Semantic Version precedence, not publish time or digits stripped from filenames.
- APK downloads must use HTTPS on approved GitHub asset hosts, match the API-advertised size, stay below 200 MB, and move from a private `.part` file only after completion.
- Before PackageInstaller receives an APK, verify its application ID, signing certificate against the installed app, and a strictly greater Android `versionCode`.
- The PackageInstaller status `IntentSender` and `InstallResultReceiver` are the authoritative completion path on every supported Android version. Do not rely on a process-bound `SessionCallback` for final UI state.
- `STATUS_PENDING_USER_ACTION` must open Android's confirmation intent. If it cannot open, abandon the session and report failure so the UI can retry instead of remaining stuck.

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
| `PI_SERVER_ADDR` | `0.0.0.0:3142` | Listen address; reachable over home LAN and Tailscale |
| `PI_SERVER_AUTH_TOKEN` | _(none)_ | Bearer token for API auth |
| `PI_SERVER_CWD` | `.` | Default working directory |
| `PI_SERVER_DATA_DIR` | `.data/pi-server` | Persisted data |
| `PI_SERVER_ALLOWED_ROOTS` | `.` | Restrict session CWDs |
| `PI_SERVER_ALLOWED_ORIGINS` | _(automatic private-network policy)_ | Optional strict CORS origin override |
| `PI_SERVER_MAX_SESSIONS` | `8` | Max concurrent sessions (0 = unlimited) |
| `PI_SERVER_MAX_ACTIVE_RUNS` | `8` | Hub-wide active local/remote/relay run limit (0 = unlimited) |
| `PI_SERVER_MAX_RUNS_PER_SESSION` | `1` | Active runs allowed per session |
| `PI_SERVER_MAX_RUNS_PER_WORKER` | `4` | Active runs allowed per worker |
| `PI_SERVER_MAX_QUEUED_RUNS` | `32` | Hub admission queue bound (0 = reject when busy) |
| `PI_SERVER_DISTRIBUTED_RUN_TIMEOUT` | `2h` | Fallback lease timeout for missing distributed lifecycle events (0 = disabled) |
| `PI_SERVER_PI_BINARY` | `pi` | Path to Pi CLI |
| `PI_SERVER_PI_EXTENSIONS` | _(none)_ | Extensions to load |
| `PI_SERVER_EVENT_JOURNAL_SYNC_INTERVAL` | `0` | Event-journal fsync interval; `0` keeps strict per-event durability |

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
6. **Don't use `tool_use` / `input` in history parsers** — Pi uses `toolCall` / `arguments` (OpenAI format).
7. **Don't skip `toolResult` role in history parsing** — Pi stores tool results as `role: "toolResult"`, not `role: "tool"`.
8. **Don't use `!!` in Compose functions** — causes ClassCastException during recomposition race. Use `?: ""` or `?.let`.
9. **Don't create new list instances in `_items.update` CAS lambdas** — causes recomposition storms. Reuse existing item references.
10. **Windows: Child processes need `CREATE_NO_WINDOW`** — set `SysProcAttr{HideWindow: true}` before `cmd.Start()` to prevent CMD popups.
11. **Git `--format` tabs:** use `%09`, not `\t`, as a `for-each-ref` field separator, and never `TrimSpace` the blob — it eats the first branch's leading-space HEAD padding and silently drops it.
12. **Windows `git worktree remove` fails with “Permission denied”** even on clean checkouts — `removeOwnedWorktree` falls back to clearing read-only attrs + `os.RemoveAll` + `worktree prune` + `branch -D`.
13. **`createWorktree.enabled` vs `worktreePath` are mutually exclusive** — the server must reject both set at once, and the auto branch must be recorded in `Metadata["worktreeBranch"]` so delete can clean it up.
14. **Remote tag deletion does not remove local tags.** `git tag --list` reads local refs, while GitHub shows remote refs. Compare with `git ls-remote --tags origin`. Only prune local-only tags when the user explicitly requests it.
