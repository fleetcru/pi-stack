package com.example.picompanion.ui.sessions

import com.example.picompanion.data.model.ServerSession
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

class SessionGroupingTest {
  @Test
  fun idleAndProcessRunningSessionsAreNotGroupedAsWorking() {
    val now = Instant.parse("2026-09-14T12:00:00Z")
    val working = ServerSession(id = "w", title = "Working", status = "working", updatedAt = now.toString())
    val idle = ServerSession(id = "i", title = "Idle", status = "idle", updatedAt = now.toString())
    val processRunning = ServerSession(id = "r", title = "Attached", status = "running", updatedAt = now.toString())
    val older = ServerSession(id = "o", title = "Older", status = "idle", updatedAt = "2026-01-01T00:00:00Z")

    val groups = groupSessions(listOf(working, idle, processRunning, older), now)

    assertEquals(listOf("w"), groups.single { it.title == "Running" }.sessions.map { it.id })
    assertEquals(setOf("i", "r"), groups.single { it.title == "Recent" }.sessions.map { it.id }.toSet())
    assertEquals(listOf("o"), groups.single { it.title == "Older" }.sessions.map { it.id })
  }

  @Test
  fun waitingForInputCountsAsRunning() {
    val now = Instant.parse("2026-09-14T12:00:00Z")
    val waiting = ServerSession(
      id = "ask",
      title = "Question",
      status = "waiting_for_input",
      updatedAt = now.toString(),
    )

    val groups = groupSessions(listOf(waiting), now)

    assertTrue(groups.single().title == "Running")
  }
}
