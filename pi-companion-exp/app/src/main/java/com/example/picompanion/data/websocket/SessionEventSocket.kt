package com.example.picompanion.data.websocket

import com.example.picompanion.data.api.apiJson
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import android.util.Log
import java.util.concurrent.atomic.AtomicLong

class SessionEventSocket(
  private val okHttpClient: OkHttpClient,
  private val json: Json = apiJson,
) {

  private var webSocket: WebSocket? = null
  // Bounded buffer prevents OOM from a stalled collector while still
  // retaining enough events for normal interactive use. 2000 events
  // covers ~10 minutes of typical streaming at 3 events/sec.
  private val _events = Channel<SocketEvent>(2000)
  val events: Flow<SocketEvent> = _events.receiveAsFlow()

  @Volatile
  private var connected = false
  private val generation = AtomicLong(0)
  private val eventSequence = EventSequenceTracker()

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
    // Carry the duplicate window across reconnects so replayed events are not
    // rendered twice. Only an explicit disconnect clears this state.
    eventSequence.beginConnection(since)

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
        // Connected is a critical lifecycle event — if dropped, the UI stays
        // stuck as "connecting" forever. Ensure delivery by draining stale
        // data events if the channel is full.
        if (!_events.trySend(SocketEvent.Connected).isSuccess) {
          // Channel is full — drain oldest items to make room for Connected.
          repeat(100) { _events.tryReceive() }
          _events.trySend(SocketEvent.Connected)
        }
      }

      override fun onMessage(webSocket: WebSocket, text: String) {
        if (generation.get() != connectionId) return
        try {
          val jsonObj = json.decodeFromString<JsonObject>(text)
          eventSequence.process(jsonObj).forEach { event ->
            val result = _events.trySend(event)
            if (result.isClosed) {
              Log.w("SessionEventSocket", "Channel closed — event dropped: ${event::class.simpleName}")
            }
          }
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
        response?.close()
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
    eventSequence.clear()
  }

  fun isConnected(): Boolean = connected
}
