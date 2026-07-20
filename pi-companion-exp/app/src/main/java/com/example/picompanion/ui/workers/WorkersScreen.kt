package com.example.picompanion.ui.workers

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.Dns
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Add
import androidx.compose.material.icons.filled.Edit
import androidx.compose.material.icons.filled.Delete
import androidx.compose.material.icons.filled.MonitorHeart
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.style.TextAlign
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.picompanion.data.model.ServerWorker
import com.example.picompanion.ui.components.StatusPill

@Composable
fun WorkersScreen(
  modifier: Modifier = Modifier,
  viewModel: WorkersViewModel = viewModel(),
) {
  val uiState by viewModel.uiState.collectAsStateWithLifecycle()
  var editingWorker by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf<ServerWorker?>(null) }
  var addingWorker by androidx.compose.runtime.remember { androidx.compose.runtime.mutableStateOf(false) }

  if (addingWorker || editingWorker != null) WorkerEditorDialog(
    worker = editingWorker,
    onDismiss = { addingWorker = false; editingWorker = null },
    onSave = { id, url, token, tags -> viewModel.saveWorker(id, url, token, tags); addingWorker = false; editingWorker = null },
  )

  Column(
    modifier
      .fillMaxSize()
      .padding(horizontal = 18.dp),
  ) {
    // Header
    Row(
      Modifier
        .fillMaxWidth()
        .padding(start = 4.dp, top = 28.dp, bottom = 16.dp),
      horizontalArrangement = Arrangement.SpaceBetween,
      verticalAlignment = Alignment.CenterVertically,
    ) {
      Column {
        Text(
          text = "Workers",
          style = MaterialTheme.typography.headlineMedium,
          fontWeight = FontWeight.Bold,
        )
        Text(
          text = "Local and remote compute nodes",
          style = MaterialTheme.typography.bodyMedium,
          color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }
      Row {
        IconButton(onClick = { addingWorker = true }) { Icon(Icons.Default.Add, contentDescription = "Add worker") }
        IconButton(onClick = { viewModel.refresh() }) { Icon(Icons.Default.Refresh, contentDescription = "Refresh") }
      }
    }

    // Content
    when (val state = uiState) {
      is WorkersUiState.Loading -> {
        Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
          CircularProgressIndicator()
        }
      }

      is WorkersUiState.Empty -> {
        Box(Modifier.fillMaxSize().padding(top = 80.dp), contentAlignment = Alignment.TopCenter) {
          Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(
              "No workers",
              style = MaterialTheme.typography.titleMedium,
              fontWeight = FontWeight.SemiBold,
            )
            Spacer(Modifier.height(4.dp))
            Text(
              "Add a worker to pi-server to get started",
              style = MaterialTheme.typography.bodySmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
              textAlign = TextAlign.Center,
            )
          }
        }
      }

      is WorkersUiState.Error -> {
        Box(Modifier.fillMaxSize().padding(top = 80.dp), contentAlignment = Alignment.TopCenter) {
          Column(horizontalAlignment = Alignment.CenterHorizontally) {
            Text(
              "Failed to load workers",
              style = MaterialTheme.typography.titleMedium,
              fontWeight = FontWeight.SemiBold,
              color = MaterialTheme.colorScheme.error,
            )
            Spacer(Modifier.height(4.dp))
            Text(
              state.message,
              style = MaterialTheme.typography.bodySmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
              textAlign = TextAlign.Center,
            )
            Spacer(Modifier.height(16.dp))
            OutlinedButton(onClick = { viewModel.refresh() }) {
              Icon(Icons.Default.Refresh, contentDescription = null, modifier = Modifier.size(16.dp))
              Spacer(Modifier.width(6.dp))
              Text("Retry")
            }
          }
        }
      }

      is WorkersUiState.Content -> {
        LazyColumn(verticalArrangement = Arrangement.spacedBy(12.dp)) {
          items(state.workers, key = { it.id }, contentType = { "worker_item" }) { worker ->
            WorkerListItem(
              worker = worker,
              onEdit = { editingWorker = worker },
              onDelete = { viewModel.deleteWorker(worker.id) },
              onHealth = { viewModel.checkHealth(worker.id) },
            )
          }
        }
      }
    }
  }
}

@Composable
private fun WorkerListItem(
  worker: ServerWorker,
  onEdit: () -> Unit,
  onDelete: () -> Unit,
  onHealth: () -> Unit,
  modifier: Modifier = Modifier,
) {
  val statusText = when {
    worker.status == "online" -> "Online"
    worker.status == "error" -> "Error"
    worker.lastHeartbeat?.startsWith("0001") == true -> "New"
    else -> worker.status?.ifBlank { "Unknown" } ?: "Unknown"
  }

  Surface(
    modifier = modifier.fillMaxWidth(),
    shape = RoundedCornerShape(16.dp),
    color = MaterialTheme.colorScheme.surface,
    border = BorderStroke(1.dp, MaterialTheme.colorScheme.outline),
  ) {
    Row(
      Modifier.padding(horizontal = 16.dp, vertical = 14.dp),
      horizontalArrangement = Arrangement.spacedBy(14.dp),
      verticalAlignment = Alignment.CenterVertically,
    ) {
      // Icon
      Surface(
        modifier = Modifier.size(42.dp),
        shape = CircleShape,
        color = MaterialTheme.colorScheme.surfaceVariant,
      ) {
        Icon(
          imageVector = Icons.Default.Dns,
          contentDescription = null,
          modifier = Modifier.padding(10.dp),
          tint = MaterialTheme.colorScheme.onSurfaceVariant,
        )
      }

      // Info
      Column(Modifier.weight(1f), verticalArrangement = Arrangement.spacedBy(3.dp)) {
        Row(
          Modifier.fillMaxWidth(),
          horizontalArrangement = Arrangement.SpaceBetween,
          verticalAlignment = Alignment.CenterVertically,
        ) {
          Text(
            text = worker.id,
            style = MaterialTheme.typography.titleSmall,
            fontWeight = FontWeight.Bold,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
            modifier = Modifier.weight(1f, fill = false),
          )
          StatusPill(statusText)
        }

        if (!worker.url.isNullOrBlank() && worker.url != "local") {
          Text(
            text = worker.url,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
            maxLines = 1,
            overflow = TextOverflow.Ellipsis,
          )
        }

        // Sessions + capacity
        if (worker.maxSessions > 0) {
          Text(
            text = "${worker.activeSessions} / ${worker.maxSessions} sessions",
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.outline,
          )
        }

        // Tags
        if (worker.tags.isNotEmpty()) {
          Text(
            text = worker.tags.joinToString(" · "),
            style = MaterialTheme.typography.labelSmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
          )
        }
        Row {
          IconButton(onClick = onHealth, modifier = Modifier.size(32.dp)) { Icon(Icons.Default.MonitorHeart, contentDescription = "Check worker health") }
          IconButton(onClick = onEdit, modifier = Modifier.size(32.dp)) { Icon(Icons.Default.Edit, contentDescription = "Edit worker") }
          IconButton(onClick = onDelete, modifier = Modifier.size(32.dp)) { Icon(Icons.Default.Delete, contentDescription = "Delete worker") }
        }
      }
    }
  }
}
