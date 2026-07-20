# 02 Sessions Page

Create:

```text
app/src/main/java/com/example/picompanion/ui/sessions/SessionsScreen.kt
app/src/main/java/com/example/picompanion/ui/sessions/SessionListItem.kt
```

Purpose: mobile version of a sessions sidebar.

Layout:
- title: `Sessions`
- subtitle: `Local and remote Pi workspaces`
- fake search/filter bar
- vertical list of session cards
- selected/current session visually highlighted

Each session item should show:
- icon
- title
- project
- status pill
- last message
- updated time

Example:

```text
[icon] Build Android companion    Running
       pi-companion
       Updating Compose dashboard components      now
```

Use `mockSessions` only.
