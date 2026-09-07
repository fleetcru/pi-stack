package com.example.picompanion.ui.settings

import android.app.Application
import android.net.Uri
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.settings.AppSettings
import com.example.picompanion.data.settings.ServerEntry
import com.example.picompanion.data.updater.AppUpdater
import com.example.picompanion.data.updater.CompanionRelease
import com.example.picompanion.di.AppModule
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
  private val updater = AppUpdater(application)

  val settings: StateFlow<AppSettings> = dataStore.settingsFlow
    .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5000), AppSettings())

  sealed interface UpdateState {
    data object Idle : UpdateState
    data object Checking : UpdateState
    data object UpToDate : UpdateState
    data object Downloading : UpdateState
    data object Installing : UpdateState
    data class UpdateAvailable(val release: CompanionRelease) : UpdateState
    data class Failed(val message: String) : UpdateState
    data object PermissionRequired : UpdateState
  }

  private val _updateState = MutableStateFlow<UpdateState>(UpdateState.Idle)
  val updateState: StateFlow<UpdateState> = _updateState.asStateFlow()

  val currentVersion: String
    get() = updater.currentVersionName()

  fun checkForUpdates() {
    if (_updateState.value is UpdateState.Checking || _updateState.value is UpdateState.Downloading || _updateState.value is UpdateState.Installing) return
    viewModelScope.launch {
      _updateState.value = UpdateState.Checking
      val release = updater.newestRelease()
      if (release == null) {
        _updateState.value = UpdateState.Failed("Could not reach GitHub releases")
        return@launch
      }
      // If the newest release is not actually newer than the installed build,
      // report up-to-date instead of offering a downgrade/no-op install.
      if (!release.isNewerThan(updater.currentVersionName())) {
        _updateState.value = UpdateState.UpToDate
        return@launch
      }
      if (!updater.canRequestInstall()) {
        _updateState.value = UpdateState.PermissionRequired
        return@launch
      }
      _updateState.value = UpdateState.UpdateAvailable(release)
    }
  }

  fun downloadAndInstall(release: CompanionRelease) {
    if (_updateState.value is UpdateState.Downloading || _updateState.value is UpdateState.Installing) return
    viewModelScope.launch {
      _updateState.value = UpdateState.Downloading
      try {
        val apk = updater.download(release)
        val downloadedCode = updater.downloadedVersionCode(apk)
        if (downloadedCode != null && downloadedCode < updater.currentVersionCode()) {
          _updateState.value = UpdateState.Failed("Downloaded APK is older than the installed app")
          return@launch
        }
        _updateState.value = UpdateState.Installing
        updater.install(apk)
        // Once install() returns, the system installer has taken over.
        _updateState.value = UpdateState.UpToDate
      } catch (error: Exception) {
        _updateState.value = UpdateState.Failed(error.message ?: "Update failed")
      }
    }
  }

  fun installPermissionIntent() = updater.installPermissionIntent()

  /**
   * Re-evaluates install permission after the user returns from the system
   * "install unknown apps" screen. If permission was just granted, kick off a
   * fresh check so the update option appears without the user tapping again.
   */
  fun refreshInstallPermission() {
    if (_updateState.value !is UpdateState.PermissionRequired) return
    if (updater.canRequestInstall()) {
      checkForUpdates()
    }
  }

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
