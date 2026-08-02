package com.example.picompanion.data.websocket

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull

/**
 * Tracks the server's monotonic event IDs across WebSocket reconnects.
 *
 * The tracker preserves its duplicate window during reconnects so replayed
 * events are not rendered twice. [clear] is reserved for an explicit user
 * disconnect, which starts a wholly new stream.
 */
internal class EventSequenceTracker {
  private var lastEventId: Long? = null
  private var resynchronizing = false
  private val seenEventIds = object : LinkedHashMap<Long, Boolean>(1024) {
    override fun removeEldestEntry(eldest: MutableMap.MutableEntry<Long, Boolean>?): Boolean = size > 10_000
  }

  @Synchronized
  fun beginConnection(since: Long?) {
    resynchronizing = false
    // Long.MAX_VALUE suppresses replay on the first connection; HTTP history
    // is loaded independently and therefore provides no event-ID baseline.
    lastEventId = since?.takeUnless { it == Long.MAX_VALUE }
  }

  @Synchronized
  fun clear() {
    lastEventId = null
    resynchronizing = false
    seenEventIds.clear()
  }

  @Synchronized
  fun process(jsonObj: JsonObject): List<SocketEvent> {
    val output = mutableListOf<SocketEvent>()
    val eventId = jsonObj["_daemonEventId"]?.jsonPrimitive?.longOrNull
    if (eventId != null) {
      val previous = lastEventId
      if (resynchronizing) {
        lastEventId = eventId
        resynchronizing = false
      } else if (previous != null && eventId > previous + 1) {
        resynchronizing = true
        output += SocketEvent.EventsLost(previous, eventId)
      } else if (previous == null || eventId > previous) {
        lastEventId = eventId
      }

      val isDuplicate = seenEventIds.put(eventId, true) != null
      if (isDuplicate) return output
    }

    val type = jsonObj["type"]?.jsonPrimitive?.content ?: "unknown"
    if (type == "events_lost") {
      val expectedAfter = jsonObj["expectedAfter"]?.jsonPrimitive?.longOrNull ?: lastEventId ?: 0
      val received = jsonObj["received"]?.jsonPrimitive?.longOrNull ?: expectedAfter + 1
      // A locally detected gap may be followed by the server's own sentinel.
      // One recovery is enough; keep the resync state without triggering a
      // second history reload for the same interruption.
      if (!resynchronizing) output += SocketEvent.EventsLost(expectedAfter, received)
      resynchronizing = true
    } else {
      output += SocketEvent.Message(jsonObj, type, eventId)
    }
    return output
  }
}
