# pi-companion-exp — file guide

Android client: Kotlin, Jetpack Compose, DataStore, OkHttp, Hilt. Uses WebSocket for live events, and talks REST for everything else. Package root: `com.example.picompanion`.

## Data layer (`data/`)

`ui/sessiondetail/MessageInputBar.kt` provides inline suggestions for common slash commands. Commands are sent as ordinary prompt text so custom Pi commands continue to work.
`SessionDetailViewModel` also renders provider and Pi errors from completed assistant messages in the timeline and clears the sending state.

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
- `SessionDetailViewModel.kt` — the core: loads JSONL history via the parser, runs the event pipeline (`Channel → tracker → handleEvent → _items`), keeps `_items` updates atomic (CAS), serializes assistant bubble transitions under `assistantMutex`, guards against stale HTTP overwrites with `historyGeneration`, and handles extension-UI questions. A `ConnectivityManager.NetworkCallback` schedules backoff reconnects on `onLost` and a clean `transport.disconnect` + immediate reconnect on `onAvailable`, so a brief Wi-Fi/data blip self-heals without a manual refresh.
- `SessionTransportCoordinator.kt` — owns ticket acquisition and socket lifecycle independently of UI state so reconnects don't leak or drop resources.
- `SessionHistoryParser.kt` — pure (CPU-only, off-main-thread) conversion of persisted Pi JSONL into chat items; must understand every history shape listed in AGENTS.md.
- `SessionHistoryState.kt` — history load state machine (loading/pages/resync) with tests.
- `SessionStateCache.kt` — small in-memory LRU so switching sessions back and forth is instant.
- `PendingPromptQueue.kt` — queues prompts sent while the agent is mid-run; drains when the turn settles.
- `PromptImageEncoder.kt` — image → base64 attachment prep.
- `SessionDetailScreen.kt` — the chat screen composition (LazyColumn of timeline items, keyed uniquely via seen-set + index fallback). Item placement only animates after the first scroll settles, avoiding a visible "screen jump" while cache, history, and live events fill the list on open.
- `ChatBubble.kt`, `TimelineRows.kt`, `ChatEmptyState.kt`, `SessionHeader.kt`, `MessageInputBar.kt` — individual timeline/message UI pieces.
- `ExtensionUiDialog.kt` — renders blocking `ask_user`/select/confirm/input questions from Pi and posts the response.
- `FileBrowserSheet.kt`, `UnifiedActionsSheet.kt` — bottom sheets for file browsing and session actions (model, thinking level, abort, git quick actions).

### Workers (`ui/workers/`)
- `WorkersViewModel.kt` / `WorkersScreen.kt` — worker list with health.
- `WorkerEditorDialog.kt` — add/edit a worker (URL, token).

### Settings (`ui/settings/`)
- `SettingsViewModel.kt` / `SettingsScreen.kt` / `SettingsRow.kt` / `SettingsSection.kt` — settings UI over the DataStore, including a first-run QR pairing shortcut that creates the server entry automatically. The server-address hint uses a home-LAN address on the shared `3142` port. The About section shows an in-app updater that queries GitHub Releases.
- `PairingScanActivity.kt`, `PairingScanOverlayView.kt`, `PairingScanSquareLayout.kt` — camera QR scanning for server pairing (fullscreen overlay activity with a square preview layout).

### Updater (`data/updater/`)
- `ReleaseStore.kt` — queries the public GitHub Releases API for `fleetcru/pi-stack`, selects the newest `pi-companion-*.apk` asset by publish time across both prereleases and stable, and returns its download URL.
- `AppUpdater.kt` — downloads the selected APK to app cache, checks the downloaded `versionCode` against the installed one to avoid downgrades, and stages a silent install through Android's `PackageInstaller` (requires REQUEST_INSTALL_PACKAGES).

## Theme

- `theme/Color.kt`, `Theme.kt`, `Type.kt` — Material 3 theming with dark mode support.

## Build and release

`.circleci/config.yml` is a dynamic setup configuration. The certified `circleci/path-filtering` orb continues to `.circleci/continue_config.yml` and enables the Android workflow only when `pi-companion-exp/**` or `.circleci/**` changed. Other commits run only the small setup job. Its setup job explicitly accepts `v*` tags; without that filter CircleCI creates a tag pipeline with no jobs. A `v*` tag always enables the continuation workflow so releases cannot be skipped by path filtering.

The continuation config builds and signs the release APK in `cimg/android:2026.08.1`. CircleCI is the sole automatic publisher for `v*` tags; `.github/workflows/build-android.yml` remains available only through manual dispatch so the two CI systems cannot race to create one release. The manual workflow validates its SemVer input, checks the decoded keystore before compiling, and always publishes a uniquely numbered prerelease. The `pi-stack-signing` context supplies `KEYSTORE_BASE64`, `KEYSTORE_PASSWORD`, `KEY_ALIAS`, and `KEY_PASSWORD`. Store context values without trailing CR or LF characters. The workflow decodes the keystore and runs `keytool -list` before compilation so malformed signing secrets fail early. The build job stores `release-artifacts/` in a CircleCI workspace. For `v*` tags, a separate small job uses the certified `circleci/github-cli` orb and `GH_TOKEN` to create the GitHub Release and upload `pi-companion-<version>.apk`. Tags containing a suffix such as `-beta.1` create prereleases. Reruns replace an existing APK asset.

## Tests

Unit tests under `app/src/test/` cover the pure logic (tracker, parser, queue, dedup, encoder, inventory state). Run with `./gradlew :app:testDebugUnitTest`; compile check with `:app:compileDebugKotlin`.

## Networking

- `res/xml/network_security_config.xml` — permits cleartext HTTP for the trusted home-LAN and Tailscale deployment model (private RFC1918 and `100.64.0.0/10` addresses with the bearer-token credential), while system trust anchors still apply to HTTPS. Public HTTP is not a supported deployment.
