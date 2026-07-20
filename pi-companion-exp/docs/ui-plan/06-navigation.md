# 06 Navigation Wiring

Preferred route structure:

```kotlin
sealed interface AppRoute : NavKey {
  data object Home : AppRoute
  data object Sessions : AppRoute
  data class SessionDetail(val sessionId: String) : AppRoute
  data object Workers : AppRoute
  data object Settings : AppRoute
}
```

Goal:
- Bottom nav switches between Home, Sessions, Workers, Settings.
- Tapping a session opens `SessionDetail(sessionId)`.

If full navigation is too much for the first pass, create previewable screens first and leave clickable nav behavior as TODO.

Keep navigation changes small and careful.
