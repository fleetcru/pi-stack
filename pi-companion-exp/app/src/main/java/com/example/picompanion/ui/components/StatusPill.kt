package com.example.picompanion.ui.components

import androidx.compose.animation.core.RepeatMode
import androidx.compose.animation.core.animateFloat
import androidx.compose.animation.core.infiniteRepeatable
import androidx.compose.animation.core.rememberInfiniteTransition
import androidx.compose.animation.core.tween
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

@Composable
fun StatusPill(text: String, modifier: Modifier = Modifier) {
  val normalized = text.lowercase().replace('_', ' ')
  val isActive = normalized in setOf("running", "starting", "working", "reconnecting")
  val isWaiting = normalized == "waiting for input"
  val isError = normalized in setOf("error", "failed", "stopped")

  val pulseAlpha by rememberInfiniteTransition(label = "status_pulse")
    .animateFloat(
      initialValue = 0.6f,
      targetValue = 1f,
      animationSpec = infiniteRepeatable(
        animation = tween(durationMillis = 800),
        repeatMode = RepeatMode.Reverse,
      ),
      label = "pulse_alpha",
    )

  val bgColor = when {
    isError -> MaterialTheme.colorScheme.errorContainer
    isWaiting -> MaterialTheme.colorScheme.tertiaryContainer
    isActive -> MaterialTheme.colorScheme.primaryContainer
    else -> MaterialTheme.colorScheme.surfaceVariant
  }
  val textColor = when {
    isError -> MaterialTheme.colorScheme.onErrorContainer
    isWaiting -> MaterialTheme.colorScheme.onTertiaryContainer
    isActive -> MaterialTheme.colorScheme.onPrimaryContainer
    else -> MaterialTheme.colorScheme.onSurfaceVariant
  }
  val borderColor = when {
    isError -> MaterialTheme.colorScheme.error.copy(alpha = 0.3f)
    isWaiting -> MaterialTheme.colorScheme.tertiary.copy(alpha = 0.3f)
    isActive -> MaterialTheme.colorScheme.primary.copy(alpha = 0.3f)
    else -> MaterialTheme.colorScheme.outline
  }

  Surface(
    modifier = modifier.graphicsLayer {
      alpha = if (isActive || isWaiting) pulseAlpha else 1f
    },
    shape = RoundedCornerShape(999.dp),
    color = bgColor,
    border = BorderStroke(1.dp, borderColor),
  ) {
    Text(
      text = normalized.replaceFirstChar { it.uppercase() },
      modifier = Modifier.padding(horizontal = 10.dp, vertical = 5.dp),
      style = MaterialTheme.typography.labelMedium,
      fontWeight = FontWeight.SemiBold,
      color = textColor,
    )
  }
}
