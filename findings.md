# Pi-Stack Comprehensive Audit — Findings

**Date:** 2025-07-30
**Scope:** Bugs & correctness, Performance problems, Code quality & maintainability
**Projects:** pi-server-exp, pi-webby-exp, pi-desktop-app, pi-companion-exp, pi-webby-shared

---

## Summary

| Category | Fixed | Remaining |
|----------|-------|-----------|
| Bugs & correctness | 12 | 5 |
| Performance | 1 | 6 |
| Code quality | 10 | 8 |
| **Total** | **23** | **19** |

---

## ✅ All Issues Fixed (23)

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

## ⚠️ Remaining Medium-Priority Issues

### Server

| # | File | Issue |
|---|------|-------|
| 1 | `server.go:57-67` | **Dead code — session re-link never executes.** Checks `s.sessionBridge != nil` before it's assigned in `New()`. Sessions loaded from disk never get re-linked to the bridge after restart. |
| 2 | `security.go:23-40` | **`allowedCWD` calls `EvalSymlinks()` on every request.** Symlink resolution involves syscalls. Should pre-resolve and cache at startup. |
| 3 | `openapi.go` | **OpenAPI spec rebuilt on every request.** The entire spec (schemas + paths) is computed from scratch on every `GET /openapi.json`. Should be computed once and cached. |
| 4 | `session_inventory.go:170-185` | **Title update goroutine panic leaves permanent entry.** If the debounce goroutine panics before the deferred `delete`, the entry stays forever and all future title updates for that session are silently dropped. |

### Webby/Desktop

| # | File | Issue |
|---|------|-------|
| 5 | `session-inspector.tsx:356` | **`autoRetry` toggle never synced from server.** Initialized to `true` locally, never fetched. Toggle shows "on" regardless of actual server state. |
| 6 | `session-inspector.tsx:92,347` | **Aggressive polling.** `state` every 2s, `stats` every 5s, plus a duplicate `state` query in Settings. |
| 7 | `workspace-shell.tsx` | **1113-line file with 15+ components.** Should be split into 3-4 files. |

### Companion

| # | File | Issue |
|---|------|-------|
| 8 | `SessionsViewModel`, `HomeViewModel` | **No pagination for session lists.** Fetches all sessions in one call every 10 seconds. |
| 9 | `build.gradle.kts:34` | **`isMinifyEnabled = false` in release.** Ships unoptimized APK without R8 minification. |

---

## ⚠️ Remaining Low-Priority Issues

### Server

| # | File | Issue |
|---|------|-------|
| 10 | `file_content.go:86` | 1MB buffer allocated per request — could use `sync.Pool` |
| 11 | `external_history.go:83` | `readRelayMessages` reads entire session file (up to 32MB) into memory |
| 12 | `file_handlers.go:40` | `fileTree` hardcodes limit=300, should accept `?limit=` param |
| 13 | `git_handlers.go:522` | `runGit` discards stderr on success (git warnings lost) |
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
| 23 | `HomeViewModel.kt:120` | Silent failure on machine session open — no user feedback |
| 24 | `ShellScreen.kt` | ViewModel recreated on tab switch — loses cached state |
| 25 | `NavigationKeys.kt:13` | Obsolete `Main` route — duplicates `AppRoute.Home` |

---

## Architecture Notes

### What Works Well
- **Protocol design** — LF-delimited JSONL with monotonic event IDs, `_daemonEventId` for dedup, `events_lost` sentinel, ticket-based WS auth
- **Lock ordering** — `SessionRegistry.mu → PiProcess.mu` maintained throughout
- **Extension bridge** — Lease rotation, exponential backoff with jitter, generation counters, HTTP fallback, command dedup
- **Reconnection** — All clients implement exponential backoff with jitter; event replay via `since` cursor
- **Shared package** — `pi-webby-shared` eliminates code duplication between Desktop and Webby
- **Singleton services** — `AppModule` in Companion eliminates duplicate OkHttp pools and DataStore handles

### Key Risk Areas
1. **Companion `isMinifyEnabled = false`** (#9) — release APK ships unoptimized
2. **Server dead code** (#1) — session re-link after restart never happens
3. **Webby inspector over-polling** (#6) — `state` polled every 2s even when idle
4. **Companion no pagination** (#8) — all sessions fetched every 10s, doesn't scale
