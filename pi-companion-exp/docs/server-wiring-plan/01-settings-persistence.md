# 01 Settings Persistence

Purpose: store connection settings before calling the server.

Create:

```text
data/settings/AppSettings.kt
data/settings/SettingsDataStore.kt
```

Fields:

```kotlin
data class AppSettings(
  val serverUrl: String = "http://127.0.0.1:3141",
  val authToken: String = "",
  val reconnectOnLaunch: Boolean = true,
  val rememberLastSession: Boolean = true,
  val replayEventsSinceLastSeen: Boolean = true,
  val showFileChangeEvents: Boolean = true,
  val showToolEvents: Boolean = true,
  val showDaemonEvents: Boolean = true,
  val defaultProjectRoot: String = "",
)
```

Use DataStore Preferences.

Expose:

```kotlin
val settingsFlow: Flow<AppSettings>
suspend fun updateServerUrl(value: String)
suspend fun updateAuthToken(value: String)
suspend fun updateBooleanSetting(...)
```

UI changes:
- Settings screen should read values from a ViewModel.
- Server URL and token should be editable.
- Token should be visually masked by default.
- Add fake/real `Test connection` button after API client exists.

Acceptance:
- values survive app restart
- no hardcoded server URL in repository/client code
