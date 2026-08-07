package com.example.picompanion.ui.components

import com.example.picompanion.R
import androidx.compose.foundation.Image
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.rounded.Menu
import androidx.compose.material.icons.rounded.Refresh
import androidx.compose.material.icons.rounded.Settings
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.res.painterResource
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp

@Composable
fun TopAppBarCompact(
  onMenuClick: () -> Unit = {},
  onSettingsClick: () -> Unit = {},
  onRefresh: (() -> Unit)? = null,
  modifier: Modifier = Modifier,
) {
  Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween, verticalAlignment = Alignment.CenterVertically) {
    IconTile(Icons.Rounded.Menu, "Menu", onClick = onMenuClick)
    Image(
      painter = painterResource(R.drawable.pi_stack_logo),
      contentDescription = "Pi Stack logo",
      modifier = Modifier.size(40.dp),
    )
    Column(verticalArrangement = Arrangement.spacedBy(1.dp), modifier = Modifier.weight(1f).padding(horizontal = 10.dp)) {
      Text("Pi Companion", style = MaterialTheme.typography.titleLarge, fontWeight = FontWeight.Bold)
      Text("Pi Dev Control", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
    IconTile(Icons.Rounded.Settings, "Settings", onClick = onSettingsClick)
    if (onRefresh != null) {
      IconTile(Icons.Rounded.Refresh, "Refresh", onClick = onRefresh)
    }
  }
}
