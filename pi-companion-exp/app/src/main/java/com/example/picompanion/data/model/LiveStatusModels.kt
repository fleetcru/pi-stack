package com.example.picompanion.data.model

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** Lightweight runtime state delivered by the server-wide status socket. */
data class LiveSessionStatus(
  val sessionId: String,
  val workerId: String = "local",
  val state: String,
  val reason: String? = null,
  val detail: String? = null,
  val runId: String? = null,
  val position: Int? = null,
  val queuedAt: String? = null,
  val updatedAt: String? = null,
)

val LiveSessionStatus.isActive: Boolean
  get() = state in setOf("queued", "starting", "working", "waiting_for_input", "reconnecting")

/** Merge socket state over inventory metadata without discarding the metadata. */
fun ServerSession.withLiveStatus(status: LiveSessionStatus?): ServerSession {
  if (status == null) return this
  val runtime = buildMap {
    put("state", JsonPrimitive(status.state))
    status.reason?.let { put("reason", JsonPrimitive(it)) }
    status.detail?.let { put("detail", JsonPrimitive(it)) }
    status.runId?.let { put("runId", JsonPrimitive(it)) }
  }
  return copy(
    status = status.state,
    state = (state ?: emptyMap()) + ("runtimeStatus" to JsonObject(runtime)),
  )
}
