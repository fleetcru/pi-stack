package com.example.picompanion.ui.sessiondetail

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.animation.animateContentSize
import androidx.compose.ui.Alignment
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.CheckCircle
import androidx.compose.material.icons.filled.Code
import androidx.compose.material.icons.filled.Error
import androidx.compose.material.icons.filled.ExpandLess
import androidx.compose.material.icons.filled.ExpandMore
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material.icons.filled.Search
import androidx.compose.material.icons.filled.Terminal
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.clip
import androidx.compose.foundation.clickable
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import java.io.File
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.picompanion.data.model.ServerSession

@Composable
fun SessionDetailScreen(
  sessionId: String,
  onBack: () -> Unit,
  sharedTransitionScope: SharedTransitionScope,
  animatedVisibilityScope: AnimatedVisibilityScope,
  modifier: Modifier = Modifier,
  // Navigation3 may retain the same ViewModel store while changing detail
  // entries. Key it by server session ID so history/socket state never leaks
  // from one conversation into another.
  viewModel: SessionDetailViewModel = viewModel(
    key = "session-detail-$sessionId",
    factory = SessionDetailViewModel.factory(
      application = androidx.compose.ui.platform.LocalContext.current.applicationContext as android.app.Application,
      sessionId = sessionId,
    )
  ),
) {
  val items by viewModel.items.collectAsStateWithLifecycle()
  val connectionState by viewModel.connectionState.collectAsStateWithLifecycle()
  val sendState by viewModel.sendState.collectAsStateWithLifecycle()
  val agentWorking by viewModel.agentWorking.collectAsStateWithLifecycle()
  val listState = rememberLazyListState()
  var controlsOpen by rememberSaveable { mutableStateOf(false) }
  var filesOpen by rememberSaveable { mutableStateOf(false) }
  var modelControlsOpen by rememberSaveable { mutableStateOf(false) }
  var attachments by remember { mutableStateOf<List<android.net.Uri>>(emptyList()) }
  val context = androidx.compose.ui.platform.LocalContext.current
  val galleryLauncher = rememberLauncherForActivityResult(ActivityResultContracts.GetContent()) { uri ->
    if (uri != null) attachments = attachments + uri
  }
  var pendingCameraUri by remember { mutableStateOf<android.net.Uri?>(null) }
  val cameraLauncher = rememberLauncherForActivityResult(ActivityResultContracts.TakePicture()) { saved ->
    if (saved) pendingCameraUri?.let { attachments = attachments + it }
    pendingCameraUri = null
  }
  val sessionCwd by viewModel.sessionCwd.collectAsStateWithLifecycle()
  val extensionRequest by viewModel.extensionRequest.collectAsStateWithLifecycle()
  val modelControls by viewModel.modelControls.collectAsStateWithLifecycle()
  val relayHealth by viewModel.relayHealth.collectAsStateWithLifecycle()
  val hasOlderHistory by viewModel.hasOlderHistory.collectAsStateWithLifecycle()
  val loadingOlderHistory by viewModel.loadingOlderHistory.collectAsStateWithLifecycle()
  var extensionValue by remember { mutableStateOf("") }

  // Follow a streaming reply only while the reader is already at the end.
  // This keeps live output readable without dragging someone away from history.
  val lastItem = items.lastOrNull()
  val streamVersion = when (lastItem) {
    is SessionTimelineItem.Chat -> lastItem.text.length
    is SessionTimelineItem.Tool -> (lastItem.output?.length ?: 0) + lastItem.status.hashCode()
    else -> items.size
  }
  LaunchedEffect(items.size, streamVersion) {
    if (items.isNotEmpty()) {
      val lastVisible = listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: -1
      if (lastVisible >= items.lastIndex - 1) listState.scrollToItem(items.lastIndex)
    }
  }

  Column(
    modifier
      .fillMaxSize()
      .imePadding(),
  ) {
    // Header
    SessionHeader(
      sessionId = sessionId,
      onBack = onBack,
      connectionState = connectionState,
      relayHealth = relayHealth,
      onReconnect = { viewModel.reconnect() },
      onCompact = { viewModel.compact() },
      onControls = { controlsOpen = true },
      onFiles = { filesOpen = true },
      onModelControls = { modelControlsOpen = true; viewModel.loadModelControls() },
      sharedTransitionScope = sharedTransitionScope,
      animatedVisibilityScope = animatedVisibilityScope,
    )

    extensionRequest?.let { request ->
      androidx.compose.material3.AlertDialog(
        onDismissRequest = { viewModel.respondToExtension(cancelled = true) },
        title = { Text("Pi extension request") },
        text = {
          Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(request.message)
            Text(
              "Request ID: ${request.id}. Only respond if you expected this extension prompt. " +
                "A reconnect can replay an older request; Ignore hides it without approving or cancelling it.",
              style = MaterialTheme.typography.bodySmall,
              color = MaterialTheme.colorScheme.onSurfaceVariant,
            )
            androidx.compose.material3.OutlinedTextField(
              value = extensionValue,
              onValueChange = { extensionValue = it },
              label = { Text(request.placeholder ?: "Response") },
              modifier = Modifier.fillMaxWidth(),
            )
          }
        },
        confirmButton = {
          androidx.compose.material3.TextButton(onClick = {
            viewModel.respondToExtension(value = extensionValue.ifBlank { null }, confirmed = true)
            extensionValue = ""
          }) { Text("Confirm") }
        },
        dismissButton = {
          Row {
            androidx.compose.material3.TextButton(onClick = { viewModel.ignoreExtensionRequest(); extensionValue = "" }) { Text("Ignore") }
            androidx.compose.material3.TextButton(onClick = {
              viewModel.respondToExtension(cancelled = true)
              extensionValue = ""
            }) { Text("Cancel") }
          }
        }, 
      )
    }

    if (modelControlsOpen) {
      ModelControlsSheet(
        controls = modelControls,
        onDismiss = { modelControlsOpen = false },
        onSelectModel = viewModel::setModel,
        onSelectThinking = viewModel::setThinkingLevel,
      )
    }

    if (filesOpen && sessionCwd.isNotBlank()) {
      FileBrowserSheet(
        server = viewModel.activeServerForUi(),
        initialPath = sessionCwd,
        onDismiss = { filesOpen = false },
      )
    }

    if (controlsOpen) {
      SessionControlsDialog(
        initialTitle = viewModel.sessionTitle.collectAsStateWithLifecycle().value,
        initialProject = viewModel.sessionProject.collectAsStateWithLifecycle().value,
        onDismiss = { controlsOpen = false },
        onSaveMetadata = viewModel::updateMetadata,
        onAction = viewModel::runSessionAction,
        onGit = viewModel::showGit,
      )
    }

    Box(
      modifier = Modifier
        .weight(1f)
        .fillMaxWidth(),
    ) {
      LazyColumn(
        state = listState,
        modifier = Modifier
          .fillMaxSize()
          .padding(horizontal = 16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        contentPadding = PaddingValues(top = 10.dp, bottom = 118.dp),
      ) {
        if (hasOlderHistory) {
          item(key = "load-older") {
            androidx.compose.material3.OutlinedButton(
              onClick = viewModel::loadOlderHistory,
              enabled = !loadingOlderHistory,
              modifier = Modifier.fillMaxWidth(),
            ) { Text(if (loadingOlderHistory) "Loading older messages…" else "Load older messages") }
          }
        }
        // Stable timestamps/call IDs preserve the reader's anchor while an
        // older page is prepended. Live rows fall back to their current index.
        itemsIndexed(items, key = { index, item ->
          when (item) {
            is SessionTimelineItem.Chat -> item.time.takeIf { it.isNotBlank() }?.let { "chat-$it" } ?: "chat-live-$index"
            is SessionTimelineItem.Tool -> "tool-${item.callId}"
            is SessionTimelineItem.FileChange -> "file-${item.operation}-${item.path}"
            is SessionTimelineItem.System -> "system-$index"
          }
        }, contentType = { _, item ->
          when (item) {
            is SessionTimelineItem.Chat -> "chat"
            is SessionTimelineItem.Tool -> "tool"
            is SessionTimelineItem.FileChange -> "file"
            is SessionTimelineItem.System -> "system"
          }
        }) { _, item ->
          when (item) {
            is SessionTimelineItem.Chat -> ChatBubble(
              author = item.author,
              text = item.text,
              time = item.time,
              isUser = item.isUser,
              imageUris = item.imageUris,
            )
            is SessionTimelineItem.Tool -> ToolEventRow(item)
            is SessionTimelineItem.FileChange -> FileChangeRow(item)
            is SessionTimelineItem.System -> SystemMessageRow(item)
          }
        }
      }

      MessageInputBar(
        onSend = { text -> viewModel.sendPrompt(text, attachments).also { attachments = emptyList() } },
        sending = sendState is SendState.Sending || sendState is SendState.Accepted || sendState is SendState.Delivered || sendState is SendState.Running,
        agentWorking = agentWorking,
        status = when (sendState) {
          SendState.Sending -> "Sending…"
          SendState.Accepted -> "Queued for Pi…"
          SendState.Delivered -> "Delivered to Pi…"
          SendState.Running -> "Pi responding…"
          is SendState.Failed -> "Send failed — try again"
          SendState.Idle -> null
        },
        onAbort = { viewModel.abort() },
        onSteer = { text -> viewModel.sendSteer(text) },
        onPickImage = { galleryLauncher.launch("image/*") },
        onTakePhoto = {
          val image = File(context.cacheDir, "prompt-images/${System.currentTimeMillis()}.jpg").also { it.parentFile?.mkdirs() }
          pendingCameraUri = FileProvider.getUriForFile(context, "${context.packageName}.fileprovider", image)
          pendingCameraUri?.let(cameraLauncher::launch)
        },
        attachmentCount = attachments.size,
        attachmentNames = attachments.map { it.lastPathSegment?.substringAfterLast('/') ?: "Image" },
        onRemoveAttachment = { index -> attachments = attachments.toMutableList().apply { removeAt(index) } },
        modifier = Modifier.align(Alignment.BottomCenter),
      )
    }
  }
}

@Composable
private fun ToolEventRow(item: SessionTimelineItem.Tool, modifier: Modifier = Modifier) {
  var expanded by rememberSaveable(item.callId) { mutableStateOf(false) }
  val (icon, label) = when (item.name.lowercase()) {
    "bash", "terminal" -> Icons.Filled.Terminal to "Terminal"
    "read", "read_file" -> Icons.Filled.Code to "Read file"
    "write", "edit", "apply_patch" -> Icons.Filled.Code to "Edit file"
    "find", "grep", "search" -> Icons.Filled.Search to "Search"
    "ls", "list", "list_directory" -> Icons.Filled.Folder to "Browse files"
    else -> Icons.Filled.Code to item.name
  }
  val statusIcon = when (item.status) {
    "running" -> null
    "completed" -> Icons.Filled.CheckCircle
    else -> Icons.Filled.Error
  }
  val statusColor = when (item.status) {
    "completed" -> MaterialTheme.colorScheme.primary
    "failed" -> MaterialTheme.colorScheme.error
    else -> MaterialTheme.colorScheme.tertiary
  }

  Surface(
    modifier = modifier
      .fillMaxWidth()
      .clip(RoundedCornerShape(10.dp))
      .clickable { expanded = !expanded }
      .animateContentSize(),
    shape = RoundedCornerShape(10.dp),
    color = MaterialTheme.colorScheme.surfaceVariant.copy(alpha = 0.38f),
  ) {
    Column(Modifier.padding(horizontal = 10.dp, vertical = 7.dp)) {
      Row(verticalAlignment = Alignment.CenterVertically) {
        Icon(icon, contentDescription = null, tint = statusColor, modifier = Modifier.size(16.dp))
        Spacer(Modifier.width(8.dp))
        Text(label, style = MaterialTheme.typography.bodySmall, fontWeight = FontWeight.SemiBold, modifier = Modifier.weight(1f))
        Text(
          when (item.status) {
            "running" -> "Running"
            "completed" -> "Done"
            else -> "Failed"
          },
          style = MaterialTheme.typography.labelSmall,
          color = statusColor,
          modifier = Modifier.padding(end = 6.dp),
        )
        if (statusIcon == null) {
          CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp, color = statusColor)
        } else {
          Icon(statusIcon, contentDescription = item.status, tint = statusColor, modifier = Modifier.size(20.dp))
        }
        Spacer(Modifier.width(4.dp))
        Icon(
          if (expanded) Icons.Filled.ExpandLess else Icons.Filled.ExpandMore,
          contentDescription = if (expanded) "Hide tool details" else "Show tool details",
        )
      }
      if (expanded) {
        item.args?.let {
          ToolDetail("Arguments", it)
        }
        item.output?.let {
          ToolDetail(if (item.status == "running") "Live output" else "Output", it)
        }
        if (item.args == null && item.output == null) {
          Text("Waiting for tool details…", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 10.dp))
        }
      }
    }
  }
}

@Composable
private fun ToolDetail(label: String, value: String) {
  val maxPreviewChars = 80_000
  val preview = value.take(maxPreviewChars)
  Column(Modifier.padding(top = 12.dp)) {
    Text(label, style = MaterialTheme.typography.labelSmall, fontWeight = FontWeight.SemiBold, color = MaterialTheme.colorScheme.onSurfaceVariant)
    Text(preview, style = MaterialTheme.typography.bodySmall, modifier = Modifier.padding(top = 4.dp))
    if (value.length > maxPreviewChars) Text("Output preview limited to 80 KB.", style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant, modifier = Modifier.padding(top = 4.dp))
  }
}

@Composable
private fun FileChangeRow(item: SessionTimelineItem.FileChange, modifier: Modifier = Modifier) {
  Row(modifier.fillMaxWidth(), horizontalArrangement = Arrangement.spacedBy(8.dp)) {
    Text(
      item.operation,
      style = MaterialTheme.typography.labelSmall,
      fontWeight = FontWeight.SemiBold,
      color = MaterialTheme.colorScheme.secondary,
    )
    Text(
      item.path,
      style = MaterialTheme.typography.bodySmall,
      color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
  }
}

@Composable
private fun SystemMessageRow(item: SessionTimelineItem.System, modifier: Modifier = Modifier) {
  Text(
    item.text,
    modifier = modifier.fillMaxWidth(),
    style = MaterialTheme.typography.bodySmall,
    color = MaterialTheme.colorScheme.outline,
  )
}
