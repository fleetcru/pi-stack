package com.example.picompanion.ui.components

import androidx.compose.animation.core.Spring
import androidx.compose.animation.core.animateFloatAsState
import androidx.compose.animation.core.spring
import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.defaultMinSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Dns
import androidx.compose.material.icons.rounded.Home
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material.icons.rounded.Terminal
import androidx.compose.material3.Icon
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.graphicsLayer
import androidx.compose.ui.graphics.vector.ImageVector
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

enum class NavTab(val label: String, val icon: ImageVector) {
  Home("Home", Icons.Rounded.Home),
  Sessions("Sessions", Icons.Rounded.Terminal),
  Workers("Workers", Icons.Rounded.Dns),
  Settings("Settings", Icons.Rounded.Settings),
}

@Composable
fun BottomNavBar(
  selectedTab: NavTab,
  onTabSelected: (NavTab) -> Unit,
  modifier: Modifier = Modifier,
) {
  Surface(
    modifier = modifier.fillMaxWidth(),
    shape = RoundedCornerShape(24.dp),
    color = MaterialTheme.colorScheme.surface,
    border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
  ) {
    Row(
      Modifier.padding(horizontal = 8.dp, vertical = 6.dp),
      horizontalArrangement = Arrangement.SpaceEvenly,
      verticalAlignment = Alignment.CenterVertically,
    ) {
      NavTab.entries.forEach { tab ->
        NavItem(
          icon = tab.icon,
          label = tab.label,
          selected = tab == selectedTab,
          onClick = {
            if (tab != selectedTab) onTabSelected(tab)
          },
          modifier = Modifier.weight(1f),
        )
      }
    }
  }
}

@Composable
private fun NavItem(
  icon: ImageVector,
  label: String,
  selected: Boolean,
  onClick: () -> Unit,
  modifier: Modifier = Modifier,
) {
  // Only scale uses spring animation — the bouncy feel on tab selection.
  // Colors and font weight are direct so theme switches are instant (no interpolation).
  val scale by animateFloatAsState(
    targetValue = if (selected) 1.12f else 1f,
    animationSpec = spring(
      dampingRatio = Spring.DampingRatioMediumBouncy,
      stiffness = Spring.StiffnessMedium,
    ),
    label = "nav_icon_scale",
  )

  val tint = if (selected) MaterialTheme.colorScheme.primary
  else MaterialTheme.colorScheme.onSurfaceVariant

  val fontWeight = if (selected) FontWeight.Bold else FontWeight.Normal

  Column(
    horizontalAlignment = Alignment.CenterHorizontally,
    verticalArrangement = Arrangement.spacedBy(3.dp),
    modifier = modifier
      .defaultMinSize(minHeight = 56.dp)
      .padding(vertical = 10.dp, horizontal = 12.dp)
      .clickable(
        interactionSource = remember { MutableInteractionSource() },
        indication = null,
        onClick = onClick,
      ),
  ) {
    Icon(
      icon,
      contentDescription = label,
      tint = tint,
      modifier = Modifier.graphicsLayer {
        scaleX = scale
        scaleY = scale
      },
    )
    Text(
      label,
      style = MaterialTheme.typography.labelSmall,
      fontWeight = fontWeight,
      color = tint,
    )
  }
}
