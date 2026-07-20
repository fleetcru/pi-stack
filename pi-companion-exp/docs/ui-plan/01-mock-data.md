# 01 Mock Data

Create:

```text
app/src/main/java/com/example/picompanion/ui/mock/MockModels.kt
```

Add mock-only models:

```kotlin
data class MockSession(
  val id: String,
  val title: String,
  val project: String,
  val cwd: String,
  val status: String,
  val lastMessage: String,
  val updatedAt: String,
)

data class MockChatMessage(
  val id: String,
  val author: String,
  val text: String,
  val time: String,
  val isUser: Boolean,
)

data class MockWorker(
  val id: String,
  val name: String,
  val status: String,
  val capacity: String,
)
```

Add example lists:

```kotlin
val mockSessions = listOf(
  MockSession("1", "Build Android companion", "pi-companion", "C:/Users/basin/Desktop/pi-companion", "Running", "Updating Compose dashboard components", "now"),
  MockSession("2", "Pi server hardening", "pi-server", "C:/Users/basin/pi-server", "Idle", "go test ./... passed", "12m"),
  MockSession("3", "Browser test client", "pi-server-full-test", "C:/Users/basin/Desktop/pi-server-full-test", "Stopped", "Waiting for next task", "1h"),
)

val mockChatMessages = listOf(
  MockChatMessage("1", "You", "Can you make the Android app look more like the mockup?", "9:14 PM", true),
  MockChatMessage("2", "Pi Agent", "Sure — I updated the dark dashboard, icons, and bottom navigation.", "9:15 PM", false),
  MockChatMessage("3", "You", "Now make a session sidebar and chat UI example.", "9:22 PM", true),
  MockChatMessage("4", "Pi Agent", "I can scaffold the mock session list, detail chat screen, and settings page.", "9:22 PM", false),
)
```
