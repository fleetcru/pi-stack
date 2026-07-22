package com.example.picompanion.ui.sessions

import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.combinedClickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Terminal
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
import com.example.picompanion.ui.components.StatusPill

@Composable
fun SessionListItem(
  session: ServerSession,
  isSelected: Boolean,
  onClick: () -> Unit,
  onLongClick: () -> Unit,
  sharedTransitionScope: SharedTransitionScope,
  animatedVisibilityScope: AnimatedVisibilityScope,
  modifier: Modifier = Modifier,
) {
  val sessionShape = RoundedCornerShape(16.dp)
  val displayTitle = session.title ?: session.project ?: session.cwd ?: session.id
  val displayProject = session.project ?: session.cwd ?: ""
  // Extract granular runtime state from session.state if available
  val runtimeState = (session.state?.get("runtimeStatus") as? kotlinx.serialization.json.JsonObject)
    ?.get("state")?.toString()?.trim('"')
  val runtimeDetail = (session.state?.get("runtimeStatus") as? kotlinx.serialization.json.JsonObject)
    ?.get("detail")?.toString()?.trim('"')
  val effectiveStatus = runtimeState ?: session.status ?: "unknown"

  with(sharedTransitionScope) {
    Surface(
      modifier = modifier
        .fillMaxWidth()
        .sharedBounds(
          sharedContentState = rememberSharedContentState(key = "session-${session.id}"),
          animatedVisibilityScope = animatedVisibilityScope,
          clipInOverlayDuringTransition = OverlayClip(sessionShape),
        )
        .combinedClickable(
          interactionSource = remember { MutableInteractionSource() },
          indication = null,
          onClick = onClick,
          onLongClick = onLongClick,
        ),
      shape = sessionShape,
      color = if (isSelected) MaterialTheme.colorScheme.primaryContainer else MaterialTheme.colorScheme.surface,
      border = BorderStroke(
        1.dp,
        if (isSelected) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline,
      ),
    ) {
      Row(
        Modifier.padding(horizontal = 16.dp, vertical = 15.dp),
        horizontalArrangement = Arrangement.spacedBy(12.dp),
        verticalAlignment = Alignment.Top,
      ) {
        // Icon
        Surface(
          modifier = Modifier.size(40.dp),
          shape = CircleShape,
          color = MaterialTheme.colorScheme.surfaceVariant,
        ) {
          Icon(
            imageVector = Icons.Default.Terminal,
            contentDescription = null,
            modifier = Modifier.padding(8.dp),
            tint = MaterialTheme.colorScheme.onSurfaceVariant,
          )
        }

        // Content
        Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(4.dp)) {
          Row(
            Modifier.fillMaxWidth(),
            horizontalArrangement = Arrangement.SpaceBetween,
            verticalAlignment = Alignment.CenterVertically,
          ) {
            Text(
              text = displayTitle,
              style = MaterialTheme.typography.titleSmall,
              fontWeight = FontWeight.Bold,
              maxLines = 1,
              overflow = TextOverflow.Ellipsis,
              modifier = Modifier.weight(1f, fill = false),
            )
            StatusPill(effectiveStatus)
          }

          if (runtimeDetail != null && runtimeState != "idle" && runtimeState != "created") {
            Text(
              text = runtimeDetail,
              style = MaterialTheme.typography.bodySmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant.copy(alpha = 0.6f),
              maxLines = 1,
              overflow = TextOverflow.Ellipsis,
            )
          } else if (displayProject.isNotEmpty()) {
            Text(
              text = displayProject,
              style = MaterialTheme.typography.bodySmall,
              fontWeight = FontWeight.SemiBold,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
          }

          if (!session.owner.isNullOrBlank()) {
            Text(
              text = session.owner,
              style = MaterialTheme.typography.bodySmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
              maxLines = 1,
              overflow = TextOverflow.Ellipsis,
            )
          }

          if (!session.updatedAt.isNullOrBlank()) {
            Text(
              text = session.updatedAt,
              style = MaterialTheme.typography.labelSmall,
              color = MaterialTheme.colorScheme.outline,
            )
          }
        }
      }
    }
  }
}
