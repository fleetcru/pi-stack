# Pi-Stack Comprehensive Audit — Findings

**Original audit date:** 2025-07-30
**Baseline revision:** `bebb3f0`
**Status:** Historical snapshot. Counts and remaining items may have changed; verify against the current branch and tests.
**Scope:** Bugs and correctness, performance, code quality, and maintainability
**Projects:** pi-server-exp, pi-webby-exp, pi-desktop-app, pi-companion-exp, pi-webby-shared

---

## Current branch status

The `fix/comprehensive-audit` branch supersedes the old numeric summary. It adds server lifecycle and multiplex fixes, installer checksum and ACL hardening, Android history and image fixes, frontend error handling, tray update guards, and broader CI coverage.

Validation required before merge:

- `pi-server-exp`: tests, vet, build, and race tests in CI
- `pi-server-tray`: tests and vet
- `pi-webby-exp`: typecheck, lint, tests, and build
- `pi-desktop-app`: typecheck, lint, tests, frontend build, Rust check, and packaged Tauri build in CI
- `pi-companion-exp`: debug compilation, unit tests, and release APK build
- Windows installer checksum helper tests

The sections below preserve the original audit record. Their counts are historical and must not be used as current totals.

---

## Historical fixes

### Round 1 — Interconnectivity Audit

| # | Project | File | Fix |
|---|---------|------|-----|
| 1 | Server | `workers.go` | `WorkerRegistry.Save()` now uses `writeJSONAtomic` instead of raw `os.WriteFile` |
| 2 | Server | `http_util.go` | `writeJSONAtomic` uses unique temp filenames to prevent concurrent collision |
| 3 | Server | `worker_ws_proxy.go` | WS proxy now forwards query params (`?since=`, `?watch=`) to remote workers |
| 4 | Server | `ws_handler.go` | File watcher stops when last WS subscriber disconnects |
| 5 | Server | `external_command_store.go` | Uses unique temp filenames for consistency |
| 6 | Server | `rpc.go` | Added `PiProcess.SubscriberCount()` for watcher lifecycle |
| 7 | Companion | `PiServerClient.kt` | Wrapped `doGet`/`doPost`/`doPut`/`doPatch` in `response.use{}` to fix OkHttp connection leak |
| 8 | Companion | `SessionEventSocket.kt` | `Channel.UNLIMITED` → `Channel(2000)` to prevent OOM |
| 9 | Companion | `SessionDetailViewModel.kt` | Added `runtime_state`, `model_select`, `thinking_level_select` event handling |
| 10 | Companion | `SessionDetailViewModel.kt` | `file_change` reads `change` field (was reading wrong field) |
| 11 | Companion | `SessionDetailViewModel.kt` | Removed ~15 dead event type aliases |
| 12 | Companion | `SessionModels.kt` | Added `worktreePath` to `CreateSessionRequest` |
| 13 | Desktop | `session-workspace.tsx` | Added `runtime_state` WS event handling (was HTTP-only) |
| 14 | Desktop | `app-store.ts` | Uses `pi-desktop-ui` localStorage key (was colliding with webby) |
| 15 | Desktop | `tauri.conf.json` | Broadened CSP to allow `ws://*` and `http://*` |
| 16 | Desktop+Webby | `session-workspace.tsx` | Added `file_change`/`model_select`/`thinking_level_select` rendering in timeline |
| 17 | Shared | `pi-webby-shared/` | Extracted shared API/socket/hooks/store package, eliminated code duplication |

### Round 2 — Second Audit Pass

| # | Project | File | Fix |
|---|---------|------|-----|
| 18 | Server | `config.go` | `envList` filters empty strings from comma-separated values |
| 19 | Server | `session_inventory.go` | Added nil guard for `Get()` result in `listSessions` goroutine |
| 20 | Server | `session_handlers.go` | Check `GetSpec` ok return in `deleteSession` |
| 21 | Companion | Multiple | Removed dead code: `MockModels.kt`, `ProjectSessionCard.kt`, `RecentActivityCard.kt` |
| 22 | Companion | `SettingsScreen.kt` | Use `BuildConfig.VERSION_NAME`/`BUILD_TYPE` instead of hardcoded values |
| 23 | Companion | `WorkersViewModel.kt`, `WorkersScreen.kt` | Proper imports instead of fully-qualified references |
| 24 | Webby | `workspace-shell.tsx` | Fixed `setTimeout` leak in `MachineSessionList` (cleanup on unmount) |

### Round 3 — High-Priority Fixes

| # | Project | File | Fix |
|---|---------|------|-----|
| 25 | Server | `workers.go`, `server.go` | `updateCapacity` uses `atomic.Int64` for `MaxSessions` (fixes data race) |
| 26 | Companion | `SettingsDataStore.kt` | Moved `migrateTokensIfNeeded()` out of `onStart` into mutex-guarded `ensureMigrationDone()` (fixes DataStore deadlock) |
| 27 | Companion | `SettingsDataStore.kt` | Migration uses mutex for idempotent single-execution (fixes race with `updateServers()`) |
| 28 | Companion | `AppModule.kt` + 8 files | Created singleton `PiServerClient` + `SettingsDataStore` via `AppModule` (eliminates 6+ duplicate OkHttp pools and DataStore handles) |
| 29 | Companion | deleted | Removed broken `MainScreenTest.kt` (referenced obsolete API) |
| 30 | Webby | `session-inspector.tsx` | `Workspace` (7 git queries) only renders when Files tab is active |

---

## Medium-priority follow-ups

Webby and Desktop now render automatic retry and compaction toggles only when Pi reports those values. This prevents the clients from presenting invented defaults as current server state.

---

## Medium-priority issues fixed in round 4

| # | Project | File | Fix |
|---|---------|------|-----|
| 31 | Server | `server.go` | Moved session re-link loop after `sessionBridge` initialization (was dead code) |
| 32 | Server | `security.go`, `server.go` | Pre-resolve allowed root symlinks at startup via `resolveAllowedRoots()` |
| 33 | Server | `openapi.go` | Cache OpenAPI spec with `sync.Once` instead of rebuilding per request |
| 34 | Server | `session_inventory.go` | Added `recover()` in title update goroutine to prevent permanent entry leak |
| 35 | Webby | `session-inspector.tsx` | `autoRetry` toggle now syncs from `state?.autoRetryEnabled` (ready for server support) |
| 36 | Webby | `session-inspector.tsx` | Reduced `state` poll from 2s→10s, `stats` from 5s→15s; Settings poll matched to 10s |
| 37 | Webby | `workspace-shell.tsx` | Split 1113-line file into 4 files: `sidebar-tree.tsx`, `capacity-control.tsx`, `machine-session-list.tsx`, `workspace-shell.tsx` (498 lines) |
| 38 | Companion | `PiServerClient.kt`, `HomeViewModel.kt` | Added `listRecentSessions()` with client-side limit of 50 for home screen |
| 39 | Companion | `build.gradle.kts`, `proguard-rules.pro` | Enabled R8 minification + shrinkResources with keep rules for kotlinx.serialization |

## Historical low-priority snapshot

The tables below preserve the earlier audit notes. Confirm each item against the current code before treating it as active work. Maintained work is tracked in [`REMAINING-WORK.md`](REMAINING-WORK.md).

### Server

| # | File | Issue |
|---|------|-------|
| 10 | `file_content.go:86` | 1MB buffer allocated per request — could use `sync.Pool` |
| 11 | `external_history.go:83` | `readRelayMessages` reads entire session file (up to 32MB) into memory |
| 14 | `session_handlers.go:46` | Dead code: `r.Body != http.NoBody` guard always passes |

### Webby/Desktop

| # | File | Issue |
|---|------|-------|
| 15 | `session-inspector.tsx:258` | `window.confirm()` for git ops — blocking, inconsistent with app UX |
| 16 | `workspace-shell.tsx:768` | `groupByProject()` in render body without `useMemo` |
| 17 | `workspace-shell.tsx:69` | `openSession` not wrapped in `useCallback` — passed to 4+ children |
| 18 | `session-inspector.tsx` | Unused `useCallback`/`useMemo` imports despite expensive computations |

### Companion

| # | File | Issue |
|---|------|-------|
| 19 | Multiple VMs | `settingsFlow.first()` called repeatedly — should cache in StateFlow |
| 20 | `HomeViewModel:73` | 5 parallel HTTP requests every 10s — aggressive for mobile |
| 21 | `SettingsRow.kt:88` | Debounced persist may lose final edit if user navigates within 300ms |
| 22 | `SettingsViewModel.kt:53` | Race in `removeServer()` — stale read of `activeServerId` |
| 24 | `ShellScreen.kt` | ViewModel recreated on tab switch — loses cached state |

---

## Architecture Notes

### What Works Well
- **Protocol design** — LF-delimited JSONL with monotonic event IDs, `_daemonEventId` for dedup, `events_lost` sentinel, ticket-based WS auth
- **Lock ordering** — `SessionRegistry.mu → PiProcess.mu` maintained throughout
- **Extension bridge** — Lease rotation, exponential backoff with jitter, generation counters, HTTP fallback, command dedup
- **Reconnection** — All clients implement exponential backoff with jitter; event replay via `since` cursor
- **Shared package** — `pi-webby-shared` eliminates code duplication between Desktop and Webby
- **Singleton services** — `AppModule` in Companion eliminates duplicate OkHttp pools and DataStore handles

### Current risk areas

The current backlog lives in [`REMAINING-WORK.md`](REMAINING-WORK.md). The largest risks are file-access race hardening, real-time event-ordering tests, relay and worker integration tests, Android ViewModel and socket tests, and Windows process-tree cleanup.

Recent low-risk follow-ups also added bounded file-tree limits, visible machine-session failures, truthful runtime-setting toggles, and removal of the obsolete Android `Main` route. `runGit` already preserved successful stderr through `CombinedOutput`, so that historical finding required no code change.
