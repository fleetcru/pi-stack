package com.example.picompanion.ui.sessions

import com.example.picompanion.data.model.GlobalSession
import com.example.picompanion.data.model.MachineSession
import com.example.picompanion.data.model.ServerSession
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.CompletableDeferred

/** Process-memory coordination for the session list and detail screens. */
internal object SessionInventoryState {
  data class Snapshot(
    val activeSessions: List<ServerSession> = emptyList(),
    val machineSessions: List<MachineSession> = emptyList(),
    val globalSessions: List<GlobalSession> = emptyList(),
  )

  data class MetadataPatch(
    val serverId: String,
    val sessionId: String,
    val title: String,
    val project: String,
    val status: String?,
    val updatedAt: String,
  )

  data class InventoryRequestKey(
    val serverId: String,
    val url: String,
    val tokenFingerprint: Int,
    val scope: String,
    val limit: Int?,
  )

  sealed interface InventoryResult {
    data class Success(val sessions: List<ServerSession>) : InventoryResult
    data class Failure(val message: String) : InventoryResult
  }

  private val revisions = LinkedHashMap<String, Long>()
  private val loadedRevisions = LinkedHashMap<String, Long>()
  private val snapshots = LinkedHashMap<String, Snapshot>()
  private val pendingPatches = LinkedHashMap<String, MetadataPatch>()
  private val _metadataUpdates = MutableSharedFlow<MetadataPatch>(extraBufferCapacity = 16)
  val metadataUpdates = _metadataUpdates.asSharedFlow()

  @Synchronized fun markStale(serverId: String) {
    revisions[serverId] = (revisions[serverId] ?: 0) + 1
  }

  @Synchronized fun isStale(serverId: String): Boolean =
    (revisions[serverId] ?: 0) > (loadedRevisions[serverId] ?: 0)

  @Synchronized fun currentRevision(serverId: String): Long = revisions[serverId] ?: 0

  @Synchronized fun markFresh(serverId: String, throughRevision: Long) {
    loadedRevisions[serverId] = maxOf(loadedRevisions[serverId] ?: 0, throughRevision)
  }

  @Synchronized fun snapshot(serverId: String): Snapshot? = snapshots[serverId]

  /** Stable identity for sharing in-flight reads without crossing credential/server changes. */
  fun requestKey(server: ServerEntry, scope: String = "all", limit: Int? = null) =
    InventoryRequestKey(server.id, server.url.trimEnd('/'), server.authToken.hashCode(), scope, limit)

  private data class InFlightRequest(
    val result: CompletableDeferred<InventoryResult>,
    var waiters: Int,
  )

  private val inFlight = LinkedHashMap<InventoryRequestKey, InFlightRequest>()

  /** Share identical inventory fetches, propagating the same success or failure to every waiter. */
  suspend fun coalesceSessions(
    key: InventoryRequestKey,
    request: suspend () -> InventoryResult,
  ): InventoryResult {
    val (entry, owner) = synchronized(inFlight) {
      val existing = inFlight[key]
      if (existing != null) {
        existing.waiters++
        existing to false
      } else {
        InFlightRequest(CompletableDeferred(), waiters = 1).also { inFlight[key] = it } to true
      }
    }
    var completed = false
    try {
      if (owner) entry.result.complete(request())
      val result = entry.result.await()
      completed = true
      return result
    } catch (error: Throwable) {
      if (owner) entry.result.completeExceptionally(error)
      throw error
    } finally {
      synchronized(inFlight) {
        entry.waiters--
        if (inFlight[key] === entry && (completed || entry.result.isCompleted || entry.waiters == 0)) {
          inFlight.remove(key)
        }
      }
    }
  }

  @Synchronized fun updateSnapshot(
    serverId: String,
    activeSessions: List<ServerSession>? = null,
    machineSessions: List<MachineSession>? = null,
    globalSessions: List<GlobalSession>? = null,
  ): Snapshot {
    val current = snapshots[serverId] ?: Snapshot()
    return current.copy(
      activeSessions = activeSessions ?: current.activeSessions,
      machineSessions = machineSessions ?: current.machineSessions,
      globalSessions = globalSessions ?: current.globalSessions,
    ).also { snapshots[serverId] = it }
  }

  fun publishMetadata(patch: MetadataPatch) {
    synchronized(pendingPatches) { pendingPatches["${patch.serverId}:${patch.sessionId}"] = patch }
    _metadataUpdates.tryEmit(patch)
  }

  fun applyPending(serverId: String, sessions: List<ServerSession>): List<ServerSession> =
    synchronized(pendingPatches) {
      sessions.map { session ->
        val key = "$serverId:${session.id}"
        val patch = pendingPatches[key] ?: return@map session
        // Once the server reports the edited title/project, drop the patch.
        // Never copy patch.status over inventory status: a mid-turn "working"
        // snapshot must not make an idle session look active after refresh.
        if (patch.title == session.title.orEmpty() && patch.project == session.project.orEmpty()) {
          pendingPatches.remove(key)
          session
        } else {
          patch.applyTo(session)
        }
      }
    }

  private fun MetadataPatch.applyTo(session: ServerSession): ServerSession = session.copy(
    title = title,
    project = project,
    updatedAt = updatedAt,
  )
}
