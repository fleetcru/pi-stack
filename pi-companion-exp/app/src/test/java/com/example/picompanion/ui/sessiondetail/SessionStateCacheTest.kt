package com.example.picompanion.ui.sessiondetail

import org.junit.Assert.assertEquals
import org.junit.Test

class SessionStateCacheTest {
  @Test
  fun trimmingKeepsEverySegmentFromBoundaryMessage() {
    val first = SessionTimelineItem.Chat("Pi Agent", "first", "t0", false, sourceId = "history-0-segment-0")
    val boundaryStart = SessionTimelineItem.Chat("Pi Agent", "before tool", "t1", false, sourceId = "history-1-segment-0")
    val boundaryEnd = boundaryStart.copy(text = "after tool", sourceId = "history-1-segment-1")
    val rest = (2..500).map { index ->
      SessionTimelineItem.Chat("Pi Agent", "message $index", "t$index", false, sourceId = "history-$index-segment-0")
    }
    val history = listOf(first, boundaryStart, boundaryEnd) + rest
    val key = "history-recovery:boundary"

    SessionStateCache.put(
      key,
      SessionStateCache.Entry(
        items = history,
        historicalItems = history,
        nextHistoryOffset = 501,
        hasOlder = true,
        title = "Session",
        project = "",
        cwd = "/tmp",
        lastEventId = 12,
        totalHistoryMessages = 501,
      ),
    )

    val cached = requireNotNull(SessionStateCache.get(key))
    assertEquals(501, cached.historicalItems.size)
    assertEquals("history-1-segment-0", (cached.historicalItems.first() as SessionTimelineItem.Chat).sourceId)
    assertEquals(500, cached.nextHistoryOffset)
  }

  @Test
  fun trimmingCacheKeepsPaginationContiguous() {
    val history = (400 until 1_000).map { index ->
      SessionTimelineItem.Chat(
        author = "Pi Agent",
        text = "message $index",
        time = "t$index",
        isUser = false,
        sourceId = "history-$index-segment-0",
      )
    }
    val key = "history-recovery:trimmed"

    SessionStateCache.put(
      key,
      SessionStateCache.Entry(
        items = history,
        historicalItems = history,
        nextHistoryOffset = 600,
        hasOlder = false,
        title = "Session",
        project = "",
        cwd = "/tmp",
        lastEventId = 12,
        totalHistoryMessages = 1_000,
      ),
    )

    val cached = requireNotNull(SessionStateCache.get(key))
    assertEquals(500, cached.historicalItems.size)
    assertEquals("history-500-segment-0", (cached.historicalItems.first() as SessionTimelineItem.Chat).sourceId)
    assertEquals(500, cached.nextHistoryOffset)
    assertEquals(true, cached.hasOlder)
  }
}
