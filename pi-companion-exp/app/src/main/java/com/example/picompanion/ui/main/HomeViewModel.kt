package com.example.picompanion.ui.main

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.model.ServerWorker
import com.example.picompanion.data.model.GlobalSession
import com.example.picompanion.data.model.MachineSession
import com.example.picompanion.data.settings.SettingsDataStore
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
import java.util.concurrent.atomic.AtomicBoolean

class HomeViewModel(application: Application) : AndroidViewModel(application) {

  private val client = PiServerClient()
  private val settingsDataStore = SettingsDataStore(application)

  private val _uiState = MutableStateFlow<HomeUiState>(HomeUiState.Loading)
  val uiState: StateFlow<HomeUiState> = _uiState.asStateFlow()
  private var refreshJob: Job? = null
  private var pollingJob: Job? = null
  private val refreshIntervalMs = 10_000L
  private val pollingActive = AtomicBoolean(false)

  init {
    refresh(showLoading = true)
  }

  /** Start automatic polling when the Home tab is visible. */
  fun startPolling() {
    if (pollingActive.getAndSet(true)) return
    pollingJob = viewModelScope.launch {
      while (isActive) {
        delay(refreshIntervalMs)
        refresh(showLoading = false)
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
      val sessionsDeferred = async(Dispatchers.IO) { client.listSessions(server) }
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
