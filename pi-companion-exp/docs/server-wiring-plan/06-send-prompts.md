# 06 Sending Prompts

Purpose: make Session Detail input send prompts to `pi-server`.

Endpoint:

```text
POST /v1/sessions/{id}/send
```

Create request model after checking current server shape. Suggested initial shape if compatible:

```kotlin
@Serializable
data class SendPromptRequest(
  val prompt: String,
)
```

Repository:

```kotlin
suspend fun sendPrompt(sessionId: String, prompt: String): HttpResult<Unit>
```

UI behavior:
- disable send button while request is in flight
- optimistically add user message or wait for server echo; choose one explicitly
- show error if send fails
- keep unsent text if failure occurs
- support multiline prompts

Later controls:
- steer/follow-up endpoint
- retry
- compact
- abort/cancel if server exposes it

Acceptance:
- typing and pressing send sends to real session
- failure is visible and non-destructive
- WebSocket eventually shows assistant/tool response
