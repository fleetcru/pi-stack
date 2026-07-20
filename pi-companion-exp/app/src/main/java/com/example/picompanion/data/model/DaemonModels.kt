package com.example.picompanion.data.model

import kotlinx.serialization.Serializable

@Serializable
data class HealthResponse(
  val ok: Boolean = false,
  val sessions: List<String> = emptyList(),
  val capacity: ServerCapacity? = null,
)

@Serializable
data class ServerCapacity(
  val activeSessions: Int = 0,
  val maxSessions: Int = 0,
)
