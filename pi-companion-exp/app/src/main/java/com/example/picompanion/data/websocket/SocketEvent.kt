package com.example.picompanion.data.websocket

import kotlinx.serialization.json.JsonObject

sealed interface SocketEvent {
  data object Connected : SocketEvent
  /** The monotonic daemon event cursor jumped; reload durable history. */
  data class EventsLost(val expectedAfter: Long, val received: Long) : SocketEvent
  data class Disconnected(val reason: String) : SocketEvent
  data class Message(
    val raw: JsonObject,
    val type: String,
    val eventId: Long? = null,
  ) : SocketEvent
  data class RawMessage(val text: String) : SocketEvent
  data class Error(val message: String, val cause: Throwable? = null) : SocketEvent
}
