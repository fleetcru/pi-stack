package com.example.picompanion.ui.sessiondetail

import org.junit.Assert.assertEquals
import org.junit.Test

class SessionHistoryStateTest {
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
