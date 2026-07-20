package com.example.picompanion.ui.main

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.fadeIn
import androidx.compose.animation.fadeOut
import androidx.compose.animation.scaleIn
import androidx.compose.animation.scaleOut
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.rounded.KeyboardArrowRight
import androidx.compose.material.icons.rounded.Dns
import androidx.compose.material.icons.rounded.Groups
import androidx.compose.material.icons.rounded.KeyboardArrowUp
import androidx.compose.material.icons.rounded.Person
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Timeline
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.SmallFloatingActionButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.tooling.preview.Preview
import androidx.compose.ui.unit.dp
import com.example.picompanion.AppRoute
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.theme.PiCompanionTheme
import com.example.picompanion.ui.components.IconTile
import com.example.picompanion.ui.components.LoadingScreen
import com.example.picompanion.ui.components.StatCard
import com.example.picompanion.ui.components.StatusPill
import com.example.picompanion.ui.components.TopAppBarCompact
import androidx.compose.runtime.DisposableEffect
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.compose.ui.platform.LocalLifecycleOwner
import kotlinx.coroutines.launch

@Composable
fun MainScreen(
  onSessionClick: (String) -> Unit,
  onNavigate: (AppRoute) -> Unit,
  onMenuClick: () -> Unit = {},
  modifier: Modifier = Modifier,
  viewModel: HomeViewModel = viewModel(),
) {
  val uiState by viewModel.uiState.collectAsStateWithLifecycle()

  // Start/stop polling based on screen visibility to save battery.
  val lifecycleOwner = LocalLifecycleOwner.current
  DisposableEffect(lifecycleOwner, viewModel) {
    val observer = LifecycleEventObserver { _, event ->
      when (event) {
        Lifecycle.Event.ON_RESUME -> viewModel.startPolling()
        Lifecycle.Event.ON_PAUSE -> viewModel.stopPolling()
        else -> {}
      }
    }
    lifecycleOwner.lifecycle.addObserver(observer)
    onDispose {
      lifecycleOwner.lifecycle.removeObserver(observer)
      viewModel.stopPolling()
    }
  }

  when (val state = uiState) {
    HomeUiState.Loading -> LoadingScreen(modifier = modifier)
    HomeUiState.NoServer -> HomeNoServerContent(
      onNavigate = onNavigate,
      onMenuClick = onMenuClick,
      modifier = modifier,
    )
    is HomeUiState.Content -> HomeContent(
      state = state,
      onSessionClick = onSessionClick,
      onNavigate = onNavigate,
      onMenuClick = onMenuClick,
      onRefresh = { viewModel.refresh() },
      onGlobalSessionClick = { id -> viewModel.attachGlobalSession(id, onSessionClick) },
      onMachineSessionClick = { id -> viewModel.openMachineSession(id, onSessionClick) },
      onUpdateCapacity = { viewModel.updateCapacity(it) },
      modifier = modifier,
    )
    is HomeUiState.Error -> HomeErrorContent(
      message = state.message,
      serverName = state.serverName,
      onRetry = { viewModel.refresh() },
      onNavigate = onNavigate,
      onMenuClick = onMenuClick,
      modifier = modifier,
    )
  }
}

@Composable
internal fun HomeContent(
  state: HomeUiState.Content,
  onSessionClick: (String) -> Unit,
  onNavigate: (AppRoute) -> Unit,
  onMenuClick: () -> Unit = {},
  onRefresh: () -> Unit = {},
  onGlobalSessionClick: (String) -> Unit = {},
  onMachineSessionClick: (String) -> Unit = {},
  onUpdateCapacity: (Int) -> Unit = {},
  modifier: Modifier = Modifier,
) {
  val listState = rememberLazyListState()
  val coroutineScope = rememberCoroutineScope()

  val showScrollToTop by remember {
    derivedStateOf { listState.firstVisibleItemIndex > 0 }
  }

  Box(modifier = modifier.fillMaxSize()) {
    LazyColumn(
      state = listState,
      modifier = Modifier.fillMaxSize().padding(horizontal = 16.dp),
      verticalArrangement = Arrangement.spacedBy(12.dp),
      contentPadding = PaddingValues(bottom = 80.dp),
    ) {
      // Header
      item { TopAppBarCompact(onMenuClick = onMenuClick, onSettingsClick = { onNavigate(AppRoute.Settings) }) }

      // Connection status
      if (!state.connected) {
        item {
          Surface(
            modifier = Modifier.fillMaxWidth(),
            shape = RoundedCornerShape(12.dp),
            color = MaterialTheme.colorScheme.errorContainer,
          ) {
            Row(Modifier.padding(14.dp), verticalAlignment = Alignment.CenterVertically) {
              Text(
                "Not connected to ${state.serverName}",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onErrorContainer,
                modifier = Modifier.weight(1f),
              )
              IconButton(onClick = onRefresh, modifier = Modifier.size(32.dp)) {
                Icon(Icons.Rounded.Refresh, contentDescription = "Retry", modifier = Modifier.size(16.dp))
              }
            }
          }
        }
      }

      // Stat cards
      item {
        var showCapacityDialog by remember { mutableStateOf(false) }
        Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(10.dp)) {
          StatCard("Sessions", "${state.sessions.size}", Icons.Rounded.Person, Modifier.weight(1f))
          StatCard("Workers", "${state.workers.size}", Icons.Rounded.Groups, Modifier.weight(1f))
          Surface(
            modifier = Modifier.weight(1f).clickable { showCapacityDialog = true },
            shape = RoundedCornerShape(16.dp),
            color = MaterialTheme.colorScheme.surface,
            border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
          ) {
            Column(Modifier.padding(14.dp), verticalArrangement = Arrangement.spacedBy(12.dp)) {
              Row(Modifier.fillMaxWidth(), verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                IconTile(Icons.Rounded.Timeline, "Active")
                Text("Active", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
              }
              Text(
                if (state.maxSessions > 0) "${state.activeSessions}/${state.maxSessions}" else "${state.activeSessions}",
                style = MaterialTheme.typography.headlineMedium,
                fontWeight = FontWeight.Bold,
              )
              Text("Tap to change", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
            }
          }
        }
          if (showCapacityDialog) {
          CapacityDialog(
            current = state.maxSessions,
            onDismiss = { showCapacityDialog = false },
            onConfirm = { onUpdateCapacity(it); showCapacityDialog = false },
          )
        }
      }

      // Latest/active session card
      val latest = state.latestSession
      if (latest != null) {
        item {
          LatestSessionCard(
            session = latest,
            onClick = { onSessionClick(latest.id) },
          )
        }
      }

      if (state.machineSessions.isNotEmpty()) {
        item { Text("This machine", style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 8.dp)) }
        items(state.machineSessions.take(20), key = { it.id }) { machine ->
          LatestSessionCard(
            session = ServerSession(id = machine.id, cwd = machine.cwd, title = machine.cwd.substringAfterLast('\\').substringAfterLast('/'), status = "Saved"),
            onClick = { onMachineSessionClick(machine.id) },
          )
        }
      }

      if (state.globalSessions.isNotEmpty()) {
        item { Text("Global sessions", style = MaterialTheme.typography.titleSmall, fontWeight = FontWeight.Bold, modifier = Modifier.padding(top = 8.dp)) }
        items(state.globalSessions, key = { it.id }) { global ->
          LatestSessionCard(session = global.session, onClick = { onGlobalSessionClick(global.id) })
        }
      }
    }

    // Scroll-to-top FAB
    AnimatedVisibility(
      visible = showScrollToTop,
      enter = fadeIn() + scaleIn(),
      exit = fadeOut() + scaleOut(),
      modifier = Modifier
        .align(Alignment.BottomEnd)
        .padding(end = 24.dp, bottom = 96.dp),
    ) {
      SmallFloatingActionButton(
        onClick = {
          coroutineScope.launch {
            listState.animateScrollToItem(0)
          }
        },
        containerColor = MaterialTheme.colorScheme.surfaceVariant,
        contentColor = MaterialTheme.colorScheme.onSurfaceVariant,
      ) {
        Icon(Icons.Rounded.KeyboardArrowUp, contentDescription = "Scroll to top")
      }
    }
  }
}

@Composable
private fun HomeErrorContent(
  message: String,
  serverName: String,
  onRetry: () -> Unit,
  onNavigate: (AppRoute) -> Unit,
  onMenuClick: () -> Unit,
  modifier: Modifier = Modifier,
) {
  Box(modifier.fillMaxSize().padding(horizontal = 16.dp)) {
    Column(Modifier.fillMaxSize()) {
      TopAppBarCompact(onMenuClick = onMenuClick, onSettingsClick = { onNavigate(AppRoute.Settings) })
      Spacer(Modifier.height(80.dp))
      Column(
        Modifier.fillMaxWidth(),
        horizontalAlignment = Alignment.CenterHorizontally,
      ) {
        Text(
          "Can't reach $serverName",
          style = MaterialTheme.typography.titleMedium,
          fontWeight = FontWeight.SemiBold,
        )
        Spacer(Modifier.height(4.dp))
        Text(
          message,
          style = MaterialTheme.typography.bodySmall,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(16.dp))
        OutlinedButton(onClick = onRetry) {
          Icon(Icons.Rounded.Refresh, contentDescription = null, modifier = Modifier.size(16.dp))
          Spacer(Modifier.width(6.dp))
          Text("Retry")
        }
      }
    }
  }
}

@Composable
private fun HomeNoServerContent(
  onNavigate: (AppRoute) -> Unit,
  onMenuClick: () -> Unit,
  modifier: Modifier = Modifier,
) {
  Box(modifier.fillMaxSize().padding(horizontal = 16.dp)) {
    Column(Modifier.fillMaxSize()) {
      TopAppBarCompact(onMenuClick = onMenuClick, onSettingsClick = { onNavigate(AppRoute.Settings) })
      Spacer(Modifier.height(80.dp))
      Column(
        Modifier.fillMaxWidth(),
        horizontalAlignment = Alignment.CenterHorizontally,
      ) {
        Text(
          "Welcome to Pi Companion",
          style = MaterialTheme.typography.titleLarge,
          fontWeight = FontWeight.Bold,
        )
        Spacer(Modifier.height(8.dp))
        Text(
          "Add a pi-server in Settings to get started",
          style = MaterialTheme.typography.bodyMedium,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        Spacer(Modifier.height(24.dp))
        OutlinedButton(onClick = { onNavigate(AppRoute.Settings) }) {
          Text("Open Settings")
        }
      }
    }
  }
}

@Composable
private fun LatestSessionCard(
  session: ServerSession,
  onClick: () -> Unit,
  modifier: Modifier = Modifier,
) {
  val title = session.title ?: session.project ?: session.cwd ?: session.id
  val subtitle = session.project ?: session.cwd ?: ""
  val status = session.status ?: "Unknown"

  Surface(
    modifier = modifier
      .fillMaxWidth()
      .clickable(
        interactionSource = remember { MutableInteractionSource() },
        indication = null,
        onClick = onClick,
      ),
    shape = RoundedCornerShape(14.dp),
    color = MaterialTheme.colorScheme.surface,
    border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
  ) {
    Row(
      Modifier.padding(horizontal = 14.dp, vertical = 12.dp),
      verticalAlignment = Alignment.CenterVertically,
      horizontalArrangement = Arrangement.spacedBy(12.dp),
    ) {
      Surface(
        modifier = Modifier.size(38.dp),
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surfaceVariant,
      ) {
        Icon(
          imageVector = Icons.Rounded.Dns,
          contentDescription = null,
          modifier = Modifier.padding(9.dp),
          tint = MaterialTheme.colorScheme.onSurface,
        )
      }
      Column(Modifier.weight(1f), verticalArrangement = androidx.compose.foundation.layout.Arrangement.spacedBy(2.dp)) {
        Text(
          text = title,
          style = MaterialTheme.typography.titleSmall,
          fontWeight = FontWeight.Bold,
          maxLines = 1,
          overflow = TextOverflow.Ellipsis,
        )
        if (subtitle.isNotEmpty()) {
          Text(
            text = subtitle,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
          )
        }
      }
      StatusPill(status)
      Icon(Icons.AutoMirrored.Rounded.KeyboardArrowRight, contentDescription = null, tint = MaterialTheme.colorScheme.onSurfaceVariant)
    }
  }
}

@Composable
private fun CapacityDialog(current: Int, onDismiss: () -> Unit, onConfirm: (Int) -> Unit) {
  var text by remember { mutableStateOf(if (current > 0) "$current" else "") }
  var error by remember { mutableStateOf<String?>(null) }

  AlertDialog(
    onDismissRequest = onDismiss,
    title = { Text("Session limit") },
    text = {
      Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
        Text("Maximum number of concurrent Pi sessions. Set to 0 for unlimited.", style = MaterialTheme.typography.bodySmall)
        OutlinedTextField(
          value = text,
          onValueChange = { text = it; error = null },
          label = { Text("Max sessions") },
          keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Number),
          singleLine = true,
          isError = error != null,
          supportingText = error?.let { { Text(it) } },
          modifier = Modifier.fillMaxWidth(),
        )
      }
    },
    confirmButton = {
      TextButton(onClick = {
        val num = text.toIntOrNull()
        if (num == null || num < 0) {
          error = "Enter 0 or a positive number"
        } else {
          onConfirm(num)
        }
      }) { Text("Save") }
    },
    dismissButton = {
      TextButton(onClick = onDismiss) { Text("Cancel") }
    },
  )
}

@Preview(showBackground = true, widthDp = 360, heightDp = 720)
@Composable
fun HomeContentPreview() {
  PiCompanionTheme(darkTheme = true) {
    HomeContent(
      state = HomeUiState.Content(
        connected = true,
        serverName = "Local Pi",
        sessions = emptyList(),
        workers = emptyList(),
        activeSessions = 0,
        maxSessions = 4,
      ),
      onSessionClick = {},
      onNavigate = {},
      modifier = Modifier.padding(16.dp),
    )
  }
}
