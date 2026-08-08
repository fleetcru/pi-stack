package com.example.picompanion.ui.main

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.di.AppModule
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.model.ServerWorker
import com.example.picompanion.data.model.GlobalSession
import com.example.picompanion.data.model.MachineSession
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch
import kotlinx.coroutines.isActive
import android.content.Context
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import java.util.concurrent.atomic.AtomicBoolean
import com.example.picompanion.ui.sessiondetail.SessionHistoryParser
import com.example.picompanion.ui.sessiondetail.SessionStateCache
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive

class HomeViewModel(application: Application) : AndroidViewModel(application) {

  private val client = AppModule.client
  private val settingsDataStore = AppModule.settingsDataStore

  private val _uiState = MutableStateFlow<HomeUiState>(HomeUiState.Loading)
  val uiState: StateFlow<HomeUiState> = _uiState.asStateFlow()
  private var refreshJob: Job? = null
  private var pollingJob: Job? = null
  // The home screen fans out to five endpoints per refresh. Thirty seconds
  // keeps it current while reducing mobile radio wakeups and server load.
  private val refreshIntervalMs = 30_000L
  private val pollingActive = AtomicBoolean(false)
  private val connectivityManager = application.getSystemService(Context.CONNECTIVITY_SERVICE) as ConnectivityManager

  private fun isNetworkAvailable(): Boolean {
    val network = connectivityManager.activeNetwork ?: return false
    val caps = connectivityManager.getNetworkCapabilities(network) ?: return false
    return caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
  }

  init {
    refresh(showLoading = true)
  }

  /** Start automatic polling when the Home tab is visible. */
  fun startPolling() {
    if (pollingActive.getAndSet(true)) return
    pollingJob = viewModelScope.launch {
      while (isActive) {
        delay(refreshIntervalMs)
        if (isNetworkAvailable()) {
          refresh(showLoading = false)
        }
      }
    }
  }

  /** Stop automatic polling when the Home tab is not visible. */
  fun stopPolling() {
    pollingActive.set(false)
    pollingJob?.cancel()
    pollingJob = null
  }

  /** Background refresh keeps Home current without replacing visible content with a spinner. */
  fun refresh(showLoading: Boolean = _uiState.value !is HomeUiState.Content) {
    if (refreshJob?.isActive == true) return
    refreshJob = viewModelScope.launch {
      if (showLoading) _uiState.value = HomeUiState.Loading
      val settings = settingsDataStore.settingsFlow.first()
      val server = settings.activeServer

      if (server == null || !server.isConfigured) {
        _uiState.value = HomeUiState.NoServer
        return@launch
      }

      // Fetch all data in parallel
      val healthDeferred = async(Dispatchers.IO) { client.checkHealth(server) }
      val sessionsDeferred = async(Dispatchers.IO) { client.listRecentSessions(server) }
      val workersDeferred = async(Dispatchers.IO) { client.listWorkers(server) }
      val globalDeferred = async(Dispatchers.IO) { client.listGlobalSessions(server) }
      val machineDeferred = async(Dispatchers.IO) { client.listMachineSessions(server) }

      val health = healthDeferred.await()
      val sessions = sessionsDeferred.await()
      val workers = workersDeferred.await()
      val global = globalDeferred.await()
      val machine = machineDeferred.await()

      if (health is HttpResult.Failure && sessions is HttpResult.Failure) {
        _uiState.value = HomeUiState.Error(
          message = (health as HttpResult.Failure).userMessage,
          serverName = server.name.ifBlank { server.url },
        )
        return@launch
      }

      val connected = health is HttpResult.Success
      val sessionList = ((sessions as? HttpResult.Success)?.value?.sessions ?: emptyList())
        .sortedByDescending { it.updatedAt ?: it.createdAt ?: "" }
      val workerList = (workers as? HttpResult.Success)?.value?.workers ?: emptyList()
      val capacity = (health as? HttpResult.Success)?.value?.capacity

      _uiState.value = HomeUiState.Content(
        connected = connected,
        serverName = server.name.ifBlank { server.url },
        sessions = sessionList,
        workers = workerList,
        globalSessions = (global as? HttpResult.Success)?.value?.sessions ?: emptyList(),
        machineSessions = (machine as? HttpResult.Success)?.value?.sessions ?: emptyList(),
        activeSessions = capacity?.activeSessions ?: sessionList.size,
        maxSessions = capacity?.maxSessions ?: 0,
      )
      // Warm only the two most likely next sessions, after visible Home data is
      // ready. This avoids delaying the list while making taps feel immediate.
      launch(Dispatchers.IO) { prefetchRecentSessions(server, sessionList.take(2)) }
    }
  }

  private fun prefetchRecentSessions(
    server: com.example.picompanion.data.settings.ServerEntry,
    sessions: List<ServerSession>,
  ) {
    sessions.forEach { session ->
      val key = "${server.id}:${session.id}"
      if (SessionStateCache.contains(key)) return@forEach
      val result = client.getSessionMessages(server, session.id, limit = 30)
      val payload = (result as? HttpResult.Success)?.value ?: return@forEach
      val data = payload["data"] as? JsonObject ?: return@forEach
      val messages = data["messages"] as? JsonArray ?: return@forEach
      val parsed = SessionHistoryParser.parse(messages)
      val history = data["history"] as? JsonObject
      SessionStateCache.put(key, SessionStateCache.Entry(
        items = parsed,
        historicalItems = parsed,
        nextHistoryOffset = history?.get("nextOffset")?.jsonPrimitive?.intOrNull ?: parsed.size,
        hasOlder = history?.get("hasOlder")?.jsonPrimitive?.booleanOrNull == true,
        title = session.title.orEmpty(),
        project = session.project.orEmpty(),
        cwd = session.cwd.orEmpty(),
        lastEventId = 0,
      ))
    }
  }

  override fun onCleared() {
    pollingJob?.cancel()
    refreshJob?.cancel()
    super.onCleared()
  }

  fun openMachineSession(machineId: String, onOpened: (String) -> Unit) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.openMachineSession(server, machineId) }) {
        is HttpResult.Success -> onOpened(result.value.id)
        is HttpResult.Failure -> refresh(showLoading = false)
      }
    }
  }

  fun attachGlobalSession(globalId: String, onAttached: (String) -> Unit) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.attachGlobalSession(server, globalId) }) {
        is HttpResult.Success -> onAttached(result.value.id)
        is HttpResult.Failure -> refresh()
      }
    }
  }

  fun updateCapacity(maxSessions: Int, onDone: () -> Unit = {}) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      kotlinx.coroutines.withContext(Dispatchers.IO) { client.updateCapacity(server, maxSessions) }
      refresh(showLoading = false)
      onDone()
    }
  }
}

sealed interface HomeUiState {
  data object Loading : HomeUiState
  data object NoServer : HomeUiState
  data class Content(
    val connected: Boolean,
    val serverName: String,
    val sessions: List<ServerSession>,
    val workers: List<ServerWorker>,
    val globalSessions: List<GlobalSession> = emptyList(),
    val machineSessions: List<MachineSession> = emptyList(),
    val activeSessions: Int,
    val maxSessions: Int,
  ) : HomeUiState {
    val latestSession: ServerSession? get() = sessions.firstOrNull()
  }
  data class Error(val message: String, val serverName: String) : HomeUiState
}
