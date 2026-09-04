package com.example.picompanion.ui.sessions

import com.example.picompanion.data.model.ServerSession
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SessionInventoryStateTest {
  @Test
  fun staleRevisionClearsOnlyThroughLoadedRevision() {
    val serverId = "revision-test"
    SessionInventoryState.markFresh(serverId, SessionInventoryState.currentRevision(serverId))
    assertFalse(SessionInventoryState.isStale(serverId))

    SessionInventoryState.markStale(serverId)
    val requestedRevision = SessionInventoryState.currentRevision(serverId)
    SessionInventoryState.markStale(serverId)
    SessionInventoryState.markFresh(serverId, requestedRevision)

    assertTrue(SessionInventoryState.isStale(serverId))
    SessionInventoryState.markFresh(serverId, SessionInventoryState.currentRevision(serverId))
    assertFalse(SessionInventoryState.isStale(serverId))
  }

  @Test
  fun metadataPatchIsScopedToServerAndYieldsToConfirmedServerData() {
    val session = ServerSession(id = "inventory-test", title = "Old", project = "One", status = "idle")
    SessionInventoryState.publishMetadata(
      SessionInventoryState.MetadataPatch(
        serverId = "server-a",
        sessionId = session.id,
        title = "New",
        project = "Two",
        status = "working",
        updatedAt = "2026-01-01T00:00:00Z",
      ),
    )

    assertEquals("Old", SessionInventoryState.applyPending("server-b", listOf(session)).single().title)
    val patched = SessionInventoryState.applyPending("server-a", listOf(session)).single()
    assertEquals("New", patched.title)
    assertEquals("working", patched.status)

    val confirmed = session.copy(title = "New", project = "Two", status = "idle", updatedAt = "server-time")
    assertEquals(confirmed, SessionInventoryState.applyPending("server-a", listOf(confirmed)).single())
    assertEquals(confirmed, SessionInventoryState.applyPending("server-a", listOf(confirmed)).single())
  }
}
