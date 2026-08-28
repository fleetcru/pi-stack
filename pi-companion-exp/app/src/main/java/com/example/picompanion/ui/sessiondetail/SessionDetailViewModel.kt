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
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.ViewModel
import androidx.lifecycle.ViewModelProvider
import androidx.lifecycle.viewModelScope
import com.example.picompanion.di.AppModule
import com.example.picompanion.data.repository.SessionsRepository
import com.example.picompanion.data.websocket.SocketEvent
import com.example.picompanion.data.model.ExtensionUiRequest
import com.example.picompanion.data.model.parseExtensionUiRequest
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
  private val transport = SessionTransportCoordinator(client, sessionId)
  private val repository = SessionsRepository(client, settingsDataStore)

  private val _items = MutableStateFlow<List<SessionTimelineItem>>(emptyList())
  val items: StateFlow<List<SessionTimelineItem>> = _items.asStateFlow()

  private val _connectionState = MutableStateFlow<ConnectionState>(ConnectionState.Connecting)
  val connectionState: StateFlow<ConnectionState> = _connectionState.asStateFlow()

  private val _sendState = MutableStateFlow<SendState>(SendState.Idle)
  val sendState: StateFlow<SendState> = _sendState.asStateFlow()
  private val _agentWorking = MutableStateFlow(false)
  val agentWorking: StateFlow<Boolean> = _agentWorking.asStateFlow()
  private var agentWorkingJob: kotlinx.coroutines.Job? = null

  /** Debounced setter for _agentWorking to batch rapid event bursts into one recomposition. */
  private fun setAgentWorking(value: Boolean, delayMs: Long = 80) {
    agentWorkingJob?.cancel()
    agentWorkingJob = viewModelScope.launch {
      if (delayMs > 0 && value) kotlinx.coroutines.delay(delayMs)
      _agentWorking.value = value
    }
  }

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
  // Submit state for the current extension request. The request itself stays
  // visible on HTTP failure so the user can retry; only a successful post (or
  // an extension_ui_closed) clears it.
  private val _extensionSubmitting = MutableStateFlow(false)
  val extensionSubmitting: StateFlow<Boolean> = _extensionSubmitting.asStateFlow()
  private val _extensionSubmitError = MutableStateFlow<String?>(null)
  val extensionSubmitError: StateFlow<String?> = _extensionSubmitError.asStateFlow()
  private var lastSubmittedExtensionRequestId: String? = null
  private var lastClosedExtensionRequestId: String? = null
  private val dismissedExtensionRequestIds = LinkedHashSet<String>()
  private var pendingExtensionAbsentPolls = 0
  private var externalSession: Boolean? = null
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
  private val historyState = SessionHistoryState()
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
  private var promptReconcileJob: Job? = null
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
  // Exposes the order of the currently-streaming assistant bubble to the UI
  // so ChatBubble can use plain Text during streaming and Markdown after.
  private val _streamingAssistantOrder = MutableStateFlow(0L)
  val streamingAssistantOrder: StateFlow<Long> = _streamingAssistantOrder.asStateFlow()
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
    observeTransport()
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
            transport.disconnect()
            connect()
          }
        }
      }
    }
  }

  private fun connect() {
    // Manual/settings/network reconnects must replace, not race, a scheduled
    // backoff reconnect that could otherwise open a second ticketed stream.
    reconnectJob?.cancel()
    reconnectJob = null
    // A reconnect must not let buffered tokens from the old socket append to
    // the first assistant message received on the new socket.
    assistantFlushJob?.cancel()
    assistantFlushJob = null
    // Flush any remaining assistant deltas from a previous interrupted
    // response so the partial text is preserved in the timeline.
    synchronized(pendingAssistantDeltas) {
      val remaining = pendingAssistantDeltas.toString()
      pendingAssistantDeltas.clear()
      if (remaining.isNotEmpty()) {
        // Append to the existing assistant bubble instead of creating a new one.
        _items.update { current ->
          val index = if (assistantTextOpen) current.indexOfLast {
            it is SessionTimelineItem.Chat && !it.isUser && it.author == "Pi Agent" && it.order == currentAssistantOrder
          } else -1
          if (index >= 0) {
            val existing = current.getOrNull(index) as? SessionTimelineItem.Chat
            if (existing != null) {
              current.toMutableList().also {
                it[index] = existing.copy(text = existing.text + remaining)
              }
            } else {
              current + SessionTimelineItem.Chat("Pi Agent", remaining, "", false)
            }
          } else {
            current + SessionTimelineItem.Chat("Pi Agent", remaining, "", false)
          }
        }
      }
      assistantTextOpen = false
    }
    toolFlushJob?.cancel()
    toolFlushJob = null
    synchronized(pendingToolUpdates) { pendingToolUpdates.clear() }
    assistantTextOpen = false
    receivedAssistantTextInMessage = false
    currentAssistantOrder = 0
    _streamingAssistantOrder.value = 0
    // Reset agent state so the UI doesn't show a stuck spinner after reconnect.
    _agentWorking.value = false
    _sendState.value = SendState.Idle
    historyGeneration++
    // Clear stale extension requests from the previous connection.
    _extensionRequest.value = null
    _extensionSubmitError.value = null
    _extensionSubmitting.value = false

    viewModelScope.launch {
      _connectionState.value = ConnectionState.Connecting
      val appSettings = settingsDataStore.settingsFlow.first()
      val server = appSettings.activeServer

      if (server == null || !server.isConfigured) {
        _connectionState.value = ConnectionState.Error("No server configured")
        return@launch
      }

      activeServer = server
      restoreCachedSession(server.id)
      launch { loadMetadata() }
      launch { loadGitChanges() }
      launch { loadModelControls() }
      launch { loadHistory() }
      relayHealthJob?.cancel()
      relayHealthJob = launch {
        // Retry an inconclusive first state request instead of permanently
        // disabling relay health/pending-dialog recovery after one network
        // hiccup. Stop only after a successful response proves this is local.
        var external: Boolean? = null
        while (!closed && external != false) {
          external = refreshRelayHealth()
          if (external != false) delay(5_000)
        }
      }

      // Ticket acquisition runs alongside metadata/history startup. Resume from
      // the cached cursor when available; fresh sessions intentionally skip replay
      // because durable history is loading in parallel.
      openTicketedStream(server, lastEventId.takeIf { it > 0 } ?: Long.MAX_VALUE)
    }
  }

  private fun observeTransport() {
    viewModelScope.launch {
      transport.events.collect { event ->
        when (event) {
          is SocketEvent.Connected -> {
            reconnectJob?.cancel()
            reconnectAttempt = 0
            _connectionState.value = ConnectionState.Connected
            flushQueuedPrompts()
          }
          is SocketEvent.EventsLost -> {
            appendItem(SessionTimelineItem.System("Connection missed session events; restoring conversation history"))
            // Use one reconnect path and resume from the last event actually
            // delivered to this ViewModel. Opening with null requested a full
            // ring replay and raced the normal backoff reconnect.
            reconnectJob?.cancel()
            reconnectJob = null
            viewModelScope.launch { loadHistory() }
            scheduleReconnect(immediate = true)
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
          is SocketEvent.RawMessage -> appendItem(SessionTimelineItem.System(event.text))
        }
      }
    }
  }

  private fun restoreCachedSession(serverId: String) {
    val cached = SessionStateCache.get("$serverId:$sessionId") ?: return
    _items.value = cached.items
    historyState.restore(cached.historicalItems, cached.nextHistoryOffset, cached.hasOlder)
    _hasOlderHistory.value = cached.hasOlder
    _sessionTitle.value = cached.title
    _sessionProject.value = cached.project
    _sessionCwd.value = cached.cwd
    lastEventId = cached.lastEventId
    timelineSeq = cached.items.maxOfOrNull { item -> when (item) {
      is SessionTimelineItem.Chat -> item.order
      is SessionTimelineItem.Tool -> item.order
      is SessionTimelineItem.FileChange -> item.order
      is SessionTimelineItem.System -> item.order
    } } ?: 0
  }

  private fun cacheCurrentSession() {
    val serverId = activeServer?.id ?: return
    SessionStateCache.put("$serverId:$sessionId", SessionStateCache.Entry(
      items = _items.value,
      historicalItems = historyState.historicalItems,
      nextHistoryOffset = historyState.nextOffset,
      hasOlder = _hasOlderHistory.value,
      title = _sessionTitle.value,
      project = _sessionProject.value,
      cwd = _sessionCwd.value,
      lastEventId = lastEventId,
    ))
  }

  private suspend fun openTicketedStream(
    server: com.example.picompanion.data.settings.ServerEntry,
    since: Long?,
  ) {
    transport.open(server, since)?.let { message ->
      _connectionState.value = ConnectionState.Error(message)
      scheduleReconnect()
    }
  }

  fun loadOlderHistory() {
    if (!_hasOlderHistory.value || _loadingOlderHistory.value) return
    viewModelScope.launch { loadHistory(historyState.nextOffset, appendOld = true) }
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
        when (val result = repository.getSessionMessages(sessionId, offset = offset, limit = if (appendOld) 75 else 40)) {
          is com.example.picompanion.data.api.HttpResult.Success -> {
            val data = result.value["data"]?.jsonObject
            val messages = data?.get("messages") as? JsonArray
            if (messages != null) {
              // Parsing persisted JSON and tool payloads can be expensive for long
              // sessions; keep it off Main, then merge atomically on return.
              val history = withContext(Dispatchers.Default) {
                SessionHistoryParser.parse(messages)
              }
              val historyMeta = data["history"]?.jsonObject
              val hasOlder = historyMeta?.get("hasOlder")?.jsonPrimitive?.booleanOrNull == true
              val nextOffset = historyMeta?.get("nextOffset")?.jsonPrimitive?.intOrNull ?: 0
              _hasOlderHistory.value = hasOlder
              _historyLoadError.value = null
              // Atomic update prevents live events arriving during the HTTP
              // request from being overwritten by the history snapshot. HTTP
              // history and WS replay use different timestamp formats, so data
              // class equality is not sufficient for cross-source deduplication.
              // Build merged list OUTSIDE _items.update to avoid withOrder()
              // being called on CAS retries (which would create different
              // order values and cause LazyColumn key instability).
              _items.value = historyState.applyPage(
                page = history,
                appendOld = appendOld,
                nextOffset = nextOffset,
                hasOlder = hasOlder,
                liveItems = _items.value,
                stamp = ::withOrder,
              )
              cacheCurrentSession()
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

  private suspend fun refreshRelayHealth(): Boolean? {
    val server = activeServer ?: return null
    return when (val result = withContext(Dispatchers.IO) { client.getSessionState(server, sessionId) }) {
      is com.example.picompanion.data.api.HttpResult.Success -> {
        val state = result.value["data"] as? JsonObject ?: return null
        val external = state["external"]?.jsonPrimitive?.booleanOrNull == true
        externalSession = external
        _relayHealth.value = if (external) RelayHealth(
          connected = state["relayConnected"]?.jsonPrimitive?.booleanOrNull == true,
          latencyMs = state["relayLatencyMs"]?.jsonPrimitive?.longOrNull,
        ) else null
        // Fresh session views intentionally skip old event replay because
        // durable history loads separately. Recover a currently blocking UI
        // request from state so ask_user still appears when mobile opens late.
        val pendingRequest = (state["pendingExtensionUiRequest"] as? JsonObject)
          ?.let(::parseExtensionUiRequest)
        if (pendingRequest != null) {
          pendingExtensionAbsentPolls = 0
          if (
            _extensionRequest.value == null &&
            pendingRequest.id != lastSubmittedExtensionRequestId &&
            pendingRequest.id != lastClosedExtensionRequestId &&
            pendingRequest.id !in dismissedExtensionRequestIds
          ) {
            _extensionRequest.value = pendingRequest
            _extensionSubmitError.value = null
          }
        } else if (external && _extensionRequest.value != null) {
          // Require two absent polls so an older in-flight state response cannot
          // erase a newer socket event. This also heals a missed close event.
          pendingExtensionAbsentPolls++
          if (pendingExtensionAbsentPolls >= 2) {
            _extensionRequest.value = null
            _extensionSubmitError.value = null
            pendingExtensionAbsentPolls = 0
          }
        }
        external
      }
      is com.example.picompanion.data.api.HttpResult.Failure -> null
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
          _streamingAssistantOrder.value = 0
          // Reset the turn guard so streaming deltas for this new turn are not
          // dropped by the stale-delta check in appendAssistantDelta.
          turnCompleteGeneration = 0
          return
        }
        if (role == "user") {
          // User sent a prompt from the TUI — show it in the timeline
          val text = message?.findText().orEmpty()
          val images = message?.findImages().orEmpty()
          if (text.isBlank() && images.isEmpty()) return
          // Deduplicate against the optimistic insert from sendPrompt().
          // The WS echo arrives with a server timestamp while the optimistic
          // insert uses "now". Match by text content against the map values.
          val matchedKey = synchronized(recentSentPrompts) {
            recentSentPrompts.entries.firstOrNull { it.value == text }?.key
          }
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
            imageData = images,
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
          _streamingAssistantOrder.value = 0
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
        // Only Pi dialog methods need a client response. The server stamps
        // `_daemonExtensionUiRequiresResponse` for those; status, widget, and
        // notification events share this RPC type but are verbose output.
        val request = parseExtensionUiRequest(raw) ?: return
        if (
          request.id in dismissedExtensionRequestIds ||
          request.id == lastSubmittedExtensionRequestId ||
          request.id == lastClosedExtensionRequestId
        ) return
        if (lastSubmittedExtensionRequestId != null && lastSubmittedExtensionRequestId != request.id) {
          lastSubmittedExtensionRequestId = null
        }
        if (lastClosedExtensionRequestId != null && lastClosedExtensionRequestId != request.id) {
          lastClosedExtensionRequestId = null
        }
        pendingExtensionAbsentPolls = 0
        _extensionRequest.value = request
        _extensionSubmitError.value = null
        return
      }
      "extension_ui_closed" -> {
        // The extension resolved the request on its own (timeout, TUI answer,
        // or another client); dismiss the matching dialog without responding.
        val id = raw.getString("id") ?: return
        pendingExtensionAbsentPolls = 0
        lastClosedExtensionRequestId = id
        dismissedExtensionRequestIds.remove(id)
        if (lastSubmittedExtensionRequestId == id) lastSubmittedExtensionRequestId = null
        if (_extensionRequest.value?.id == id) {
          _extensionRequest.value = null
          _extensionSubmitError.value = null
        }
        return
      }
      "bridge_receipt" -> {
        if (raw.getString("status") == "failed") {
          _sendState.value = SendState.Failed("The TUI could not accept this message")
          _agentWorking.value = false
        } else {
          _sendState.value = SendState.Delivered
        }
        return
      }
      "agent_start", "agent_end", "agent_settled", "turn_start", "turn_end" -> {
        if (type == "agent_start" && pendingPromptIds.isNotEmpty()) {
          _sendState.value = SendState.Running
          setAgentWorking(true)
        }
        // A relay agent_end may be followed by automatic retry, compaction, or
        // queued continuation. Keep its UI running until agent_settled, matching
        // the server's relay admission lifecycle.
        val relaySession = externalSession == true
        if (type == "agent_settled" || (type == "agent_end" && !relaySession)) {
          pendingPromptIds.clear()
          promptReconcileJob?.cancel()
          promptReconcileJob = null
          _sendState.value = SendState.Idle
          setAgentWorking(false, delayMs = 0)
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
        setAgentWorking(
          (state == "working" || state == "starting" || state == "reconnecting")
            && gen == turnCompleteGeneration,
        )
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
        _modelControls.update { it.copy(selectedProvider = provider, selectedModelId = modelId) }
        return // don't show system message — the model picker reflects this
      }
      "thinking_level_select" -> {
        val level = raw.getString("level") ?: return
        _modelControls.update { it.copy(thinkingLevel = level) }
        return // don't show system message — the picker reflects this
      }
      // Available models list update (from bridge extension on relay sessions)
      "available_models" -> {
        val modelsJson = raw["models"]
        if (modelsJson is JsonArray) {
          val providers = mutableMapOf<String, MutableList<ModelChoice>>()
          for (element in modelsJson) {
            val obj = element as? JsonObject ?: continue
            val p = obj.getString("provider") ?: continue
            val id = obj.getString("id") ?: continue
            val name = obj.getString("name") ?: id
            providers.getOrPut(p) { mutableListOf() }.add(ModelChoice(p, id, name))
          }
          val allModels = providers.values.flatten()
          if (allModels.isNotEmpty()) {
            _modelControls.update { it.copy(models = allModels) }
          }
        }
        return // don't show as system message
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
    // Batch tool updates at 8ms cadence — fast enough to feel instant,
    // still avoids Compose recomposition churn during rapid updates.
    toolFlushJob = viewModelScope.launch {
      delay(8)
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
      delay(8)
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
          _streamingAssistantOrder.value = stamped.order
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
    _streamingAssistantOrder.value = 0
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
      if (remaining.isNotEmpty()) {
        _items.update { current ->
          val index = if (assistantTextOpen) current.indexOfLast {
            it is SessionTimelineItem.Chat && !it.isUser && it.author == "Pi Agent" && it.order == currentAssistantOrder
          } else -1
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
  private fun JsonElement.findImages(): List<String> = when (this) {
    is JsonObject -> if (getString("type") == "image") listOfNotNull(getString("data")) else this["content"]?.findImages().orEmpty()
    is JsonArray -> flatMap { it.findImages() }
    else -> emptyList()
  }

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
              // Downsample large images to prevent OOM and reduce payload size.
              val maxDimension = 1024
              val options = BitmapFactory.Options().apply { inJustDecodeBounds = true }
              context.contentResolver.openInputStream(uri)?.use { BitmapFactory.decodeStream(it, null, options) }
              val scale = maxOf(1, maxOf(options.outWidth, options.outHeight) / maxDimension)
              val decodeOptions = BitmapFactory.Options().apply { inSampleSize = scale }
              val bitmap = context.contentResolver.openInputStream(uri)?.use {
                BitmapFactory.decodeStream(it, null, decodeOptions)
              } ?: return@mapNotNull null
              try {
                PromptImageEncoder.encodeJpeg(bitmap)
              } finally {
                bitmap.recycle()
              }
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
        is com.example.picompanion.data.api.HttpResult.Success -> {
          _sendState.value = SendState.Accepted
          schedulePromptReconciliation()
        }
        is com.example.picompanion.data.api.HttpResult.Failure -> {
          pendingPromptIds.remove(requestId)
          _sendState.value = SendState.Idle
          appendItem(SessionTimelineItem.System("Could not send prompt: ${result.message}"))
        }
      }
    }
  }

  private fun schedulePromptReconciliation() {
    promptReconcileJob?.cancel()
    val reconciledIds = synchronized(pendingPromptIds) { pendingPromptIds.toSet() }
    val promptText = lastSentPrompt ?: return
    promptReconcileJob = viewModelScope.launch {
      // Relay commands can be accepted just as a mobile WebSocket reconnects.
      // Periodic durable-history refresh prevents the accepted user message and
      // response from disappearing when those live events land in that gap.
      repeat(30) {
        delay(2_000)
        if (reconciledIds.none { it in pendingPromptIds }) return@launch
        loadHistory()
        if (durableHistoryHasResponseAfter(promptText)) {
          pendingPromptIds.removeAll(reconciledIds)
          if (pendingPromptIds.isEmpty()) _sendState.value = SendState.Idle
          return@launch
        }
      }
      pendingPromptIds.removeAll(reconciledIds)
      if (pendingPromptIds.isEmpty()) _sendState.value = SendState.Idle
    }
  }

  private fun durableHistoryHasResponseAfter(promptText: String): Boolean {
    val history = historyState.historicalItems
    val userIndex = history.indexOfLast {
      it is SessionTimelineItem.Chat && it.isUser && it.text == promptText
    }
    return userIndex >= 0 && history.drop(userIndex + 1).any {
      it is SessionTimelineItem.Chat && !it.isUser
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

  /** Locally dismisses an undeliverable request without claiming an answer. */
  fun dismissExtensionRequest() {
    val request = _extensionRequest.value ?: return
    dismissedExtensionRequestIds.add(request.id)
    while (dismissedExtensionRequestIds.size > 20) {
      dismissedExtensionRequestIds.remove(dismissedExtensionRequestIds.first())
    }
    _extensionRequest.value = null
    _extensionSubmitError.value = null
  }

  /**
   * Submits a structured answer to the current extension UI request.
   *
   * On HTTP failure the request is kept visible and the error is exposed so
   * the user can retry; only a successful post clears the request.
   */
  fun submitExtensionResponse(
    cancelled: Boolean = false,
    value: String? = null,
    confirmed: Boolean? = null,
    selections: List<String>? = null,
    comment: String? = null,
    responseKind: String? = null,
  ) {
    val request = _extensionRequest.value ?: return
    val server = activeServer ?: return
    viewModelScope.launch {
      _extensionSubmitting.value = true
      _extensionSubmitError.value = null
      val result = withContext(Dispatchers.IO) {
        client.respondToExtensionUi(
          server = server,
          sessionId = sessionId,
          id = request.id,
          cancelled = cancelled,
          value = value,
          confirmed = confirmed,
          selections = selections,
          comment = comment,
          responseKind = responseKind,
        )
      }
      _extensionSubmitting.value = false
      when (result) {
        is com.example.picompanion.data.api.HttpResult.Success -> {
          // Suppress stale state snapshots until the server/bridge close event
          // confirms this exact request has finished.
          lastSubmittedExtensionRequestId = request.id
          if (_extensionRequest.value?.id == request.id) _extensionRequest.value = null
          _extensionSubmitError.value = null
        }
        is com.example.picompanion.data.api.HttpResult.Failure -> {
          if (_extensionRequest.value?.id == request.id) {
            _extensionSubmitError.value =
              "Could not send response: ${result.userMessage}"
          }
        }
      }
    }
  }

  fun activeServerForUi(): com.example.picompanion.data.settings.ServerEntry? = activeServer

  fun refresh() {
    if (closed || _refreshing.value) return
    viewModelScope.launch {
      _refreshing.value = true
      try {
        val broken = _connectionState.value is ConnectionState.Disconnected || _connectionState.value is ConnectionState.Error
        if (broken) {
          // Full reconnection path: reset stuck spinner/send state, flush
          // buffered deltas, and reopen the stream. Using reconnect() here
          // left _agentWorking/_sendState stuck, disabling the send button.
          connect()
        } else {
          // Connection is healthy: just reload the latest history snapshot
          // without disrupting the live stream.
          loadMetadata()
          loadHistory()
          refreshRelayHealth()
        }
      } finally {
        _refreshing.value = false
      }
    }
  }

  fun reconnect() {
    reconnectJob?.cancel()
    transport.disconnect()
    scheduleReconnect(immediate = true)
  }

  override fun onCleared() {
    cacheCurrentSession()
    closed = true
    reconnectJob?.cancel()
    relayHealthJob?.cancel()
    if (networkCallbackRegistered) connectivityManager.unregisterNetworkCallback(networkCallback)
    transport.close()
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
    val imageData: List<String> = emptyList(),
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
