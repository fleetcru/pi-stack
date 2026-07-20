package com.example.picompanion.ui.components

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Menu
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

@Composable
fun TopAppBarCompact(
  onMenuClick: () -> Unit = {},
  onSettingsClick: () -> Unit = {},
  modifier: Modifier = Modifier,
) {
  Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
    IconTile(Icons.Rounded.Menu, "Menu", onClick = onMenuClick)
    Column(verticalArrangement = Arrangement.spacedBy(1.dp), modifier = Modifier.weight(1f).padding(horizontal = 14.dp)) {
      Text("Pi Companion", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
      Text("Pi Dev Control", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
    IconTile(Icons.Rounded.Settings, "Settings", onClick = onSettingsClick)
  }
}
