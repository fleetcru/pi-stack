package com.example.picompanion.ui.main

import android.widget.Toast
import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.WindowInsets
import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.ime
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.platform.LocalDensity
import androidx.compose.ui.unit.dp
import com.example.picompanion.AppRoute
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.di.AppModule
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.repository.SessionsRepository
import com.example.picompanion.data.settings.AppSettings
import com.example.picompanion.ui.components.BottomNavBar
import com.example.picompanion.ui.components.DirectoryBrowserSheet
import com.example.picompanion.ui.components.NavTab
import com.example.picompanion.ui.components.SessionDrawer
import com.example.picompanion.ui.sessions.SessionInventoryState
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
  var isCreatingSession by remember { mutableStateOf(false) }
  val sessionsRepository = remember { SessionsRepository(AppModule.client, AppModule.settingsDataStore) }
  val keyboardOpen = WindowInsets.ime.getBottom(LocalDensity.current) > 0

  // Sessions for the drawer
  var drawerSessions by remember { mutableStateOf<List<ServerSession>>(emptyList()) }
  var drawerLoading by remember { mutableStateOf(false) }
  val client = AppModule.client
  val settingsDataStore = AppModule.settingsDataStore
  val settings by settingsDataStore.settingsFlow.collectAsStateWithLifecycle(initialValue = AppSettings())
  val coroutineScope = rememberCoroutineScope()
  val context = LocalContext.current

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
    Scaffold(
      modifier = Modifier.fillMaxSize(),
      contentWindowInsets = WindowInsets(0, 0, 0, 0),
      bottomBar = {
        if (!keyboardOpen) {
          Box(
            modifier = Modifier
              .fillMaxWidth()
              .padding(start = 20.dp, end = 20.dp, bottom = 24.dp),
          ) {
            BottomNavBar(
              selectedTab = selectedTab,
              onTabSelected = { selectedTab = it },
            )
          }
        }
      },
    ) { innerPadding ->
      Box(
        Modifier
          .fillMaxSize()
          .padding(innerPadding)
          .consumeWindowInsets(innerPadding),
      ) {
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
      isCreating = isCreatingSession,
      onDismiss = { if (!isCreatingSession) showNewSessionBrowser = false },
      onSelect = { selection ->
        coroutineScope.launch {
          isCreatingSession = true
          try {
            val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
            val outcomes = sessionsRepository.createSessions(
              cwd = selection.cwd,
              prompt = selection.prompt,
              count = selection.count,
              title = selection.title,
              createWorktree = selection.createWorktree,
              workerId = selection.workerId,
              server = server,
            )
            val createdIds = outcomes.mapNotNull { it.sessionId }
            if (createdIds.isNotEmpty()) SessionInventoryState.markStale(server.id)
            val failures = outcomes.mapNotNull { it.error }
            if (failures.isNotEmpty()) {
              val message = if (createdIds.isEmpty()) failures.first()
              else "${createdIds.size} of ${outcomes.size} sessions completed without errors"
              Toast.makeText(context, message, Toast.LENGTH_LONG).show()
            }
            showNewSessionBrowser = false
            if (selection.count == 1) {
              createdIds.lastOrNull()?.let { onNavigate(AppRoute.SessionDetail(it)) }
            }
          } finally {
            isCreatingSession = false
          }
        }
      },
    )
  }
}
