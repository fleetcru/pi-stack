package com.example.picompanion.ui.sessiondetail

import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Compress
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.MoreVert
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
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
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

@Composable
fun SessionHeader(
  sessionId: String,
  onBack: () -> Unit,
  connectionState: ConnectionState,
  relayHealth: RelayHealth?,
  onReconnect: () -> Unit,
  onCompact: () -> Unit,
  onControls: () -> Unit,
  onFiles: () -> Unit,
  onModelControls: () -> Unit,
  sharedTransitionScope: SharedTransitionScope,
  animatedVisibilityScope: AnimatedVisibilityScope,
  modifier: Modifier = Modifier,
) {
  val headerShape = RoundedCornerShape(18.dp)
  var menuExpanded by remember { mutableStateOf(false) }

  with(sharedTransitionScope) {
    Surface(
      modifier = modifier
        .fillMaxWidth()
        .padding(horizontal = 12.dp, vertical = 10.dp)
        .sharedBounds(
          sharedContentState = rememberSharedContentState(key = "session-$sessionId"),
          animatedVisibilityScope = animatedVisibilityScope,
          clipInOverlayDuringTransition = OverlayClip(headerShape),
        ),
      shape = headerShape,
      color = MaterialTheme.colorScheme.surface,
      border = BorderStroke(1.dp, MaterialTheme.colorScheme.outlineVariant),
    ) {
      Column(
        modifier = Modifier.padding(start = 4.dp, end = 12.dp, top = 8.dp, bottom = 10.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
      ) {
        Row(
          Modifier.fillMaxWidth(),
          horizontalArrangement = Arrangement.spacedBy(6.dp),
          verticalAlignment = Alignment.CenterVertically,
        ) {
          IconButton(onClick = onBack) {
            Icon(
              imageVector = Icons.AutoMirrored.Filled.ArrowBack,
              contentDescription = "Back to sessions",
              tint = MaterialTheme.colorScheme.onSurface,
            )
          }

          Text(
            text = "Session $sessionId",
            style = MaterialTheme.typography.titleMedium,
            fontWeight = FontWeight.Bold,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f),
          )

          // Connection state indicator
          ConnectionIndicator(connectionState)

          if (connectionState is ConnectionState.Connected) {
            IconButton(onClick = onFiles, modifier = Modifier.size(40.dp)) {
              Icon(Icons.Default.Folder, contentDescription = "Browse files", modifier = Modifier.size(20.dp))
            }
            Box {
              IconButton(onClick = { menuExpanded = true }, modifier = Modifier.size(40.dp)) {
                Icon(Icons.Default.MoreVert, contentDescription = "More session actions", modifier = Modifier.size(22.dp))
              }
              DropdownMenu(expanded = menuExpanded, onDismissRequest = { menuExpanded = false }) {
                DropdownMenuItem(
                  text = { Text("Model & effort") },
                  onClick = { menuExpanded = false; onModelControls() },
                )
                DropdownMenuItem(
                  text = { Text("Compact session") },
                  leadingIcon = { Icon(Icons.Default.Compress, contentDescription = null) },
                  onClick = { menuExpanded = false; onCompact() },
                )
                DropdownMenuItem(
                  text = { Text("Session controls") },
                  leadingIcon = { Icon(Icons.Default.Settings, contentDescription = null) },
                  onClick = { menuExpanded = false; onControls() },
                )
              }
            }
          }

          if (connectionState is ConnectionState.Error || connectionState is ConnectionState.Disconnected) {
            IconButton(onClick = onReconnect, modifier = Modifier.size(40.dp)) {
              Icon(Icons.Default.Refresh, contentDescription = "Reconnect", modifier = Modifier.size(20.dp))
            }
          }
        }

        Text(
          text = connectionStatusText(connectionState, relayHealth),
          style = MaterialTheme.typography.labelSmall,
          color = connectionStatusColor(connectionState, relayHealth),
          maxLines = 1,
          overflow = TextOverflow.Ellipsis,
          modifier = Modifier.padding(start = 54.dp, end = 4.dp),
        )
      }
    }
  }
}

@Composable
private fun connectionStatusText(state: ConnectionState, relayHealth: RelayHealth?): String = when (state) {
  ConnectionState.Connecting -> "Connecting…"
  ConnectionState.Connected -> when {
    relayHealth == null -> "Connected"
    relayHealth.connected && relayHealth.latencyMs != null -> "Connected · Relay ${relayHealth.latencyMs} ms"
    relayHealth.connected -> "Connected · Relay connected"
    else -> "Connected · Relay disconnected — commands queue on server"
  }
  is ConnectionState.Disconnected -> "Disconnected: ${state.reason}"
  is ConnectionState.Error -> "Error: ${state.message}"
}

@Composable
private fun connectionStatusColor(state: ConnectionState, relayHealth: RelayHealth?) = when {
  state is ConnectionState.Connected && relayHealth?.connected == false -> MaterialTheme.colorScheme.tertiary
  state is ConnectionState.Connected -> MaterialTheme.colorScheme.primary
  state is ConnectionState.Connecting -> MaterialTheme.colorScheme.onSurfaceVariant
  else -> MaterialTheme.colorScheme.error
}

@Composable
private fun ConnectionIndicator(state: ConnectionState) {
  when (state) {
    ConnectionState.Connecting -> {
      CircularProgressIndicator(modifier = Modifier.size(16.dp), strokeWidth = 2.dp)
    }
    ConnectionState.Connected -> {
      Surface(
        modifier = Modifier.size(10.dp),
        shape = RoundedCornerShape(5.dp),
        color = MaterialTheme.colorScheme.primary,
      ) {}
    }
    else -> {
      Surface(
        modifier = Modifier.size(10.dp),
        shape = RoundedCornerShape(5.dp),
        color = MaterialTheme.colorScheme.error,
      ) {}
    }
  }
}
