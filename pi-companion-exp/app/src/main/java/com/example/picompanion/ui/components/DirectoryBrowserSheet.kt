package com.example.picompanion.ui.components

import androidx.compose.foundation.clickable
import androidx.compose.foundation.interaction.MutableInteractionSource
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.automirrored.filled.ArrowBack
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Home
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.DirectoryEntry
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DirectoryBrowserSheet(
  visible: Boolean,
  server: ServerEntry?,
  onDismiss: () -> Unit,
  onSelect: (String) -> Unit,
) {
  if (!visible || server == null) return

  val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
  val scope = rememberCoroutineScope()
  val client = remember { PiServerClient() }

  var currentPath by remember { mutableStateOf<String?>(null) }
  var parentPath by remember { mutableStateOf<String?>(null) }
  var directories by remember { mutableStateOf<List<DirectoryEntry>>(emptyList()) }
  var isLoading by remember { mutableStateOf(false) }
  var error by remember { mutableStateOf<String?>(null) }

  fun load(path: String? = null) {
    scope.launch {
      isLoading = true
      error = null
      val result = withContext(Dispatchers.IO) {
        client.listDirectories(server, path)
      }
      when (result) {
        is HttpResult.Success -> {
          currentPath = result.value.path
          parentPath = result.value.parent
          directories = if (path == null) {
            result.value.roots.ifEmpty { result.value.directories }
          } else {
            result.value.directories
          }
          isLoading = false
        }
        is HttpResult.Failure -> {
          error = result.message
          isLoading = false
        }
      }
    }
  }

  // Load root on first show
  LaunchedEffect(visible) {
    if (visible) load()
  }

  ModalBottomSheet(
    onDismissRequest = onDismiss,
    sheetState = sheetState,
  ) {
    Column(
      Modifier
        .fillMaxWidth()
        .padding(horizontal = 16.dp)
        .padding(bottom = 32.dp),
    ) {
      // Header
      Row(
        Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
      ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
          if (parentPath != null && parentPath != currentPath) {
            IconButton(onClick = { load(parentPath) }) {
              Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
            }
          } else {
            IconButton(onClick = { load() }) {
              Icon(Icons.Default.Home, contentDescription = "Roots")
            }
          }
          Column {
            Text(
              text = "Browse Directories",
              style = MaterialTheme.typography.titleMedium,
              fontWeight = FontWeight.Bold,
            )
            if (currentPath != null) {
              Text(
                text = currentPath!!,
                style = MaterialTheme.typography.labelSmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                maxLines = 1,
                overflow = TextOverflow.Ellipsis,
              )
            }
          }
        }
      }

      Spacer(Modifier.height(12.dp))

      // Select current directory button
      if (currentPath != null) {
        Surface(
          modifier = Modifier
            .fillMaxWidth()
            .clickable(
              interactionSource = remember { MutableInteractionSource() },
              indication = null,
              onClick = { onSelect(currentPath!!) },
            ),
          shape = RoundedCornerShape(12.dp),
          color = MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.3f),
        ) {
          Row(
            Modifier.padding(14.dp),
            verticalAlignment = Alignment.CenterVertically,
          ) {
            Icon(Icons.Default.Add, contentDescription = null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.width(10.dp))
            Column {
              Text("Use this folder", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.primary)
              Text(currentPath!!, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
          }
        }
        Spacer(Modifier.height(12.dp))
        HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.2f))
        Spacer(Modifier.height(8.dp))
      }

      // Loading
      if (isLoading) {
        Column(Modifier.fillMaxWidth().padding(40.dp), horizontalAlignment = Alignment.CenterHorizontally) {
          CircularProgressIndicator()
        }
      }

      // Error
      if (error != null) {
        Column(Modifier.fillMaxWidth().padding(20.dp), horizontalAlignment = Alignment.CenterHorizontally) {
          Text(error!!, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
          Spacer(Modifier.height(8.dp))
          TextButton(onClick = { load(currentPath) }) { Text("Retry") }
        }
      }

      // Directory list
      if (!isLoading && error == null) {
        LazyColumn {
          items(directories, key = { it.path }) { dir ->
            DirectoryRow(
              entry = dir,
              onClick = { load(dir.path) },
            )
          }
          if (directories.isEmpty()) {
            item {
              Text(
                "No subdirectories",
                modifier = Modifier.padding(20.dp),
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
              )
            }
          }
        }
      }
    }
  }
}

@Composable
private fun DirectoryRow(
  entry: DirectoryEntry,
  onClick: () -> Unit,
) {
  Row(
    Modifier
      .fillMaxWidth()
      .clickable(
        interactionSource = remember { MutableInteractionSource() },
        indication = null,
        onClick = onClick,
      )
      .padding(vertical = 10.dp, horizontal = 4.dp),
    verticalAlignment = Alignment.CenterVertically,
  ) {
    Icon(
      Icons.Default.Folder,
      contentDescription = null,
      modifier = Modifier.size(20.dp),
      tint = MaterialTheme.colorScheme.secondary,
    )
    Spacer(Modifier.width(12.dp))
    Text(
      text = entry.name,
      style = MaterialTheme.typography.bodyMedium,
      maxLines = 1,
      overflow = TextOverflow.Ellipsis,
    )
  }
}
