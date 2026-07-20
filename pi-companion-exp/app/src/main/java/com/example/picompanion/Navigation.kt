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
import androidx.compose.ui.Modifier
import androidx.navigation3.runtime.entryProvider
import androidx.navigation3.runtime.rememberNavBackStack
import androidx.navigation3.ui.LocalNavAnimatedContentScope
import androidx.navigation3.ui.NavDisplay
import com.example.picompanion.ui.components.NavTab
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

    NavDisplay(
      backStack = backStack,
      onBack = { backStack.removeLastOrNull() },
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
            onNavigate = { route -> backStack.add(route) },
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
            onNavigate = { route -> backStack.add(route) },
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
            onNavigate = { route -> backStack.add(route) },
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
            onNavigate = { route -> backStack.add(route) },
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
            onBack = {
              backStack.removeLastOrNull()
              if (backStack.lastOrNull() !is AppRoute.Sessions) {
                backStack.add(AppRoute.Sessions)
              }
            },
            sharedTransitionScope = sharedTransitionScope,
            animatedVisibilityScope = LocalNavAnimatedContentScope.current,
            modifier = Modifier.safeDrawingPadding(),
          )
        }

        entry<Main> {
          ShellScreen(
            initialTab = NavTab.Home,
            onNavigate = { route -> backStack.add(route) },
            onBack = { backStack.removeLastOrNull() },
            darkTheme = darkTheme,
            onDarkThemeChange = onDarkThemeChange,
            sharedTransitionScope = sharedTransitionScope,
            animatedVisibilityScope = LocalNavAnimatedContentScope.current,
            modifier = Modifier.safeDrawingPadding(),
          )
        }
      },
    )
  }
}
