package com.example.picompanion.data.model

import org.junit.Assert.assertEquals
import org.junit.Test

class SessionInventoryDedupTest {
  private val active = listOf(ServerSession(id = "active-1"))

  @Test
  fun `machine inventory hides histories already represented by active sessions`() {
    val machine = listOf(
      MachineSession(id = "history-1", path = "one.jsonl", cwd = "/project", serverSessionId = "active-1"),
      MachineSession(id = "history-2", path = "two.jsonl", cwd = "/other"),
    )

    assertEquals(listOf("history-2"), visibleMachineSessions(active, machine).map { it.id })
  }

  @Test
  fun `global inventory hides local duplicates but preserves worker sessions`() {
    val global = listOf(
      GlobalSession(id = "local:active-1", originId = "active-1", workerId = "local", session = active.first()),
      GlobalSession(id = "worker:active-1", originId = "active-1", workerId = "worker", session = active.first()),
    )

    assertEquals(listOf("worker:active-1"), visibleGlobalSessions(active, global).map { it.id })
  }
}
