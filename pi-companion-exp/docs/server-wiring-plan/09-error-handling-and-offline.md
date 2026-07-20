# 09 Error Handling and Offline Behavior

Purpose: make the app pleasant on unreliable private networks.

States to handle everywhere:
- missing server URL
- DNS/host unreachable
- connection refused
- auth failure
- malformed JSON
- server 4xx/5xx
- WebSocket disconnected
- no sessions/workers

UI patterns:
- small inline error cards
- retry buttons
- clear empty states
- no crashes from network failure

Suggested shared model:

```kotlin
data class UiError(
  val title: String,
  val message: String,
  val retryable: Boolean = true,
)
```

Acceptance:
- app opens without server running
- Settings remains usable when offline
- failed session stream can reconnect or show disconnected state
