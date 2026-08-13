package com.example.picompanion.data.websocket

import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class EventSequenceTrackerTest {

  private fun event(id: Long, type: String = "message_update") = Json.parseToJsonElement(
    """{"_daemonEventId":$id,"type":"$type"}""",
  ).let { it as kotlinx.serialization.json.JsonObject }

  @Test
  fun reconnectDeduplicatesReplayedEventsAndAcceptsNewEvents() {
    val tracker = EventSequenceTracker()
    tracker.beginConnection(Long.MAX_VALUE)
    assertEquals(1, tracker.process(event(1)).size)

    tracker.beginConnection(1)
    assertTrue("replayed event should be suppressed", tracker.process(event(1)).isEmpty())

    val events = tracker.process(event(2))
    assertEquals(1, events.size)
    assertEquals(2L, (events.single() as SocketEvent.Message).eventId)
  }

  @Test
  fun missingEventEmitsRecoverySignalAndResynchronizes() {
    val tracker = EventSequenceTracker()
    tracker.beginConnection(10)

    val gap = tracker.process(event(12))
    assertEquals(2, gap.size)
    assertEquals(SocketEvent.EventsLost(10, 12), gap.first())
    assertEquals(12L, (gap.last() as SocketEvent.Message).eventId)

    val next = tracker.process(event(13))
    assertEquals(1, next.size)
    assertEquals(13L, (next.single() as SocketEvent.Message).eventId)
  }

  @Test
  fun serverSentinelAfterLocalGapDoesNotTriggerSecondRecovery() {
    val tracker = EventSequenceTracker()
    tracker.beginConnection(10)
    assertEquals(2, tracker.process(event(12)).size)
    val sentinel = Json.parseToJsonElement(
      """{"type":"events_lost","expectedAfter":10,"received":20}""",
    ) as kotlinx.serialization.json.JsonObject

    assertTrue(tracker.process(sentinel).isEmpty())
    assertEquals(1, tracker.process(event(20)).size)
  }

  @Test
  fun serverEventsLostSentinelResetsBaselineForFreshReplay() {
    val tracker = EventSequenceTracker()
    tracker.beginConnection(5)
    val sentinel = Json.parseToJsonElement(
      """{"type":"events_lost","expectedAfter":5,"received":20}""",
    ) as kotlinx.serialization.json.JsonObject

    assertEquals(listOf(SocketEvent.EventsLost(5, 20)), tracker.process(sentinel))
    assertEquals(1, tracker.process(event(20)).size)
  }

  @Test
  fun zeroIdLossSentinelPreservesReplayDeduplication() {
    val tracker = EventSequenceTracker()
    tracker.beginConnection(null)
    tracker.process(event(5))
    val sentinel = Json.parseToJsonElement(
      """{"_daemonEventId":0,"type":"events_lost","expectedAfter":5,"received":8}""",
    ) as kotlinx.serialization.json.JsonObject

    assertEquals(listOf(SocketEvent.EventsLost(5, 8)), tracker.process(sentinel))
    assertTrue("retained replay should remain deduplicated", tracker.process(event(5)).isEmpty())
    assertEquals(1, tracker.process(event(8)).size)
  }

  @Test
  fun forgottenDroppedEventCanBeRecoveredByReplay() {
    val tracker = EventSequenceTracker()
    tracker.beginConnection(null)
    assertEquals(1, tracker.process(event(7)).size)
    tracker.forget(7)
    tracker.beginConnection(6)

    assertEquals(1, tracker.process(event(7)).size)
  }

  @Test
  fun explicitDisconnectClearsDuplicateWindow() {
    val tracker = EventSequenceTracker()
    tracker.beginConnection(null)
    tracker.process(event(1))
    tracker.clear()
    tracker.beginConnection(null)

    assertEquals(1, tracker.process(event(1)).size)
  }
}
