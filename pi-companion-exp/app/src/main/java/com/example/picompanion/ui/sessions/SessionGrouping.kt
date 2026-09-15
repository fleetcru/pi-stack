package com.example.picompanion.ui.sessions

import com.example.picompanion.data.model.ServerSession
import java.time.Instant
import java.time.OffsetDateTime
import java.time.ZoneOffset
import java.time.temporal.ChronoUnit

data class SessionGroup(
  val title: String,
  val sessions: List<ServerSession>,
)

private val runningStates = setOf("working", "starting", "reconnecting", "waiting_for_input", "waiting for input")
private val processOnlyStates = setOf("running", "active", "created", "exited", "")

fun groupSessions(
  sessions: List<ServerSession>,
  now: Instant = Instant.now(),
): List<SessionGroup> {
  val recentCutoff = now.minus(7, ChronoUnit.DAYS)
  val running = mutableListOf<ServerSession>()
  val recent = mutableListOf<ServerSession>()
  val older = mutableListOf<ServerSession>()

  sessions.forEach { session ->
    if (session.displayStatus() in runningStates) {
      running += session
    } else if (session.lastActivityInstant()?.isAfter(recentCutoff) == true) {
      recent += session
    } else {
      older += session
    }
  }

  return listOf(
    SessionGroup("Running", running),
    SessionGroup("Recent", recent),
    SessionGroup("Older", older),
  ).filter { it.sessions.isNotEmpty() }
}

internal fun ServerSession.displayStatus(): String {
  val raw = effectiveRuntimeStatus()
  return when {
    raw in runningStates -> if (raw == "waiting_for_input") "waiting for input" else raw
    raw in setOf("failed", "error", "stopped") -> raw
    raw in processOnlyStates -> "idle"
    else -> raw.ifBlank { "idle" }
  }
}

private fun ServerSession.effectiveRuntimeStatus(): String {
  val runtimeState = (state?.get("runtimeStatus") as? kotlinx.serialization.json.JsonObject)
    ?.get("state")?.toString()?.trim('"')
  return (runtimeState ?: status).orEmpty().lowercase().replace('_', ' ').let { value ->
    if (value == "waiting for input") "waiting_for_input" else value
  }
}

private fun ServerSession.lastActivityInstant(): Instant? =
  sequenceOf(updatedAt, createdAt)
    .filterNotNull()
    .mapNotNull { value ->
      runCatching { Instant.parse(value) }.getOrNull()
        ?: runCatching { OffsetDateTime.parse(value).toInstant() }.getOrNull()
        ?: runCatching { java.time.LocalDateTime.parse(value).toInstant(ZoneOffset.UTC) }.getOrNull()
    }
    .firstOrNull()
