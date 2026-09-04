package com.example.picompanion

import androidx.compose.animation.SharedTransitionLayout
import androidx.compose.animation.core.tween
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.slideInHorizontally
import androidx.compose.animation.slideOutHorizontally
import androidx.compose.animation.togetherWith
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import com.example.picompanion.di.AppModule
import androidx.compose.ui.Modifier
import androidx.navigation3.runtime.entryProvider
import androidx.navigation3.runtime.rememberNavBackStack
import androidx.navigation3.ui.LocalNavAnimatedContentScope
import androidx.navigation3.ui.NavDisplay
import com.example.picompanion.ui.components.NavTab
import kotlinx.coroutines.launch
import com.example.picompanion.ui.main.ShellScreen
import com.example.picompanion.ui.sessiondetail.SessionDetailScreen

@Composable
fun MainNavigation(
  darkTheme: Boolean,
  onDarkThemeChange: (Boolean) -> Unit,
) {
  SharedTransitionLayout {
    val backStack = rememberNavBackStack(AppRoute.Home)
    val sharedTransitionScope = this@SharedTransitionLayout
    val settings by AppModule.settingsDataStore.settingsFlow.collectAsStateWithLifecycle(
      initialValue = com.example.picompanion.data.settings.AppSettings(),
    )
    val scope = rememberCoroutineScope()
    LaunchedEffect(settings.rememberLastSession, settings.lastSessionServerId, settings.lastSessionId) {
      val sessionId = settings.lastSessionId
      if (
        settings.rememberLastSession &&
        sessionId.isNotBlank() &&
        settings.lastSessionServerId == settings.activeServer?.id &&
        backStack.none { it == AppRoute.SessionDetail(sessionId) }
      ) {
        backStack.add(AppRoute.SessionDetail(sessionId))
      }
    }
    val navigate: (AppRoute) -> Unit = { route ->
      if (route is AppRoute.SessionDetail && settings.rememberLastSession) {
        settings.activeServer?.id?.takeIf { it.isNotBlank() }?.let { serverId ->
          scope.launch { AppModule.settingsDataStore.setLastSession(serverId, route.sessionId) }
        }
      }
      backStack.add(route)
    }
    fun returnToSessions() {
      while (backStack.size > 1) backStack.removeLastOrNull()
      if (backStack.lastOrNull() != AppRoute.Sessions) backStack.add(AppRoute.Sessions)
    }

    NavDisplay(
      backStack = backStack,
      onBack = {
        if (backStack.lastOrNull() is AppRoute.SessionDetail) returnToSessions()
        else backStack.removeLastOrNull()
      },
      transitionSpec = {
        fadeIn(animationSpec = tween(60)) togetherWith
          fadeOut(animationSpec = tween(40))
      },
      popTransitionSpec = {
        fadeIn(animationSpec = tween(60)) togetherWith
          fadeOut(animationSpec = tween(40))
      },
      predictivePopTransitionSpec = {
        fadeIn(animationSpec = tween(60)) togetherWith
          fadeOut(animationSpec = tween(40))
      },
      entryProvider = entryProvider {
        entry<AppRoute.Home> {
          ShellScreen(
            initialTab = NavTab.Home,
            onNavigate = navigate,
            onBack = { backStack.removeLastOrNull() },
            darkTheme = darkTheme,
            onDarkThemeChange = onDarkThemeChange,
            sharedTransitionScope = sharedTransitionScope,
            animatedVisibilityScope = LocalNavAnimatedContentScope.current,
            modifier = Modifier.safeDrawingPadding(),
          )
        }
        entry<AppRoute.Sessions> {
          ShellScreen(
            initialTab = NavTab.Sessions,
            onNavigate = navigate,
            onBack = { backStack.removeLastOrNull() },
            darkTheme = darkTheme,
            onDarkThemeChange = onDarkThemeChange,
            sharedTransitionScope = sharedTransitionScope,
            animatedVisibilityScope = LocalNavAnimatedContentScope.current,
            modifier = Modifier.safeDrawingPadding(),
          )
        }
        entry<AppRoute.Workers> {
          ShellScreen(
            initialTab = NavTab.Workers,
            onNavigate = navigate,
            onBack = { backStack.removeLastOrNull() },
            darkTheme = darkTheme,
            onDarkThemeChange = onDarkThemeChange,
            sharedTransitionScope = sharedTransitionScope,
            animatedVisibilityScope = LocalNavAnimatedContentScope.current,
            modifier = Modifier.safeDrawingPadding(),
          )
        }
        entry<AppRoute.Settings> {
          ShellScreen(
            initialTab = NavTab.Settings,
            onNavigate = navigate,
            onBack = { backStack.removeLastOrNull() },
            darkTheme = darkTheme,
            onDarkThemeChange = onDarkThemeChange,
            sharedTransitionScope = sharedTransitionScope,
            animatedVisibilityScope = LocalNavAnimatedContentScope.current,
            modifier = Modifier.safeDrawingPadding(),
          )
        }
        entry<AppRoute.SessionDetail> { key ->
          SessionDetailScreen(
            sessionId = key.sessionId,
            onBack = ::returnToSessions,
            sharedTransitionScope = sharedTransitionScope,
            animatedVisibilityScope = LocalNavAnimatedContentScope.current,
            modifier = Modifier.safeDrawingPadding(),
          )
        }
      },
    )
  }
}
