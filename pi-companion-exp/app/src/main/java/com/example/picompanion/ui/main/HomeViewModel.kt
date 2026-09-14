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
import com.example.picompanion.data.model.visibleGlobalSessions
import com.example.picompanion.data.model.visibleMachineSessions
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
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
import com.example.picompanion.ui.sessiondetail.historyPageStartIndex
import com.example.picompanion.ui.sessions.SessionInventoryState
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
  private var refreshPending = false
  private var contentServerId: String? = null
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
    refresh()
  }

  /** Start automatic polling when the Home tab is visible. */
  fun startPolling() {
    if (pollingActive.getAndSet(true)) return
    // Refresh immediately on foreground instead of waiting for the first
    // polling interval.
    if (isNetworkAvailable()) refresh(showLoading = false)
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
    if (refreshJob?.isActive == true) {
      refreshPending = true
      return
    }
    refreshJob = viewModelScope.launch {
      try {
      val settings = settingsDataStore.settingsFlow.first()
      val server = settings.activeServer

      if (server == null || !server.isConfigured) {
        _uiState.value = HomeUiState.NoServer
        return@launch
      }

      val serverName = server.name.ifBlank { server.url }
      val cachedSnapshot = SessionInventoryState.snapshot(server.id)
      if (cachedSnapshot != null) {
        val current = (_uiState.value as? HomeUiState.Content)
          ?.takeIf { contentServerId == server.id }
        _uiState.value = HomeUiState.Content(
          connected = isNetworkAvailable(),
          serverName = serverName,
          sessions = cachedSnapshot.activeSessions,
          workers = current?.workers.orEmpty(),
          globalSessions = cachedSnapshot.globalSessions,
          machineSessions = cachedSnapshot.machineSessions,
          activeSessions = current?.activeSessions ?: cachedSnapshot.activeSessions.size,
          maxSessions = current?.maxSessions ?: 0,
        )
        contentServerId = server.id
      } else if (showLoading && _uiState.value !is HomeUiState.Content) {
        _uiState.value = HomeUiState.Loading
      }

      // Start every request together, but let the main session inventory paint
      // without waiting for the slower worker and machine discovery calls.
      val healthDeferred = async(Dispatchers.IO) { client.checkHealth(server) }
      val sessionsDeferred = async(Dispatchers.IO) { client.listRecentSessions(server) }
      val workersDeferred = async(Dispatchers.IO) { client.listWorkers(server) }
      val globalDeferred = async(Dispatchers.IO) { client.listGlobalSessions(server) }
      val machineDeferred = async(Dispatchers.IO) { client.listMachineSessions(server) }

      val sessions = sessionsDeferred.await()
      val sessionList = when (sessions) {
        is HttpResult.Success -> sessions.value.sessions
          .sortedByDescending { it.updatedAt ?: it.createdAt ?: "" }
        is HttpResult.Failure -> cachedSnapshot?.activeSessions.orEmpty()
      }
      if (sessions is HttpResult.Success) {
        val snapshot = SessionInventoryState.updateSnapshot(server.id, activeSessions = sessionList)
        val current = (_uiState.value as? HomeUiState.Content)
          ?.takeIf { contentServerId == server.id }
        _uiState.value = HomeUiState.Content(
          connected = true,
          serverName = serverName,
          sessions = sessionList,
          workers = current?.workers.orEmpty(),
          globalSessions = snapshot.globalSessions,
          machineSessions = snapshot.machineSessions,
          activeSessions = current?.activeSessions ?: sessionList.size,
          maxSessions = current?.maxSessions ?: 0,
        )
        contentServerId = server.id
      }

      val health = healthDeferred.await()
      val workers = workersDeferred.await()
      val global = globalDeferred.await()
      val machine = machineDeferred.await()

      if (health is HttpResult.Failure && sessions is HttpResult.Failure && cachedSnapshot == null) {
        _uiState.value = HomeUiState.Error(
          message = health.userMessage,
          serverName = serverName,
        )
        return@launch
      }

      val workerList = (workers as? HttpResult.Success)?.value?.workers
        ?: (_uiState.value as? HomeUiState.Content)
          ?.takeIf { contentServerId == server.id }
          ?.workers
          .orEmpty()
      val capacity = (health as? HttpResult.Success)?.value?.capacity
      val machineSessions = when (machine) {
        is HttpResult.Success -> visibleMachineSessions(sessionList, machine.value.sessions)
        is HttpResult.Failure -> cachedSnapshot?.machineSessions.orEmpty()
      }
      val globalSessions = when (global) {
        is HttpResult.Success -> visibleGlobalSessions(sessionList, global.value.sessions)
        is HttpResult.Failure -> cachedSnapshot?.globalSessions.orEmpty()
      }
      SessionInventoryState.updateSnapshot(
        serverId = server.id,
        activeSessions = sessionList.takeIf { sessions is HttpResult.Success },
        machineSessions = machineSessions.takeIf { machine is HttpResult.Success },
        globalSessions = globalSessions.takeIf { global is HttpResult.Success },
      )

      _uiState.value = HomeUiState.Content(
        connected = health is HttpResult.Success,
        serverName = serverName,
        sessions = sessionList,
        workers = workerList,
        globalSessions = globalSessions,
        machineSessions = machineSessions,
        activeSessions = capacity?.activeSessions ?: sessionList.size,
        maxSessions = capacity?.maxSessions ?: 0,
      )
      contentServerId = server.id
      // Warm only the two most likely next sessions, after visible Home data is
      // ready. This avoids delaying the list while making taps feel immediate.
      launch(Dispatchers.IO) { prefetchRecentSessions(server, sessionList.take(2)) }
      } finally {
        val rerun = refreshPending
        refreshPending = false
        refreshJob = null
        if (rerun) refresh(showLoading = false)
      }
    }
  }

  private suspend fun prefetchRecentSessions(
    server: com.example.picompanion.data.settings.ServerEntry,
    sessions: List<ServerSession>,
  ) = coroutineScope {
    sessions.map { session ->
      async {
        val key = "${server.id}:${session.id}"
        if (SessionStateCache.contains(key)) return@async
        val result = client.getSessionMessages(server, session.id, limit = 30)
        val payload = (result as? HttpResult.Success)?.value ?: return@async
        val data = payload["data"] as? JsonObject ?: return@async
        val messages = data["messages"] as? JsonArray ?: return@async
        val history = data["history"] as? JsonObject
        val totalMessages = history?.get("total")?.jsonPrimitive?.intOrNull ?: messages.size
        val pageStartIndex = historyPageStartIndex(totalMessages, offset = 0, pageSize = messages.size)
        val parsed = SessionHistoryParser.parse(messages, pageStartIndex)
        SessionStateCache.put(key, SessionStateCache.Entry(
          items = parsed,
          historicalItems = parsed,
          nextHistoryOffset = history?.get("nextOffset")?.jsonPrimitive?.intOrNull ?: parsed.size,
          hasOlder = history?.get("hasOlder")?.jsonPrimitive?.booleanOrNull == true,
          title = session.title.orEmpty(),
          project = session.project.orEmpty(),
          cwd = session.cwd.orEmpty(),
          lastEventId = 0,
          totalHistoryMessages = totalMessages,
        ))
      }
    }.awaitAll()
  }

  override fun onCleared() {
    pollingJob?.cancel()
    refreshJob?.cancel()
    super.onCleared()
  }

  fun openMachineSession(machineId: String, onOpened: (String) -> Unit, onError: (String) -> Unit) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.openMachineSession(server, machineId) }) {
        is HttpResult.Success -> {
          SessionInventoryState.markStale(server.id)
          onOpened(result.value.id)
        }
        is HttpResult.Failure -> {
          onError(result.userMessage)
          refresh(showLoading = false)
        }
      }
    }
  }

  fun attachGlobalSession(globalId: String, onAttached: (String) -> Unit) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.attachGlobalSession(server, globalId) }) {
        is HttpResult.Success -> {
          SessionInventoryState.markStale(server.id)
          onAttached(result.value.id)
        }
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
