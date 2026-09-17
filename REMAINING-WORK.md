# Remaining work

This list was rechecked against the current repository. Statuses below describe the code that is currently checked in, not planned work.

Items 8 and 19 were completed previously and remain omitted from the numbering.

## High priority

### 1. File-access TOCTOU protection

**Status: Partially addressed.** `pi-server-exp/internal/server/file_content.go` validates the resolved path, opens it, and compares the opened file with the validated file using `os.SameFile`. This closes the common replacement race. Fully descriptor-relative access for every path component is still not implemented.

### 2. Remote cleartext transport

**Status: Partially addressed.** Android and Desktop warn users when a non-loopback HTTP server is configured, but both still permit `http://` and `ws://`. HTTPS/WSS enforcement or explicit per-server approval for cleartext connections is not implemented.

### 3. Runtime event ordering

**Status: Not complete.** The server and clients contain ordering protections and focused tests, but there is no deterministic end-to-end test covering idle-before-tool-completion, reconnect during assistant deltas, lifecycle races, replay overlap, and `events_lost` during an active turn.

### 4. End-to-end relay and worker tests

**Status: Partially addressed.** Focused tests cover relay, worker fencing, proxy behavior, history, and admission. A complete integration suite covering restart during relay commands, queued command delivery, worker disconnects, replay, and local/remote/relay admission interactions is still missing.

### 5. Android ViewModel and socket tests

**Status: Partially addressed.** Unit tests cover parsing, caching, history merging, deduplication, socket behavior, and prompt queues. Full tests for tab recreation, process restoration, background/foreground reconnects, extension UI recovery, and complete event-pipeline ordering are still missing.

### 6. Windows process-tree cleanup

**Status: Not complete.** Windows cleanup still kills the direct process. No Windows Job Object or equivalent descendant-process ownership is implemented in `pi-server-exp` or `pi-server-tray`.

### 7. Runtime API validation

**Status: Partially addressed.** Individual handlers validate several fields, including worker URLs, Git refs, extension UI responses, limits, and worktree paths. A single validation layer for all configuration, session, worker, Git, relay, and runtime mutation inputs is not implemented.

## Medium priority

### 9. Inspector mutation consistency

**Status: Not complete.** Webby and Desktop still carry parallel copies of `session-inspector.tsx`, including Git mutation handling. The copies are currently synchronized, but the consolidation described here has not happened.

### 10. Shared frontend implementation

**Status: Partially addressed.** `pi-webby-shared` now contains shared API, state, hooks, socket, and timeline code. Major UI components such as the workspace, inspector, sidebar, and dialogs are still duplicated in Webby and Desktop.

### 11. Companion state and polling

**Status: Not complete.** Home polling now uses a 30-second interval, cached inventory, and concurrent requests. Repeated `settingsFlow.first()` calls and the broader review of polling, persistence debounce, active-server races, and ViewModel lifetimes remain open.

### 12. File and history memory usage

**Status: Partially addressed.** File content reads are capped at 1 MiB. Relay HTTP history uses bounded paging and an index instead of loading the complete transcript for every request. A legacy full-history helper remains, so this area is not fully cleaned up.

### 13. Browser-native confirmation dialogs

**Status: Not complete.** Git actions in both Webby and Desktop still call `window.confirm()` from `session-inspector.tsx`.

### 14. Strict TypeScript configuration

**Status: Not complete.** Webby and Desktop use strict TypeScript settings, but `pi-webby-shared/tsconfig.json` still sets `strict` to `false`. There is no shared strict base configuration.

### 15. Cross-client behavioral tests

**Status: Not complete.** Webby and shared timeline tests exist, but there is no common fixture suite that feeds the same history and live events through Webby, Desktop, and Companion.

## Release and process concerns

### 16. Large branch review

**Status: Complete as a current repository concern.** The repository is on `main`, and no large audit diff is present. The only current changes are this documentation update. This item requires no further code work unless a new large branch is created.

### 17. Local Go race tests unavailable

**Status: Still blocked in this environment.** `cd pi-server-exp && go test ./...` passes. `go test ./... -race` cannot start here because CGO is disabled. The race-enabled check still needs to pass in CI or an environment with CGO enabled.

### 18. Dependency security remediation

**Status: Not complete.** The documented npm advisory remediation, Go and Rust vulnerability scans, Android dependency scanning, and GitHub Actions SHA pinning remain open. The old advisory counts should be refreshed by a new scan before being treated as current.

## Completed low-risk follow-ups

These remain complete:

- Runtime setting toggles appear only when Pi reports their current values.
- The obsolete Android `Main` navigation route was removed.
- Machine-session opening failures show a visible error.
- `runGit` preserves successful stderr through `CombinedOutput`.
- File-tree `limit` is validated and bounded from 1 to 2,000.
- Historical Companion UI and server-wiring plans were archived under `docs/archive`.
- Available dependency audits were run, with unresolved work recorded above.
