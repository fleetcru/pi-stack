package com.example.picompanion.ui.sessiondetail

import org.junit.Assert.assertEquals
import org.junit.Test

class SessionHistoryStateTest {
  @Test
  fun historyFetchedAfterLiveRowStillAppearsFirst() {
    val state = SessionHistoryState()
    val historical = SessionTimelineItem.Chat("You", "earlier prompt", "2026-08-08T20:00:00Z", true)
    val live = SessionTimelineItem.Chat("Pi Agent", "current response", "", false, order = 1)
    var sequence = 1L

    val merged = state.applyPage(
      page = listOf(historical),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(live),
      stamp = { item ->
        when (item) {
          is SessionTimelineItem.Chat -> if (item.order > 0) item else item.copy(order = ++sequence)
          else -> item
        }
      },
    )

    assertEquals(listOf("earlier prompt", "current response"), merged.map { (it as SessionTimelineItem.Chat).text })
    // Numeric identity order reflects arrival, but must not override transcript order.
    assertEquals(listOf(2L, 1L), merged.map { it.order })
  }

  @Test
  fun newestRefreshKeepsAlreadyLoadedOlderPrefix() {
    val state = SessionHistoryState()
    val older = SessionTimelineItem.Chat("You", "oldest", "t1", true, order = 1)
    val overlap = SessionTimelineItem.Chat("Pi Agent", "existing", "t2", false, order = 2)
    state.restore(listOf(older, overlap), offset = 2, older = true)
    val newest = SessionTimelineItem.Chat("You", "newest", "t3", true)
    var sequence = 2L

    val merged = state.applyPage(
      page = listOf(overlap.copy(order = 0), newest),
      appendOld = false,
      nextOffset = 2,
      hasOlder = true,
      liveItems = listOf(older, overlap),
      stamp = { item ->
        when (item) {
          is SessionTimelineItem.Chat -> if (item.order > 0) item else item.copy(order = ++sequence)
          else -> item
        }
      },
    )

    assertEquals(listOf("oldest", "existing", "newest"), merged.map { (it as SessionTimelineItem.Chat).text })
    assertEquals(listOf("oldest", "existing", "newest"), state.historicalItems.map { (it as SessionTimelineItem.Chat).text })
  }

  @Test
  fun completedHistoryResponseReplacesLongStreamedPrefix() {
    val state = SessionHistoryState()
    val prefix = "This response has enough streamed text to match"
    val live = SessionTimelineItem.Chat("Pi Agent", prefix, "", false, order = 4)
    val durable = SessionTimelineItem.Chat("Pi Agent", "$prefix the durable ending.", "t2", false)

    val merged = state.applyPage(
      page = listOf(durable),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(live),
      stamp = { item -> if (item is SessionTimelineItem.Chat && item.order == 0L) item.copy(order = 5) else item },
    )

    assertEquals(1, merged.size)
    assertEquals(durable.text, (merged.single() as SessionTimelineItem.Chat).text)
  }

  @Test
  fun refreshKeepsCachedOlderRowsBeforeNewerRows() {
    val state = SessionHistoryState()
    val older = SessionTimelineItem.Chat("Pi Agent", "older", "2026-08-08T20:00:00Z", false, order = 10)
    state.restore(listOf(older), offset = 1, older = false)
    val newer = SessionTimelineItem.Chat("Pi Agent", "newer", "2026-08-08T21:00:00Z", false)

    val merged = state.applyPage(
      page = listOf(newer),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(older),
      stamp = { item ->
        when (item) {
          is SessionTimelineItem.Chat -> if (item.order > 0) item else item.copy(order = 11)
          else -> item
        }
      },
    )

    assertEquals(listOf("older", "newer"), merged.map { (it as SessionTimelineItem.Chat).text })
  }

  @Test
  fun refreshedHistoryPreservesCachedOrders() {
    val state = SessionHistoryState()
    val cached = SessionTimelineItem.Chat(
      author = "Pi Agent",
      text = "stable text",
      time = "2026-08-08T20:00:00Z",
      isUser = false,
      order = 17,
    )
    state.restore(listOf(cached), offset = 1, older = false)

    val refreshed = cached.copy(order = 0)
    val merged = state.applyPage(
      page = listOf(refreshed),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(cached),
      stamp = { item ->
        when (item) {
          is SessionTimelineItem.Chat -> if (item.order > 0) item else item.copy(order = 99)
          else -> item
        }
      },
    )

    assertEquals(17, merged.single().order)
    assertEquals(17, state.historicalItems.single().order)
  }

  @Test
  fun durableHistoryReplacesOptimisticTextMessage() {
    val state = SessionHistoryState()
    val optimistic = SessionTimelineItem.Chat(
      author = "You",
      text = "hello from mobile",
      time = "now",
      isUser = true,
      order = 10,
    )
    val durable = optimistic.copy(time = "2026-08-11T08:29:23Z", order = 0)

    val merged = state.applyPage(
      page = listOf(durable),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(optimistic),
      stamp = { it },
    )

    assertEquals(1, merged.size)
    assertEquals("2026-08-11T08:29:23Z", (merged.single() as SessionTimelineItem.Chat).time)
  }

  @Test
  fun durableHistoryRetainsOptimisticImagePreview() {
    val state = SessionHistoryState()
    val preview = SessionTimelineItem.Chat(
      author = "You",
      text = "look at this",
      time = "now",
      isUser = true,
      imageUris = listOf(android.net.Uri.EMPTY),
      order = 10,
    )
    val durable = preview.copy(time = "2026-08-11T08:29:23Z", imageUris = emptyList(), order = 0)

    val merged = state.applyPage(
      page = listOf(durable),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(preview),
      stamp = { it },
    )

    assertEquals(1, merged.size)
    assertEquals(preview.imageUris, (merged.single() as SessionTimelineItem.Chat).imageUris)
    assertEquals("2026-08-11T08:29:23Z", (merged.single() as SessionTimelineItem.Chat).time)
  }

  @Test
  fun durableHistoryReplacesLiveAssistantResponse() {
    val state = SessionHistoryState()
    val live = SessionTimelineItem.Chat(
      author = "Pi Agent",
      text = "The task is complete.\n",
      time = "",
      isUser = false,
      order = 11,
    )
    val durable = live.copy(
      text = "The task is complete.",
      time = "2026-08-11T08:30:10Z",
      order = 0,
    )

    val merged = state.applyPage(
      page = listOf(durable),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(live),
      stamp = { it },
    )

    assertEquals(1, merged.size)
    assertEquals("2026-08-11T08:30:10Z", (merged.single() as SessionTimelineItem.Chat).time)
  }

  @Test
  fun partialLiveAssistantResponseSurvivesHistoryRefresh() {
    val state = SessionHistoryState()
    val durable = SessionTimelineItem.Chat(
      author = "Pi Agent",
      text = "An earlier response",
      time = "2026-08-11T08:30:10Z",
      isUser = false,
      order = 10,
    )
    val partial = durable.copy(text = "The current response is stream", time = "", order = 11)

    val merged = state.applyPage(
      page = listOf(durable),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(durable, partial),
      stamp = { it },
    )

    assertEquals(2, merged.size)
    assertEquals("The current response is stream", (merged.last() as SessionTimelineItem.Chat).text)
  }

  @Test
  fun olderIdenticalResponseDoesNotReplaceCurrentLiveResponse() {
    val state = SessionHistoryState()
    val older = SessionTimelineItem.Chat(
      author = "Pi Agent",
      text = "Done.",
      time = "2026-08-11T08:30:10Z",
      isUser = false,
      order = 10,
    )
    val current = older.copy(time = "", order = 11)

    val merged = state.applyPage(
      page = listOf(older),
      appendOld = false,
      nextOffset = 1,
      hasOlder = false,
      liveItems = listOf(older, current),
      stamp = { it },
    )

    assertEquals(2, merged.size)
    assertEquals("", (merged.last() as SessionTimelineItem.Chat).time)
  }
}
