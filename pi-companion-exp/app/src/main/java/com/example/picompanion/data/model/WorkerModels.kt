package com.example.picompanion.data.model

import kotlinx.serialization.Serializable

@Serializable
data class ServerWorker(
  val id: String,
  val url: String? = null,
  val tags: List<String> = emptyList(),
  val status: String? = null,
  val lastHeartbeat: String? = null,
  val activeSessions: Int = 0,
  val maxSessions: Int = 0,
)

@Serializable
data class WorkerListResponse(
  val workers: List<ServerWorker> = emptyList(),
)

@Serializable
data class WorkerWriteRequest(
  val id: String,
  val url: String,
  val token: String? = null,
  val tags: List<String> = emptyList(),
)
