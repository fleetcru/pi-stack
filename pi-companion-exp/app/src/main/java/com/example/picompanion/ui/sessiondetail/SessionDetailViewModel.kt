package com.example.picompanion.ui.sessiondetail

import android.Manifest
import android.app.Application
import android.content.Context
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.Network
import android.net.Uri
import android.graphics.Bitmap
import android.graphics.BitmapFactory
import android.util.Base64
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.settings.SettingsDataStore
import com.example.picompanion.data.repository.SessionsRepository
import com.example.picompanion.data.websocket.SessionEventSocket
import com.example.picompanion.data.websocket.SocketEvent
import kotlinx.coroutines.Job
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.ByteArrayOutputStream
import java.util.UUID
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.booleanOrNull
import com.example.picompanion.BuildConfig

class SessionDetailViewModel(
  application: Application,
  private val sessionId: String,
) : AndroidViewModel(application) {

  private val settingsDataStore = SettingsDataStore(application)
  private val socket = SessionEventSocket()
  private val client = com.example.picompanion.data.api.PiServerClient()
  private val repository = SessionsRepository(client, settingsDataStore)

  private val _items = MutableStateFlow<List<SessionTimelineItem>>(emptyList())
  val items: StateFlow<List<SessionTimelineItem>> = _items.asStateFlow()

  private val _connectionState = MutableStateFlow<ConnectionState>(ConnectionState.Connecting)
  val connectionState: StateFlow<ConnectionState> = _connectionState.asStateFlow()

  private val _sendState = MutableStateFlow<SendState>(SendState.Idle)
  val sendState: StateFlow<SendState> = _sendState.asStateFlow()
  private val _agentWorking = MutableStateFlow(false)
  val agentWorking: StateFlow<Boolean> = _agentWorking.asStateFlow()

  private val _sessionTitle = MutableStateFlow("")
  val sessionTitle: StateFlow<String> = _sessionTitle.asStateFlow()
  private val _sessionProject = MutableStateFlow("")
  val sessionProject: StateFlow<String> = _sessionProject.asStateFlow()
  private val _modelControls = MutableStateFlow(ModelControls())
  val modelControls: StateFlow<ModelControls> = _modelControls.asStateFlow()
  private val _relayHealth = MutableStateFlow<RelayHealth?>(null)
  val relayHealth: StateFlow<RelayHealth?> = _relayHealth.asStateFlow()

  private val _extensionRequest = MutableStateFlow<ExtensionUiRequest?>(null)
  val extensionRequest: StateFlow<ExtensionUiRequest?> = _extensionRequest.asStateFlow()
  private val _gitOutput = MutableStateFlow<Pair<String, String>?>(null)
  val gitOutput: StateFlow<Pair<String, String>?> = _gitOutput.asStateFlow()

  private val _sessionCwd = MutableStateFlow("")
  val sessionCwd: StateFlow<String> = _sessionCwd.asStateFlow()
  private val _hasOlderHistory = MutableStateFlow(false)
  val hasOlderHistory: StateFlow<Boolean> = _hasOlderHistory.asStateFlow()
  private val _loadingOlderHistory = MutableStateFlow(false)
  val loadingOlderHistory: StateFlow<Boolean> = _loadingOlderHistory.asStateFlow()
  private var nextHistoryOffset = 0
  private var historicalItems: List<SessionTimelineItem> = emptyList()
  private val historyMutex = kotlinx.coroutines.sync.Mutex()

  private var lastEventId: Long = 0
  private var lastSentPrompt: String? = null
  private var activeServer: com.example.picompanion.data.settings.ServerEntry? = null
  private var reconnectJob: Job? = null
  private var relayHealthJob: Job? = null
  private var reconnectAttempt = 0
  private var closed = false
  private var pendingPromptId: String? = null
  private val queuedPrompts = java.util.ArrayDeque<String>()
  private val connectivityManager = application.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
  private var networkCallbackRegistered = false
  private val networkCallback = object : ConnectivityManager.NetworkCallback() {
    override fun onAvailable(network: Network) {
      // Cancel any pending backoff delay so reconnection starts immediately
      // when Wi-Fi returns, instead of waiting for the timer to fire.
      reconnectJob?.cancel()
      reconnectJob = null
      scheduleReconnect()
    }
    override fun onLost(network: Network) {
      // Wi-Fi lost: mark disconnected and stop pending reconnect attempts.
      // The next onAvailable will trigger reconnection when Wi-Fi returns.
      reconnectJob?.cancel()
      reconnectJob = null
      _connectionState.value = ConnectionState.Disconnected("Network lost")
    }
  }
  // Pi emits a separate message_update for every text token. Keep one chat row
  // and append deltas to it rather than rendering a bubble per token.
  private var receivedAssistantTextInMessage = false
  // A new Pi message_start must always create a new assistant row. Without
  // this boundary, consecutive turns were merged into one giant response.
  private var assistantTextOpen = false
  private val pendingAssistantDeltas = StringBuilder()
  private var assistantFlushJob: Job? = null

  init {
    // ACCESS_NETWORK_STATE is a normal manifest permission, but keep the
    // session screen usable when an older APK is still installed or a device
    // policy strips it. Reconnection can still be requested manually.
    if (application.checkSelfPermission(Manifest.permission.ACCESS_NETWORK_STATE) == PackageManager.PERMISSION_GRANTED) {
      connectivityManager.registerDefaultNetworkCallback(networkCallback)
      networkCallbackRegistered = true
    }
    connect()
  }

  private fun connect() {
    viewModelScope.launch {
      _connectionState.value = ConnectionState.Connecting
      val appSettings = settingsDataStore.settingsFlow.first()
      val server = appSettings.activeServer

      if (server == null || !server.isConfigured) {
        _connectionState.value = ConnectionState.Error("No server configured")
        return@launch
      }

      activeServer = server
      loadMetadata()
      relayHealthJob = launch {
        // Poll once to check if this is an external (relay) session. If not,
        // skip the polling loop entirely — local RPC sessions get state from
        // the WebSocket stream, not HTTP polling.
        refreshRelayHealth()
        if (_relayHealth.value != null) {
          while (!closed) {
            delay(5_000)
            refreshRelayHealth()
          }
        }
      }

      launch {
        socket.events.collect { event ->
          when (event) {
            is SocketEvent.Connected -> {
              reconnectJob?.cancel()
              reconnectAttempt = 0
              _connectionState.value = ConnectionState.Connected
              flushQueuedPrompts()
            }
            is SocketEvent.EventsLost -> {
              _items.value = _items.value + SessionTimelineItem.System("Connection missed session events; restoring conversation history")
              // Full replay plus persisted JSONL history reconciles any gap
              // caused by a bounded server ring or slow subscriber.
              activeServer?.let { server ->
                viewModelScope.launch { openTicketedStream(server, null) }
                viewModelScope.launch { loadHistory() }
              }
            }
            is SocketEvent.Disconnected -> {
              _connectionState.value = ConnectionState.Disconnected(event.reason)
              scheduleReconnect()
            }
            is SocketEvent.Error -> {
              _connectionState.value = ConnectionState.Error(event.message)
              scheduleReconnect()
            }
            is SocketEvent.Message -> {
              lastEventId = event.eventId ?: lastEventId
              handleEvent(event.raw, event.type)
            }
            is SocketEvent.RawMessage -> {
              _items.value = _items.value + SessionTimelineItem.System(event.text)
            }
          }
        }
      }

      // Open the low-latency stream first. A fresh Pi process can take several
      // seconds to answer get_messages; waiting for history here made mobile
      // interaction appear disconnected even though the server was reachable.
      openTicketedStream(server, Long.MAX_VALUE)
      launch { loadHistory() }
    }
  }

  private suspend fun openTicketedStream(
    server: com.example.picompanion.data.settings.ServerEntry,
    since: Long?,
  ) {
    when (val ticket = withContext(Dispatchers.IO) {
      client.issueWebSocketTicket(server, sessionId)
    }) {
      is com.example.picompanion.data.api.HttpResult.Success ->
        socket.connect(server, ticket.value.ws, since)
      is com.example.picompanion.data.api.HttpResult.Failure -> {
        _connectionState.value = ConnectionState.Error(ticket.message)
        scheduleReconnect()
      }
    }
  }

  fun loadOlderHistory() {
    if (!_hasOlderHistory.value || _loadingOlderHistory.value) return
    viewModelScope.launch { loadHistory(nextHistoryOffset, appendOld = true) }
  }

  private suspend fun loadHistory(offset: Int = 0, appendOld: Boolean = false) {
    // Prevent concurrent loads from interleaving and creating duplicate items.
    if (!historyMutex.tryLock()) return
    try {
    if (appendOld) _loadingOlderHistory.value = true
    when (val result = repository.getSessionMessages(sessionId, offset = offset)) {
      is com.example.picompanion.data.api.HttpResult.Success -> {
        val messages = result.value["data"]?.jsonObject?.get("messages") as? JsonArray
        if (messages != null) {
          val history = messages.mapNotNull { element ->
            val message = element as? JsonObject ?: return@mapNotNull null
            val role = message.getString("role") ?: return@mapNotNull null
            if (role != "user" && role != "assistant") return@mapNotNull null
            val text = message.findText()?.trim()?.takeIf { it.isNotEmpty() } ?: return@mapNotNull null
            SessionTimelineItem.Chat(
              author = if (role == "user") "You" else "Pi Agent",
              text = text,
              time = message.getString("timestamp").orEmpty(),
              isUser = role == "user",
            )
          }
          val historyMeta = result.value["data"]?.jsonObject?.get("history")?.jsonObject
          _hasOlderHistory.value = historyMeta?.get("hasOlder")?.jsonPrimitive?.booleanOrNull == true
          nextHistoryOffset = historyMeta?.get("nextOffset")?.jsonPrimitive?.intOrNull ?: 0
          // Keep a canonical history list so refreshes do not discard older
          // pages, while live events remain at the bottom during reconnects.
          val live = _items.value.filter { item -> item !in historicalItems }
          historicalItems = if (appendOld) history + historicalItems else history
          _items.value = historicalItems + live
        }
      }
      is com.example.picompanion.data.api.HttpResult.Failure -> {
        android.util.Log.w("SessionWS", "Could not load session history: ${result.message}")
      }
    }
    if (appendOld) _loadingOlderHistory.value = false
    } finally {
      historyMutex.unlock()
    }
  }

  private suspend fun refreshRelayHealth() {
    val server = activeServer ?: return
    when (val result = withContext(Dispatchers.IO) { client.getSessionState(server, sessionId) }) {
      is com.example.picompanion.data.api.HttpResult.Success -> {
        val state = result.value["data"]?.jsonObject
        val external = state?.get("external")?.jsonPrimitive?.booleanOrNull == true
        _relayHealth.value = if (external) RelayHealth(
          connected = state?.get("relayConnected")?.jsonPrimitive?.booleanOrNull == true,
          latencyMs = state?.get("relayLatencyMs")?.jsonPrimitive?.longOrNull,
        ) else null
      }
      is com.example.picompanion.data.api.HttpResult.Failure -> Unit
    }
  }

  private suspend fun loadMetadata() {
    when (val sessions = repository.listSessions()) {
      is com.example.picompanion.data.api.HttpResult.Success -> {
        sessions.value.firstOrNull { it.id == sessionId }?.let {
          _sessionTitle.value = it.title.orEmpty()
          _sessionProject.value = it.project.orEmpty()
          _sessionCwd.value = it.cwd.orEmpty()
        }
      }
      is com.example.picompanion.data.api.HttpResult.Failure -> Unit
    }
  }

  private fun scheduleReconnect(immediate: Boolean = false) {
    if (closed || reconnectJob?.isActive == true) return
    val server = activeServer ?: return
    reconnectJob = viewModelScope.launch {
      val baseDelays = longArrayOf(1_000, 2_000, 5_000, 10_000, 15_000, 30_000)
      if (!immediate) {
        val base = baseDelays[reconnectAttempt.coerceAtMost(baseDelays.lastIndex)]
        // Add ±20% jitter to prevent thundering-herd when multiple clients
        // reconnect simultaneously after an outage.
        val jitter = (base * 0.2 * (Math.random() * 2 - 1)).toLong()
        delay((base + jitter).coerceAtLeast(500))
      }
      reconnectAttempt++
      _connectionState.value = ConnectionState.Connecting
      // Replay missed events immediately. Do not block reconnection on a full
      // get_messages RPC round trip; the timeline already has its history.
      openTicketedStream(server, lastEventId.takeIf { it > 0 })
    }
  }

  private fun handleEvent(raw: JsonObject, type: String) {
    // Log all event types for debugging
    if (BuildConfig.DEBUG) android.util.Log.d("SessionWS", "Event type: $type, keys: ${raw.keys}")

    val item = when (type) {
      // Pi RPC streams assistant output as nested message_update events.
      "message_start" -> {
        val message = raw["message"]?.jsonObject
        val role = message?.getString("role")
        if (role == "assistant") {
          _agentWorking.value = true
          _sendState.value = SendState.Running
          receivedAssistantTextInMessage = false
          assistantTextOpen = false
          return
        }
        if (role == "user") {
          // User sent a prompt from the TUI — show it in the timeline
          val text = message?.findText()?.takeIf { it.isNotBlank() } ?: return
          // Deduplicate against the optimistic insert from sendPrompt().
          // The WS echo arrives with a server timestamp while the optimistic
          // item uses "now"; text comparison avoids showing the same message twice.
          if (text == lastSentPrompt) {
            lastSentPrompt = null
            return
          }
          SessionTimelineItem.Chat(
            author = "You",
            text = text,
            time = formatTime(raw),
            isUser = true,
          )
        } else {
          return
        }
      }
      "message_update" -> {
        val update = raw["assistantMessageEvent"]?.jsonObject ?: return
        when (update.getString("type")) {
          "text_start" -> receivedAssistantTextInMessage = false
          "text_delta" -> update.getString("delta")?.let(::appendAssistantDelta)
          // text_end contains the entire content, which we have already built
          // from deltas; appending it would duplicate the response.
        }
        return
      }
      "message_end" -> {
        val message = raw["message"]?.jsonObject ?: return
        // Pi emits message_end for the user's accepted prompt before agent
        // work begins. Resetting here hid the Abort button immediately.
        if (message.getString("role") != "assistant") return
        _sendState.value = SendState.Idle
        _agentWorking.value = false
        assistantTextOpen = false
        // Some providers do not emit text_delta. Show their final text once.
        if (!receivedAssistantTextInMessage) {
          message.findText()?.takeIf { it.isNotBlank() }?.let(::appendAssistantMessage)
        }
        return
      }
      // Compatibility with older/custom event names.
      "assistant_text", "assistant", "assistant_message", "text", "output", "chunk", "delta" -> {
        _sendState.value = SendState.Idle
        val text = raw.getString("text") ?: raw.getString("message") ?: raw.getString("content")
          ?: raw.getString("output") ?: raw.getString("chunk") ?: return
        SessionTimelineItem.Chat(
          author = "Pi Agent",
          text = text,
          time = formatTime(raw),
          isUser = false,
        )
      }
      // User messages
      "user_text", "user", "user_message", "prompt", "input" -> {
        val text = raw.getString("message") ?: raw.getString("text") ?: raw.getString("content") ?: return
        SessionTimelineItem.Chat(
          author = "You",
          text = text,
          time = formatTime(raw),
          isUser = true,
        )
      }
      // Tool events use toolCallId; update the existing card rather than
      // adding a misleading start and completion row for the same operation.
      "tool_execution_start", "tool_use", "tool_start" -> {
        _agentWorking.value = true
        val name = raw.getString("toolName") ?: raw.getString("name") ?: raw.getString("tool") ?: "tool"
        SessionTimelineItem.Tool(
          callId = raw.getString("toolCallId") ?: "tool-${System.nanoTime()}",
          name = name,
          status = "running",
          args = raw["args"]?.toString(),
        )
      }
      "tool_execution_update" -> {
        updateTool(
          callId = raw.getString("toolCallId"),
          output = raw["partialResult"]?.findText(),
          status = "running",
        )
        return
      }
      "tool_execution_end", "tool_result", "tool_end" -> {
        val callId = raw.getString("toolCallId")
        val isError = raw["isError"]?.toString() == "true" || raw.getString("success") == "false"
        updateTool(
          callId = callId,
          output = raw["result"]?.findText() ?: raw.getString("error"),
          status = if (isError) "failed" else "completed",
        )
        return
      }
      // File changes
      "file_change", "file_write", "file_edit" -> {
        val path = raw.getString("path") ?: raw.getString("file") ?: return
        val op = raw.getString("operation") ?: raw.getString("type") ?: "modified"
        SessionTimelineItem.FileChange(path = path, operation = op)
      }
      "extension_ui_request" -> {
        // Only Pi dialog methods need a client response. Status, widget, and
        // notification events share this RPC type but are verbose output.
        if (raw["_daemonExtensionUiRequiresResponse"]?.toString() != "true") return
        val id = raw.getString("id") ?: return
        _extensionRequest.value = ExtensionUiRequest(id, raw.getString("message") ?: raw.getString("title") ?: "Extension input requested", raw.getString("placeholder"))
        return
      }
      "bridge_receipt" -> {
        _sendState.value = SendState.Delivered
        return
      }
      "agent_start", "agent_end", "agent_settled", "turn_start", "turn_end" -> {
        if (type == "agent_start" && pendingPromptId != null) {
          _sendState.value = SendState.Running
          _agentWorking.value = true
        }
        if (type == "agent_settled" || type == "agent_end") {
          pendingPromptId = null
          _sendState.value = SendState.Idle
          _agentWorking.value = false
        }
        return
      }
      // Daemon events
      "daemon_error", "daemon_start", "daemon_exit" -> {
        _sendState.value = SendState.Idle
        val text = raw.getString("error") ?: raw.getString("message") ?: type
        SessionTimelineItem.System(text)
      }
      // Response to commands
      "response" -> {
        val responseId = raw.getString("id")
        if (responseId == null || responseId != pendingPromptId) return
        if (raw.getString("success") == "false") {
          val error = raw.getString("error") ?: "Prompt rejected"
          pendingPromptId = null
          _sendState.value = SendState.Failed(error)
          _items.value = _items.value + SessionTimelineItem.System("Prompt failed: $error")
          return
        }
        _sendState.value = SendState.Accepted
        return
      }
      // Do not flood the conversation with daemon/extension payloads the app
      // does not render yet. They remain available in Logcat for diagnosis.
      else -> return
    }
    _items.value = _items.value + item
  }

  private data class ToolUpdate(val callId: String?, val output: String?, val status: String)
  private val pendingToolUpdates = mutableListOf<ToolUpdate>()
  private var toolFlushJob: Job? = null

  private fun updateTool(callId: String?, output: String?, status: String) {
    pendingToolUpdates.add(ToolUpdate(callId, output, status))
    if (toolFlushJob?.isActive == true) return
    // Batch tool updates at 16ms cadence to avoid Compose recomposition
    // churn during rapid tool_execution_update events.
    toolFlushJob = viewModelScope.launch {
      delay(16)
      val batch = pendingToolUpdates.toList()
      pendingToolUpdates.clear()
      val items = _items.value.toMutableList()
      for (update in batch) {
        val index = items.indexOfLast { it is SessionTimelineItem.Tool && it.callId == update.callId }
        if (index < 0) continue
        val current = items[index] as SessionTimelineItem.Tool
        items[index] = current.copy(status = update.status, output = update.output ?: current.output)
      }
      _items.value = items
    }
  }

  private fun appendAssistantDelta(delta: String) {
    if (delta.isEmpty()) return
    receivedAssistantTextInMessage = true
    pendingAssistantDeltas.append(delta)
    if (assistantFlushJob?.isActive == true) return
    // Pi can emit one event per token. Render at frame-ish cadence instead of
    // recomposing the entire Compose timeline for every individual token.
    assistantFlushJob = viewModelScope.launch {
      delay(16)
      val buffered = pendingAssistantDeltas.toString()
      pendingAssistantDeltas.clear()
      if (buffered.isEmpty()) return@launch
      val last = _items.value.lastOrNull()
      if (assistantTextOpen && last is SessionTimelineItem.Chat && !last.isUser && last.author == "Pi Agent") {
        _items.value = _items.value.dropLast(1) + last.copy(text = last.text + buffered)
      } else {
        appendAssistantMessage(buffered)
        assistantTextOpen = true
      }
    }
  }

  private fun appendAssistantMessage(text: String) {
    _items.value = _items.value + SessionTimelineItem.Chat(
      author = "Pi Agent",
      text = text,
      time = "",
      isUser = false,
    )
  }

  private fun formatTime(raw: JsonObject): String {
    return raw.getString("timestamp") ?: raw.getString("time") ?: ""
  }

  private fun JsonObject.getString(key: String): String? {
    val element = this[key] ?: return null
    return (element as? JsonPrimitive)?.contentOrNull
  }

  /** Extract text from Pi's nested message/content structures. */
  private fun JsonElement.findText(): String? = when (this) {
    is JsonPrimitive -> if (isString) contentOrNull else null
    is JsonObject -> getString("text") ?: getString("content") ?: getString("delta")
      ?: getString("message") ?: this["content"]?.findText()
    is JsonArray -> joinToString("") { it.findText().orEmpty() }.ifEmpty { null }
    else -> null
  }

  // ── User actions ─────────────────────────────────────

  fun sendPrompt(message: String, imageUris: List<Uri> = emptyList()) {
    if (message.isBlank() && imageUris.isEmpty()) return
    if (imageUris.isNotEmpty()) {
      val server = activeServer ?: return
      _items.value = _items.value + SessionTimelineItem.Chat(
        author = "You", text = message, time = "now", isUser = true, imageUris = imageUris,
      )
      viewModelScope.launch {
        _sendState.value = SendState.Sending
        val images = withContext(Dispatchers.IO) {
          imageUris.mapNotNull { uri ->
            try {
              val context = getApplication<Application>()
              val mimeType = context.contentResolver.getType(uri) ?: "image/jpeg"
              // Downsample large images to prevent OOM and reduce payload size.
              val maxDimension = 1024
              val options = BitmapFactory.Options().apply { inJustDecodeBounds = true }
              context.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, options) }
              val scale = maxOf(1, maxOf(options.outWidth, options.outHeight) / maxDimension)
              val decodeOptions = BitmapFactory.Options().apply { inSampleSize = scale }
              val bitmap = context.contentResolver.openInputStream(uri)?.use {
                BitmapFactory.decodeStream(it, null, decodeOptions)
              } ?: return@mapNotNull null
              val baos = ByteArrayOutputStream()
              bitmap.compress(Bitmap.CompressFormat.JPEG, 85, baos)
              bitmap.recycle()
              val bytes = baos.toByteArray()
              com.example.picompanion.data.api.PromptImage(
                base64 = Base64.encodeToString(bytes, Base64.NO_WRAP),
                mimeType = mimeType,
              )
            } catch (_: Exception) { null }
          }
        }
        when (val result = withContext(Dispatchers.IO) { client.sendPrompt(server, sessionId, message, images) }) {
          is com.example.picompanion.data.api.HttpResult.Success -> _sendState.value = SendState.Accepted
          is com.example.picompanion.data.api.HttpResult.Failure -> _sendState.value = SendState.Failed(result.userMessage)
        }
      }
      return
    }
    if (_connectionState.value is ConnectionState.Connecting) {
      // Previously the prompt was silently discarded here. Queue it and flush
      // once the stream connects so slow networks don't eat typed messages.
      if (queuedPrompts.size < 20) queuedPrompts.add(message)
      _items.value = _items.value + SessionTimelineItem.System("Connecting — message will be sent when the session is reachable")
      return
    }

    val requestId = "companion-${UUID.randomUUID()}"
    pendingPromptId = requestId
    _sendState.value = SendState.Sending
    lastSentPrompt = message

    // Optimistic: add user message to timeline immediately
    _items.value = _items.value + SessionTimelineItem.Chat(
      author = "You",
      text = message,
      time = "now",
      isUser = true,
    )

    // REST is the authoritative route for both local RPC and bridged TUI
    // sessions. Raw session WebSocket commands bypassed the relay queue on
    // some reconnect paths, making Companion prompts appear to disappear.
    viewModelScope.launch {
      when (val result = repository.sendPrompt(sessionId, message)) {
        is com.example.picompanion.data.api.HttpResult.Success -> _sendState.value = SendState.Accepted
        is com.example.picompanion.data.api.HttpResult.Failure -> {
          pendingPromptId = null
          _sendState.value = SendState.Idle
          _items.value = _items.value + SessionTimelineItem.System("Could not send prompt: ${result.message}")
        }
      }
    }
  }

  private fun flushQueuedPrompts() {
    viewModelScope.launch {
      while (true) {
        val message = queuedPrompts.pollFirst() ?: break
        sendPrompt(message)
        // Small delay between queued sends to avoid overwhelming the server
        // and to show a more natural send cadence.
        delay(100)
      }
    }
  }

  fun sendSteer(message: String) {
    if (message.isBlank()) return
    if (_connectionState.value !is ConnectionState.Connected) return

    _sendState.value = SendState.Sending
    // Route through REST API for consistent behavior across local RPC and
    // relay sessions. Raw WebSocket commands bypass the relay queue.
    viewModelScope.launch {
      when (val result = repository.sendSteer(sessionId, message)) {
        is com.example.picompanion.data.api.HttpResult.Success -> _sendState.value = SendState.Accepted
        is com.example.picompanion.data.api.HttpResult.Failure -> {
          _sendState.value = SendState.Idle
          _items.value = _items.value + SessionTimelineItem.System("Could not steer: ${result.message}")
        }
      }
    }
  }

  fun abort() {
    val server = activeServer ?: return
    viewModelScope.launch {
      when (val result = withContext(Dispatchers.IO) { client.postSessionAction(server, sessionId, "abort") }) {
        is com.example.picompanion.data.api.HttpResult.Success -> {
          _sendState.value = SendState.Idle
          _agentWorking.value = false
          _items.value = _items.value + SessionTimelineItem.System("Stop requested")
        }
        is com.example.picompanion.data.api.HttpResult.Failure -> _items.value = _items.value + SessionTimelineItem.System("Could not stop Pi: ${result.message}")
      }
    }
  }

  fun compact() {
    // Route through REST like abort: the raw WS command is silently ignored
    // for relay (external) sessions, whereas the REST path returns a real
    // error we can surface.
    val server = activeServer ?: return
    viewModelScope.launch {
      when (val result = withContext(Dispatchers.IO) { client.postSessionAction(server, sessionId, "compact") }) {
        is com.example.picompanion.data.api.HttpResult.Success ->
          _items.value = _items.value + SessionTimelineItem.System("Compacting…")
        is com.example.picompanion.data.api.HttpResult.Failure ->
          _items.value = _items.value + SessionTimelineItem.System("Could not compact: ${result.message}")
      }
    }
  }

  fun updateMetadata(title: String, project: String) {
    val server = activeServer ?: return
    viewModelScope.launch {
      when (val result = withContext(Dispatchers.IO) {
        client.updateSessionMetadata(server, sessionId, title, project)
      }) {
        is com.example.picompanion.data.api.HttpResult.Success -> {
          _sessionTitle.value = title
          _sessionProject.value = project
          _items.value = _items.value + SessionTimelineItem.System("Session details saved")
        }
        is com.example.picompanion.data.api.HttpResult.Failure ->
          _items.value = _items.value + SessionTimelineItem.System("Could not save session details: ${result.userMessage}")
      }
    }
  }

  fun runSessionAction(action: String, body: String = "{}") {
    val server = activeServer ?: return
    viewModelScope.launch {
      when (val result = withContext(Dispatchers.IO) {
        client.postSessionAction(server, sessionId, action, body)
      }) {
        is com.example.picompanion.data.api.HttpResult.Success ->
          _items.value = _items.value + SessionTimelineItem.System("${action.replace('-', ' ')} requested")
        is com.example.picompanion.data.api.HttpResult.Failure ->
          _items.value = _items.value + SessionTimelineItem.System("Could not ${action.replace('-', ' ')}: ${result.userMessage}")
      }
    }
  }

  fun loadModelControls() {
    val server = activeServer ?: return
    viewModelScope.launch {
      val (modelsResult, stateResult) = withContext(Dispatchers.IO) {
        client.getSessionModels(server, sessionId) to client.getSessionState(server, sessionId)
      }
      val models = (modelsResult as? com.example.picompanion.data.api.HttpResult.Success)?.value
        ?.get("data")?.jsonObject?.get("models") as? JsonArray
      val choices = models?.mapNotNull { raw ->
        val model = raw as? JsonObject ?: return@mapNotNull null
        val provider = model.getString("provider") ?: return@mapNotNull null
        val id = model.getString("id") ?: return@mapNotNull null
        ModelChoice(provider, id, model.getString("name") ?: id)
      } ?: emptyList()
      val state = (stateResult as? com.example.picompanion.data.api.HttpResult.Success)?.value?.get("data")?.jsonObject
      _modelControls.value = ModelControls(
        models = choices,
        selectedProvider = state?.get("model")?.jsonObject?.getString("provider"),
        selectedModelId = state?.get("model")?.jsonObject?.getString("id"),
        thinkingLevel = state?.getString("thinkingLevel"),
      )
    }
  }

  fun setModel(provider: String, modelId: String) {
    val server = activeServer ?: return
    viewModelScope.launch {
      withContext(Dispatchers.IO) { client.setSessionModel(server, sessionId, provider, modelId) }
      loadModelControls()
    }
  }

  fun setThinkingLevel(level: String) {
    val server = activeServer ?: return
    viewModelScope.launch {
      withContext(Dispatchers.IO) { client.setThinkingLevel(server, sessionId, level) }
      loadModelControls()
    }
  }

  fun showGit(resource: String) {
    val server = activeServer ?: return
    viewModelScope.launch {
      when (val result = withContext(Dispatchers.IO) { client.getSessionGit(server, sessionId, resource) }) {
        is com.example.picompanion.data.api.HttpResult.Success ->
          _gitOutput.value = resource.replaceFirstChar { it.uppercase() } to (result.value["output"]?.toString()?.trim('"') ?: "No Git output")
        is com.example.picompanion.data.api.HttpResult.Failure ->
          _gitOutput.value = "Git error" to result.userMessage
      }
    }
  }

  fun closeGitOutput() {
    _gitOutput.value = null
  }

  fun writeGit(action: String, body: JsonObject) {
    val server = activeServer ?: return
    viewModelScope.launch {
      when (val result = withContext(Dispatchers.IO) { client.writeSessionGit(server, sessionId, action, body) }) {
        is com.example.picompanion.data.api.HttpResult.Success -> _gitOutput.value = action.replaceFirstChar { it.uppercase() } to (result.value["output"]?.toString()?.trim('"') ?: "Completed")
        is com.example.picompanion.data.api.HttpResult.Failure -> _gitOutput.value = "Git error" to result.userMessage
      }
    }
  }

  /** Hides a replayed/stale extension request without sending an approval. */
  fun ignoreExtensionRequest() {
    _extensionRequest.value = null
  }

  fun respondToExtension(value: String? = null, confirmed: Boolean? = null, cancelled: Boolean = false) {
    val request = _extensionRequest.value ?: return
    val server = activeServer ?: return
    viewModelScope.launch {
      withContext(Dispatchers.IO) { client.respondToExtensionUi(server, sessionId, request.id, value, confirmed, cancelled) }
      _extensionRequest.value = null
    }
  }

  fun activeServerForUi(): com.example.picompanion.data.settings.ServerEntry? = activeServer

  fun reconnect() {
    reconnectJob?.cancel()
    socket.disconnect()
    scheduleReconnect(immediate = true)
  }

  override fun onCleared() {
    closed = true
    reconnectJob?.cancel()
    relayHealthJob?.cancel()
    if (networkCallbackRegistered) connectivityManager.unregisterNetworkCallback(networkCallback)
    socket.disconnect()
    super.onCleared()
  }

  companion object {
    fun factory(application: Application, sessionId: String): ViewModelProvider.Factory {
      return object : ViewModelProvider.Factory {
        @Suppress("UNCHECKED_CAST")
        override fun <T : ViewModel> create(modelClass: Class<T>): T {
          return SessionDetailViewModel(application, sessionId) as T
        }
      }
    }
  }
}

data class RelayHealth(val connected: Boolean, val latencyMs: Long?)

data class ModelChoice(val provider: String, val id: String, val name: String)
data class ModelControls(
  val models: List<ModelChoice> = emptyList(),
  val selectedProvider: String? = null,
  val selectedModelId: String? = null,
  val thinkingLevel: String? = null,
)

data class ExtensionUiRequest(val id: String, val message: String, val placeholder: String?)

sealed interface ConnectionState {
  data object Connecting : ConnectionState
  data object Connected : ConnectionState
  data class Disconnected(val reason: String) : ConnectionState
  data class Error(val message: String) : ConnectionState
}

sealed interface SendState {
  data object Idle : SendState
  data object Sending : SendState
  data object Accepted : SendState
  data object Delivered : SendState
  data object Running : SendState
  data class Failed(val message: String) : SendState
}

sealed interface SessionTimelineItem {
  data class Chat(
    val author: String,
    val text: String,
    val time: String,
    val isUser: Boolean,
    val imageUris: List<Uri> = emptyList(),
  ) : SessionTimelineItem

  data class Tool(
    val callId: String,
    val name: String,
    val status: String,
    val args: String? = null,
    val output: String? = null,
  ) : SessionTimelineItem

  data class FileChange(
    val path: String,
    val operation: String,
  ) : SessionTimelineItem

  data class System(val text: String) : SessionTimelineItem
}
