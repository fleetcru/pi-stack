# TODO

This checklist was rechecked against the current Companion implementation.

- [x] Restore the active session quickly and reliably. `SessionStateCache` restores cached timelines before network work and avoids overwriting live state.
- [x] Keep session history stable during initial load, refresh, reconnect, and pagination. History generations and reconciliation prevent stale responses and duplicate rows.
- [x] Preserve scroll position and avoid duplicate or missing history items after reloads. Pagination uses server offsets, event deduplication uses a bounded sequence window, and LazyColumn keys have a duplicate fallback.
- [x] Reconnect cleanly after backgrounding and refresh session state without losing messages. Backgrounding caches the current timeline and foregrounding reconnects from the last event cursor.
- [ ] Add full recovery tests for app backgrounding, process death, network loss, and server reconnects. Current tests cover the underlying cache, history, socket, and deduplication units, but not complete lifecycle or instrumentation flows.

Relevant implementation lives in:

- `pi-companion-exp/app/src/main/java/com/example/picompanion/ui/sessiondetail/SessionDetailViewModel.kt`
- `pi-companion-exp/app/src/main/java/com/example/picompanion/ui/sessiondetail/SessionStateCache.kt`
- `pi-companion-exp/app/src/main/java/com/example/picompanion/ui/sessiondetail/SessionHistoryState.kt`
- `pi-companion-exp/app/src/test/java/com/example/picompanion/ui/sessiondetail/`
