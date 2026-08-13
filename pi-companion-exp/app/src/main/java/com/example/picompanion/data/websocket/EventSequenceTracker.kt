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
  fun forget(eventId: Long?) {
    if (eventId != null) seenEventIds.remove(eventId)
  }

  @Synchronized
  fun process(jsonObj: JsonObject): List<SocketEvent> {
    val output = mutableListOf<SocketEvent>()
    val type = jsonObj["type"]?.jsonPrimitive?.content ?: "unknown"
    val eventId = jsonObj["_daemonEventId"]?.jsonPrimitive?.longOrNull
    // The server deliberately stamps the synthetic events_lost sentinel with
    // ID 0. It is control metadata, not a restarted event sequence, so it must
    // not clear the duplicate window needed for the retained-ring replay.
    if (eventId != null && !(type == "events_lost" && eventId == 0L)) {
      // A retained-ring replay may begin with events already delivered before
      // the gap. Ignore those without consuming the resynchronization baseline.
      if (seenEventIds.containsKey(eventId)) return output
      val previous = lastEventId
      if (resynchronizing) {
        lastEventId = eventId
        resynchronizing = false
      } else if (previous != null && eventId > previous + 1) {
        resynchronizing = true
        output += SocketEvent.EventsLost(previous, eventId)
      } else if (previous != null && eventId < previous) {
        // Server event ID went backward — likely a server restart with a
        // fresh ID sequence. Reset the baseline so the new sequence is
        // accepted and we don't permanently think events are lost.
        lastEventId = eventId
        seenEventIds.clear()
      } else if (previous == null || eventId > previous) {
        lastEventId = eventId
      }

      seenEventIds[eventId] = true
    }

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
