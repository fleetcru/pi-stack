# 02 API Client Foundation

Purpose: centralize HTTP calls to `pi-server`.

Create:

```text
data/api/PiServerClient.kt
data/api/HttpResult.kt
data/api/JsonConfig.kt
```

Use existing dependencies:
- OkHttp
- kotlinx.serialization-json

Client constructor:

```kotlin
class PiServerClient(
  private val okHttpClient: OkHttpClient,
  private val json: Json,
  private val settingsDataStore: SettingsDataStore,
)
```

Required behavior:
- build URLs from SettingsDataStore `serverUrl`
- add `Authorization: Bearer <token>` only when token is non-blank
- JSON decode with `ignoreUnknownKeys = true`
- convert network failures into typed UI-friendly errors
- do not crash on malformed server responses

Suggested result type:

```kotlin
sealed interface HttpResult<out T> {
  data class Success<T>(val value: T) : HttpResult<T>
  data class Failure(val message: String, val cause: Throwable? = null) : HttpResult<Nothing>
}
```

First endpoints:

```text
GET /v1/daemon
GET /v1/sessions
GET /v1/workers
```

Acceptance:
- `Test connection` can call daemon/status endpoint
- auth token is respected
- connection failures show helpful errors
