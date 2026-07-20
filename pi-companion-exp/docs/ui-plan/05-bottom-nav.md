# 05 Bottom Navigation

Current nav should be changed to:

```text
Home | Sessions | Workers | Settings
```

Reasoning:
- Home: dashboard summary
- Sessions: active Pi sessions and session list/sidebar
- Workers: local/remote machines later
- Settings: server URL, auth token, behavior

Use Material Icons Extended:

```kotlin
Icons.Rounded.Home
Icons.Rounded.Terminal
Icons.Rounded.Dns
Icons.Rounded.Settings
```

Optional later addition:

```text
Connected · 3 sessions
```

For now, keep the nav simple and fixed to the bottom of the screen.
