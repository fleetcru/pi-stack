# Remaining work

This file records the remaining work identified after the comprehensive audit remediation on `fix/comprehensive-audit`.

**Current count:** 17 numbered concerns remain. Items 8 and 19 are complete and were removed. Items 11 and 12 were narrowed after their low-risk parts were completed.

## High priority

### 1. File-access TOCTOU protection

The server validates paths before opening them. A symlink or path component could change between validation and file access.

A proper fix needs descriptor-relative operations or equivalent platform-specific handling. This is not a quick patch.

Risk: local file access outside configured roots under hostile concurrent filesystem manipulation.

### 2. Remote cleartext transport

Android and Desktop permit remote `http://` and `ws://` connections by design.

Bearer tokens and session content can be exposed if traffic crosses an untrusted network. Tailscale reduces this risk, but the applications do not enforce that distinction.

Possible future changes:

- Warn clearly for non-loopback cleartext endpoints.
- Prefer HTTPS and WSS.
- Optionally require explicit per-server approval for cleartext remote connections.

### 3. Runtime event ordering

The real-time pipeline depends on event ordering across lifecycle, runtime-state, tool, and message events. Unit tests cover parsing and timeline behavior, but not the complete process-to-client sequence.

Problem cases include:

- `runtime_state: idle` arriving before a tool completion
- Reconnect during an assistant delta
- Lifecycle completion racing with buffered text
- Replay overlapping live events
- `events_lost` during an active turn

Add deterministic event-ordering tests with a fake Pi process.

### 4. End-to-end relay and worker tests

The server has focused unit and handler tests, but needs complete integration tests for:

- External relay reconnect
- Durable queued command delivery
- Lease replacement
- Server restart during a relay command
- Worker disconnect during a session
- Event replay after reconnect
- Local, remote, and relay admission-limit interaction

This is the largest remaining reliability gap.

### 5. Android ViewModel and socket tests

Android's most stateful code still has limited automated coverage.

Tests are needed for:

- Reconnect during an active turn
- Stale history responses
- Buffered assistant deltas
- Tool completion ordering
- `events_lost`
- Extension UI recovery
- Duplicate event suppression
- Tab recreation and process restoration
- Batch creation errors

The parser tests do not verify the full event pipeline.

### 6. Windows process-tree cleanup

The Windows server and tray lifecycle code may terminate the direct process without reliably killing every descendant.

Risk: orphaned Pi, shell, Git, or Node processes after shutdown or failed updates.

A Windows Job Object would be the strongest solution.

### 7. Runtime API validation

Some server configuration and request fields rely on scattered handler validation. Centralize validation for:

- Limits and timeouts
- Session metadata
- Worker settings
- Git operation inputs
- Relay command payloads
- Runtime configuration changes

Malformed values should fail consistently before changing state.

## Medium priority

### 9. Inspector mutation consistency

Consolidate Git mutation handling across Webby and Desktop rather than maintaining parallel component logic.

### 10. Shared frontend implementation

`pi-webby-shared` handles APIs, hooks, state, and timeline logic, but Webby and Desktop still duplicate substantial component behavior. This creates drift and requires fixes to be ported between clients.

### 11. Companion state and polling

Review:

- Repeated `settingsFlow.first()` calls
- Frequent Home polling
- Settings persistence debounce
- Active-server removal race
- ViewModel lifetime across tab changes

These are mostly performance and UX issues, but some can cause stale state or lost edits.

### 12. File and history memory usage

Known server inefficiencies include:

- Per-request file buffers
- Entire relay history files read into memory

These can become visible with many sessions or large histories.

### 13. Browser-native confirmation dialogs

Some Git actions still use `window.confirm()`. Replace these with a shared confirmation dialog for consistent keyboard behavior, styling, and mutation feedback.

### 14. Strict TypeScript configuration

Webby, Desktop, and shared code should use one strict base TypeScript configuration. Separate settings can allow code to compile in one client but fail in another.

### 15. Cross-client behavioral tests

Add a shared fixture suite that feeds identical history and live events into Webby, Desktop, and Companion and verifies the normalized timeline result.

## Release and process concerns

### 16. Large branch review

At the time this list was created, the tracked diff covered roughly 85 files, 2,005 additions, and 410 deletions, plus untracked source and test files.

Split commits by area:

1. Server correctness and security
2. Installer and launcher hardening
3. Tray lifecycle and updates
4. Shared and Webby behavior and tests
5. Desktop tests and mutation handling
6. Android parser, image, clipboard, and UI fixes
7. CI and documentation

### 17. Local Go race tests unavailable

`go test -race` could not run locally because GCC was unavailable. CI covers it, but the branch should not merge until that job passes.

### 18. Dependency security remediation

The April 2026 audit found 24 npm advisories in Webby and 20 in Desktop at moderate severity or higher. Most paths run through development tools such as shadcn, jsdom, ESLint, Vite, and OpenAPI tooling. The direct `react-router` dependency also has an advisory, although this application does not use its RSC mode.

Dependency upgrades require separate review and approval. Go and Rust scans remain incomplete because `govulncheck` and `cargo-audit` are not installed. The Android release dependency graph was reviewed manually because the project has no vulnerability scanner configured. GitHub Actions use mutable version tags rather than commit SHAs.

## Completed low-risk follow-ups

- Runtime setting toggles now appear only when Pi reports their current values.
- Removed the obsolete Android `Main` navigation route.
- Machine-session opening failures now show a visible error.
- Verified that `runGit` already preserves successful stderr through `CombinedOutput`.
- Added a validated `limit` query parameter for file trees, bounded from 1 to 2,000.
- Archived the historical Companion UI and server-wiring plans under `docs/archive`.
- Ran the available dependency audits and recorded unresolved remediation above.
