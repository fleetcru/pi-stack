package com.example.picompanion.ui.main

import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.padding
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.dp
import com.example.picompanion.AppRoute
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.CreateSessionRequest
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.settings.AppSettings
import com.example.picompanion.data.settings.SettingsDataStore
import com.example.picompanion.ui.components.BottomNavBar
import com.example.picompanion.ui.components.DirectoryBrowserSheet
import com.example.picompanion.ui.components.NavTab
import com.example.picompanion.ui.components.SessionDrawer
import com.example.picompanion.ui.sessions.SessionsScreen
import com.example.picompanion.ui.settings.SettingsScreen
import com.example.picompanion.ui.workers.WorkersScreen
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@Composable
fun ShellScreen(
  initialTab: NavTab,
  onNavigate: (AppRoute) -> Unit,
  onBack: () -> Unit,
  darkTheme: Boolean,
  onDarkThemeChange: (Boolean) -> Unit,
  sharedTransitionScope: SharedTransitionScope,
  animatedVisibilityScope: AnimatedVisibilityScope,
  modifier: Modifier = Modifier,
) {
  var selectedTab by remember { mutableStateOf(initialTab) }
  var drawerOpen by remember { mutableStateOf(false) }
  var showNewSessionBrowser by remember { mutableStateOf(false) }
  val keyboardOpen = WindowInsets.ime.getBottom(LocalDensity.current) > 0

  // Sessions for the drawer
  var drawerSessions by remember { mutableStateOf<List<ServerSession>>(emptyList()) }
  var drawerLoading by remember { mutableStateOf(false) }
  val context = LocalContext.current
  val client = remember { PiServerClient() }
  val settingsDataStore = remember { SettingsDataStore(context) }
  val settings by settingsDataStore.settingsFlow.collectAsStateWithLifecycle(initialValue = AppSettings())
  val coroutineScope = rememberCoroutineScope()

  LaunchedEffect(drawerOpen) {
    if (drawerOpen) {
      drawerLoading = true
      val settings = settingsDataStore.settingsFlow.first()
      val server = settings.activeServer
      if (server != null && server.isConfigured) {
        val result = withContext(Dispatchers.IO) {
          client.listSessions(server)
        }
        drawerSessions = (result as? HttpResult.Success)?.value?.sessions ?: emptyList()
      }
      drawerLoading = false
    }
  }

  Box(modifier.fillMaxSize()) {
    Box(Modifier.fillMaxSize()) {
      if (selectedTab == NavTab.Home) {
        MainScreen(
          onSessionClick = { sessionId -> onNavigate(AppRoute.SessionDetail(sessionId)) },
          onNavigate = onNavigate,
          onMenuClick = { drawerOpen = true },
        )
      }
      if (selectedTab == NavTab.Sessions) {
        SessionsScreen(
          onSessionClick = { sessionId -> onNavigate(AppRoute.SessionDetail(sessionId)) },
          sharedTransitionScope = sharedTransitionScope,
          animatedVisibilityScope = animatedVisibilityScope,
        )
      }
      if (selectedTab == NavTab.Workers) {
        WorkersScreen()
      }
      if (selectedTab == NavTab.Settings) {
        SettingsScreen(darkTheme = darkTheme, onDarkThemeChange = onDarkThemeChange)
      }
    }

    // Bottom nav
    if (!keyboardOpen) {
      BottomNavBar(
        selectedTab = selectedTab,
        onTabSelected = { selectedTab = it },
        modifier = Modifier
          .align(Alignment.BottomCenter)
          .padding(start = 20.dp, end = 20.dp, bottom = 24.dp),
      )
    }

    // Session drawer
    SessionDrawer(
      visible = drawerOpen,
      onDismiss = { drawerOpen = false },
      onSessionClick = { sessionId ->
        onNavigate(AppRoute.SessionDetail(sessionId))
        drawerOpen = false
      },
      onNewSession = {
        drawerOpen = false
        showNewSessionBrowser = true
      },
      sessions = drawerSessions,
      isLoading = drawerLoading,
    )

    DirectoryBrowserSheet(
      visible = showNewSessionBrowser,
      server = settings.activeServer,
      onDismiss = { showNewSessionBrowser = false },
      onSelect = { cwd ->
        showNewSessionBrowser = false
        coroutineScope.launch {
          val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
          val created = withContext(Dispatchers.IO) {
            client.createSession(server, CreateSessionRequest(cwd = cwd, start = true))
          }
          if (created is HttpResult.Success) {
            onNavigate(AppRoute.SessionDetail(created.value.id))
          }
        }
      },
    )
  }
}
