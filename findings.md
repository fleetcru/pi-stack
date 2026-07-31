# Pi-Stack Comprehensive Audit — Findings

**Date:** 2025-07-30
**Scope:** Bugs & correctness, Performance problems, Code quality & maintainability
**Projects:** pi-server-exp, pi-webby-exp, pi-desktop-app, pi-companion-exp, pi-webby-shared

---

## Summary

| Category | Fixed | Documented (not fixed) |
|----------|-------|------------------------|
| Bugs & correctness | 5 | 10 |
| Performance | 0 | 7 |
| Code quality | 5 | 8 |
| **Total** | **10** | **25** |

---

## ✅ Issues Fixed

### Server (pi-server-exp)

**1. `envList` allows empty strings as allowed roots** — `config.go`
- `strings.Split(",", ",")` returns `["", ""]`. `filepath.Abs("")` returns CWD, inadvertently adding the server's working directory as an allowed root.
- **Fix:** Filter empty/whitespace-only entries from the split result.

**2. Nil pointer in `session_inventory.go`** — `listSessions` goroutine
- `s.sessions.Get(spec.ID)` returns `(nil, false)` for relay specs or specs loaded from disk without a process. Calling `p.Status()` on nil panics.
- **Fix:** Added `ok` check before calling `Status()`.

**3. Discarded `ok` return in `deleteSession`** — `session_handlers.go`
- `spec, _ := s.sessions.GetSpec(id)` discarded the boolean. If the spec doesn't exist, `spec.ManagedSessionDir` is empty, so cleanup silently skips.
- **Fix:** Check `specOk` before using the spec.

### Companion (pi-companion-exp)

**4. Dead code removal** — `MockModels.kt`, `ProjectSessionCard.kt`, `RecentActivityCard.kt`
- Three files with zero references anywhere in the codebase. Removed.

**5. Hardcoded version in SettingsScreen** — `SettingsScreen.kt`
- App version was `"0.1 debug"` and build type was `"Debug"` — hardcoded strings instead of `BuildConfig.VERSION_NAME` and `BuildConfig.BUILD_TYPE`.
- **Fix:** Now reads from BuildConfig.

**6. Fully-qualified imports** — `WorkersViewModel.kt`, `WorkersScreen.kt`
- `kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO)` and `androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(...) }` instead of proper imports.
- **Fix:** Added proper imports, used short forms.

### Webby (pi-webby-exp)

**7. setTimeout leak in MachineSessionList** — `workspace-shell.tsx`
- `setTimeout(() => setOpenError(null), 5000)` had no cleanup on unmount. Could call `setState` on an unmounted component.
- **Fix:** Store timer in a ref, clear on unmount via `useEffect` cleanup.

---

## ⚠️ Issues Documented (Not Fixed)

### Server — Bugs & Correctness

| # | Severity | File | Issue |
|---|----------|------|-------|
| 1 | **High** | `workers.go:172-175` | **`updateCapacity` data race on `s.cfg.MaxSessions`** — writes to `s.cfg.MaxSessions` (plain int, no sync) while other goroutines read it. Should use `atomic.Int64` or always read via the mutex-protected `s.sessions.maxSessions`. |
| 2 | **Medium** | `server.go:57-67` | **Dead code — session re-link never executes** — checks `s.sessionBridge != nil` before it's assigned, so the re-link loop is dead code. Sessions loaded from disk never get re-linked to the bridge after restart. |
| 3 | **Medium** | `session_inventory.go:170-185` | **Title update goroutine panic leaves permanent entry** — if the debounce goroutine panics before the deferred `delete`, the entry stays forever and all future title updates for that session are silently dropped. |
| 4 | **Low** | `session_handlers.go:46-50` | **`r.Body != http.NoBody` guard is dead code** — `MaxBytesReader` wraps the body, so this check always passes. |
| 5 | **Low** | `git_handlers.go:522-534` | **`runGit` discards stderr on success** — git warnings written to stderr are silently lost. |

### Server — Performance

| # | Severity | File | Issue |
|---|----------|------|-------|
| 6 | **Medium** | `security.go:23-40` | **`allowedCWD` calls `filepath.EvalSymlinks()` on every request** — symlink resolution involves syscalls. Should pre-resolve and cache at startup. |
| 7 | **Medium** | `openapi.go` | **OpenAPI spec rebuilt on every request** — the entire spec (schemas + paths) is computed from scratch on every `GET /openapi.json`. Should be computed once and cached. |
| 8 | **Low** | `file_content.go:86-89` | **1MB buffer allocated per request** — `make([]byte, limit+1)` on every file content fetch. Could use `sync.Pool`. |
| 9 | **Low** | `external_history.go:83-105` | **`readRelayMessages` reads entire session file into memory** — up to 32MB. Should stream the last N messages. |

### Server — Code Quality

| # | Severity | File | Issue |
|---|----------|------|-------|
| 10 | **Low** | `file_handlers.go:40-60` | **`fileTree` hardcodes limit=300** — should accept a `?limit=` query parameter. |

### Webby/Desktop — Bugs & Correctness

| # | Severity | File | Issue |
|---|----------|------|-------|
| 11 | **Medium** | `session-inspector.tsx:356-359` | **`autoRetry` state never synced from server** — initialized to `true` locally, never fetched. Toggle shows "on" regardless of actual server state. |
| 12 | **Low** | `session-inspector.tsx:258-266` | **`window.confirm()` for destructive git ops** — blocking modal dialogs break React render cycle and are inconsistent with the app's dialog-based UX. |

### Webby/Desktop — Performance

| # | Severity | File | Issue |
|---|----------|------|-------|
| 13 | **High** | `session-inspector.tsx:211-219` | **`Workspace` fires 7 git queries regardless of active tab** — `git status`, branches, worktrees, diff, log, file tree all poll even when the user is on Overview or Activity tab. ~7 wasted HTTP requests per poll cycle. |
| 14 | **Medium** | `session-inspector.tsx:92-96` | **Aggressive polling** — `state` polls every 2s, `stats` every 5s. Combined with the git queries, the inspector generates 8+ concurrent polling requests. |
| 15 | **Medium** | `session-inspector.tsx:347-349` | **Duplicate `state` query** — `Settings` independently polls `state` every 5s, duplicating the 2s poll from `Overview`. React Query deduplicates by key, but the different intervals are wasteful. |
| 16 | **Low** | `workspace-shell.tsx:768` | **`groupByProject()` called in render body** — creates a new `Map` on every render without `useMemo`. |

### Webby/Desktop — Code Quality

| # | Severity | File | Issue |
|---|----------|------|-------|
| 17 | **Medium** | `workspace-shell.tsx` | **1113-line component with 15+ sub-components** — should be split into `sidebar-tree.tsx`, `mobile-workspace.tsx`, `capacity-control.tsx`. |
| 18 | **Low** | `session-inspector.tsx:258-266` | **200+ character single-line onClick handlers** — should be extracted into named async functions. |
| 19 | **Low** | `workspace-shell.tsx:69-72` | **`openSession` not wrapped in `useCallback`** — passed to 4+ child components, causing unnecessary re-renders. |
| 20 | **Low** | `session-inspector.tsx:1` | **`useState` imported but `useCallback`/`useMemo` never used** — despite having expensive computations. |

### Companion — Bugs & Correctness

| # | Severity | File | Issue |
|---|----------|------|-------|
| 21 | **High** | `SettingsDataStore.kt:40` | **DataStore deadlock via `onStart` → `migrateTokensIfNeeded()`** — `onStart` calls `context.dataStore.data.first()`, creating a nested subscription to the same DataStore. Documented as prohibited. Should be called once from Application.onCreate. |
| 22 | **High** | `SettingsDataStore.kt:84-92` | **Migration can overwrite concurrent server edits** — between reading the server list and writing the stripped version, another coroutine's `updateServers()` write gets overwritten. |
| 23 | **Medium** | Multiple ViewModels | **6+ `PiServerClient()` instances** — each creates a fresh OkHttpClient with its own connection pool. Should be a singleton. |
| 24 | **Medium** | Multiple ViewModels | **Multiple `SettingsDataStore()` instances** — DataStore documentation warns against multiple instances on the same file. Can corrupt the preferences file. |
| 25 | **Medium** | `MainScreenTest.kt:17` | **Broken test** — `MainScreen(FAKE_DATA)` doesn't match current composable signature. Test won't compile. |

### Companion — Performance

| # | Severity | File | Issue |
|---|----------|------|-------|
| 26 | **Medium** | `SessionsViewModel`, `HomeViewModel` | **No pagination for session lists** — fetches all sessions in a single call every 10 seconds. Should add limit/offset. |
| 27 | **Low** | Multiple VMs | **`settingsFlow.first()` called repeatedly** — triggers full DataStore read + token hydration + JSON parse on every call. Should cache in a `StateFlow`. |
| 28 | **Low** | `HomeViewModel:73-77` | **5 parallel HTTP requests every 10s** — aggressive for mobile. Consider a single aggregated endpoint. |

### Companion — Code Quality

| # | Severity | File | Issue |
|---|----------|------|-------|
| 29 | **Medium** | `build.gradle.kts:34` | **`isMinifyEnabled = false` in release** — ships unoptimized APK without R8 minification. |
| 30 | **Low** | `SettingsRow.kt:88-92` | **Debounced persist may lose final edit** — if user navigates away within 300ms, the last keystroke is lost. |
| 31 | **Low** | `SettingsViewModel.kt:53-60` | **Race in `removeServer()`** — stale read of `activeServerId` between the filter and the active-server update. |
| 32 | **Low** | `HomeViewModel.kt:120-135` | **Silent failure on machine session open** — `HomeViewModel` refreshes on error but shows no message to the user. |
| 33 | **Low** | `ShellScreen.kt` | **ViewModel recreated on tab switch** — scoped to NavBackStackEntry per tab, losing cached state. |
| 34 | **Low** | `NavigationKeys.kt:13` | **Obsolete `Main` route** — comment says "Keep for backward compat during transition" but duplicates `AppRoute.Home`. |

---

## Architecture Notes

### What Works Well
- **Protocol design** — LF-delimited JSONL with monotonic event IDs, `_daemonEventId` for dedup, `events_lost` sentinel, ticket-based WS auth
- **Lock ordering** — `SessionRegistry.mu → PiProcess.mu` maintained throughout
- **Extension bridge** — Lease rotation, exponential backoff with jitter, generation counters, HTTP fallback, command dedup
- **Reconnection** — All clients implement exponential backoff with jitter; event replay via `since` cursor
- **Shared package** — `pi-webby-shared` eliminates code duplication between Desktop and Webby

### Key Risk Areas
1. **Companion DataStore deadlock** (#21) — most concerning runtime risk, could cause ANR
2. **Companion singleton issues** (#23, #24) — 6+ OkHttp pools and multiple DataStore instances on every screen
3. **Webby inspector over-polling** (#13) — 7 wasted HTTP requests per cycle when viewing non-Files tab
4. **Server `updateCapacity` data race** (#1) — could cause intermittent incorrect session limits
