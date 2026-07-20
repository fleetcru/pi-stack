# 05 Session WebSocket Stream

Purpose: show real session activity in Session Detail.

Create:

```text
data/websocket/SessionEventSocket.kt
data/repository/SessionStreamRepository.kt
ui/sessiondetail/SessionDetailViewModel.kt
```

Connect to:

```text
GET /v1/sessions/{id}/ws?since={lastEventId}
```

Behavior:
- use OkHttp WebSocket
- add Authorization header when token exists
- subscribe when SessionDetail opens
- close socket when ViewModel is cleared
- reconnect only if setting allows it
- persist latest `_daemonEventId` per session later

Event handling:
- parse each inbound message as JSON
- preserve raw JSON for unknown event types
- map known events to chat/activity rows
- support daemon events, tool events, assistant/user text, file changes

Suggested UI model:

```kotlin
sealed interface SessionTimelineItem {
  data class Chat(val author: String, val text: String, val time: String, val isUser: Boolean) : SessionTimelineItem
  data class Tool(val name: String, val status: String, val detail: String?) : SessionTimelineItem
  data class FileChange(val path: String, val operation: String) : SessionTimelineItem
  data class System(val text: String) : SessionTimelineItem
}
```

Acceptance:
- opening a session shows live events
- disconnect state is visible
- unknown event types do not crash app
