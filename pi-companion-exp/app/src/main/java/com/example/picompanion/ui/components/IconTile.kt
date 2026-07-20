package com.example.picompanion.ui.components

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.runtime.Composable
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.unit.dp

@Composable
fun IconTile(
  icon: ImageVector,
  contentDescription: String?,
  modifier: Modifier = Modifier,
  onClick: (() -> Unit)? = null,
) {
  val tileShape = RoundedCornerShape(12.dp)
  val baseModifier = modifier
    .size(38.dp)
    .clip(tileShape)
    .background(MaterialTheme.colorScheme.surfaceVariant)

  val clickableModifier = if (onClick != null) {
    baseModifier.clickable(
      interactionSource = remember { MutableInteractionSource() },
      indication = null,
      onClick = onClick,
    )
  } else {
    baseModifier
  }

  Box(
    modifier = clickableModifier,
    contentAlignment = Alignment.Center,
  ) {
    Icon(imageVector = icon, contentDescription = contentDescription, tint = MaterialTheme.colorScheme.onSurface)
  }
}
