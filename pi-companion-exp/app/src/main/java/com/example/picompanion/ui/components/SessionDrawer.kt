package com.example.picompanion.ui.components

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxHeight
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.statusBarsPadding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.example.picompanion.data.model.ServerSession

@Composable
fun SessionDrawer(
  visible: Boolean,
  onDismiss: () -> Unit,
  onSessionClick: (String) -> Unit,
  onNewSession: () -> Unit,
  sessions: List<ServerSession>,
  isLoading: Boolean = false,
  modifier: Modifier = Modifier,
) {
  if (visible) {
    Box(modifier.fillMaxSize()) {
      // Scrim overlay
      Box(
        modifier = Modifier
          .fillMaxSize()
          .background(MaterialTheme.colorScheme.scrim.copy(alpha = 0.4f))
          .clickable(
            interactionSource = remember { MutableInteractionSource() },
            indication = null,
            onClick = onDismiss,
          ),
      )

      // Drawer panel
      Surface(
        modifier = Modifier
          .fillMaxHeight()
          .width(300.dp)
          .align(Alignment.CenterStart)
          .statusBarsPadding()
          .padding(top = 8.dp, bottom = 16.dp, start = 8.dp),
        shape = RoundedCornerShape(topEnd = 20.dp, bottomEnd = 20.dp),
        color = MaterialTheme.colorScheme.surface,
        border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
        shadowElevation = 16.dp,
      ) {
        Column(Modifier.padding(16.dp)) {
          // Header
          Text(
            text = "Sessions",
            style = MaterialTheme.typography.titleLarge,
            fontWeight = FontWeight.Bold,
          )
          val activeCount = sessions.count {
            it.status?.equals("running", ignoreCase = true) == true ||
              it.status?.equals("active", ignoreCase = true) == true
          }
          Text(
            text = if (isLoading) "Loading…" else "$activeCount active · ${sessions.size} total",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            modifier = Modifier.padding(top = 2.dp),
          )

          HorizontalDivider(
            modifier = Modifier.padding(vertical = 14.dp),
            color = MaterialTheme.colorScheme.outline.copy(alpha = 0.3f),
          )

          // Session list
          if (isLoading) {
            Box(Modifier.fillMaxWidth().padding(40.dp), contentAlignment = Alignment.Center) {
              CircularProgressIndicator()
            }
          } else if (sessions.isEmpty()) {
            Box(Modifier.fillMaxWidth().padding(20.dp), contentAlignment = Alignment.Center) {
              Text(
                "No sessions",
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
              )
            }
          } else {
            Column(
              modifier = Modifier.weight(1f),
              verticalArrangement = Arrangement.spacedBy(4.dp),
            ) {
              sessions.forEach { session ->
                DrawerSessionItem(
                  session = session,
                  onClick = {
                    onSessionClick(session.id)
                    onDismiss()
                  },
                )
              }
            }
          }

          // New session button
          Spacer(Modifier.height(12.dp))
          Button(
            onClick = onNewSession,
            modifier = Modifier.fillMaxWidth(),
            shape = RoundedCornerShape(14.dp),
            colors = ButtonDefaults.buttonColors(
              containerColor = MaterialTheme.colorScheme.primaryContainer,
              contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
            ),
          ) {
            Icon(Icons.Default.Add, contentDescription = null, modifier = Modifier.size(18.dp))
            Spacer(Modifier.width(8.dp))
            Text("New Session")
          }
        }
      }
    }
  }
}

@Composable
private fun DrawerSessionItem(
  session: ServerSession,
  onClick: () -> Unit,
  modifier: Modifier = Modifier,
) {
  val title = session.title ?: session.project ?: session.cwd ?: session.id
  val subtitle = session.project ?: session.cwd ?: ""
  val status = session.status ?: "Unknown"
  val isRunning = status.equals("running", ignoreCase = true) ||
    status.equals("active", ignoreCase = true)

  Surface(
    modifier = modifier
      .fillMaxWidth()
      .clickable(
        interactionSource = remember { MutableInteractionSource() },
        indication = null,
        onClick = onClick,
      ),
    shape = RoundedCornerShape(12.dp),
    color = if (isRunning) MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.3f)
    else MaterialTheme.colorScheme.surface,
  ) {
    Row(
      Modifier.padding(horizontal = 12.dp, vertical = 10.dp),
      horizontalArrangement = Arrangement.spacedBy(10.dp),
      verticalAlignment = Alignment.CenterVertically,
    ) {
      // Icon
      Surface(
        modifier = Modifier.size(34.dp),
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surfaceVariant,
      ) {
        Icon(
          imageVector = Icons.Default.Terminal,
          contentDescription = null,
          modifier = Modifier.padding(7.dp),
          tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }

      // Info
      Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(2.dp)) {
        Text(
          text = title,
          style = MaterialTheme.typography.bodyMedium,
          fontWeight = FontWeight.SemiBold,
          maxLines = 1,
          overflow = TextOverflow.Ellipsis,
        )
        if (subtitle.isNotEmpty()) {
          Text(
            text = subtitle,
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
          )
        }
      }

      // Status
      StatusPill(status)
    }
  }
}
