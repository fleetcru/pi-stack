package com.example.picompanion.ui.sessiondetail

import androidx.compose.animation.AnimatedVisibilityScope
import androidx.compose.animation.SharedTransitionScope
import androidx.compose.ui.Alignment
import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.PaddingValues
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.itemsIndexed
import androidx.compose.foundation.lazy.rememberLazyListState
import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material.icons.Icons
import androidx.compose.material.icons.filled.KeyboardArrowDown
import androidx.compose.runtime.derivedStateOf
import androidx.compose.runtime.rememberCoroutineScope
import kotlinx.coroutines.launch
import androidx.compose.material.icons.filled.Folder
import androidx.compose.material.icons.filled.Refresh
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.Icon
import androidx.compose.material3.IconButton
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableIntStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.draw.alpha
import androidx.compose.foundation.layout.imePadding
import androidx.compose.foundation.verticalScroll
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.core.content.FileProvider
import java.io.File
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.viewModel
import com.example.picompanion.data.model.ServerSession
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts

@Composable
fun SessionDetailScreen(
  sessionId: String,
  onBack: () -> Unit,
  sharedTransitionScope: SharedTransitionScope,
  animatedVisibilityScope: AnimatedVisibilityScope,
  modifier: Modifier = Modifier,
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
  val streamingOrder by viewModel.streamingAssistantOrder.collectAsStateWithLifecycle()
  val hasOlderHistory by viewModel.hasOlderHistory.collectAsStateWithLifecycle()
  val loadingOlderHistory by viewModel.loadingOlderHistory.collectAsStateWithLifecycle()
  val historyLoadError by viewModel.historyLoadError.collectAsStateWithLifecycle()
  val timelinePrefixItems = (if (hasOlderHistory) 1 else 0) + (if (historyLoadError != null) 1 else 0)
  val timelineEndIndex = (items.lastIndex + timelinePrefixItems).coerceAtLeast(0)
  val initialEndIndex = remember(sessionId) { timelineEndIndex }
  val listState = rememberLazyListState(initialFirstVisibleItemIndex = initialEndIndex)
  val scope = rememberCoroutineScope()
  var actionsOpen by rememberSaveable { mutableStateOf(false) }
  var actionsTab by rememberSaveable { mutableIntStateOf(0) }
  var filesOpen by rememberSaveable { mutableStateOf(false) }
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
  val refreshing by viewModel.refreshing.collectAsStateWithLifecycle()
  val gitOutput by viewModel.gitOutput.collectAsStateWithLifecycle()
  val gitChanges by viewModel.gitChanges.collectAsStateWithLifecycle()
  var extensionValue by remember { mutableStateOf("") }

  // Follow a streaming reply only while the reader is already at the end.
  val lastItem = items.lastOrNull()
  val streamVersion = when (lastItem) {
    is SessionTimelineItem.Chat -> lastItem.text.length
    is SessionTimelineItem.Tool -> (lastItem.output?.length ?: 0) + lastItem.status.hashCode()
    else -> items.size
  }
  // Auto-scroll to bottom on initial load, then follow streaming replies
  // when already near the bottom.
  var initialScrollDone by remember(sessionId) { mutableStateOf(items.isNotEmpty()) }
  LaunchedEffect(items.size, streamVersion) {
    if (items.isEmpty()) return@LaunchedEffect
    if (!initialScrollDone) {
      // Position before revealing uncached history. Animating from index zero
      // briefly paints old rows while entering a session.
      listState.scrollToItem(timelineEndIndex)
      initialScrollDone = true
    } else if (!listState.isScrollInProgress) {
      val lastVisible = listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: -1
      if (lastVisible >= timelineEndIndex - 2) {
        listState.animateScrollToItem(timelineEndIndex)
      }
    }
  }

  // Stable keys derived from each item's monotonic order.
  val itemKeys = remember(items) {
    val seen = mutableSetOf<String>()
    items.mapIndexed { index, item ->
      val prefix = when (item) {
        is SessionTimelineItem.Chat -> "chat"
        is SessionTimelineItem.Tool -> "tool"
        is SessionTimelineItem.FileChange -> "file"
        is SessionTimelineItem.System -> "system"
      }
      var key = "$prefix-${item.order}"
      if (!seen.add(key)) {
        key = "$prefix-${item.order}-$index"
      }
      key
    }
  }

  Column(
    modifier
      .fillMaxSize()
      .imePadding(),
  ) {
    // Debug log banner (tap header to toggle)
    // Header — simplified with fewer actions
    SessionHeader(
      sessionId = sessionId,
      onBack = onBack,
      connectionState = connectionState,
      relayHealth = relayHealth,
      refreshing = refreshing,
      onRefresh = viewModel::refresh,
      onCompact = { viewModel.compact() },
      onControls = { actionsOpen = true },
      onFiles = { filesOpen = true },
      onModelControls = { actionsOpen = true; actionsTab = 0; viewModel.loadModelControls() },
      modelControls = modelControls,
      title = viewModel.sessionTitle.collectAsStateWithLifecycle().value,
      sharedTransitionScope = sharedTransitionScope,
      animatedVisibilityScope = animatedVisibilityScope,

    )

    // Git output dialog
    gitOutput?.let { (title, output) ->
      androidx.compose.material3.AlertDialog(
        onDismissRequest = viewModel::closeGitOutput,
        title = { Text("Git · $title") },
        text = {
          androidx.compose.foundation.text.selection.SelectionContainer {
            Box(Modifier.fillMaxWidth().heightIn(max = 420.dp)) {
              Column(Modifier.verticalScroll(androidx.compose.foundation.rememberScrollState())) {
                Text(output, style = MaterialTheme.typography.bodySmall)
              }
            }
          }
        },
        confirmButton = {
          androidx.compose.material3.TextButton(onClick = viewModel::closeGitOutput) { Text("Done") }
        },
      )
    }

    // Extension request dialog
    extensionRequest?.let { request ->
      androidx.compose.material3.AlertDialog(
        onDismissRequest = { viewModel.respondToExtension(cancelled = true) },
        title = { Text("Pi extension request") },
        text = {
          Column(verticalArrangement = Arrangement.spacedBy(8.dp)) {
            Text(request.message)
            Text(
              "Request ID: ${request.id}. Only respond if you expected this extension prompt.",
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

    // Unified actions sheet (replaces separate ModelControlsSheet + SessionControlsDialog)
    if (actionsOpen) {
      UnifiedActionsSheet(
        initialTab = actionsTab,
        initialTitle = viewModel.sessionTitle.collectAsStateWithLifecycle().value,
        initialProject = viewModel.sessionProject.collectAsStateWithLifecycle().value,
        controls = modelControls,
        onDismiss = { actionsOpen = false },
        onSelectModel = viewModel::setModel,
        onSelectThinking = viewModel::setThinkingLevel,
        onSaveMetadata = viewModel::updateMetadata,
        onAction = viewModel::runSessionAction,
        onGit = viewModel::showGit,
        onGitWrite = { action, body -> viewModel.writeGit(action, Json.parseToJsonElement(body).jsonObject) },
      )
    }

    // File browser sheet
    if (filesOpen && sessionCwd.isNotBlank()) {
      FileBrowserSheet(
        server = viewModel.activeServerForUi(),
        initialPath = sessionCwd,
        onDismiss = { filesOpen = false },
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
          .alpha(if (items.isEmpty() || initialScrollDone) 1f else 0f)
          .padding(horizontal = 16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
        contentPadding = PaddingValues(top = 10.dp, bottom = 118.dp),
      ) {
        // History load error banner
        if (historyLoadError != null) {
          item {
            androidx.compose.material3.Surface(
              modifier = Modifier.fillMaxWidth(),
              shape = RoundedCornerShape(10.dp),
              color = MaterialTheme.colorScheme.errorContainer,
            ) {
              Row(
                Modifier.padding(horizontal = 12.dp, vertical = 10.dp),
                horizontalArrangement = Arrangement.spacedBy(8.dp),
                verticalAlignment = Alignment.CenterVertically,
              ) {
                Text(
                  text = "Could not load history: $historyLoadError",
                  style = MaterialTheme.typography.labelSmall,
                  color = MaterialTheme.colorScheme.onErrorContainer,
                  modifier = Modifier.weight(1f),
                )
                androidx.compose.material3.TextButton(
                  onClick = { viewModel.loadOlderHistory() },
                  contentPadding = PaddingValues(horizontal = 8.dp, vertical = 0.dp),
                ) {
                  Text("Retry", style = MaterialTheme.typography.labelSmall)
                }
              }
            }
          }
        }

        if (items.isEmpty() && !hasOlderHistory && historyLoadError == null) {
          item {
            ChatEmptyState(
              isLoading = loadingOlderHistory,
              modifier = Modifier.padding(top = 80.dp),
            )
          }
        } else {
        if (hasOlderHistory) {
          item(key = "load-older") {
            androidx.compose.material3.OutlinedButton(
              onClick = viewModel::loadOlderHistory,
              enabled = !loadingOlderHistory,
              modifier = Modifier.fillMaxWidth(),
            ) { Text(if (loadingOlderHistory) "Loading older messages…" else "Load older messages") }
          }
        }
        itemsIndexed(
          items,
          key = { index, item ->
            itemKeys.getOrNull(index) ?: "timeline-fallback-$index-${item.hashCode()}"
          },
          contentType = { _, item ->
            when (item) {
              is SessionTimelineItem.Chat -> "chat"
              is SessionTimelineItem.Tool -> "tool"
              is SessionTimelineItem.FileChange -> "file"
              is SessionTimelineItem.System -> "system"
            }
          }) { index, item ->
          when (item) {
            is SessionTimelineItem.Chat -> ChatBubble(
              author = item.author,
              text = item.text,
              time = item.time,
              isUser = item.isUser,
              imageUris = item.imageUris,
              streaming = !item.isUser && item.order == streamingOrder,
              modifier = Modifier.animateItem(),
            )
            is SessionTimelineItem.Tool -> ToolEventRow(item, modifier = Modifier.animateItem())
            is SessionTimelineItem.FileChange -> FileChangeRow(item)
            is SessionTimelineItem.System -> SystemMessageRow(item)
          }
        }
        if (gitChanges.isNotEmpty()) {
          item(key = "changed-files") {
            ChangedFilesCard(gitChanges)
          }
        }
        } // end else (items not empty)
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

      // Scroll-to-bottom FAB — appears when user scrolls up
      val showScrollDown by remember {
        derivedStateOf {
          val lastVisible = listState.layoutInfo.visibleItemsInfo.lastOrNull()?.index ?: -1
          items.isNotEmpty() && lastVisible < timelineEndIndex - 3
        }
      }
      androidx.compose.animation.AnimatedVisibility(
        visible = showScrollDown,
        enter = androidx.compose.animation.fadeIn() + androidx.compose.animation.scaleIn(
          initialScale = 0.8f,
        ),
        exit = androidx.compose.animation.fadeOut() + androidx.compose.animation.scaleOut(
          targetScale = 0.8f,
        ),
        modifier = Modifier
          .align(Alignment.BottomEnd)
          .padding(end = 16.dp, bottom = 140.dp),
      ) {
        androidx.compose.material3.FloatingActionButton(
          onClick = {
            scope.launch {
              listState.animateScrollToItem(timelineEndIndex)
            }
          },
          containerColor = MaterialTheme.colorScheme.primaryContainer,
          contentColor = MaterialTheme.colorScheme.onPrimaryContainer,
          modifier = Modifier.size(40.dp),
          shape = androidx.compose.foundation.shape.CircleShape,
        ) {
          Icon(
            imageVector = Icons.Default.KeyboardArrowDown,
            contentDescription = "Scroll to latest",
            modifier = Modifier.size(20.dp),
          )
        }
      }
    }
  }
}
