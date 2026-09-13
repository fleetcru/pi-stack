package com.example.picompanion.data.notifications

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SessionNotificationManagerTest {
  @Test
  fun initialSnapshotDoesNotNotify() {
    assertNull(runtimeNotificationForTransition("session-123", null, "idle"))
  }

  @Test
  fun replayedStateDoesNotNotify() {
    assertNull(runtimeNotificationForTransition("session-123", "waiting_for_input", "waiting_for_input"))
  }

  @Test
  fun completionUsesShortSessionId() {
    val notification = runtimeNotificationForTransition(
      "1234567890abcdef",
      "working",
      "idle",
    )

    assertEquals("Pi finished", notification?.title)
    assertEquals("Session 12345678 completed its task.", notification?.body)
  }

  @Test
  fun idleAfterNonWorkingStateDoesNotNotify() {
    assertNull(runtimeNotificationForTransition("session-123", "waiting_for_input", "idle"))
  }

  @Test
  fun waitingAndFailureUseGenericPrivateText() {
    assertEquals(
      "Session session- is waiting for your response.",
      runtimeNotificationForTransition("session-123", "working", "waiting_for_input")?.body,
    )
    assertEquals(
      "Session session- failed.",
      runtimeNotificationForTransition("session-123", "working", "failed")?.body,
    )
  }
}
