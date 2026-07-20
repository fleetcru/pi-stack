package com.example.picompanion.ui.workers

import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import com.example.picompanion.data.model.ServerWorker

@Composable
fun WorkerEditorDialog(worker: ServerWorker? = null, onDismiss: () -> Unit, onSave: (String, String, String, List<String>) -> Unit) {
  var id by remember(worker) { mutableStateOf(worker?.id.orEmpty()) }
  var url by remember(worker) { mutableStateOf(worker?.url.orEmpty()) }
  var token by remember { mutableStateOf("") }
  var tags by remember(worker) { mutableStateOf(worker?.tags?.joinToString(", ").orEmpty()) }
  AlertDialog(
    onDismissRequest = onDismiss,
    title = { Text(if (worker == null) "Add worker" else "Edit worker") },
    text = { Column {
      OutlinedTextField(id, { id = it }, label = { Text("Worker ID") }, enabled = worker == null, modifier = Modifier.fillMaxWidth())
      OutlinedTextField(url, { url = it }, label = { Text("Server URL") }, modifier = Modifier.fillMaxWidth())
      OutlinedTextField(token, { token = it }, label = { Text("Token (optional; leave blank to keep existing)") }, modifier = Modifier.fillMaxWidth())
      OutlinedTextField(tags, { tags = it }, label = { Text("Tags (comma separated)") }, modifier = Modifier.fillMaxWidth())
    } },
    confirmButton = { TextButton(onClick = { if (id.isNotBlank() && url.isNotBlank()) onSave(id.trim(), url.trim(), token, tags.split(',').map { it.trim() }.filter { it.isNotEmpty() }) }) { Text("Save") } },
    dismissButton = { TextButton(onClick = onDismiss) { Text("Cancel") } },
  )
}
