package com.example.picompanion.ui.settings

import androidx.compose.animation.AnimatedVisibility
import androidx.compose.animation.expandVertically
import androidx.compose.animation.shrinkVertically
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.NetworkCheck
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.IconButtonDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.ImeAction
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.text.input.VisualTransformation
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.picompanion.data.settings.ServerEntry

@Composable
fun SettingsScreen(
  darkTheme: Boolean,
  onDarkThemeChange: (Boolean) -> Unit,
  modifier: Modifier = Modifier,
  viewModel: SettingsViewModel = viewModel(),
) {
  val settings by viewModel.settings.collectAsStateWithLifecycle()
  val connectionResults by viewModel.connectionResults.collectAsStateWithLifecycle()

  Column(
    modifier
      .fillMaxSize()
      .verticalScroll(rememberScrollState())
      .imePadding()
      .padding(horizontal = 18.dp),
    verticalArrangement = Arrangement.spacedBy(14.dp),
  ) {
    // Page header
    Column(Modifier.padding(start = 4.dp, top = 28.dp, bottom = 4.dp)) {
      Text(
        text = "Settings",
        style = MaterialTheme.typography.headlineMedium,
        fontWeight = FontWeight.Bold,
      )
      Text(
        text = "Configure your Pi Companion",
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
      )
    }

    // Servers
    SettingsSection(title = "Servers") {
      settings.servers.forEach { server ->
        ServerCard(
          server = server,
          isActive = server.id == settings.activeServerId,
          connectionTest = connectionResults[server.id] ?: ConnectionTestResult.Idle,
          onSetActive = { viewModel.setActiveServer(server.id) },
          onUpdate = { updated ->
            viewModel.updateServer(updated)
            viewModel.testConnection(updated)
          },
          onTest = { viewModel.testConnection(server) },
          onRemove = if (settings.servers.size > 1) {
            { viewModel.removeServer(server.id) }
          } else null,
        )
        Spacer(Modifier.height(8.dp))
      }
      OutlinedButton(
        onClick = { viewModel.addServer() },
        modifier = Modifier.fillMaxWidth(),
        shape = RoundedCornerShape(12.dp),
      ) {
        Icon(Icons.Default.Add, contentDescription = null, modifier = Modifier.size(18.dp))
        Spacer(Modifier.width(8.dp))
        Text("Add Server")
      }
    }

    // App behavior
    SettingsSection(title = "App behavior") {
      SettingsEditableRow(
        label = "Default project root",
        value = settings.defaultProjectRoot,
        onValueChange = viewModel::updateDefaultProjectRoot,
        placeholder = "C:/Users/you/projects",
      )
      SettingsToggleRow(
        label = "Reconnect WebSocket on launch",
        checked = settings.reconnectOnLaunch,
        onCheckedChange = viewModel::updateReconnectOnLaunch,
      )
      SettingsToggleRow(
        label = "Remember last selected session",
        checked = settings.rememberLastSession,
        onCheckedChange = viewModel::updateRememberLastSession,
      )
      SettingsToggleRow(
        label = "Dark theme",
        checked = darkTheme,
        onCheckedChange = onDarkThemeChange,
      )
    }

    // Session behavior
    SettingsSection(title = "Session behavior") {
      SettingsToggleRow(
        label = "Replay events since last seen",
        checked = settings.replayEventsSinceLastSeen,
        onCheckedChange = viewModel::updateReplayEvents,
      )
      SettingsToggleRow(
        label = "Show file change events",
        checked = settings.showFileChangeEvents,
        onCheckedChange = viewModel::updateShowFileChanges,
      )
      SettingsToggleRow(
        label = "Show tool events",
        checked = settings.showToolEvents,
        onCheckedChange = viewModel::updateShowToolEvents,
      )
      SettingsToggleRow(
        label = "Show daemon events",
        checked = settings.showDaemonEvents,
        onCheckedChange = viewModel::updateShowDaemonEvents,
      )
    }

    // About
    SettingsSection(title = "About") {
      SettingsRow(label = "App version", value = "0.1 debug")
      SettingsRow(label = "API target", value = "pi-server /v1")
      SettingsRow(label = "Build type", value = "Debug")
    }

    // Bottom spacer
    Spacer(Modifier.height(120.dp))
  }
}

@Composable
private fun ServerCard(
  server: ServerEntry,
  isActive: Boolean,
  connectionTest: ConnectionTestResult,
  onSetActive: () -> Unit,
  onUpdate: (ServerEntry) -> Unit,
  onTest: () -> Unit,
  onRemove: (() -> Unit)?,
  modifier: Modifier = Modifier,
) {
  var expanded by remember(server.id) { mutableStateOf(server.name.isBlank() || server.url.isBlank()) }
  var editName by remember(server.id) { mutableStateOf(server.name) }
  var editUrl by remember(server.id) { mutableStateOf(server.url) }
  var editToken by remember(server.id) { mutableStateOf(server.authToken) }

  Surface(
    modifier = modifier.fillMaxWidth(),
    shape = RoundedCornerShape(14.dp),
    color = if (isActive) MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.25f)
    else MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.5f),
    border = BorderStroke(
      1.dp,
      if (isActive) MaterialTheme.colorScheme.primary.copy(alpha = 0.5f)
      else MaterialTheme.colorScheme.outline.copy(alpha = 0.3f),
    ),
  ) {
    Column(Modifier.padding(14.dp)) {
      // ── Collapsed summary row ──
      Row(
        Modifier
          .fillMaxWidth()
          .clickable(
            interactionSource = remember { MutableInteractionSource() },
            indication = null,
            onClick = { expanded = !expanded },
          ),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.CenterVertically,
      ) {
        // Status dot
        StatusDot(connectionTest)

        // Name + URL
        Column(Modifier.weight(1f)) {
          Text(
            text = server.name.ifBlank { "Unnamed" },
            style = MaterialTheme.typography.bodyMedium,
            fontWeight = FontWeight.SemiBold,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
          )
          Text(
            text = server.url,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
          )
        }

        // Active badge
        if (isActive) {
          Text(
            "Active",
            style = MaterialTheme.typography.labelSmall,
            fontWeight = FontWeight.SemiBold,
            color = MaterialTheme.colorScheme.primary,
          )
        }

        // Edit button
        IconButton(onClick = { expanded = !expanded }, modifier = Modifier.size(32.dp)) {
          Icon(
            Icons.Default.Edit,
            contentDescription = "Edit",
            modifier = Modifier.size(16.dp),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
          )
        }

        // Remove button
        if (onRemove != null) {
          IconButton(onClick = onRemove, modifier = Modifier.size(32.dp)) {
            Icon(
              Icons.Default.Delete,
              contentDescription = "Remove",
              modifier = Modifier.size(16.dp),
              tint = MaterialTheme.colorScheme.error.copy(alpha = 0.7f),
            )
          }
        }
      }

      // Connection status line
      if (!expanded && connectionTest !is ConnectionTestResult.Idle) {
        Spacer(Modifier.height(6.dp))
        ConnectionStatusLine(connectionTest)
      }

      // ── Expanded edit form ──
      AnimatedVisibility(
        visible = expanded,
        enter = expandVertically(),
        exit = shrinkVertically(),
      ) {
        Column(
          Modifier.padding(top = 14.dp),
          verticalArrangement = Arrangement.spacedBy(10.dp),
        ) {
          ServerTextField(
            label = "Name",
            value = editName,
            onValueChange = { editName = it },
            placeholder = "e.g. Local Pi, Office Server",
            imeAction = ImeAction.Next,
          )

          ServerTextField(
            label = "URL",
            value = editUrl,
            onValueChange = { editUrl = it },
            placeholder = "http://127.0.0.1:3141",
            imeAction = ImeAction.Next,
            keyboardType = KeyboardType.Uri,
          )

          ServerTextField(
            label = "Auth token",
            value = editToken,
            onValueChange = { editToken = it },
            placeholder = "Optional",
            masked = true,
            imeAction = ImeAction.Done,
          )

          // Action buttons
          Row(
            Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.spacedBy(10.dp),
          ) {
            // Set active (if not active)
            if (!isActive) {
              OutlinedButton(
                onClick = onSetActive,
                modifier = Modifier.weight(1f),
                shape = RoundedCornerShape(10.dp),
              ) {
                Text("Set active", style = MaterialTheme.typography.labelMedium)
              }
            }

            // Save + test
            Button(
              onClick = {
                onUpdate(
                  server.copy(
                    name = editName,
                    url = editUrl.trimEnd('/'),
                    authToken = editToken,
                  )
                )
                expanded = false
              },
              modifier = Modifier.weight(1f),
              shape = RoundedCornerShape(10.dp),
              colors = ButtonDefaults.buttonColors(
                containerColor = if (isActive) MaterialTheme.colorScheme.primary
                else MaterialTheme.colorScheme.secondaryContainer,
                contentColor = if (isActive) MaterialTheme.colorScheme.onPrimary
                else MaterialTheme.colorScheme.onSecondaryContainer,
              ),
            ) {
              Text("Save & Test")
            }
          }

          // Connection result in expanded mode
          if (connectionTest !is ConnectionTestResult.Idle) {
            ConnectionStatusLine(connectionTest)
          }
        }
      }
    }
  }
}

@Composable
private fun StatusDot(result: ConnectionTestResult) {
  val color = when (result) {
    ConnectionTestResult.Idle -> MaterialTheme.colorScheme.outline
    ConnectionTestResult.Testing -> MaterialTheme.colorScheme.secondary
    is ConnectionTestResult.Success -> MaterialTheme.colorScheme.primary
    is ConnectionTestResult.Error -> MaterialTheme.colorScheme.error
  }
  val icon = when (result) {
    ConnectionTestResult.Testing -> null
    is ConnectionTestResult.Success -> Icons.Default.CheckCircle
    is ConnectionTestResult.Error -> Icons.Default.Error
    else -> null
  }

  if (result is ConnectionTestResult.Testing) {
    CircularProgressIndicator(modifier = Modifier.size(20.dp), strokeWidth = 2.dp, color = color)
  } else if (icon != null) {
    Icon(icon, contentDescription = null, modifier = Modifier.size(20.dp), tint = color)
  } else {
    Surface(modifier = Modifier.size(10.dp), shape = CircleShape, color = color) {}
  }
}

@Composable
private fun ConnectionStatusLine(result: ConnectionTestResult) {
  when (result) {
    is ConnectionTestResult.Success -> Text(
      "Connected — ${result.sessions} sessions, capacity ${result.capacity}",
      style = MaterialTheme.typography.bodySmall,
      color = MaterialTheme.colorScheme.primary,
    )
    is ConnectionTestResult.Error -> Text(
      "Failed: ${result.message}",
      style = MaterialTheme.typography.bodySmall,
      color = MaterialTheme.colorScheme.error,
    )
    ConnectionTestResult.Testing -> Text(
      "Testing connection…",
      style = MaterialTheme.typography.bodySmall,
      color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    else -> {}
  }
}

@Composable
private fun ServerTextField(
  label: String,
  value: String,
  onValueChange: (String) -> Unit,
  modifier: Modifier = Modifier,
  placeholder: String = "",
  masked: Boolean = false,
  imeAction: ImeAction = ImeAction.Done,
  keyboardType: KeyboardType = KeyboardType.Text,
) {
  Column(modifier.fillMaxWidth(), verticalArrangement = Arrangement.spacedBy(3.dp)) {
    Text(
      text = label,
      style = MaterialTheme.typography.labelSmall,
      color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    OutlinedTextField(
      value = value,
      onValueChange = onValueChange,
      modifier = Modifier.fillMaxWidth(),
      shape = RoundedCornerShape(10.dp),
      singleLine = true,
      textStyle = MaterialTheme.typography.bodyMedium,
      placeholder = {
        Text(placeholder, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.outline)
      },
      visualTransformation = if (masked) PasswordVisualTransformation() else VisualTransformation.None,
      keyboardOptions = KeyboardOptions(imeAction = imeAction, keyboardType = keyboardType),
      colors = OutlinedTextFieldDefaults.colors(
        focusedBorderColor = MaterialTheme.colorScheme.primary,
        unfocusedBorderColor = MaterialTheme.colorScheme.outline,
        cursorColor = MaterialTheme.colorScheme.primary,
      ),
    )
  }
}
