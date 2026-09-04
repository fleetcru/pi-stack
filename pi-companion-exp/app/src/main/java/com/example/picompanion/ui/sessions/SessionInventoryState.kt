package com.example.picompanion.ui.sessions

import com.example.picompanion.data.model.ServerSession
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow

/** Process-memory coordination for the session list and detail screens. */
internal object SessionInventoryState {
  data class MetadataPatch(
    val serverId: String,
    val sessionId: String,
    val title: String,
    val project: String,
    val status: String?,
    val updatedAt: String,
  )

  private val revisions = LinkedHashMap<String, Long>()
  private val loadedRevisions = LinkedHashMap<String, Long>()
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

  fun publishMetadata(patch: MetadataPatch) {
    synchronized(pendingPatches) { pendingPatches["${patch.serverId}:${patch.sessionId}"] = patch }
    _metadataUpdates.tryEmit(patch)
  }

  fun applyPending(serverId: String, sessions: List<ServerSession>): List<ServerSession> =
    synchronized(pendingPatches) {
      sessions.map { session ->
        val key = "$serverId:${session.id}"
        val patch = pendingPatches[key] ?: return@map session
        // Once the server reports the edited fields, its timestamp and status
        // become authoritative again.
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
    status = status ?: session.status,
    updatedAt = updatedAt,
  )
}
