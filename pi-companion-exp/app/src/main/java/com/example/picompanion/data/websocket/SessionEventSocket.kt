package com.example.picompanion.data.websocket

import com.example.picompanion.data.api.apiJson
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.channels.Channel
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.merge
import kotlinx.coroutines.flow.receiveAsFlow
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
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
  // Loss/control events use a separate conflated channel, so recovery cannot
  // be blocked behind the saturated data channel that triggered it.
  private val _control = Channel<SocketEvent>(Channel.CONFLATED)
  val events: Flow<SocketEvent> = merge(_events.receiveAsFlow(), _control.receiveAsFlow())

  @Volatile
  private var connected = false
  private val generation = AtomicLong(0)
  private val eventSequence = EventSequenceTracker()
  // Decode large JSON/MessagePack payloads away from OkHttp's callback thread.
  // The single worker preserves server event ordering.
  private val parsingScope = CoroutineScope(SupervisorJob() + Dispatchers.Default.limitedParallelism(1))

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
        add("codec=msgpack")
      }.joinToString("&")
      java.net.URI(wsScheme, null, uri.host, uri.port, path, query.ifBlank { null }, null).toString()
    } catch (e: Exception) {
      emitEvent(SocketEvent.Error("Invalid server URL: ${e.message}", e))
      return
    }

    // The ticket is deliberately the only credential sent during the upgrade.
    webSocket = okHttpClient.newWebSocket(Request.Builder().url(wsUrl).build(), object : WebSocketListener() {
      override fun onOpen(webSocket: WebSocket, response: Response) {
        if (generation.get() != connectionId) return
        connected = true
        emitEvent(SocketEvent.Connected)
      }

      override fun onMessage(webSocket: WebSocket, text: String) {
        parsingScope.launch {
          if (generation.get() != connectionId) return@launch
          try {
            val jsonObj = json.decodeFromString<JsonObject>(text)
            if (generation.get() == connectionId) eventSequence.process(jsonObj).forEach(::emitEvent)
          } catch (e: Exception) {
            emitEvent(SocketEvent.RawMessage(text))
          }
        }
      }

      override fun onMessage(webSocket: WebSocket, bytes: okio.ByteString) {
        parsingScope.launch {
          if (generation.get() != connectionId) return@launch
          try {
            val value = org.msgpack.core.MessagePack.newDefaultUnpacker(bytes.toByteArray()).use { unpacker ->
              unpacker.unpackValue()
            }
            val jsonObj = msgpackToJson(value) as? JsonObject
              ?: throw IllegalArgumentException("MessagePack event must be an object")
            if (generation.get() == connectionId) eventSequence.process(jsonObj).forEach(::emitEvent)
          } catch (e: Exception) {
            Log.w("SessionEventSocket", "Failed to decode MessagePack: ${e.message}")
          }
        }
      }

      override fun onClosing(webSocket: WebSocket, code: Int, reason: String) {
        if (generation.get() != connectionId) return
        webSocket.close(1000, null)
        connected = false
        emitEvent(SocketEvent.Disconnected("Closing: $reason"))
      }

      override fun onClosed(webSocket: WebSocket, code: Int, reason: String) {
        if (generation.get() != connectionId) return
        connected = false
        emitEvent(SocketEvent.Disconnected("Closed: $reason"))
      }

      override fun onFailure(webSocket: WebSocket, t: Throwable, response: Response?) {
        response?.close()
        if (generation.get() != connectionId) return
        connected = false
        emitEvent(SocketEvent.Error(t.message ?: "WebSocket error", t))
      }
    })
  }

  private fun msgpackToJson(value: org.msgpack.value.Value): JsonElement = when (value.valueType) {
    org.msgpack.value.ValueType.NIL -> JsonNull
    org.msgpack.value.ValueType.BOOLEAN -> JsonPrimitive(value.asBooleanValue().boolean)
    org.msgpack.value.ValueType.INTEGER -> JsonPrimitive(value.asIntegerValue().toLong())
    org.msgpack.value.ValueType.FLOAT -> JsonPrimitive(value.asFloatValue().toDouble())
    org.msgpack.value.ValueType.STRING -> JsonPrimitive(value.asStringValue().asString())
    org.msgpack.value.ValueType.BINARY -> JsonPrimitive(
      java.util.Base64.getEncoder().encodeToString(value.asBinaryValue().asByteArray()),
    )
    org.msgpack.value.ValueType.ARRAY -> JsonArray(value.asArrayValue().list().map(::msgpackToJson))
    org.msgpack.value.ValueType.MAP -> JsonObject(value.asMapValue().map().entries.associate { (key, entry) ->
      val name = if (key.isStringValue) key.asStringValue().asString() else key.toString()
      name to msgpackToJson(entry)
    })
    org.msgpack.value.ValueType.EXTENSION -> JsonPrimitive(value.toString())
  }

  private fun emitEvent(event: SocketEvent) {
    val result = _events.trySend(event)
    if (result.isSuccess) return
    if (result.isClosed) {
      Log.w("SessionEventSocket", "Channel closed — event dropped: ${event::class.simpleName}")
      return
    }
    // Keep the data buffer bounded while independently signaling durable
    // history reconciliation. CONFLATED prevents reconnect storms.
    _control.trySend(SocketEvent.EventsLost(expectedAfter = 0, received = 0))
    Log.w("SessionEventSocket", "Event channel full — scheduling history reconciliation")
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

  /** Permanently disposes this socket and its parser worker. */
  fun close() {
    disconnect()
    parsingScope.cancel()
    _events.close()
    _control.close()
  }

  fun isConnected(): Boolean = connected
}
