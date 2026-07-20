package com.example.picompanion.ui.settings

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.settings.AppSettings
import com.example.picompanion.data.settings.ServerEntry
import com.example.picompanion.data.settings.SettingsDataStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.util.UUID

class SettingsViewModel(application: Application) : AndroidViewModel(application) {

  private val dataStore = SettingsDataStore(application)
  private val client = PiServerClient()

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

  fun addServer() {
    val current = settings.value.servers
    val newServer = ServerEntry(
      id = UUID.randomUUID().toString().take(8),
      name = "New Server",
      url = "http://192.168.1.x:3141",
    )
    viewModelScope.launch { dataStore.updateServers(current + newServer) }
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

  fun setActiveServer(id: String) {
    viewModelScope.launch { dataStore.setActiveServer(id) }
  }

  // --- Other settings ---

  fun updateReconnectOnLaunch(value: Boolean) {
    viewModelScope.launch { dataStore.updateReconnectOnLaunch(value) }
  }

  fun updateRememberLastSession(value: Boolean) {
    viewModelScope.launch { dataStore.updateRememberLastSession(value) }
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
