package com.example.picompanion.ui.sessions

import com.example.picompanion.data.model.MachineSession
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.async
import kotlinx.coroutines.cancelAndJoin
import kotlinx.coroutines.launch
import kotlinx.coroutines.runBlocking
import java.util.concurrent.atomic.AtomicInteger
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
  fun snapshotMakesLoadedSessionsAvailableToOtherScreens() {
    val serverId = "snapshot-test"
    val sessions = listOf(ServerSession(id = "cached", title = "Cached"))

    SessionInventoryState.updateSnapshot(serverId, activeSessions = sessions)
    SessionInventoryState.updateSnapshot(serverId, globalSessions = emptyList())

    assertEquals(sessions, SessionInventoryState.snapshot(serverId)?.activeSessions)
    assertEquals(
      emptyList<MachineSession>(),
      SessionInventoryState.snapshot(serverId)?.machineSessions,
    )
  }

  @Test
  fun inventoryRequestsCoalesceOnlyForSameCredentialIdentity() = runBlocking {
    val requests = AtomicInteger()
    val server = ServerEntry("same", url = "http://host/", authToken = "secret")
    val key = SessionInventoryState.requestKey(server, scope = "all", limit = null)
    val request: suspend () -> SessionInventoryState.InventoryResult = {
      requests.incrementAndGet()
      kotlinx.coroutines.delay(20)
      SessionInventoryState.InventoryResult.Success(listOf(ServerSession(id = "shared")))
    }
    val first = async { SessionInventoryState.coalesceSessions(key, request) }
    val second = async { SessionInventoryState.coalesceSessions(key, request) }

    assertEquals(first.await(), second.await())
    assertEquals(1, requests.get())

    val differentLimit = SessionInventoryState.requestKey(server, scope = "all", limit = 20)
    val differentScope = SessionInventoryState.requestKey(server, scope = "local", limit = null)
    SessionInventoryState.coalesceSessions(differentLimit, request)
    SessionInventoryState.coalesceSessions(differentScope, request)
    val changedTokenKey = SessionInventoryState.requestKey(server.copy(authToken = "rotated"), scope = "all", limit = null)
    SessionInventoryState.coalesceSessions(changedTokenKey, request)
    assertEquals(4, requests.get())
  }

  @Test
  fun cancelledOwnerDoesNotLeaveAnInFlightRequestOrBlockLaterWaiters() = runBlocking {
    val server = ServerEntry("cancel-owner", url = "http://host")
    val key = SessionInventoryState.requestKey(server)
    val started = kotlinx.coroutines.CompletableDeferred<Unit>()
    val owner = kotlinx.coroutines.CoroutineScope(coroutineContext).launch {
      SessionInventoryState.coalesceSessions(key) {
        started.complete(Unit)
        kotlinx.coroutines.awaitCancellation()
      }
    }
    started.await()
    owner.cancelAndJoin()

    val recovered = SessionInventoryState.coalesceSessions(key) {
      SessionInventoryState.InventoryResult.Success(listOf(ServerSession(id = "recovered")))
    }
    assertEquals(listOf(ServerSession(id = "recovered")),
      (recovered as SessionInventoryState.InventoryResult.Success).sessions)
  }

  @Test
  fun coalescedInventoryPropagatesFailureInsteadOfPublishingEmptyInventory() = runBlocking {
    val server = ServerEntry("failure-test", url = "http://host")
    val key = SessionInventoryState.requestKey(server)
    val failure = SessionInventoryState.InventoryResult.Failure("unavailable")
    val result = SessionInventoryState.coalesceSessions(key) { failure }

    assertEquals(failure, result)
  }

  @Test
  fun freshAuthoritativeInventoryReplacesSnapshotAndRemovesDeletedSessions() {
    val serverId = "authoritative-test"
    SessionInventoryState.updateSnapshot(
      serverId,
      activeSessions = listOf(ServerSession(id = "deleted"), ServerSession(id = "updated", title = "old")),
    )
    val fresh = listOf(ServerSession(id = "updated", title = "new"))

    SessionInventoryState.updateSnapshot(serverId, activeSessions = fresh)

    assertEquals(fresh, SessionInventoryState.snapshot(serverId)?.activeSessions)
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
    assertEquals("idle", patched.status)

    val confirmed = session.copy(title = "New", project = "Two", status = "idle", updatedAt = "server-time")
    assertEquals(confirmed, SessionInventoryState.applyPending("server-a", listOf(confirmed)).single())
    assertEquals(confirmed, SessionInventoryState.applyPending("server-a", listOf(confirmed)).single())
  }
}
