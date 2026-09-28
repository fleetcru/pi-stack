package com.example.picompanion.data.model

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json

/** Durable last-known session lists used to paint the Sessions screen before network refresh. */
@Serializable
data class SessionInventoryCache(
  val serverId: String,
  val activeSessions: List<ServerSession> = emptyList(),
  val machineSessions: List<MachineSession> = emptyList(),
  val globalSessions: List<GlobalSession> = emptyList(),
)

internal fun decodeSessionInventoryCache(
  encoded: String,
  serverId: String,
  json: Json,
): SessionInventoryCache? = runCatching {
  json.decodeFromString(SessionInventoryCache.serializer(), encoded)
}.getOrNull()?.takeIf { it.serverId == serverId }
