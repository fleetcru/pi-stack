# pi-companion-exp — file guide

Android client: Kotlin, Jetpack Compose, DataStore, OkHttp, Hilt. Uses WebSocket for live events, and talks REST for everything else. Package root: `com.example.picompanion`.

## Data layer (`data/`)

### API
- `data/api/PiServerClient.kt` — the OkHttp-based REST client: capabilities, session CRUD, prompt/RPC POSTs, git endpoints, file browsing, extension-UI responses, worker and daemon endpoints. Parses into the model classes; every response used with `response.use {}` to avoid connection leaks.
- `data/api/HttpResult.kt` — sealed result wrapper (Success/HttpError/NetworkError) the repositories and ViewModels pattern-match on.
- `data/api/JsonConfig.kt` — shared kotlinx-serialization configuration (lenient parsing, unknown-key tolerance) so server JSON variations don't crash the app.

### Models
- `data/model/SessionModels.kt` — session specs/summaries, history messages (handles both OpenAI `toolCall`/`toolResult` and legacy Anthropic `tool_use`/`tool` shapes).
- `data/model/DaemonModels.kt` — daemon status, diagnostics, capabilities.
- `data/model/WorkerModels.kt` — worker registration/listing payloads.
- `data/model/ExtensionUiModels.kt` — `extension_ui_request` payloads (select/confirm/input/ask_user) and the response builder the dialogs submit.
- `data/model/SessionInventoryDedup.kt` — deduplication/merge logic for the unified session inventory (local vs remote vs relay vs machine sessions).

### Repositories & settings
- `data/repository/SessionsRepository.kt` — single place the ViewModels get session data from; combines REST calls and normalizes errors.
- `data/repository/WorkersRepository.kt` — same for worker endpoints.
- `data/settings/SettingsDataStore.kt` — DataStore preferences: server URL, token, theme, display prefs. Never reads/writes inside `map{}` transforms (deadlock pitfall); migrations are suspend functions.
- `data/settings/AppSettings.kt` — typed snapshot of settings consumed by the UI.
- `data/settings/SecureTokenStore.kt` — token storage via Android Keystore/EncryptedSharedPreferences so the bearer token never sits in plaintext prefs.

### Live events
- `data/websocket/SessionEventSocket.kt` — OkHttp WebSocket/SSE transport for one session; emits `SocketEvent`s into a bounded `Channel(2000)`.
- `data/websocket/SocketEvent.kt` — transport event type (event name, JSON payload, server event ID).
- `data/websocket/EventSequenceTracker.kt` — dedup + gap detection over the server's monotonic event IDs. 2K-entry LinkedHashMap with eldest eviction and a generation counter; preserves the dup window across reconnects so replayed events aren't rendered twice, and flags `events_lost` gaps that trigger a history resync.

## Dependency injection & navigation

- `di/AppModule.kt` — Hilt module providing OkHttp, the API client, repositories, DataStore.
- `MainActivity.kt`, `Navigation.kt`, `NavigationKeys.kt` — single-activity Compose app; nav graph for home / sessions / workers / settings / session-detail routes.

## UI layer (`ui/`)

### Shared components (`ui/components/`)
- `AppHeader.kt`, `BottomNavBar.kt`, `TopAppBarCompact.kt` — navigation chrome.
- `SessionCard.kt`, `WorkerCard.kt`, `StatCard.kt`, `MetricCard.kt`, `IconTile.kt` — dashboard cards.
- `EventRow.kt` — one live-event row in the session timeline.
- `PromptBar.kt` — message input with send/stop.
- `SessionDrawer.kt`, `DirectoryBrowserSheet.kt`, `SectionCard.kt`, `StatusPill.kt`, `LoadingScreen.kt` — sheets, section wrappers, status chips, loading states.

### Main screen (`ui/main/`)
- `HomeViewModel.kt` — dashboard state: daemon health, worker counts, recent sessions; polls REST and merges inventory.
- `MainScreen.kt`, `ShellScreen.kt` — dashboard composition and the app shell with bottom navigation.

### Sessions list (`ui/sessions/`)
- `SessionsViewModel.kt` / `SessionsScreen.kt` — inventory listing with grouping.
- `SessionGrouping.kt` — sorts/filters sessions into groups (active, idle, remote, machine).
- `SessionInventoryState.kt` — process-memory singleton coordinating the list and detail screens so a session opened from the list is warm in the detail.
- `SessionListItem.kt` — row composable.

### Session detail (`ui/sessiondetail/`) — the biggest area
- `SessionDetailViewModel.kt` — the core: loads JSONL history via the parser, runs the event pipeline (`Channel → tracker → handleEvent → _items`), keeps `_items` updates atomic (CAS), serializes assistant bubble transitions under `assistantMutex`, guards against stale HTTP overwrites with `historyGeneration`, and handles extension-UI questions.
- `SessionTransportCoordinator.kt` — owns ticket acquisition and socket lifecycle independently of UI state so reconnects don't leak or drop resources.
- `SessionHistoryParser.kt` — pure (CPU-only, off-main-thread) conversion of persisted Pi JSONL into chat items; must understand every history shape listed in AGENTS.md.
- `SessionHistoryState.kt` — history load state machine (loading/pages/resync) with tests.
- `SessionStateCache.kt` — small in-memory LRU so switching sessions back and forth is instant.
- `PendingPromptQueue.kt` — queues prompts sent while the agent is mid-run; drains when the turn settles.
- `PromptImageEncoder.kt` — image → base64 attachment prep.
- `SessionDetailScreen.kt` — the chat screen composition (LazyColumn of timeline items, keyed uniquely via seen-set + index fallback).
- `ChatBubble.kt`, `TimelineRows.kt`, `ChatEmptyState.kt`, `SessionHeader.kt`, `MessageInputBar.kt` — individual timeline/message UI pieces.
- `ExtensionUiDialog.kt` — renders blocking `ask_user`/select/confirm/input questions from Pi and posts the response.
- `FileBrowserSheet.kt`, `UnifiedActionsSheet.kt` — bottom sheets for file browsing and session actions (model, thinking level, abort, git quick actions).

### Workers (`ui/workers/`)
- `WorkersViewModel.kt` / `WorkersScreen.kt` — worker list with health.
- `WorkerEditorDialog.kt` — add/edit a worker (URL, token).

### Settings (`ui/settings/`)
- `SettingsViewModel.kt` / `SettingsScreen.kt` / `SettingsRow.kt` / `SettingsSection.kt` — settings UI over the DataStore.
- `PairingScanActivity.kt`, `PairingScanOverlayView.kt`, `PairingScanSquareLayout.kt` — camera QR scanning for server pairing (fullscreen overlay activity with a square preview layout).

## Theme

- `theme/Color.kt`, `Theme.kt`, `Type.kt` — Material 3 theming with dark mode support.

## Tests

Unit tests under `app/src/test/` cover the pure logic (tracker, parser, queue, dedup, encoder, inventory state). Run with `./gradlew :app:testDebugUnitTest`; compile check with `:app:compileDebugKotlin`.
