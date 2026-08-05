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
import com.example.picompanion.di.AppModule
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
import kotlinx.coroutines.flow.update
import kotlinx.coroutines.sync.withLock
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

  private val settingsDataStore = AppModule.settingsDataStore
  private val client = AppModule.client
  private val socket = SessionEventSocket(client.okHttpClient)
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
  private val _refreshing = MutableStateFlow(false)
  val refreshing: StateFlow<Boolean> = _refreshing.asStateFlow()

  private val _extensionRequest = MutableStateFlow<ExtensionUiRequest?>(null)
  val extensionRequest: StateFlow<ExtensionUiRequest?> = _extensionRequest.asStateFlow()
  private val _gitOutput = MutableStateFlow<Pair<String, String>?>(null)
  val gitOutput: StateFlow<Pair<String, String>?> = _gitOutput.asStateFlow()
  private val _gitChanges = MutableStateFlow<List<GitFileChange>>(emptyList())
  val gitChanges: StateFlow<List<GitFileChange>> = _gitChanges.asStateFlow()

  private val _sessionCwd = MutableStateFlow("")
  val sessionCwd: StateFlow<String> = _sessionCwd.asStateFlow()
  private val _hasOlderHistory = MutableStateFlow(false)
  val hasOlderHistory: StateFlow<Boolean> = _hasOlderHistory.asStateFlow()
  private val _loadingOlderHistory = MutableStateFlow(false)
  val loadingOlderHistory: StateFlow<Boolean> = _loadingOlderHistory.asStateFlow()
  private val _historyLoadError = MutableStateFlow<String?>(null)
  val historyLoadError: StateFlow<String?> = _historyLoadError.asStateFlow()
  private var nextHistoryOffset = 0
  private var historicalItems: List<SessionTimelineItem> = emptyList()
  private val historyMutex = kotlinx.coroutines.sync.Mutex()
  // Generation counter for loadHistory: incremented on each connect() so a
  // stale in-flight history request from a previous connection is discarded.
  private var historyGeneration = 0
  // Monotonic counter to give every timeline item a stable insertion order.
  // Prevents items from appearing out of sequence when history and live WS
  // events merge after reconnect.
  private var timelineSeq: Long = 0
  // Serializes all assistant-bubble state transitions (open/close/append)
  // so rapid events from the socket collector and flush jobs don't interleave.
  private val assistantMutex = kotlinx.coroutines.sync.Mutex()

  private fun appendItem(item: SessionTimelineItem) {
    // Deduplicate before appending — prevents duplicates from history/live overlap
    val id = timelineItemId(item)
    _items.update { current ->
      if (current.any { timelineItemId(it) == id }) current
      else current + withOrder(item)
    }
  }

  /** Stamp an item with a monotonic order value if it doesn't have one yet. */
  private fun withOrder(item: SessionTimelineItem): SessionTimelineItem = when (item) {
    is SessionTimelineItem.Chat -> if (item.order > 0) item else item.copy(order = ++timelineSeq)
    is SessionTimelineItem.Tool -> if (item.order > 0) item else item.copy(order = ++timelineSeq)
    is SessionTimelineItem.FileChange -> if (item.order > 0) item else item.copy(order = ++timelineSeq)
    is SessionTimelineItem.System -> if (item.order > 0) item else item.copy(order = ++timelineSeq)
  }

  @Volatile private var lastEventId: Long = 0
  private var lastSentPrompt: String? = null
  // Tracks recently sent prompts so the WS echo of a user message doesn't
  // create a duplicate in the timeline. Uses requestId as key for robustness.
  private val recentSentPrompts = java.util.Collections.synchronizedMap(object : LinkedHashMap<String, String>(128) {
    override fun removeEldestEntry(eldest: MutableMap.MutableEntry<String, String>?): Boolean = size > 100
  })
  private var activeServer: com.example.picompanion.data.settings.ServerEntry? = null
  private var reconnectJob: Job? = null
  private var relayHealthJob: Job? = null
  private var reconnectAttempt = 0
  @Volatile private var closed = false
  // Turn counter: incremented when message_end or agent_end fires. Prevents
  // stale runtime_state events from a previous turn from re-enabling the
  // thinking spinner after the turn is already complete.
  private var turnCompleteGeneration = 0
  // Set of in-flight request IDs from sendPrompt. The server echoes back a
  // response with matching id; using a Set allows multiple rapid sends
  // without overwriting each other.
  private val pendingPromptIds = java.util.Collections.synchronizedSet(mutableSetOf<String>())
  private val queuedPrompts = PendingPromptQueue()
  private val connectivityManager = application.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager
  private var networkCallbackRegistered = false
  private val networkCallback = object : ConnectivityManager.NetworkCallback() {
    override fun onAvailable(network: Network) {
      // Cancel any pending backoff delay so reconnection starts immediately
      // when Wi-Fi returns, instead of waiting for the timer to fire.
      reconnectJob?.cancel()
      reconnectJob = null
      reconnectAttempt = 0
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
  // Tracks the order value of the current assistant bubble so post-tool
  // deltas don't merge back into a pre-tool bubble.
  private var currentAssistantOrder: Long = 0
  // All access to pendingAssistantDeltas is serialized by assistantMutex.
  private val pendingAssistantDeltas = StringBuilder()
  private var assistantFlushJob: Job? = null
  // Property initializers run before the init block. Keep this buffer above
  // init because connect() clears it immediately when the screen opens.
  // All access is synchronized on the list itself.
  private val pendingToolUpdates = java.util.Collections.synchronizedList(mutableListOf<ToolUpdate>())
  private var toolFlushJob: Job? = null

  init {
    // ACCESS_NETWORK_STATE is a normal manifest permission, but keep the
    // session screen usable when an older APK is still installed or a device
    // policy strips it. Reconnection can still be requested manually.
    // Some OEM ROMs throw SecurityException even with the permission granted.
    if (application.checkSelfPermission(Manifest.permission.ACCESS_NETWORK_STATE) == PackageManager.PERMISSION_GRANTED) {
      try {
        connectivityManager.registerDefaultNetworkCallback(networkCallback)
        networkCallbackRegistered = true
      } catch (_: SecurityException) {
        // OEM restriction — reconnection will rely on manual retry.
      }
    }
    connect()

    // Observe settings changes — if the active server URL or token changes,
    // tear down the current WebSocket and reconnect with the new config.
    var lastServerId = activeServer?.id
    var lastServerUrl = activeServer?.url
    var lastServerToken = activeServer?.authToken
    viewModelScope.launch {
      settingsDataStore.settingsFlow.collect { appSettings ->
        val server = appSettings.activeServer ?: return@collect
        if (server.id != lastServerId || server.url != lastServerUrl || server.authToken != lastServerToken) {
          lastServerId = server.id
          lastServerUrl = server.url
          lastServerToken = server.authToken
          // Only reconnect if we already had a connection (skip the initial emission
          // which is handled by connect() above).
          if (activeServer != null) {
            socket.disconnect()
            connect()
          }
        }
      }
    }
  }

  private fun connect() {
    // A reconnect must not let buffered tokens from the old socket append to
    // the first assistant message received on the new socket.
    assistantFlushJob?.cancel()
    assistantFlushJob = null
    // Flush any remaining assistant deltas from a previous interrupted
    // response (e.g. connection dropped mid-stream) so the partial text
    // is preserved in the timeline instead of being silently discarded.
    synchronized(pendingAssistantDeltas) {
      if (pendingAssistantDeltas.isNotEmpty() && assistantTextOpen) {
        val remaining = pendingAssistantDeltas.toString()
        pendingAssistantDeltas.clear()
        // Append directly — bypass appendItem dedup since this is a
        // continuation of an existing bubble that may not be findable
        // by order anymore.
        val bubble = SessionTimelineItem.Chat("Pi Agent", remaining, "", false)
        val stamped = withOrder(bubble) as SessionTimelineItem.Chat
        _items.update { it + stamped }
      } else {
        pendingAssistantDeltas.clear()
      }
    }
    toolFlushJob?.cancel()
    toolFlushJob = null
    synchronized(pendingToolUpdates) { pendingToolUpdates.clear() }
    assistantTextOpen = false
    receivedAssistantTextInMessage = false
    currentAssistantOrder = 0
    // Reset agent state so the UI doesn't show a stuck spinner after reconnect.
    _agentWorking.value = false
    _sendState.value = SendState.Idle
    historyGeneration++
    // Clear stale extension requests from the previous connection.
    _extensionRequest.value = null

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
      loadGitChanges()
      loadModelControls()
      relayHealthJob?.cancel()
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
              appendItem(SessionTimelineItem.System("Connection missed session events; restoring conversation history"))
              // Full replay plus persisted JSONL history reconciles any gap
              // caused by a bounded server ring or slow subscriber.
              activeServer?.let { server ->
                viewModelScope.launch {
                  // Load fresh history first. The history merge in loadHistory
                  // will reconcile with whatever is currently in _items.
                  // Open the new stream after history is loaded so replay
                  // events don't race with the history merge.
                  loadHistory()
                  openTicketedStream(server, null)
                }
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
              appendItem(SessionTimelineItem.System(event.text))
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
    // Queue concurrent refreshes instead of silently dropping one while a
    // previous history request is still in flight.
    val generation = historyGeneration
    historyMutex.withLock {
      // Skip if a newer connect() has started — this stale request would
      // overwrite fresh history with data from a previous connection.
      if (!appendOld && generation != historyGeneration) return
      if (appendOld) _loadingOlderHistory.value = true
      try {
        when (val result = repository.getSessionMessages(sessionId, offset = offset)) {
          is com.example.picompanion.data.api.HttpResult.Success -> {
            val data = result.value["data"]?.jsonObject
            val messages = data?.get("messages") as? JsonArray
            if (messages != null) {
              // First pass: parse all entries. tool_result entries carry the
              // output of a tool_use, so we collect them separately and merge
              // into the matching tool_use in a second pass.
              val toolResults = mutableMapOf<String, String>() // callId → output text
              // Show first 3 messages with their roles and content types
              messages.take(3).forEachIndexed { i, el ->
                val obj = el as? JsonObject
                val msgObj = obj?.get("message") as? JsonObject
                val role = msgObj?.get("role")?.toString()?.trim('"')
                val content = msgObj?.get("content")
                val contentType = when (content) {
                  is kotlinx.serialization.json.JsonArray -> "array[${content.size}] types=${content.mapNotNull { (it as? JsonObject)?.get("type")?.toString()?.trim('"') }}"
                  is kotlinx.serialization.json.JsonPrimitive -> "string(${content.content.take(50)})"
                  else -> content?.javaClass?.simpleName ?: "null"
                }
              }
              val parsed = mutableListOf<SessionTimelineItem>()
              for (element in messages) {
                val message = element as? JsonObject ?: continue
                val role = message.getString("role") ?: continue
                val historyType = message.getString("_historyType")

                // Handle tool_use entries (standalone format)
                if (historyType == "tool_use") {
                  val name = message.getString("name") ?: "tool"
                  val id = message.getString("id") ?: "tool-${System.nanoTime()}"
                  parsed.add(SessionTimelineItem.Tool(
                    callId = id,
                    name = name,
                    status = "completed",
                    args = message["input"]?.toString(),
                  ))
                  continue
                }
                if (historyType == "tool_result") {
                  val toolUseId = message.getString("tool_use_id") ?: message.getString("id")
                  if (toolUseId != null) {
                    val output = message.findText() ?: message["content"]?.findText()
                    if (output != null) toolResults[toolUseId] = output
                  }
                  continue
                }

                // Handle tool role entries (some servers emit role="tool")
                if (role == "tool") {
                  val toolUseId = message.getString("tool_use_id") ?: message.getString("toolCallId") ?: message.getString("id")
                  if (toolUseId != null) {
                    val output = message.findText() ?: message["content"]?.findText()
                    if (output != null) toolResults[toolUseId] = output
                  }
                  continue
                }
                // Handle toolResult role (Pi/OpenAI format)
                if (role == "toolResult") {
                  val toolUseId = message.getString("toolCallId") ?: message.getString("tool_use_id") ?: message.getString("id")
                  if (toolUseId != null) {
                    val output = message.findText() ?: message["content"]?.findText()
                    if (output != null) toolResults[toolUseId] = output
                  }
                  continue
                }

                if (role != "user" && role != "assistant") continue

                // Check if the message content contains tool call blocks.
                // Pi/OpenAI uses type:"toolCall", Anthropic uses type:"tool_use".
                val content = message["content"]
                if (content is JsonArray) {
                  val toolBlocks = content.filterIsInstance<JsonObject>().filter {
                    val t = it.getString("type")
                    t == "toolCall" || t == "tool_use"
                  }
                  if (toolBlocks.isNotEmpty()) {
                    toolBlocks.forEach { block ->
                      val name = block.getString("name") ?: "tool"
                      val id = block.getString("id") ?: "tool-${System.nanoTime()}"
                      // Pi/OpenAI uses "arguments", Anthropic uses "input"
                      val args = block["arguments"]?.toString() ?: block["input"]?.toString()
                      toolResults[id] = toolResults[id] ?: ""
                      parsed.add(SessionTimelineItem.Tool(
                        callId = id,
                        name = name,
                        status = "completed",
                        args = args,
                      ))
                    }
                  }
                }

                val text = message.findText()?.trim()?.takeIf { it.isNotEmpty() }
                if (text != null) {
                  parsed.add(SessionTimelineItem.Chat(
                    author = if (role == "user") "You" else "Pi Agent",
                    text = text,
                    time = message.getString("timestamp").orEmpty(),
                    isUser = role == "user",
                  ))
                }
              } // end for loop
              // Second pass: merge tool_result output into matching tool_use items.
              val toolCount = parsed.count { it is SessionTimelineItem.Tool }
              val history = parsed.map { item ->
                if (item is SessionTimelineItem.Tool && item.callId in toolResults) {
                  item.copy(output = toolResults[item.callId])
                } else {
                  item
                }
              }.distinctBy { timelineItemId(it) }
              val historyMeta = data["history"]?.jsonObject
              _hasOlderHistory.value = historyMeta?.get("hasOlder")
                ?.jsonPrimitive?.booleanOrNull == true
              nextHistoryOffset = historyMeta?.get("nextOffset")
                ?.jsonPrimitive?.intOrNull ?: 0

              // Preserve already-loaded older pages while keeping live socket
              // items at the bottom of the timeline.
              historicalItems = (if (appendOld) history + historicalItems else history)
                .distinctBy { timelineItemId(it) }
              _historyLoadError.value = null
              // Atomic update prevents live events arriving during the HTTP
              // request from being overwritten by the history snapshot. HTTP
              // history and WS replay use different timestamp formats, so data
              // class equality is not sufficient for cross-source deduplication.
              _items.update { current ->
                val merged = LinkedHashMap<String, SessionTimelineItem>()
                historicalItems.forEach { merged[timelineItemId(it)] = it }
                // Keep local image URIs when the server history only returns the
                // text portion of a multimodal user message.
                current.filter {
                  it is SessionTimelineItem.Chat && it.time == "now" && it.imageUris.isNotEmpty()
                }.forEach { merged[timelineItemId(it)] = it }
                // Keep live items that aren't in history, OR replace history
                // items with richer live versions (e.g. a Tool item from the
                // live stream has running status + partial output that the
                // history version lacks).
                current.filter { item ->
                  val isOptimisticImage = item is SessionTimelineItem.Chat &&
                    item.time == "now" && item.imageUris.isNotEmpty()
                  if (isOptimisticImage) return@filter false
                  when (item) {
                    is SessionTimelineItem.Chat -> true
                    is SessionTimelineItem.Tool -> true
                    is SessionTimelineItem.System -> true
                    is SessionTimelineItem.FileChange -> true
                  }
                }.forEach { item ->
                  val id = timelineItemId(item)
                  val existing = merged[id]
                  // Always keep live Tool items — they have richer data
                  // (running status, partial output) than history versions.
                  if (item is SessionTimelineItem.Tool || existing == null) {
                    merged[id] = item
                  }
                }
                // LinkedHashMap preserves insertion order (chronological from parser).
                // Stamp items with order=0 (from history) so Compose keys are unique,
                // but do NOT re-sort — the insertion order IS the correct order.
                merged.values.map { item ->
                  if (item.order == 0L) withOrder(item) else item
                }
              }
              // Log AFTER the update to confirm items were set
            }
          }
          is com.example.picompanion.data.api.HttpResult.Failure -> {
            android.util.Log.w(
              "SessionWS",
              "Could not load session history: ${result.message}",
            )
            _historyLoadError.value = result.message
          }
        }
      } finally {
        if (appendOld) _loadingOlderHistory.value = false
      }
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
      is com.example.picompanion.data.api.HttpResult.Failure -> {
        if (BuildConfig.DEBUG) android.util.Log.w("SessionWS", "loadMetadata failed: ${sessions.message}")
      }
    }
  }

  private fun scheduleReconnect(immediate: Boolean = false) {
    if (closed || reconnectJob?.isActive == true) return
    // Quick pre-check: bail if no server is configured at all.
    activeServer ?: return
    reconnectJob = viewModelScope.launch {
      val baseDelays = longArrayOf(1_000, 2_000, 5_000, 10_000, 15_000, 30_000)
      if (!immediate) {
        val base = baseDelays[reconnectAttempt.coerceAtMost(baseDelays.lastIndex)]
        // Add ±20% jitter to prevent thundering-herd when multiple clients
        // reconnect simultaneously after an outage.
        val jitter = (base * 0.2 * (Math.random() * 2 - 1)).toLong()
        delay((base + jitter).coerceAtLeast(500))
      }
      // Capture the server AFTER the delay so we use the latest config,
      // not a stale reference from before the delay.
      val server = activeServer ?: return@launch
      reconnectAttempt++
      _connectionState.value = ConnectionState.Connecting
      // Replay missed events immediately. Do not block reconnection on a full
      // get_messages RPC round trip; the timeline already has its history.
      openTicketedStream(server, lastEventId.takeIf { it > 0 })
    }
  }

  private suspend fun handleEvent(raw: JsonObject, type: String) {
    // Log all event types for debugging
    if (BuildConfig.DEBUG) android.util.Log.d("SessionWS", "Event type: $type, keys: ${raw.keys}")

    val item = when (type) {
      // Pi RPC streams assistant output as nested message_update events.
      "message_start" -> {
        val message = raw["message"]?.jsonObject
        val role = message?.getString("role")
        if (role == "assistant") {
          // If a previous assistant turn is still open (double message_start
          // from server), flush and close the orphan bubble first.
          if (assistantTextOpen) {
            flushAndCloseAssistantBubble()
          }
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
          // insert uses "now". Match by text content against the map values.
          val matchedKey = recentSentPrompts.entries.find { it.value == text }?.key
          if (text == lastSentPrompt || matchedKey != null) {
            lastSentPrompt = null
            if (matchedKey != null) recentSentPrompts.remove(matchedKey)
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
          // Only reset the flag if no text has been accumulated yet.
          // Some providers send text_start after text_delta events, which
          // would incorrectly reset the flag and cause the message_end
          // fallback to duplicate the response.
          "text_start" -> {
            if (pendingAssistantDeltas.isEmpty() && !assistantTextOpen) {
              receivedAssistantTextInMessage = false
            }
          }
          "text_delta" -> update.getString("delta")?.let(::appendAssistantDelta)
          // text_end contains the entire content, which we have already built
          // from deltas; appending it would duplicate the response.
        }
        return
      }
      "message_end" -> {
        val message = raw["message"]?.jsonObject ?: return
        if (message.getString("role") != "assistant") return
        _sendState.value = SendState.Idle
        _agentWorking.value = false
        turnCompleteGeneration++
        // Cancel the flush job and acquire the mutex so we serialize with any
        // in-flight flush. This prevents duplicate text from a flush job that
        // was mid-execution when we cancelled it.
        assistantFlushJob?.cancel()
        assistantFlushJob = null
        assistantMutex.withLock {
          val remaining = synchronized(pendingAssistantDeltas) {
            val s = pendingAssistantDeltas.toString()
            pendingAssistantDeltas.clear()
            s
          }
          if (remaining.isNotEmpty() && assistantTextOpen) {
            _items.update { current ->
              val index = current.indexOfLast {
                it is SessionTimelineItem.Chat && !it.isUser && it.author == "Pi Agent" && it.order == currentAssistantOrder
              }
              if (index >= 0) {
                val existing = current[index] as SessionTimelineItem.Chat
                current.toMutableList().also {
                  it[index] = existing.copy(text = existing.text + remaining)
                }
              } else {
                // Bubble was lost (e.g. dedup collision or list mutation).
                // Create a new item to preserve the text.
                current + SessionTimelineItem.Chat("Pi Agent", remaining, "", false)
              }
            }
          } else if (remaining.isNotEmpty() && !assistantTextOpen) {
            appendAssistantMessage(remaining)
          }
          assistantTextOpen = false
          currentAssistantOrder = 0
        }
        // Some providers do not emit text_delta. Show their final text once.
        // Guard: only fire if we genuinely received no text at all — not if
        // deltas arrived but the flush hadn't completed yet (the mutex drain
        // above handles that case). Use the flag + buffer check instead of
        // scanning the full items list.
        if (!receivedAssistantTextInMessage && pendingAssistantDeltas.isEmpty() && !assistantTextOpen) {
          message.findText()?.takeIf { it.isNotBlank() }?.let(::appendAssistantMessage)
        }
        return
      }
      // Tool events use toolCallId; update the existing card rather than
      // adding a misleading start and completion row for the same operation.
      "tool_execution_start" -> {
        _agentWorking.value = true
        // Close the current assistant text bubble before showing the tool.
        // This prevents tool calls from being visually merged into one giant bubble.
        flushAndCloseAssistantBubble()
        val name = raw.getString("toolName") ?: raw.getString("name") ?: raw.getString("tool") ?: "tool"
        val callId = raw.getString("toolCallId") ?: raw.getString("id") ?: raw.getString("tool_use_id") ?: "tool-${System.nanoTime()}"
        if (BuildConfig.DEBUG) android.util.Log.d("SessionWS", "Tool START: name=$name, callId=$callId")
        SessionTimelineItem.Tool(
          callId = callId,
          name = name,
          status = "running",
          args = raw["args"]?.toString(),
          startedAt = System.currentTimeMillis(),
        )
      }
      "tool_execution_update" -> {
        val callId = raw.getString("toolCallId") ?: raw.getString("id") ?: raw.getString("tool_use_id")
        if (BuildConfig.DEBUG) android.util.Log.d("SessionWS", "Tool UPDATE: callId=$callId")
        updateTool(
          callId = callId,
          output = raw["partialResult"]?.findText(),
          status = "running",
        )
        return
      }
      "tool_execution_end" -> {
        val callId = raw.getString("toolCallId") ?: raw.getString("id") ?: raw.getString("tool_use_id")
        val isError = raw["isError"]?.toString() == "true" || raw.getString("success") == "false"
        if (BuildConfig.DEBUG) android.util.Log.d("SessionWS", "Tool END: callId=$callId, isError=$isError")
        updateTool(
          callId = callId,
          output = raw["result"]?.findText() ?: raw.getString("error"),
          status = if (isError) "failed" else "completed",
          endedAt = System.currentTimeMillis(),
        )
        return
      }
      // File changes (server emits file_change; file_write/file_edit are legacy)
      "file_change" -> {
        val path = raw.getString("path") ?: raw.getString("file") ?: return
        val op = raw.getString("change") ?: raw.getString("operation") ?: raw.getString("type") ?: "modified"
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
        if (type == "agent_start" && pendingPromptIds.isNotEmpty()) {
          _sendState.value = SendState.Running
          _agentWorking.value = true
        }
        if (type == "agent_settled" || type == "agent_end") {
          pendingPromptIds.clear()
          _sendState.value = SendState.Idle
          _agentWorking.value = false
          turnCompleteGeneration++
        }
        return
      }
      // Runtime state transitions from the server (authoritative source of truth)
      "runtime_state" -> {
        val state = raw.getString("runtimeState")
        // Capture the generation at event time. If a message_end or agent_end
        // fires between now and when this state is applied, the generation
        // will have advanced and we skip re-enabling the spinner.
        val gen = turnCompleteGeneration
        _agentWorking.value = (state == "working" || state == "starting" || state == "reconnecting")
          && gen == turnCompleteGeneration
        if (state == "idle" || state == "stopped" || state == "failed") {
          _sendState.value = SendState.Idle
        }
        return
      }
      // Model/thinking level changes (from TUI or other clients)
      "model_select" -> {
        val model = raw["model"]?.jsonObject
        val provider = model?.getString("provider") ?: "?"
        val modelId = model?.getString("id") ?: "?"
        SessionTimelineItem.System("Model changed: $provider/$modelId")
      }
      "thinking_level_select" -> {
        val level = raw.getString("level") ?: return
        SessionTimelineItem.System("Thinking level: $level")
      }
      // Daemon events
      "daemon_error" -> {
        _sendState.value = SendState.Idle
        val text = raw.getString("error") ?: raw.getString("message") ?: "daemon_error"
        SessionTimelineItem.System(text)
      }
      "daemon_start", "daemon_exit" -> {
        val text = raw.getString("message") ?: type
        SessionTimelineItem.System(text)
      }
      // Response to commands
      "response" -> {
        val responseId = raw.getString("id")
        if (responseId == null || !pendingPromptIds.remove(responseId)) return
        if (raw.getString("success") == "false") {
          val error = raw.getString("error") ?: "Prompt rejected"
          _sendState.value = SendState.Failed(error)
          appendItem(SessionTimelineItem.System("Prompt failed: $error"))
          return
        }
        _sendState.value = SendState.Accepted
        return
      }
      // Do not flood the conversation with daemon/extension payloads the app
      // does not render yet. They remain available in Logcat for diagnosis.
      else -> return
    }
    appendItem(item)
  }

  private data class ToolUpdate(val callId: String?, val output: String?, val status: String, val endedAt: Long? = null)

  private fun updateTool(callId: String?, output: String?, status: String, endedAt: Long? = null) {
    synchronized(pendingToolUpdates) { pendingToolUpdates.add(ToolUpdate(callId, output, status, endedAt)) }
    if (toolFlushJob?.isActive == true) return
    // Batch tool updates at 16ms cadence to avoid Compose recomposition
    // churn during rapid tool_execution_update events.
    toolFlushJob = viewModelScope.launch {
      delay(16)
      // Atomically drain the buffer so no update added during _items.update is lost.
      val batch = synchronized(pendingToolUpdates) {
        val copy = pendingToolUpdates.toList()
        pendingToolUpdates.clear()
        copy
      }
      _items.update { currentItems ->
        val items = currentItems.toMutableList()
        for (update in batch) {
          val index = items.indexOfLast { it is SessionTimelineItem.Tool && it.callId == update.callId }
          if (index < 0) continue
          val current = items[index] as SessionTimelineItem.Tool
          items[index] = current.copy(status = update.status, output = update.output ?: current.output, endedAt = update.endedAt ?: current.endedAt)
        }
        items
      }
    }
  }

  private fun appendAssistantDelta(delta: String) {
    if (delta.isEmpty()) return
    // Discard late deltas that arrive after message_end or agent_end has
    // already closed the current turn. These are stale events from a
    // previous response that would otherwise create an orphan bubble.
    if (turnCompleteGeneration > 0 && !assistantTextOpen && pendingAssistantDeltas.isEmpty()) return
    synchronized(pendingAssistantDeltas) { pendingAssistantDeltas.append(delta) }
    receivedAssistantTextInMessage = true
    if (assistantFlushJob?.isActive == true) return
    assistantFlushJob = viewModelScope.launch {
      delay(16)
      // Drain the buffer inside the mutex so that if this job is cancelled
      // between drain and items-update, message_end's mutex.withLock will
      // see the buffer as still un-drained and pick up the text.
      assistantMutex.withLock {
        val buffered = synchronized(pendingAssistantDeltas) {
          val s = pendingAssistantDeltas.toString()
          pendingAssistantDeltas.clear()
          s
        }
        if (buffered.isEmpty()) return@withLock
        if (!assistantTextOpen) {
          // Open a new assistant bubble.
          val newBubble = SessionTimelineItem.Chat("Pi Agent", buffered, "", false)
          val stamped = withOrder(newBubble) as SessionTimelineItem.Chat
          currentAssistantOrder = stamped.order
          assistantTextOpen = true
          appendItem(stamped)
        } else {
          // Append to the current assistant bubble, identified by its order.
          _items.update { current ->
            val index = current.indexOfLast {
              it is SessionTimelineItem.Chat && !it.isUser && it.author == "Pi Agent" && it.order == currentAssistantOrder
            }
            if (index >= 0) {
              val existing = current[index] as SessionTimelineItem.Chat
              current.toMutableList().also {
                it[index] = existing.copy(text = existing.text + buffered)
              }
            } else {
              // Bubble was lost (shouldn't happen) — create a new one.
              val newBubble = SessionTimelineItem.Chat("Pi Agent", buffered, "", false)
              val stamped = withOrder(newBubble) as SessionTimelineItem.Chat
              currentAssistantOrder = stamped.order
              current + stamped
            }
          }
        }
      }
    }
  }

  private fun appendAssistantMessage(text: String) {
    val bubble = SessionTimelineItem.Chat(
      author = "Pi Agent",
      text = text,
      time = "",
      isUser = false,
    )
    val stamped = withOrder(bubble) as SessionTimelineItem.Chat
    currentAssistantOrder = stamped.order
    assistantTextOpen = false
    appendItem(stamped)
  }

  /**
   * Flushes any buffered assistant text deltas and closes the current
   * assistant text bubble so the next text starts a new one.
   * Called before tool execution starts to prevent the assistant's
   * pre-tool text and post-tool text from merging into one giant bubble.
   */
  private suspend fun flushAndCloseAssistantBubble() {
    // Suspend until any in-flight flush releases the mutex. Do not block the
    // main/UI dispatcher while processing streamed events.
    assistantFlushJob?.cancel()
    assistantFlushJob = null
    assistantMutex.withLock {
      val remaining = synchronized(pendingAssistantDeltas) {
        val s = pendingAssistantDeltas.toString()
        pendingAssistantDeltas.clear()
        s
      }
      if (remaining.isNotEmpty() && assistantTextOpen) {
        _items.update { current ->
          val index = current.indexOfLast {
            it is SessionTimelineItem.Chat && !it.isUser && it.author == "Pi Agent" && it.order == currentAssistantOrder
          }
          if (index >= 0) {
            val existing = current[index] as SessionTimelineItem.Chat
            current.toMutableList().also {
              it[index] = existing.copy(text = existing.text + remaining)
            }
          } else {
            current + SessionTimelineItem.Chat("Pi Agent", remaining, "", false)
          }
        }
      }
      assistantTextOpen = false
      currentAssistantOrder = 0
    }
  }

  private fun timelineItemId(item: SessionTimelineItem): String = when (item) {
    // Tool and FileChange IDs are naturally unique (callId / path).
    // Chat dedup uses role + text hash + length to minimize collision risk.
    // Using only hashCode() caused duplicate assistant responses to be
    // silently dropped when two responses had the same text.
    is SessionTimelineItem.Chat -> {
      val textSig = "${item.text.length}|${item.text.hashCode()}|${item.text.take(50)}"
      "chat|${item.isUser}|${item.time}|$textSig"
    }
    is SessionTimelineItem.Tool -> "tool|${item.callId}"
    is SessionTimelineItem.FileChange -> "file|${item.operation}|${item.path}"
    is SessionTimelineItem.System -> "system|${item.text}"
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

  private var lastSendTime = 0L

  fun sendPrompt(message: String, imageUris: List<Uri> = emptyList()) {
    if (message.isBlank() && imageUris.isEmpty()) return
    // Debounce rapid-fire sends (double-tap, accidental repeat) to prevent
    // duplicate messages. 300ms window catches most accidental double-taps.
    val now = System.currentTimeMillis()
    if (imageUris.isEmpty() && now - lastSendTime < 300) return
    lastSendTime = now
    if (imageUris.isNotEmpty()) {
      val server = activeServer ?: return
      // The socket echoes the user message without local Uri attachments.
      // Mark it so message_start is ignored and the image-bearing optimistic
      // row remains the only visible user message.
      lastSentPrompt = message
      recentSentPrompts["img-${UUID.randomUUID()}"] = message
      appendItem(SessionTimelineItem.Chat(
        author = "You", text = message, time = "now", isUser = true, imageUris = imageUris,
      ))
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
    if (_connectionState.value !is ConnectionState.Connected) {
      // Queue text prompts while reconnecting or offline. The next successful
      // socket connection drains this FIFO through the authoritative REST
      // route, so a temporary network loss does not discard user input.
      if (queuedPrompts.enqueue(message)) {
        appendItem(SessionTimelineItem.System("Offline — message will be sent when the session reconnects"))
      } else {
        appendItem(SessionTimelineItem.System("Offline message queue is full; prompt was not sent"))
      }
      return
    }

    val requestId = "companion-${UUID.randomUUID()}"
    pendingPromptIds.add(requestId)
    _sendState.value = SendState.Sending
    lastSentPrompt = message
    recentSentPrompts[requestId] = message

    // Optimistic: add user message to timeline immediately
    appendItem(SessionTimelineItem.Chat(
      author = "You",
      text = message,
      time = "now",
      isUser = true,
    ))

    // REST is the authoritative route for both local RPC and bridged TUI
    // sessions. Raw session WebSocket commands bypassed the relay queue on
    // some reconnect paths, making Companion prompts appear to disappear.
    viewModelScope.launch {
      when (val result = repository.sendPrompt(sessionId, message, idempotencyKey = requestId)) {
        is com.example.picompanion.data.api.HttpResult.Success -> _sendState.value = SendState.Accepted
        is com.example.picompanion.data.api.HttpResult.Failure -> {
          pendingPromptIds.remove(requestId)
          _sendState.value = SendState.Idle
          appendItem(SessionTimelineItem.System("Could not send prompt: ${result.message}"))
        }
      }
    }
  }

  private fun flushQueuedPrompts() {
    viewModelScope.launch {
      var retries = 0
      while (retries < 3) {
        val message = queuedPrompts.dequeue() ?: break
        sendPrompt(message)
        // Wait for the send to resolve by observing _sendState changes.
        // The send coroutine sets _sendState to Accepted, Failed, or Idle.
        val result = kotlinx.coroutines.withTimeoutOrNull(10_000) {
          _sendState.first { state ->
            state is SendState.Accepted || state is SendState.Failed || state is SendState.Idle
          }
        }
        if (result is SendState.Failed || result == null) {
          // Send failed or timed out — put it back at the front for retry.
          queuedPrompts.enqueueFront(message)
          retries++
          delay(1000)
        } else {
          retries = 0
        }
        delay(100)
      }
      if (queuedPrompts.size() > 0) {
        appendItem(SessionTimelineItem.System("Some queued messages could not be sent after retries"))
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
          appendItem(SessionTimelineItem.System("Could not steer: ${result.message}"))
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
          appendItem(SessionTimelineItem.System("Stop requested"))
        }
        is com.example.picompanion.data.api.HttpResult.Failure -> appendItem(SessionTimelineItem.System("Could not stop Pi: ${result.message}"))
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
          appendItem(SessionTimelineItem.System("Compacting…"))
        is com.example.picompanion.data.api.HttpResult.Failure ->
          appendItem(SessionTimelineItem.System("Could not compact: ${result.message}"))
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
          appendItem(SessionTimelineItem.System("Session details saved"))
        }
        is com.example.picompanion.data.api.HttpResult.Failure ->
          appendItem(SessionTimelineItem.System("Could not save session details: ${result.userMessage}"))
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
          appendItem(SessionTimelineItem.System("${action.replace('-', ' ')} requested"))
        is com.example.picompanion.data.api.HttpResult.Failure ->
          appendItem(SessionTimelineItem.System("Could not ${action.replace('-', ' ')}: ${result.userMessage}"))
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
      when (val result = withContext(Dispatchers.IO) { client.setSessionModel(server, sessionId, provider, modelId) }) {
        is com.example.picompanion.data.api.HttpResult.Success -> {
          appendItem(SessionTimelineItem.System("Model changed: $provider/$modelId"))
          loadModelControls()
        }
        is com.example.picompanion.data.api.HttpResult.Failure ->
          appendItem(SessionTimelineItem.System("Could not set model: ${result.userMessage}"))
      }
    }
  }

  fun setThinkingLevel(level: String) {
    val server = activeServer ?: return
    viewModelScope.launch {
      when (val result = withContext(Dispatchers.IO) { client.setThinkingLevel(server, sessionId, level) }) {
        is com.example.picompanion.data.api.HttpResult.Success -> {
          appendItem(SessionTimelineItem.System("Thinking level: $level"))
          loadModelControls()
        }
        is com.example.picompanion.data.api.HttpResult.Failure ->
          appendItem(SessionTimelineItem.System("Could not set thinking level: ${result.userMessage}"))
      }
    }
  }

  private suspend fun loadGitChanges() {
    val server = activeServer ?: return
    when (val result = withContext(Dispatchers.IO) { client.getSessionGit(server, sessionId, "status?format=json") }) {
      is com.example.picompanion.data.api.HttpResult.Success -> {
        val status = result.value["status"]?.jsonObject
        _gitChanges.value = status?.get("changes")?.let { element ->
          (element as? kotlinx.serialization.json.JsonArray)?.mapNotNull { item ->
            val obj = item as? JsonObject ?: return@mapNotNull null
            GitFileChange(
              path = obj["path"]?.jsonPrimitive?.contentOrNull ?: return@mapNotNull null,
              status = obj["status"]?.jsonPrimitive?.contentOrNull ?: "M",
              additions = obj["additions"]?.jsonPrimitive?.intOrNull ?: 0,
              deletions = obj["deletions"]?.jsonPrimitive?.intOrNull ?: 0,
            )
          } ?: emptyList()
        } ?: emptyList()
      }
      is com.example.picompanion.data.api.HttpResult.Failure -> Unit
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

  fun refresh() {
    if (closed || _refreshing.value) return
    viewModelScope.launch {
      _refreshing.value = true
      try {
        loadMetadata()
        loadHistory()
        refreshRelayHealth()
        if (_connectionState.value is ConnectionState.Disconnected || _connectionState.value is ConnectionState.Error) {
          reconnect()
        }
      } finally {
        _refreshing.value = false
      }
    }
  }

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

data class GitFileChange(
  val path: String,
  val status: String,
  val additions: Int,
  val deletions: Int,
)

sealed interface SessionTimelineItem {
  val order: Long

  data class Chat(
    val author: String,
    val text: String,
    val time: String,
    val isUser: Boolean,
    val imageUris: List<Uri> = emptyList(),
    override val order: Long = 0,
  ) : SessionTimelineItem

  data class Tool(
    val callId: String,
    val name: String,
    val status: String,
    val args: String? = null,
    val output: String? = null,
    val startedAt: Long? = null,
    val endedAt: Long? = null,
    override val order: Long = 0,
  ) : SessionTimelineItem

  data class FileChange(
    val path: String,
    val operation: String,
    override val order: Long = 0,
  ) : SessionTimelineItem

  data class System(val text: String, override val order: Long = 0) : SessionTimelineItem
}
