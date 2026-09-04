package com.example.picompanion.ui.sessions

import android.app.Application
import android.os.SystemClock
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.di.AppModule
import com.example.picompanion.data.model.CreateSessionRequest
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.model.visibleGlobalSessions
import com.example.picompanion.data.model.visibleMachineSessions
import com.example.picompanion.data.repository.SessionsRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class SessionsViewModel(application: Application) : AndroidViewModel(application) {

  private data class CreateOutcome(
    val sessionId: String?,
    val error: String?,
  )

  private val client = AppModule.client
  private val settingsDataStore = AppModule.settingsDataStore
  private val repository = SessionsRepository(client, settingsDataStore)

  private val _uiState = MutableStateFlow<SessionsUiState>(SessionsUiState.Loading)
  val uiState: StateFlow<SessionsUiState> = _uiState.asStateFlow()

  private val _createdSessionId = MutableStateFlow<String?>(null)
  val createdSessionId: StateFlow<String?> = _createdSessionId.asStateFlow()

  private val _selectedTab = MutableStateFlow(SessionTab.Active)
  val selectedTab: StateFlow<SessionTab> = _selectedTab.asStateFlow()
  private var refreshJob: Job? = null
  private var refreshPending = false
  private var activeServerId: String? = null
  private var contentServerId: String? = null
  private var lastRefreshCompletedAt = 0L
  private val resumeRefreshCooldownMs = 2_000L
  private val openingSessionIds = mutableSetOf<String>()

  init {
    viewModelScope.launch {
      SessionInventoryState.metadataUpdates.collect(::updateSessionLocally)
    }
    viewModelScope.launch {
      var observedServerId: String? = null
      var hasObservedServer = false
      settingsDataStore.settingsFlow.collect { settings ->
        val serverId = settings.activeServer?.id
        val previous = observedServerId
        observedServerId = serverId
        activeServerId = serverId
        if (hasObservedServer && previous != serverId) {
          _uiState.value = SessionsUiState.Loading
          refresh(force = true)
        }
        hasObservedServer = true
      }
    }
    refresh()
  }

  fun selectTab(tab: SessionTab) {
    _selectedTab.value = tab
  }

  fun refreshIfStale() {
    val serverId = activeServerId
    if (serverId == null || SessionInventoryState.isStale(serverId)) refresh(force = true)
  }

  private fun updateSessionLocally(patch: SessionInventoryState.MetadataPatch) {
    if (patch.serverId != activeServerId) return
    val content = _uiState.value as? SessionsUiState.Content ?: return
    fun ServerSession.applyPatch(): ServerSession = if (id != patch.sessionId) this else copy(
      title = patch.title,
      project = patch.project,
      status = patch.status ?: status,
      updatedAt = patch.updatedAt,
    )
    _uiState.value = content.copy(
      activeSessions = content.activeSessions.map { it.applyPatch() },
      globalSessions = content.globalSessions.map { global ->
        if (global.session.id == patch.sessionId || global.id == patch.sessionId) {
          global.copy(session = global.session.applyPatch())
        } else global
      },
    )
  }

  fun refresh(force: Boolean = false) {
    val now = SystemClock.elapsedRealtime()
    if (!force && now - lastRefreshCompletedAt < resumeRefreshCooldownMs) return
    // Mark the request immediately so init plus the first ON_RESUME event do
    // not queue a second inventory refresh while this one is still running.
    lastRefreshCompletedAt = now
    if (refreshJob?.isActive == true) {
      refreshPending = true
      return
    }
    val existing = _uiState.value as? SessionsUiState.Content
    if (existing == null) _uiState.value = SessionsUiState.Loading
    else _uiState.value = existing.copy(refreshing = true)

    refreshJob = viewModelScope.launch {
      try {
        val settings = settingsDataStore.settingsFlow.first()
        if (!settings.hasConfiguredServer) {
          _uiState.value = SessionsUiState.Empty
          return@launch
        }
        val server = settings.activeServer
        if (server == null) {
          _uiState.value = SessionsUiState.Empty
          return@launch
        }
        activeServerId = server.id
        val inventoryRevision = SessionInventoryState.currentRevision(server.id)
        val existingForServer = existing.takeIf { contentServerId == server.id }

        // Fetch all session types in parallel. Keep the old content visible
        // while this background refresh runs, so returning from a detail view
        // does not flash an empty/loading screen.
        val activeDeferred = async(Dispatchers.IO) { client.listSessions(server) }
        val machineDeferred = async(Dispatchers.IO) { client.listMachineSessions(server) }
        val globalDeferred = async(Dispatchers.IO) { client.listGlobalSessions(server) }

        val activeResult = activeDeferred.await()
        val machineResult = machineDeferred.await()
        val globalResult = globalDeferred.await()

        // Ignore results from a server that stopped being active while these
        // requests were in flight. The settings collector queues its refresh.
        if (settingsDataStore.settingsFlow.first().activeServer?.id != server.id) return@launch

        val activeSessions = when (activeResult) {
          is HttpResult.Success -> SessionInventoryState.applyPending(server.id, activeResult.value.sessions)
            .sortedByDescending { it.updatedAt ?: it.createdAt ?: "" }
          is HttpResult.Failure -> existingForServer?.activeSessions ?: emptyList()
        }
        val machineSessions = when (machineResult) {
          is HttpResult.Success -> visibleMachineSessions(activeSessions, machineResult.value.sessions)
          is HttpResult.Failure -> existingForServer?.machineSessions ?: emptyList()
        }
        val globalSessions = when (globalResult) {
          is HttpResult.Success -> visibleGlobalSessions(activeSessions, globalResult.value.sessions)
          is HttpResult.Failure -> existingForServer?.globalSessions ?: emptyList()
        }

        val completeSuccess =
          activeResult is HttpResult.Success &&
            machineResult is HttpResult.Success &&
            globalResult is HttpResult.Success
        if (!completeSuccess) SessionInventoryState.markStale(server.id)

        if (activeResult is HttpResult.Failure && machineResult is HttpResult.Failure && existingForServer == null) {
          _uiState.value = SessionsUiState.Error(activeResult.userMessage)
          return@launch
        }

        _uiState.value = SessionsUiState.Content(
          activeSessions = activeSessions,
          machineSessions = machineSessions,
          globalSessions = globalSessions,
          refreshing = false,
        )
        contentServerId = server.id
        if (completeSuccess) SessionInventoryState.markFresh(server.id, inventoryRevision)
        lastRefreshCompletedAt = SystemClock.elapsedRealtime()
      } finally {
        val rerun = refreshPending
        refreshPending = false
        refreshJob = null
        val state = _uiState.value as? SessionsUiState.Content
        if (state != null && state.refreshing) _uiState.value = state.copy(refreshing = false)
        if (rerun) refresh(force = true)
      }
    }
  }

  fun createSession(cwd: String, prompt: String = "", count: Int = 1) {
    viewModelScope.launch {
      val outcomes = kotlinx.coroutines.coroutineScope {
        (1..count.coerceIn(1, 12)).map { index ->
          async(Dispatchers.IO) {
            val title = if (count > 1) "New session $index" else null
            when (val created = repository.createSession(
              CreateSessionRequest(cwd = cwd, title = title, start = true)
            )) {
              is HttpResult.Success -> {
                if (prompt.isBlank()) {
                  CreateOutcome(created.value.id, null)
                } else {
                  when (val sent = repository.sendPrompt(created.value.id, prompt)) {
                    is HttpResult.Success -> CreateOutcome(created.value.id, null)
                    is HttpResult.Failure -> CreateOutcome(created.value.id, "Session created, but task was not sent: ${sent.userMessage}")
                  }
                }
              }
              is HttpResult.Failure -> CreateOutcome(null, created.userMessage)
            }
          }
        }.awaitAll()
      }
      val sessionIds = outcomes.mapNotNull { it.sessionId }
      if (sessionIds.isNotEmpty()) {
        activeServerId?.let(SessionInventoryState::markStale)
        // Keep the session list visible when creating a batch. The user can
        // choose which thread to open instead of being moved to the last one.
        if (count == 1) _createdSessionId.value = sessionIds.last()
        else refresh(force = true)
      }
      outcomes.firstOrNull { it.error != null }?.error?.let { _actionError.value = it }
    }
  }

  fun deleteSession(sessionId: String) {
    viewModelScope.launch {
      when (val result = repository.deleteSession(sessionId)) {
        is HttpResult.Success -> {
          activeServerId?.let(SessionInventoryState::markStale)
          refresh(force = true)
        }
        is HttpResult.Failure -> _uiState.value = SessionsUiState.Error(result.userMessage)
      }
    }
  }

  private val _actionError = MutableStateFlow<String?>(null)
  val actionError: StateFlow<String?> = _actionError.asStateFlow()

  fun clearActionError() {
    _actionError.value = null
  }

  fun openMachineSession(machineId: String, onOpened: (String) -> Unit) {
    synchronized(openingSessionIds) {
      if (!openingSessionIds.add(machineId)) return
    }
    viewModelScope.launch {
      try {
        val server = settingsDataStore.settingsFlow.first().activeServer ?: run {
          _actionError.value = "No server is configured"
          return@launch
        }
        when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.openMachineSession(server, machineId) }) {
          is HttpResult.Success -> {
            val sessionId = result.value.id.trim()
            if (sessionId.isEmpty()) _actionError.value = "The server returned an invalid session ID"
            else {
              SessionInventoryState.markStale(server.id)
              onOpened(sessionId)
            }
          }
          is HttpResult.Failure -> _actionError.value = "Could not open session: ${result.userMessage}"
        }
      } finally {
        synchronized(openingSessionIds) { openingSessionIds.remove(machineId) }
      }
    }
  }

  fun attachGlobalSession(globalId: String, onAttached: (String) -> Unit) {
    synchronized(openingSessionIds) {
      if (!openingSessionIds.add(globalId)) return
    }
    viewModelScope.launch {
      try {
        val server = settingsDataStore.settingsFlow.first().activeServer ?: run {
          _actionError.value = "No server is configured"
          return@launch
        }
        when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.attachGlobalSession(server, globalId) }) {
          is HttpResult.Success -> {
            val sessionId = result.value.id.trim()
            if (sessionId.isEmpty()) _actionError.value = "The server returned an invalid session ID"
            else {
              SessionInventoryState.markStale(server.id)
              onAttached(sessionId)
            }
          }
          is HttpResult.Failure -> _actionError.value = "Could not attach session: ${result.userMessage}"
        }
      } finally {
        synchronized(openingSessionIds) { openingSessionIds.remove(globalId) }
      }
    }
  }

  fun clearCreatedSession() {
    _createdSessionId.value = null
  }
}

enum class SessionTab { Active, Machine, Global }

sealed interface SessionsUiState {
  data object Loading : SessionsUiState
  data object Empty : SessionsUiState
  data class Content(
    val activeSessions: List<ServerSession>,
    val machineSessions: List<com.example.picompanion.data.model.MachineSession>,
    val globalSessions: List<com.example.picompanion.data.model.GlobalSession>,
    val refreshing: Boolean = false,
  ) : SessionsUiState {
    val hasAnySessions: Boolean get() = activeSessions.isNotEmpty() || machineSessions.isNotEmpty() || globalSessions.isNotEmpty()
  }
  data class Error(val message: String) : SessionsUiState
}
