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
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.FilterChip
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Switch
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
import com.example.picompanion.di.AppModule
import com.example.picompanion.data.model.DirectoryEntry
import com.example.picompanion.data.model.ServerWorker
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

data class NewSessionSelection(
  val cwd: String,
  val prompt: String,
  val count: Int,
  val title: String?,
  val createWorktree: Boolean,
  val workerId: String,
)

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun DirectoryBrowserSheet(
  visible: Boolean,
  server: ServerEntry?,
  onDismiss: () -> Unit,
  onSelect: (NewSessionSelection) -> Unit,
  isCreating: Boolean = false,
) {
  if (!visible || server == null) return

  val sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)
  val scope = rememberCoroutineScope()
  val client = AppModule.client

  var currentPath by remember { mutableStateOf<String?>(null) }
  var parentPath by remember { mutableStateOf<String?>(null) }
  var directories by remember { mutableStateOf<List<DirectoryEntry>>(emptyList()) }
  var isLoading by remember { mutableStateOf(false) }
  var error by remember { mutableStateOf<String?>(null) }
  var prompt by remember { mutableStateOf("") }
  var title by remember { mutableStateOf("") }
  var countText by remember { mutableStateOf("1") }
  var createWorktree by remember { mutableStateOf(false) }
  var workerId by remember { mutableStateOf("local") }
  var workers by remember { mutableStateOf<List<ServerWorker>>(emptyList()) }
  var workersError by remember { mutableStateOf<String?>(null) }

  fun resetForm() {
    prompt = ""
    title = ""
    countText = "1"
    createWorktree = false
    workerId = "local"
    currentPath = null
    parentPath = null
    directories = emptyList()
    error = null
    workersError = null
  }

  fun resetAndDismiss() {
    if (isCreating) return
    resetForm()
    onDismiss()
  }

  fun load(path: String? = null, forWorker: String = workerId) {
    scope.launch {
      isLoading = true
      error = null
      val result = withContext(Dispatchers.IO) {
        client.listDirectories(server, path, forWorker)
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

  fun loadWorkers() {
    scope.launch {
      workersError = null
      when (val result = withContext(Dispatchers.IO) { client.listWorkers(server) }) {
        is HttpResult.Success -> {
          workers = result.value.workers.filter { it.id != "local" }
        }
        is HttpResult.Failure -> {
          workers = emptyList()
          workersError = result.message
        }
      }
    }
  }

  LaunchedEffect(visible) {
    if (visible) {
      loadWorkers()
      load()
    }
  }

  ModalBottomSheet(
    onDismissRequest = ::resetAndDismiss,
    sheetState = sheetState,
  ) {
    Column(
      Modifier
        .fillMaxWidth()
        .padding(horizontal = 16.dp)
        .padding(bottom = 32.dp),
    ) {
      Row(
        Modifier.fillMaxWidth(),
        horizontalArrangement = Arrangement.SpaceBetween,
        verticalAlignment = Alignment.CenterVertically,
      ) {
        Row(verticalAlignment = Alignment.CenterVertically) {
          if (parentPath != null && parentPath != currentPath) {
            IconButton(onClick = { load(parentPath) }, enabled = !isCreating) {
              Icon(Icons.AutoMirrored.Filled.ArrowBack, contentDescription = "Back")
            }
          } else {
            IconButton(onClick = { load() }, enabled = !isCreating) {
              Icon(Icons.Default.Home, contentDescription = "Roots")
            }
          }
          Column {
            Text(
              text = "New session",
              style = MaterialTheme.typography.titleMedium,
              fontWeight = FontWeight.Bold,
            )
            if (currentPath != null) {
              Text(
                text = currentPath ?: "",
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

      Text(
        "Worker",
        style = MaterialTheme.typography.labelMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
      )
      Spacer(Modifier.height(6.dp))
      Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        FilterChip(
          selected = workerId == "local",
          onClick = {
            if (isCreating) return@FilterChip
            workerId = "local"
            load(forWorker = "local")
          },
          label = { Text("Local") },
          enabled = !isCreating,
        )
        workers.forEach { worker ->
          FilterChip(
            selected = workerId == worker.id,
            onClick = {
              if (isCreating) return@FilterChip
              workerId = worker.id
              load(forWorker = worker.id)
            },
            label = { Text(worker.id) },
            enabled = !isCreating && worker.status != "error",
          )
        }
      }
      if (workersError != null) {
        Spacer(Modifier.height(4.dp))
        Text(
          "Could not load remote workers — using Local.",
          style = MaterialTheme.typography.labelSmall,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }

      Spacer(Modifier.height(12.dp))

      OutlinedTextField(
        value = prompt,
        onValueChange = { prompt = it },
        modifier = Modifier.fillMaxWidth(),
        label = { Text("What should Pi do? (optional)") },
        placeholder = { Text("Fix the login bug and run the tests") },
        minLines = 2,
        maxLines = 4,
        enabled = !isCreating,
      )
      Spacer(Modifier.height(8.dp))
      OutlinedTextField(
        value = title,
        onValueChange = { title = it },
        modifier = Modifier.fillMaxWidth(),
        label = { Text("Title (optional)") },
        placeholder = { Text("Refactor authentication") },
        singleLine = true,
        enabled = !isCreating,
      )
      Spacer(Modifier.height(8.dp))
      OutlinedTextField(
        value = countText,
        onValueChange = { value -> countText = value.filter(Char::isDigit).take(2) },
        modifier = Modifier.fillMaxWidth(),
        label = { Text("Number of sessions") },
        supportingText = { Text("Start up to 12 sessions with the same task") },
        singleLine = true,
        enabled = !isCreating,
      )
      Spacer(Modifier.height(8.dp))
      Row(
        Modifier.fillMaxWidth(),
        verticalAlignment = Alignment.CenterVertically,
        horizontalArrangement = Arrangement.SpaceBetween,
      ) {
        Column(Modifier.weight(1f).padding(end = 12.dp)) {
          Text("Isolated git worktree", style = MaterialTheme.typography.bodyMedium, fontWeight = FontWeight.SemiBold)
          Text(
            "Spawn a fresh branch under .pi-worktrees so edits stay isolated.",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
          )
        }
        Switch(
          checked = createWorktree,
          onCheckedChange = { createWorktree = it },
          enabled = !isCreating,
        )
      }
      Spacer(Modifier.height(12.dp))

      if (currentPath != null) {
        Surface(
          modifier = Modifier
            .fillMaxWidth()
            .clickable(
              enabled = !isCreating,
              interactionSource = remember { MutableInteractionSource() },
              indication = null,
              onClick = {
                currentPath?.let { path ->
                  onSelect(
                    NewSessionSelection(
                      cwd = path,
                      prompt = prompt.trim(),
                      count = countText.toIntOrNull()?.coerceIn(1, 12) ?: 1,
                      title = title.trim().ifBlank { null },
                      createWorktree = createWorktree,
                      workerId = workerId,
                    ),
                  )
                }
              },
            ),
          shape = RoundedCornerShape(12.dp),
          color = MaterialTheme.colorScheme.primaryContainer.copy(alpha = 0.3f),
        ) {
          Row(
            Modifier.padding(14.dp),
            verticalAlignment = Alignment.CenterVertically,
          ) {
            if (isCreating) {
              CircularProgressIndicator(Modifier.size(18.dp), strokeWidth = 2.dp)
            } else {
              Icon(Icons.Default.Add, contentDescription = null, modifier = Modifier.size(18.dp), tint = MaterialTheme.colorScheme.primary)
            }
            Spacer(Modifier.width(10.dp))
            Column {
              Text(
                if (isCreating) "Creating session…" else "Use this folder",
                style = MaterialTheme.typography.bodyMedium,
                fontWeight = FontWeight.SemiBold,
                color = MaterialTheme.colorScheme.primary,
              )
              Text(currentPath ?: "", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, maxLines = 1, overflow = TextOverflow.Ellipsis)
            }
          }
        }
        Spacer(Modifier.height(12.dp))
        HorizontalDivider(color = MaterialTheme.colorScheme.outline.copy(alpha = 0.2f))
        Spacer(Modifier.height(8.dp))
      }

      if (isLoading) {
        Column(Modifier.fillMaxWidth().padding(40.dp), horizontalAlignment = Alignment.CenterHorizontally) {
          CircularProgressIndicator()
        }
      }

      if (error != null) {
        Column(Modifier.fillMaxWidth().padding(20.dp), horizontalAlignment = Alignment.CenterHorizontally) {
          Text(error ?: "", color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodySmall)
          Spacer(Modifier.height(8.dp))
          TextButton(onClick = { load(currentPath) }, enabled = !isCreating) { Text("Retry") }
        }
      }

      if (!isLoading && error == null) {
        LazyColumn {
          items(directories, key = { it.path }) { dir ->
            DirectoryRow(
              entry = dir,
              enabled = !isCreating,
              onClick = { load(dir.path) },
            )
          }
          if (directories.isEmpty()) {
            item {
              Text(
                "No subdirectories — use this folder above to create a session.",
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
  enabled: Boolean,
  onClick: () -> Unit,
) {
  Row(
    Modifier
      .fillMaxWidth()
      .clickable(
        enabled = enabled,
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
