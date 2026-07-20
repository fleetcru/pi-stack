package com.example.picompanion.ui.sessiondetail

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.ExperimentalMaterial3Api
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.ModalBottomSheet
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.material3.rememberModalBottomSheetState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.style.TextOverflow
import androidx.compose.ui.unit.dp
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.FileEntry
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext

@OptIn(ExperimentalMaterial3Api::class)
@Composable
fun FileBrowserSheet(server: ServerEntry?, initialPath: String, onDismiss: () -> Unit) {
  if (server == null) return
  val scope = rememberCoroutineScope()
  val client = remember { PiServerClient() }
  var path by remember { mutableStateOf(initialPath) }
  var files by remember { mutableStateOf<List<FileEntry>>(emptyList()) }
  var preview by remember { mutableStateOf<String?>(null) }
  var status by remember { mutableStateOf("Loading files…") }
  fun load(directory: String) = scope.launch {
    path = directory; preview = null; status = "Loading files…"
    when (val response = withContext(Dispatchers.IO) { client.listFiles(server, directory) }) {
      is HttpResult.Success -> { files = response.value.files; status = "" }
      is HttpResult.Failure -> status = response.userMessage
    }
  }
  androidx.compose.runtime.LaunchedEffect(initialPath) { load(initialPath) }
  ModalBottomSheet(onDismissRequest = onDismiss, sheetState = rememberModalBottomSheetState(skipPartiallyExpanded = true)) {
    Column(Modifier.fillMaxWidth().padding(horizontal = 16.dp).padding(bottom = 28.dp)) {
      Row(verticalAlignment = Alignment.CenterVertically) {
        IconButton(onClick = { load(path.substringBeforeLast('/', path).substringBeforeLast('\\', path)) }) { Text("↑") }
        Text(path, modifier = Modifier.weight(1f), maxLines = 1, overflow = TextOverflow.Ellipsis, style = MaterialTheme.typography.titleSmall)
      }
      HorizontalDivider()
      if (preview != null) {
        TextButton(onClick = { preview = null }) { Text("Back to files") }
        Text(preview!!, modifier = Modifier.fillMaxWidth().padding(vertical = 12.dp), fontFamily = FontFamily.Monospace, style = MaterialTheme.typography.bodySmall)
      } else if (status.isNotBlank()) Text(status, modifier = Modifier.padding(20.dp), color = MaterialTheme.colorScheme.error)
      else LazyColumn {
        items(files, key = { it.path }) { file ->
          Row(Modifier.fillMaxWidth().clickable {
            if (file.isDir) load(file.path) else scope.launch {
              status = "Loading preview…"
              when (val content = withContext(Dispatchers.IO) { client.getFileContent(server, file.path) }) {
                is HttpResult.Success -> preview = if (content.value.binary) "Binary file: ${file.name}" else content.value.content.orEmpty() + if (content.value.truncated) "\n\n[Preview truncated]" else ""
                is HttpResult.Failure -> preview = "Could not load file: ${content.userMessage}"
              }; status = ""
            }
          }.padding(vertical = 12.dp), verticalAlignment = Alignment.CenterVertically) {
            Text(if (file.isDir) "▸" else "·", color = MaterialTheme.colorScheme.primary)
            Spacer(Modifier.width(10.dp)); Text(file.name, maxLines = 1, overflow = TextOverflow.Ellipsis)
          }
        }
      }
    }
  }
}
