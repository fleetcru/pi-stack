# 04 Settings Page

Create:

```text
app/src/main/java/com/example/picompanion/ui/settings/SettingsScreen.kt
app/src/main/java/com/example/picompanion/ui/settings/SettingsRow.kt
app/src/main/java/com/example/picompanion/ui/settings/SettingsSection.kt
```

Settings sections:

## Server

Show mock/example values:
- Server URL: `http://100.x.x.x:3141`
- Auth token: `••••••••`
- Connection mode: `Tailscale / local network`
- Test connection button, fake for now

## App behavior

- Default project root: `C:/Users/basin`
- Reconnect WebSocket on launch
- Remember last selected session
- Dark theme, enabled/locked for now

## Session behavior

- Replay events since last seen
- Show file change events
- Show tool events
- Show daemon events

## About

- App version: `0.1 debug`
- API target: `pi-server /v1`
- Build type: `Debug`
