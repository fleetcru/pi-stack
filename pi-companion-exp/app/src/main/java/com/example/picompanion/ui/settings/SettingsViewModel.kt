package com.example.picompanion.ui.settings

import android.app.Application
import android.net.Uri
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.di.AppModule
import com.example.picompanion.data.settings.AppSettings
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonObject
import java.util.UUID

class SettingsViewModel(application: Application) : AndroidViewModel(application) {

  private val dataStore = AppModule.settingsDataStore
  private val client = AppModule.client

  val settings: StateFlow<AppSettings> = dataStore.settingsFlow
    .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), AppSettings())

  // Per-server connection test results
  private val _connectionResults = MutableStateFlow<Map<String, ConnectionTestResult>>(emptyMap())
  val connectionResults: StateFlow<Map<String, ConnectionTestResult>> = _connectionResults.asStateFlow()

  fun testConnection(server: ServerEntry) {
    viewModelScope.launch {
      if (!server.isConfigured) {
        _connectionResults.value = _connectionResults.value + (server.id to ConnectionTestResult.Error("No URL configured"))
        return@launch
      }
      _connectionResults.value = _connectionResults.value + (server.id to ConnectionTestResult.Testing)
      val result = withContext(Dispatchers.IO) {
        client.checkHealth(server)
      }
      _connectionResults.value = _connectionResults.value + (server.id to when (result) {
        is HttpResult.Success -> ConnectionTestResult.Success(
          sessions = result.value.sessions.size,
          capacity = result.value.capacity?.maxSessions ?: 0,
        )
        is HttpResult.Failure -> ConnectionTestResult.Error(result.userMessage)
      })
    }
  }

  // --- Server management ---

  /** Adds a blank entry and returns its id so first-run pairing can start immediately. */
  fun addServer(): String {
    val current = settings.value.servers
    val newServer = ServerEntry(
      id = UUID.randomUUID().toString().take(8),
      name = "New Server",
      url = "",
    )
    viewModelScope.launch { dataStore.updateServers(current + newServer) }
    return newServer.id
  }

  fun removeServer(id: String) {
    val current = settings.value.servers
    if (current.size <= 1) return
    val updated = current.filter { it.id != id }
    viewModelScope.launch {
      dataStore.updateServers(updated)
      if (settings.value.activeServerId == id) {
        dataStore.setActiveServer(updated.first().id)
      }
    }
  }

  fun updateServer(updated: ServerEntry) {
    val current = settings.value.servers
    val updatedList = current.map { if (it.id == updated.id) updated else it }
    viewModelScope.launch { dataStore.updateServers(updatedList) }
  }

  /** Accepts a QR payload without removing manual URL/token entry. */
  fun applyPairingPayload(serverId: String, payload: String): String? {
    val text = payload.trim()
    var url = ""
    var token = ""
    var name = ""
    try {
      val obj = Json.parseToJsonElement(text).jsonObject
      url = obj["url"]?.toString()?.trim('"').orEmpty()
      token = obj["token"]?.toString()?.trim('"').orEmpty()
      name = obj["name"]?.toString()?.trim('"').orEmpty()
    } catch (_: Exception) {
      val uri = Uri.parse(text)
      if (uri.scheme == "pi-stack" || uri.scheme == "pistack") {
        url = uri.getQueryParameter("url").orEmpty()
        token = uri.getQueryParameter("token").orEmpty()
        name = uri.getQueryParameter("name").orEmpty()
      }
    }
    if (!url.startsWith("http://") && !url.startsWith("https://")) return "QR code did not contain a valid server URL"
    if (token.isBlank()) return "QR code did not contain a device token"
    val current = settings.value.servers.find { it.id == serverId } ?: return "Server entry not found"
    updateServer(current.copy(name = name.ifBlank { current.name }, url = url.trimEnd('/'), authToken = token))
    testConnection(current.copy(url = url.trimEnd('/'), authToken = token))
    return null
  }

  fun setActiveServer(id: String) {
    viewModelScope.launch { dataStore.setActiveServer(id) }
  }

  // --- Other settings ---

  fun updateReconnectOnLaunch(value: Boolean) {
    viewModelScope.launch { dataStore.updateReconnectOnLaunch(value) }
  }

  fun updateRememberLastSession(value: Boolean) {
    viewModelScope.launch {
      dataStore.updateRememberLastSession(value)
      if (!value) dataStore.clearLastSession()
    }
  }

  fun updateReplayEvents(value: Boolean) {
    viewModelScope.launch { dataStore.updateReplayEvents(value) }
  }

  fun updateShowFileChanges(value: Boolean) {
    viewModelScope.launch { dataStore.updateShowFileChanges(value) }
  }

  fun updateShowToolEvents(value: Boolean) {
    viewModelScope.launch { dataStore.updateShowToolEvents(value) }
  }

  fun updateShowDaemonEvents(value: Boolean) {
    viewModelScope.launch { dataStore.updateShowDaemonEvents(value) }
  }

  fun updateDefaultProjectRoot(value: String) {
    viewModelScope.launch { dataStore.updateDefaultProjectRoot(value) }
  }
}

sealed interface ConnectionTestResult {
  data object Idle : ConnectionTestResult
  data object Testing : ConnectionTestResult
  data class Success(val sessions: Int, val capacity: Int) : ConnectionTestResult
  data class Error(val message: String) : ConnectionTestResult
}
