# 03 Session Chat Page

Create:

```text
app/src/main/java/com/example/picompanion/ui/sessiondetail/SessionDetailScreen.kt
app/src/main/java/com/example/picompanion/ui/sessiondetail/ChatBubble.kt
app/src/main/java/com/example/picompanion/ui/sessiondetail/SessionHeader.kt
app/src/main/java/com/example/picompanion/ui/sessiondetail/MessageInputBar.kt
```

Purpose: show an example active Pi session with chat UI.

Header:
- back/menu icon
- session title
- status pill
- project/cwd text

Example:

```text
Build Android companion       Running
pi-companion · C:/Users/basin/Desktop/pi-companion
```

Body:
- scrollable chat
- user messages aligned right
- agent messages aligned left
- rounded grey bubbles
- timestamps
- optional tool/status message style

Bottom input:

```text
[Prompt, steer, or ask something...] [send icon]
```

No real send behavior required yet. It can append local mock messages if easy.
