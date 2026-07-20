package com.example.picompanion

import androidx.navigation3.runtime.NavKey
import kotlinx.serialization.Serializable

sealed interface AppRoute : NavKey {
  @Serializable data object Home : AppRoute
  @Serializable data object Sessions : AppRoute
  @Serializable data class SessionDetail(val sessionId: String) : AppRoute
  @Serializable data object Workers : AppRoute
  @Serializable data object Settings : AppRoute
}

// Keep old Main key for backward compat during transition
@Serializable data object Main : NavKey
