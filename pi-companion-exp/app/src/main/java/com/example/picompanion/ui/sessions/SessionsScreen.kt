package com.example.picompanion.ui.sessions

import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.interaction.MutableInteractionSource
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
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Computer
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.FilterChip
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.ui.components.DirectoryBrowserSheet
import com.example.picompanion.ui.components.StatusPill
import com.example.picompanion.ui.settings.SettingsViewModel

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
  val actionError by viewModel.actionError.collectAsStateWithLifecycle()
  val settings by settingsViewModel.settings.collectAsStateWithLifecycle()
  var searchQuery by remember { mutableStateOf("") }
  var showBrowser by remember { mutableStateOf(false) }
  var actionSession by remember { mutableStateOf<ServerSession?>(null) }

  // Auto-navigate when a session is created
  LaunchedEffect(createdSessionId) {
    val id = createdSessionId
    if (id != null) {
      onSessionClick(id)
      viewModel.clearCreatedSession()
    }
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
      IconButton(onClick = { viewModel.refresh() }) {
        Icon(Icons.Default.Refresh, contentDescription = "Refresh")
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
              "No sessions",
              style = MaterialTheme.typography.titleMedium,
              fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(4.dp))
            Text(
              "Create a session from pi-server to get started",
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
            OutlinedButton(onClick = { viewModel.refresh() }) {
              Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(16.dp))
              Spacer(Modifier.width(6.dp))
              Text("Retry")
            }
          }
        }
      }

      is SessionsUiState.Content -> {
        val selectedTab by viewModel.selectedTab.collectAsStateWithLifecycle()

        // Tab row
        Row(
          Modifier
            .fillMaxWidth()
            .padding(bottom = 4.dp),
          horizontalArrangement = Arrangement.spacedBy(8.dp),
        ) {
          FilterChip(
            selected = selectedTab == SessionTab.Active,
            onClick = { viewModel.selectTab(SessionTab.Active) },
            label = { Text("Active (${state.activeSessions.size})") },
          )
          FilterChip(
            selected = selectedTab == SessionTab.Machine,
            onClick = { viewModel.selectTab(SessionTab.Machine) },
            label = { Text("Local (${state.machineSessions.size})") },
          )
          if (state.globalSessions.isNotEmpty()) {
            FilterChip(
              selected = selectedTab == SessionTab.Global,
              onClick = { viewModel.selectTab(SessionTab.Global) },
              label = { Text("Global (${state.globalSessions.size})") },
            )
          }
        }

        when (selectedTab) {
          SessionTab.Active -> {
            val filteredSessions = remember(searchQuery, state.activeSessions) {
              if (searchQuery.isBlank()) state.activeSessions
              else state.activeSessions.filter {
                (it.title?.contains(searchQuery, ignoreCase = true) == true) ||
                  (it.project?.contains(searchQuery, ignoreCase = true) == true) ||
                  (it.status?.contains(searchQuery, ignoreCase = true) == true) ||
                  it.id.contains(searchQuery, ignoreCase = true)
              }
            }

            if (filteredSessions.isEmpty()) {
              EmptySessionsMessage(
                title = if (searchQuery.isNotBlank()) "No matches for \"$searchQuery\"" else "No active sessions",
                subtitle = if (searchQuery.isNotBlank()) null else "Sessions created from the web or companion app appear here",
              )
            } else {
              LazyColumn(
                modifier = Modifier
                  .fillMaxWidth()
                  .weight(1f)
                  .padding(top = 14.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp),
              ) {
                items(filteredSessions, key = { it.id }, contentType = { "session_item" }) { session ->
                  SessionListItem(
                    session = session,
                    isSelected = false,
                    onClick = { onSessionClick(session.id) },
                    onLongClick = { actionSession = session },
                    sharedTransitionScope = sharedTransitionScope,
                    animatedVisibilityScope = animatedVisibilityScope,
                  )
                }
              }
            }
          }

          SessionTab.Machine -> {
            val filteredMachine = remember(searchQuery, state.machineSessions) {
              if (searchQuery.isBlank()) state.machineSessions
              else state.machineSessions.filter {
                it.id.contains(searchQuery, ignoreCase = true) ||
                  it.cwd.contains(searchQuery, ignoreCase = true)
              }
            }

            if (filteredMachine.isEmpty()) {
              EmptySessionsMessage(
                title = if (searchQuery.isNotBlank()) "No matches for \"$searchQuery\"" else "No local sessions",
                subtitle = if (searchQuery.isNotBlank()) null else "Sessions from ~/.pi/agent/sessions/ appear here",
              )
            } else {
              LazyColumn(
                modifier = Modifier
                  .fillMaxWidth()
                  .weight(1f)
                  .padding(top = 14.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp),
              ) {
                items(filteredMachine, key = { it.id }, contentType = { "machine_item" }) { session ->
                  MachineSessionListItem(
                    session = session,
                    onClick = { viewModel.openMachineSession(session.id) },
                  )
                }
              }
            }
          }

          SessionTab.Global -> {
            val filteredGlobal = remember(searchQuery, state.globalSessions) {
              if (searchQuery.isBlank()) state.globalSessions
              else state.globalSessions.filter {
                it.session.title?.contains(searchQuery, ignoreCase = true) == true ||
                  it.session.project?.contains(searchQuery, ignoreCase = true) == true ||
                  it.workerId.contains(searchQuery, ignoreCase = true) ||
                  it.originId.contains(searchQuery, ignoreCase = true)
              }
            }

            if (filteredGlobal.isEmpty()) {
              EmptySessionsMessage(
                title = if (searchQuery.isNotBlank()) "No matches for \"$searchQuery\"" else "No global sessions",
                subtitle = if (searchQuery.isNotBlank()) null else "Sessions from remote workers appear here",
              )
            } else {
              LazyColumn(
                modifier = Modifier
                  .fillMaxWidth()
                  .weight(1f)
                  .padding(top = 14.dp),
                verticalArrangement = Arrangement.spacedBy(14.dp),
              ) {
                items(filteredGlobal, key = { it.id }, contentType = { "global_item" }) { session ->
                  GlobalSessionListItem(
                    session = session,
                    onClick = { viewModel.attachGlobalSession(session.id) },
                  )
                }
              }
            }
          }
        }
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
    onDismiss = { showBrowser = false },
    onSelect = { cwd ->
      showBrowser = false
      viewModel.createSession(cwd)
    },
  )
}

@Composable
private fun EmptySessionsMessage(
  title: String,
  subtitle: String?,
  modifier: Modifier = Modifier,
) {
  Box(
    modifier.fillMaxWidth().padding(top = 40.dp),
    contentAlignment = Alignment.Center,
  ) {
    Column(horizontalAlignment = Alignment.CenterHorizontally) {
      Text(
        title,
        style = MaterialTheme.typography.titleMedium,
        fontWeight = FontWeight.SemiBold,
      )
      if (subtitle != null) {
        Spacer(Modifier.height(4.dp))
        Text(
          subtitle,
          style = MaterialTheme.typography.bodySmall,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
          textAlign = TextAlign.Center,
        )
      }
    }
  }
}

@Composable
private fun MachineSessionListItem(
  session: com.example.picompanion.data.model.MachineSession,
  onClick: () -> Unit,
  modifier: Modifier = Modifier,
) {
  val sessionShape = RoundedCornerShape(16.dp)
  Surface(
    modifier = modifier
      .fillMaxWidth()
      .combinedClickable(
        interactionSource = remember { MutableInteractionSource() },
        indication = null,
        onClick = onClick,
      ),
    shape = sessionShape,
    color = MaterialTheme.colorScheme.surface,
    border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
  ) {
    Row(
      Modifier.padding(horizontal = 16.dp, vertical = 15.dp),
      horizontalArrangement = Arrangement.spacedBy(12.dp),
      verticalAlignment = Alignment.Top,
    ) {
      Surface(
        modifier = Modifier.size(40.dp),
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surfaceVariant,
      ) {
        Icon(
          imageVector = Icons.Default.Computer,
          contentDescription = null,
          modifier = Modifier.padding(8.dp),
          tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }

      Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
          text = session.id,
          style = MaterialTheme.typography.titleSmall,
          fontWeight = FontWeight.Bold,
          maxLines = 1,
          overflow = TextOverflow.Ellipsis,
        )
        Text(
          text = session.cwd,
          style = MaterialTheme.typography.bodySmall,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
          maxLines = 1,
          overflow = TextOverflow.Ellipsis,
        )
        if (!session.updatedAt.isNullOrBlank()) {
          Text(
            text = session.updatedAt,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.outline,
          )
        }
      }

      StatusPill("local")
    }
  }
}

@Composable
private fun GlobalSessionListItem(
  session: com.example.picompanion.data.model.GlobalSession,
  onClick: () -> Unit,
  modifier: Modifier = Modifier,
) {
  val sessionShape = RoundedCornerShape(16.dp)
  val displayTitle = session.session.title ?: session.session.project ?: session.originId
  Surface(
    modifier = modifier
      .fillMaxWidth()
      .combinedClickable(
        interactionSource = remember { MutableInteractionSource() },
        indication = null,
        onClick = onClick,
      ),
    shape = sessionShape,
    color = MaterialTheme.colorScheme.surface,
    border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
  ) {
    Row(
      Modifier.padding(horizontal = 16.dp, vertical = 15.dp),
      horizontalArrangement = Arrangement.spacedBy(12.dp),
      verticalAlignment = Alignment.Top,
    ) {
      Surface(
        modifier = Modifier.size(40.dp),
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surfaceVariant,
      ) {
        Icon(
          imageVector = Icons.Default.Computer,
          contentDescription = null,
          modifier = Modifier.padding(8.dp),
          tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }

      Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
        Text(
          text = displayTitle,
          style = MaterialTheme.typography.titleSmall,
          fontWeight = FontWeight.Bold,
          maxLines = 1,
          overflow = TextOverflow.Ellipsis,
        )
        Text(
          text = "Worker: ${session.workerId}",
          style = MaterialTheme.typography.bodySmall,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
          maxLines = 1,
          overflow = TextOverflow.Ellipsis,
        )
        if (!session.session.updatedAt.isNullOrBlank()) {
          Text(
            text = session.session.updatedAt,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.outline,
          )
        }
      }

      StatusPill(if (session.reachable) "remote" else "offline")
    }
  }
}
