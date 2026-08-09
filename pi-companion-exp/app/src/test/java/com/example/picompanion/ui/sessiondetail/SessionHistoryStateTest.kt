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
}
