package com.example.picompanion.data.websocket

import android.util.Log
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.api.apiJson
import com.example.picompanion.data.model.LiveSessionStatus
import com.example.picompanion.data.settings.ServerEntry
import java.net.URI
import java.util.concurrent.atomic.AtomicLong
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.delay
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener

/** One app-scoped, read-only connection for statuses across every session. */
class ServerStatusStream(
  private val client: PiServerClient,
  private val json: Json = apiJson,
) {
  enum class ConnectionState { Disconnected, Connecting, Connected, Reconnecting }

  private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)
  private val connectionGeneration = AtomicLong(0)
  private var socket: WebSocket? = null
  @Volatile private var server: ServerEntry? = null
  private var reconnectJob: Job? = null
  private var reconnectAttempt = 0
  private var cursor: Long? = null
  private var hubGeneration: String? = null
  @Volatile private var stopped = true

  private val _connectionState = MutableStateFlow(ConnectionState.Disconnected)
  val connectionState: StateFlow<ConnectionState> = _connectionState.asStateFlow()

  private val _sessions = MutableStateFlow<Map<String, LiveSessionStatus>>(emptyMap())
  val sessions: StateFlow<Map<String, LiveSessionStatus>> = _sessions.asStateFlow()

  private val _workerHealth = MutableStateFlow<Map<String, String>>(emptyMap())
  val workerHealth: StateFlow<Map<String, String>> = _workerHealth.asStateFlow()

  private val _activeRuns = MutableStateFlow<Map<String, LiveSessionStatus>>(emptyMap())
  val activeRuns: StateFlow<Map<String, LiveSessionStatus>> = _activeRuns.asStateFlow()

  @Synchronized
  fun connect(target: ServerEntry) {
    if (!target.isConfigured) {
      disconnect()
      return
    }
    if (!stopped && server == target && socket != null) return
    disconnectLocked(clearState = server?.id != target.id)
    stopped = false
    server = target
    open(target, connectionGeneration.incrementAndGet())
  }

  @Synchronized
  fun disconnect() {
    disconnectLocked(clearState = true)
  }

  private fun disconnectLocked(clearState: Boolean) {
    stopped = true
    connectionGeneration.incrementAndGet()
    reconnectJob?.cancel()
    reconnectJob = null
    socket?.close(1000, "Status stream stopped")
    socket = null
    server = null
    cursor = null
    hubGeneration = null
    reconnectAttempt = 0
    _connectionState.value = ConnectionState.Disconnected
    if (clearState) {
      _sessions.value = emptyMap()
      _workerHealth.value = emptyMap()
      _activeRuns.value = emptyMap()
    }
  }

  private fun open(target: ServerEntry, connectionId: Long) {
    _connectionState.value = if (reconnectAttempt == 0) ConnectionState.Connecting else ConnectionState.Reconnecting
    scope.launch {
      val ticket = when (val result = client.issueStatusTicket(target)) {
        is HttpResult.Success -> result.value
        is HttpResult.Failure -> {
          scheduleReconnect(target, connectionId, result.userMessage)
          return@launch
        }
      }
      if (stopped || connectionGeneration.get() != connectionId) return@launch
      val wsUrl = try {
        statusWebSocketUrl(target, ticket.ws)
      } catch (error: Exception) {
        scheduleReconnect(target, connectionId, error.message ?: "Invalid status WebSocket URL")
        return@launch
      }
      val request = Request.Builder().url(wsUrl).build()
      synchronized(this@ServerStatusStream) {
        // The ticket request runs asynchronously. Re-check under the same lock
        // used by disconnect so a late result cannot leak a live socket.
        if (stopped || connectionGeneration.get() != connectionId) return@launch
        socket = client.okHttpClient.newWebSocket(request, object : WebSocketListener() {
        override fun onOpen(webSocket: WebSocket, response: Response) {
          if (connectionGeneration.get() != connectionId) return
          reconnectAttempt = 0
          _connectionState.value = ConnectionState.Connected
        }

        override fun onMessage(webSocket: WebSocket, text: String) {
          if (connectionGeneration.get() != connectionId) return
          try {
            handleEnvelope(json.decodeFromString<JsonObject>(text))
          } catch (error: Exception) {
            Log.w("ServerStatusStream", "Ignoring malformed status frame: ${error.message}")
          }
        }

        override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
          if (connectionGeneration.get() == connectionId) scheduleReconnect(target, connectionId, reason)
        }

        override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
          response?.close()
          if (connectionGeneration.get() == connectionId) scheduleReconnect(target, connectionId, t.message ?: "Status socket failed")
        }
        })
      }
    }
  }

  @Synchronized
  private fun statusWebSocketUrl(target: ServerEntry, wsPath: String): String {
    val base = URI(target.url.trimEnd('/'))
    val scheme = when (base.scheme) {
      "https" -> "wss"
      "http" -> "ws"
      else -> throw IllegalArgumentException("Server URL must use http or https")
    }
    val ticketUri = URI(wsPath)
    val path = if (ticketUri.isAbsolute) ticketUri.path else wsPath.substringBefore('?')
    val queryParts = mutableListOf<String>()
    val ticketQuery = if (ticketUri.isAbsolute) ticketUri.rawQuery else wsPath.substringAfter('?', "")
    if (!ticketQuery.isNullOrBlank()) queryParts += ticketQuery
    cursor?.let { queryParts += "since=$it" }
    hubGeneration?.let { queryParts += "epoch=$it" }
    return URI(scheme, null, base.host, base.port, path, queryParts.joinToString("&").ifBlank { null }, null).toString()
  }

  @Synchronized
  private fun scheduleReconnect(target: ServerEntry, connectionId: Long, reason: String) {
    if (stopped || connectionGeneration.get() != connectionId || reconnectJob?.isActive == true) return
    Log.w("ServerStatusStream", "Status stream disconnected: $reason")
    socket = null
    _connectionState.value = ConnectionState.Reconnecting
    val delayMs = (500L shl reconnectAttempt.coerceAtMost(4)).coerceAtMost(10_000L)
    reconnectAttempt++
    reconnectJob = scope.launch {
      delay(delayMs)
      if (!stopped && connectionGeneration.get() == connectionId && server == target) open(target, connectionId)
    }
  }

  @Synchronized
  internal fun handleEnvelope(envelope: JsonObject) {
    val type = envelope["type"]?.jsonPrimitive?.content ?: return
    val generation = envelope["generation"]?.jsonPrimitive?.content
    if (generation != null && hubGeneration != null && generation != hubGeneration) {
      cursor = null
      _sessions.value = emptyMap()
      _workerHealth.value = emptyMap()
      _activeRuns.value = emptyMap()
    }
    if (generation != null) hubGeneration = generation
    envelope["cursor"]?.jsonPrimitive?.longOrNull?.let { cursor = it }

    when (type) {
      "status_snapshot" -> {
        val nextSessions = mutableMapOf<String, LiveSessionStatus>()
        val nextWorkers = mutableMapOf<String, String>()
        val nextRuns = mutableMapOf<String, LiveSessionStatus>()
        val events = envelope["events"] as? JsonArray ?: JsonArray(emptyList())
        events.forEach { element -> applyEvent(element.jsonObject, nextSessions, nextWorkers, nextRuns) }
        _sessions.value = nextSessions
        _workerHealth.value = nextWorkers
        _activeRuns.value = nextRuns
      }
      "status_replay" -> {
        val events = envelope["events"]?.jsonArray ?: return
        events.forEach { applyDelta(it.jsonObject) }
      }
      "status" -> envelope["event"]?.jsonObject?.let(::applyDelta)
    }
  }

  private fun applyDelta(event: JsonObject) {
    val sessionId = event["sessionId"]?.jsonPrimitive?.content
    val workerId = event["workerId"]?.jsonPrimitive?.content
    if (!sessionId.isNullOrBlank()) {
      val status = parseStatus(event, sessionId) ?: return
      if (status.reason == "admission" && !status.runId.isNullOrBlank()) {
        _activeRuns.value = if (status.state == "cancelled") _activeRuns.value - status.runId
        else _activeRuns.value + (status.runId to status)
      } else {
        _sessions.value = _sessions.value + (sessionId to status)
        if (!status.runId.isNullOrBlank()) _activeRuns.value = _activeRuns.value - status.runId
      }
    } else if (!workerId.isNullOrBlank()) {
      val state = event["state"]?.jsonPrimitive?.content ?: return
      _workerHealth.value = _workerHealth.value + (workerId to state)
    }
  }

  private fun applyEvent(
    event: JsonObject,
    nextSessions: MutableMap<String, LiveSessionStatus>,
    nextWorkers: MutableMap<String, String>,
    nextRuns: MutableMap<String, LiveSessionStatus>,
  ) {
    val sessionId = event["sessionId"]?.jsonPrimitive?.content
    val workerId = event["workerId"]?.jsonPrimitive?.content
    if (!sessionId.isNullOrBlank()) {
      val status = parseStatus(event, sessionId) ?: return
      if (status.reason == "admission" && !status.runId.isNullOrBlank()) nextRuns[status.runId] = status
      else nextSessions[sessionId] = status
    } else if (!workerId.isNullOrBlank()) {
      event["state"]?.jsonPrimitive?.content?.let { nextWorkers[workerId] = it }
    }
  }

  private fun parseStatus(event: JsonObject, sessionId: String): LiveSessionStatus? {
    val state = event["state"]?.jsonPrimitive?.content ?: return null
    return LiveSessionStatus(
      sessionId = sessionId,
      workerId = event["workerId"]?.jsonPrimitive?.content ?: "local",
      state = state,
      reason = event["reason"]?.jsonPrimitive?.content,
      detail = event["detail"]?.jsonPrimitive?.content,
      runId = event["runId"]?.jsonPrimitive?.content,
      position = event["position"]?.jsonPrimitive?.intOrNull,
      queuedAt = event["queuedAt"]?.jsonPrimitive?.content,
      updatedAt = event["updatedAt"]?.jsonPrimitive?.content,
    )
  }
}
