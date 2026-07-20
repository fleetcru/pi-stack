# 04 Sessions Repository

Purpose: replace `mockSessions` with real server sessions.

Create:

```text
data/repository/SessionsRepository.kt
ui/sessions/SessionsViewModel.kt
```

Repository functions:

```kotlin
suspend fun listSessions(): HttpResult<List<ServerSession>>
suspend fun createSession(request: CreateSessionRequest): HttpResult<ServerSession>
suspend fun getSession(id: String): HttpResult<ServerSession>
```

Initial endpoints:

```text
GET /v1/sessions
POST /v1/sessions
```

UI state:

```kotlin
sealed interface SessionsUiState {
  data object Loading : SessionsUiState
  data class Content(val sessions: List<SessionListItemUi>) : SessionsUiState
  data class Error(val message: String) : SessionsUiState
}
```

UI behavior:
- show loading skeleton or text
- show empty state when no sessions exist
- preserve local search filtering
- manual refresh button
- tapping a session opens SessionDetail

Acceptance:
- Sessions page shows real sessions from server
- mock data is only used in previews
- failed requests do not crash app
