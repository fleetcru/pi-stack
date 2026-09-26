package com.example.picompanion.ui.sessions

import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.background
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.snapshotFlow
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import kotlinx.coroutines.flow.first
import androidx.lifecycle.viewmodel.compose.viewModel
import androidx.compose.runtime.DisposableEffect
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.ui.components.DirectoryBrowserSheet
import com.example.picompanion.ui.settings.SettingsViewModel

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun SessionsScreen(
  onSessionClick: (String) -> Unit,
  sharedTransitionScope: SharedTransitionScope,
  animatedVisibilityScope: AnimatedVisibilityScope,
  modifier: Modifier = Modifier,
  viewModel: SessionsViewModel = viewModel(),
  settingsViewModel: SettingsViewModel = viewModel(),
) {
  val uiState by viewModel.uiState.collectAsStateWithLifecycle()
  val createdSessionId by viewModel.createdSessionId.collectAsStateWithLifecycle()
  val isCreating by viewModel.isCreating.collectAsStateWithLifecycle()
  val actionError by viewModel.actionError.collectAsStateWithLifecycle()
  val settings by settingsViewModel.settings.collectAsStateWithLifecycle()
  var searchQuery by remember { mutableStateOf("") }
  var showBrowser by remember { mutableStateOf(false) }
  var actionSession by remember { mutableStateOf<ServerSession?>(null) }
  val activeListState = rememberLazyListState()
  val machineListState = rememberLazyListState()
  val globalListState = rememberLazyListState()

  val lifecycleOwner = LocalLifecycleOwner.current
  DisposableEffect(lifecycleOwner, viewModel) {
    val observer = LifecycleEventObserver { _, event ->
      if (event == Lifecycle.Event.ON_RESUME) viewModel.refreshIfStale()
    }
    lifecycleOwner.lifecycle.addObserver(observer)
    onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
  }

  // Sessions created from the directory picker still navigate through state.
  // Machine/global opens use their request callback directly below so two quick
  // taps cannot overwrite a single shared createdSessionId value.
  LaunchedEffect(createdSessionId) {
    val id = createdSessionId?.takeIf { it.isNotBlank() } ?: return@LaunchedEffect
    onSessionClick(id)
    viewModel.clearCreatedSession()
  }

  // Surface open/attach failures instead of silently refreshing
  actionError?.let { message ->
    AlertDialog(
      onDismissRequest = { viewModel.clearActionError() },
      title = { Text("Session unavailable") },
      text = { Text(message) },
      confirmButton = {
        TextButton(onClick = { viewModel.clearActionError() }) { Text("OK") }
      },
    )
  }

  Column(
    modifier
      .fillMaxSize()
      .padding(horizontal = 18.dp),
  ) {
    // Header with refresh
    Row(
      Modifier
        .fillMaxWidth()
        .padding(start = 4.dp, top = 28.dp, bottom = 16.dp),
      horizontalArrangement = Arrangement.SpaceBetween,
      verticalAlignment = Alignment.CenterVertically,
    ) {
      Column {
        Text(
          text = "Sessions",
          style = MaterialTheme.typography.headlineMedium,
          fontWeight = FontWeight.Bold,
        )
        Text(
          text = "Local and remote Pi workspaces",
          style = MaterialTheme.typography.bodyMedium,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }
      IconButton(
        onClick = { viewModel.refresh(force = true) },
        modifier = Modifier
          .size(36.dp)
          .background(
            MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.5f),
            shape = CircleShape,
          ),
      ) {
        Icon(
          Icons.Default.Refresh,
          contentDescription = "Refresh",
          modifier = Modifier.size(18.dp),
          tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }
    }

    // New session button
    Button(
      onClick = { showBrowser = true },
      modifier = Modifier
        .fillMaxWidth()
        .padding(bottom = 8.dp),
      shape = RoundedCornerShape(12.dp),
    ) {
      Icon(Icons.Default.Add, contentDescription = null, modifier = Modifier.size(18.dp))
      Spacer(Modifier.width(8.dp))
      Text("New Session")
    }

    // Search bar
    OutlinedTextField(
      value = searchQuery,
      onValueChange = { searchQuery = it },
      modifier = Modifier
        .fillMaxWidth()
        .padding(bottom = 4.dp),
      placeholder = { Text("Search sessions…") },
      leadingIcon = {
        Icon(Icons.Default.Search, contentDescription = null)
      },
      shape = RoundedCornerShape(14.dp),
      singleLine = true,
      colors = OutlinedTextFieldDefaults.colors(
        unfocusedBorderColor = MaterialTheme.colorScheme.outline,
      ),
    )

    // Content
    when (val state = uiState) {
      is SessionsUiState.Loading -> {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
          CircularProgressIndicator()
        }
      }

      is SessionsUiState.Empty -> {
        Box(Modifier.fillMaxSize().padding(top = 80.dp), contentAlignment = Alignment.TopCenter) {
          Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(
              "No server connected",
              style = MaterialTheme.typography.titleMedium,
              fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(4.dp))
            Text(
              "Connect a Pi server in Settings, then tap + to create a session.",
              style = MaterialTheme.typography.bodySmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
              textAlign = TextAlign.Center,
            )
          }
        }
      }

      is SessionsUiState.Error -> {
        Box(Modifier.fillMaxSize().padding(top = 80.dp), contentAlignment = Alignment.TopCenter) {
          Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(
              "Failed to load sessions",
              style = MaterialTheme.typography.titleMedium,
              fontWeight = FontWeight.SemiBold,
              color = MaterialTheme.colorScheme.error,
            )
            Spacer(Modifier.height(4.dp))
            Text(
              state.message,
              style = MaterialTheme.typography.bodySmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
              textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(16.dp))
            OutlinedButton(onClick = { viewModel.refresh(force = true) }) {
              Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(16.dp))
              Spacer(Modifier.width(6.dp))
              Text("Retry")
            }
          }
        }
      }

      is SessionsUiState.Content -> {
        SessionsScreenContent(
          state = state,
          viewModel = viewModel,
          searchQuery = searchQuery,
          onSessionClick = onSessionClick,
          onLongClickSession = { actionSession = it },
          sharedTransitionScope = sharedTransitionScope,
          animatedVisibilityScope = animatedVisibilityScope,
          activeListState = activeListState,
          machineListState = machineListState,
          globalListState = globalListState,
        )
      }
    }
  }

  actionSession?.let { session ->
    AlertDialog(
      onDismissRequest = { actionSession = null },
      title = { Text(session.title ?: "Session actions") },
      text = { Text("Choose an action for this session. Deleting stops its Pi process and removes it from pi-server.") },
      confirmButton = {
        TextButton(
          onClick = {
            actionSession = null
            viewModel.deleteSession(session.id)
          },
        ) { Text("Delete", color = MaterialTheme.colorScheme.error) }
      },
      dismissButton = {
        TextButton(onClick = { actionSession = null }) { Text("Cancel") }
      },
    )
  }

  // Directory browser sheet
  DirectoryBrowserSheet(
    visible = showBrowser,
    server = settings.activeServer,
    isCreating = isCreating,
    onDismiss = { if (!isCreating) showBrowser = false },
    onSelect = { selection ->
      viewModel.createSession(
        cwd = selection.cwd,
        prompt = selection.prompt,
        count = selection.count,
        title = selection.title,
        createWorktree = selection.createWorktree,
        workerId = selection.workerId,
      )
    },
  )

  // Keep the sheet open while creating, then close when the ViewModel finishes.
  LaunchedEffect(isCreating) {
    if (!isCreating) return@LaunchedEffect
    snapshotFlow { isCreating }.first { !it }
    showBrowser = false
  }
}
