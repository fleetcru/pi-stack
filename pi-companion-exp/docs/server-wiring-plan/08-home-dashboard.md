# 08 Home Dashboard Wiring

Purpose: replace static dashboard numbers with real status.

Data sources:

```text
GET /v1/daemon
GET /v1/sessions
GET /v1/workers
```

Home should show:
- connection status
- session count
- worker count
- latest/active session card
- recent server error if any

Create:

```text
ui/main/HomeViewModel.kt
```

UI state:

```kotlin
sealed interface HomeUiState {
  data object Loading : HomeUiState
  data class Content(
    val connected: Boolean,
    val sessionCount: Int,
    val workerCount: Int,
    val latestSession: SessionListItemUi?,
  ) : HomeUiState
  data class Error(val message: String) : HomeUiState
}
```

Acceptance:
- Home card opens the actual latest/active session
- numbers match server responses
- offline state is clear
