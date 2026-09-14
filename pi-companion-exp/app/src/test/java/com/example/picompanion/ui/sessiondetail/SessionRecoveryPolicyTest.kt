package com.example.picompanion.ui.sessiondetail

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SessionRecoveryPolicyTest {
  @Test
  fun cacheRestoresOnlyIntoAnEmptyTimeline() {
    val item = SessionTimelineItem.Chat(
      author = "Pi Agent",
      text = "new live text",
      time = "",
      isUser = false,
    )

    assertTrue(shouldRestoreCachedTimeline(emptyList(), emptyList()))
    assertFalse(shouldRestoreCachedTimeline(listOf(item), emptyList()))
    assertFalse(shouldRestoreCachedTimeline(emptyList(), listOf(item)))
  }
}
