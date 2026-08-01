package com.example.picompanion.ui.sessiondetail

import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Settings
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp

/**
 * Simplified session header. The overflow menu is removed — actions are now
 * accessed via the unified actions sheet triggered by the settings icon.
 */
@Composable
fun SessionHeader(
  sessionId: String,
  onBack: () -> Unit,
  connectionState: ConnectionState,
  relayHealth: RelayHealth?,
  refreshing: Boolean,
  onRefresh: () -> Unit,
  onCompact: () -> Unit,
  onControls: () -> Unit,
  onFiles: () -> Unit,
  onModelControls: () -> Unit,
  sharedTransitionScope: SharedTransitionScope,
  animatedVisibilityScope: AnimatedVisibilityScope,
  modifier: Modifier = Modifier,
) {
  val headerShape = RoundedCornerShape(18.dp)

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
        modifier = Modifier.padding(start = 4.dp, end = 8.dp, top = 8.dp, bottom = 10.dp),
        verticalArrangement = Arrangement.spacedBy(2.dp),
      ) {
        Row(
          Modifier.fillMaxWidth(),
          horizontalArrangement = Arrangement.spacedBy(4.dp),
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

          ConnectionIndicator(connectionState)

          IconButton(onClick = onRefresh, enabled = !refreshing, modifier = Modifier.size(38.dp)) {
            if (refreshing) {
              CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp)
            } else {
              Icon(Icons.Default.Refresh, contentDescription = "Refresh session", modifier = Modifier.size(20.dp))
            }
          }

          if (connectionState is ConnectionState.Connected) {
            IconButton(onClick = onFiles, modifier = Modifier.size(38.dp)) {
              Icon(Icons.Default.Folder, contentDescription = "Browse files", modifier = Modifier.size(20.dp))
            }
            IconButton(onClick = onControls, modifier = Modifier.size(38.dp)) {
              Icon(Icons.Default.Settings, contentDescription = "Session actions", modifier = Modifier.size(20.dp))
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
      Surface(modifier = Modifier.size(10.dp), shape = RoundedCornerShape(5.dp), color = MaterialTheme.colorScheme.primary) {}
    }
    else -> {
      Surface(modifier = Modifier.size(10.dp), shape = RoundedCornerShape(5.dp), color = MaterialTheme.colorScheme.error) {}
    }
  }
}
