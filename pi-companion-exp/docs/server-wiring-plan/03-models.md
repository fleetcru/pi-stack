# 03 API Models

Purpose: define serializable models matching `pi-server` responses, while keeping UI models stable.

Create:

```text
data/model/SessionModels.kt
data/model/WorkerModels.kt
data/model/EventModels.kt
data/model/DaemonModels.kt
```

Use `@Serializable` and nullable/default fields where server shape may evolve.

Session model should include at least:

```kotlin
@Serializable
data class ServerSession(
  val id: String,
  val cwd: String? = null,
  val status: String? = null,
  val project: String? = null,
  val title: String? = null,
  val taskType: String? = null,
  val owner: String? = null,
  val labels: List<String> = emptyList(),
)
```

Mapping rule:
- Do not bind Compose directly to raw API models forever.
- Add mapper functions to convert server models into UI state.

Example:

```kotlin
fun ServerSession.toSessionListItemUi(): SessionListItemUi
```

Acceptance:
- unknown JSON fields do not break the app
- missing metadata still renders clean fallback text
