package com.example.picompanion.data.websocket

import com.example.picompanion.data.api.apiJson
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import java.util.concurrent.atomic.AtomicLong

class SessionEventSocket(
  private val okHttpClient: OkHttpClient = OkHttpClient.Builder()
    .connectTimeout(10, java.util.concurrent.TimeUnit.SECONDS)
    .readTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
    .writeTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
    .pingInterval(25, java.util.concurrent.TimeUnit.SECONDS)
    .build(),
  private val json: Json = apiJson,
) {

  private var webSocket: WebSocket? = null
  // Unlimited: a stalled collector must never silently drop session events.
  private val _events = Channel<SocketEvent>(Channel.UNLIMITED)
  val events: Flow<SocketEvent> = _events.receiveAsFlow()

  @Volatile
  private var connected = false
  private val generation = AtomicLong(0)
  // Cursor baseline is set from the reconnect `since` value. It detects both
  // server ring truncation during replay and slow-subscriber drops while live.
  @Volatile private var lastEventId: Long? = null
  // After events_lost, suppress gap detection until the first event from the
  // fresh replay establishes a new consecutive baseline.
  @Volatile private var resynchronizing = false
  // LinkedHashMap for O(1) add/contains and O(1) eviction of oldest entries.
  // All access is synchronized to prevent races between add and size check.
  private val seenEventIds = object : LinkedHashMap<Long, Boolean>(512) {
    override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Long, Boolean>?): Boolean {
      return size > 2_000
    }
  }

  /**
   * Opens a server-ticketed session stream. `wsPath` is returned by
   * POST /v1/ws-tickets and already contains the single-use ticket.
   */
  fun connect(server: ServerEntry, wsPath: String, since: Long? = null) {
    val previous = webSocket
    val connectionId = generation.incrementAndGet()
    previous?.close(1000, "Replacing connection")
    webSocket = null
    connected = false
    resynchronizing = false
    // Long.MAX_VALUE deliberately suppresses replay on first open; history is
    // loaded separately, so it cannot serve as a gap-detection baseline.
    lastEventId = since?.takeUnless { it == Long.MAX_VALUE }

    val baseUrl = server.url.trimEnd('/')
    val wsUrl = try {
      val uri = java.net.URI(baseUrl)
      val wsScheme = when (uri.scheme) {
        "https" -> "wss"
        else -> "ws"
      }
      val ticketUri = java.net.URI(wsPath)
      val path = if (ticketUri.isAbsolute) ticketUri.path else wsPath.substringBefore('?')
      val existingQuery = if (ticketUri.isAbsolute) ticketUri.rawQuery else wsPath.substringAfter('?', "")
      val query = buildList {
        if (!existingQuery.isNullOrBlank()) add(existingQuery)
        if (since != null) add("since=$since")
      }.joinToString("&")
      java.net.URI(wsScheme, null, uri.host, uri.port, path, query.ifBlank { null }, null).toString()
    } catch (e: Exception) {
      _events.trySend(SocketEvent.Error("Invalid server URL: ${e.message}", e))
      return
    }

    // The ticket is deliberately the only credential sent during the upgrade.
    webSocket = okHttpClient.newWebSocket(Request.Builder().url(wsUrl).build(), object : WebSocketListener() {
      override fun onOpen(webSocket: WebSocket, response: Response) {
        if (generation.get() != connectionId) return
        connected = true
        _events.trySend(SocketEvent.Connected)
      }

      override fun onMessage(webSocket: WebSocket, text: String) {
        if (generation.get() != connectionId) return
        try {
          val jsonObj = json.decodeFromString<JsonObject>(text)
          val eventId = jsonObj["_daemonEventId"]?.jsonPrimitive?.longOrNull
          if (eventId != null) {
            val previous = lastEventId
            if (resynchronizing) {
              // After events_lost, accept the first replay event as the new
              // baseline without gap-checking against the stale cursor.
              lastEventId = eventId
              resynchronizing = false
            } else if (previous != null && eventId > previous + 1) {
              resynchronizing = true
              _events.trySend(SocketEvent.EventsLost(previous, eventId))
            } else {
              if (previous == null || eventId > previous) lastEventId = eventId
            }
            val isDuplicate: Boolean
            synchronized(seenEventIds) {
              isDuplicate = seenEventIds.put(eventId, true) != null
            }
            if (isDuplicate) return
          }
          val type = jsonObj["type"]?.jsonPrimitive?.content ?: "unknown"
          if (type == "events_lost") {
            val expectedAfter = jsonObj["expectedAfter"]?.jsonPrimitive?.longOrNull ?: lastEventId ?: 0
            val received = jsonObj["received"]?.jsonPrimitive?.longOrNull ?: expectedAfter + 1
            resynchronizing = true
            _events.trySend(SocketEvent.EventsLost(expectedAfter, received))
            return
          }
          _events.trySend(SocketEvent.Message(jsonObj, type, eventId))
        } catch (e: Exception) {
          _events.trySend(SocketEvent.RawMessage(text))
        }
      }

      override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
        if (generation.get() != connectionId) return
        webSocket.close(1000, null)
        connected = false
        _events.trySend(SocketEvent.Disconnected("Closing: $reason"))
      }

      override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
        if (generation.get() != connectionId) return
        connected = false
        _events.trySend(SocketEvent.Disconnected("Closed: $reason"))
      }

      override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
        if (generation.get() != connectionId) return
        connected = false
        _events.trySend(SocketEvent.Error(t.message ?: "WebSocket error", t))
      }
    })
  }

  /**
   * Enqueue a command for delivery over the active WebSocket.
   * OkHttp returns false when the socket is closing/closed, allowing the
   * caller to use the REST transport without submitting the command twice.
   */
  fun disconnect() {
    generation.incrementAndGet()
    webSocket?.close(1000, "Client disconnect")
    webSocket = null
    connected = false
    lastEventId = null
    synchronized(seenEventIds) { seenEventIds.clear() }
  }

  fun isConnected(): Boolean = connected
}
