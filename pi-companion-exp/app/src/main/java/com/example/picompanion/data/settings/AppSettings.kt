package com.example.picompanion.data.settings

import kotlinx.serialization.Serializable

@Serializable
data class ServerEntry(
  val id: String,
  val name: String = "",
  val url: String = "",
  val authToken: String = "",
) {
  val isConfigured: Boolean get() = url.isNotBlank()
}

@Serializable
data class AppSettings(
  val servers: List<ServerEntry> = emptyList(),
  val activeServerId: String = "",
  val reconnectOnLaunch: Boolean = true,
  val rememberLastSession: Boolean = true,
  val replayEventsSinceLastSeen: Boolean = true,
  val showFileChangeEvents: Boolean = true,
  val showToolEvents: Boolean = true,
  val showDaemonEvents: Boolean = true,
  val defaultProjectRoot: String = "",
) {
  val activeServer: ServerEntry?
    get() = servers.find { it.id == activeServerId }
      ?: servers.firstOrNull()

  val hasServers: Boolean get() = servers.isNotEmpty()
  val hasConfiguredServer: Boolean get() = servers.any { it.isConfigured }
}
