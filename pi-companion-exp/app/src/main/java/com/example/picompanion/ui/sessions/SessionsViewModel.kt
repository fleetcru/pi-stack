package com.example.picompanion.ui.sessions

import android.app.Application
import android.os.SystemClock
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.di.AppModule
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.model.SessionInventoryCache
import com.example.picompanion.data.model.decodeSessionInventoryCache
import com.example.picompanion.data.model.visibleGlobalSessions
import com.example.picompanion.data.model.visibleMachineSessions
import com.example.picompanion.data.repository.SessionsRepository
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.booleanOrNull
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class SessionsViewModel(application: Application) : AndroidViewModel(application) {

  private val client = AppModule.client
  private val settingsDataStore = AppModule.settingsDataStore
  private val repository = SessionsRepository(client, settingsDataStore)
  private val cacheJson = Json { ignoreUnknownKeys = true }

  private val _uiState = MutableStateFlow<SessionsUiState>(SessionsUiState.Loading)
  val uiState: StateFlow<SessionsUiState> = _uiState.asStateFlow()

  private val _createdSessionId = MutableStateFlow<String?>(null)
  val createdSessionId: StateFlow<String?> = _createdSessionId.asStateFlow()

  private val _isCreating = MutableStateFlow(false)
  val isCreating: StateFlow<Boolean> = _isCreating.asStateFlow()

  private val _selectedTab = MutableStateFlow(SessionTab.Active)
  val selectedTab: StateFlow<SessionTab> = _selectedTab.asStateFlow()
  private var refreshJob: Job? = null
  private var refreshPending = false
  private var activeServerId: String? = null
  private var contentServerId: String? = null
  private var lastRefreshCompletedAt = 0L
  private var prefetchJob: Job? = null
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

  private suspend fun loadDurableSnapshot(serverId: String): SessionInventoryState.Snapshot? =
    kotlinx.coroutines.withContext(Dispatchers.IO) {
      val encoded = settingsDataStore.loadSessionInventoryCache() ?: return@withContext null
      val cached = decodeSessionInventoryCache(encoded, serverId, cacheJson) ?: return@withContext null
      SessionInventoryState.updateSnapshot(
        serverId = serverId,
        activeSessions = cached.activeSessions,
        machineSessions = cached.machineSessions,
        globalSessions = cached.globalSessions,
      )
    }

  private suspend fun saveDurableSnapshot(serverId: String, snapshot: SessionInventoryState.Snapshot) {
    kotlinx.coroutines.withContext(Dispatchers.IO) {
      try {
        settingsDataStore.saveSessionInventoryCache(
          cacheJson.encodeToString(
            SessionInventoryCache(
              serverId = serverId,
              activeSessions = snapshot.activeSessions,
              machineSessions = snapshot.machineSessions,
              globalSessions = snapshot.globalSessions,
            ),
          ),
        )
      } catch (cancelled: kotlinx.coroutines.CancellationException) {
        throw cancelled
      } catch (_: Exception) {
        // A cache write failure must not replace the in-memory inventory.
      }
    }
  }

  fun selectTab(tab: SessionTab) {
    _selectedTab.value = tab
  }

  fun refreshIfStale() {
    val serverId = activeServerId
    if (refreshJob?.isActive == true) {
      if (serverId != null && SessionInventoryState.isStale(serverId)) {
        refresh(force = true)
      }
      return
    }
    if (serverId != null && SessionInventoryState.isStale(serverId)) {
      refresh(force = true)
    }
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
    lastRefreshCompletedAt = now
    if (refreshJob?.isActive == true) {
      refreshPending = true
      return
    }
    val existing = _uiState.value as? SessionsUiState.Content
    if (existing != null) _uiState.value = existing.copy(refreshing = true)

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
        val cachedSnapshot = SessionInventoryState.snapshot(server.id)
          ?: loadDurableSnapshot(server.id)
        val existingForServer = existing.takeIf { contentServerId == server.id }
        val baseline = existingForServer ?: cachedSnapshot?.let {
          SessionsUiState.Content(
            activeSessions = it.activeSessions,
            machineSessions = it.machineSessions,
            globalSessions = it.globalSessions,
            refreshing = true,
          )
        }
        if (existingForServer == null && baseline != null) {
          _uiState.value = baseline
          contentServerId = server.id
        } else if (existing == null && baseline == null) {
          _uiState.value = SessionsUiState.Loading
        }

        val machineDeferred = async(Dispatchers.IO) { client.listMachineSessions(server) }
        val globalDeferred = async(Dispatchers.IO) { client.listGlobalSessions(server) }

        val coalescedResult = try {
          Result.success(
            SessionInventoryState.coalesceSessions(
              SessionInventoryState.requestKey(server, scope = "all", limit = null),
            ) {
              when (val result = client.listSessions(server)) {
                is HttpResult.Success -> SessionInventoryState.InventoryResult.Success(result.value.sessions)
                is HttpResult.Failure -> SessionInventoryState.InventoryResult.Failure(result.userMessage)
              }
            },
          )
        } catch (cancelled: kotlinx.coroutines.CancellationException) {
          throw cancelled
        } catch (error: Exception) {
          Result.failure(error)
        }
        val sessions = (coalescedResult.getOrNull() as? SessionInventoryState.InventoryResult.Success)?.sessions
        val activeSessions = sessions?.let {
          SessionInventoryState.applyPending(server.id, it)
            .sortedByDescending { it.updatedAt ?: it.createdAt ?: "" }
        } ?: baseline?.activeSessions.orEmpty()
        if (activeServerId != server.id) return@launch
        val activeSuccess = sessions != null
        if (activeSuccess) {
          val snapshot = SessionInventoryState.updateSnapshot(server.id, activeSessions = activeSessions)
          val current = (_uiState.value as? SessionsUiState.Content)
            ?.takeIf { contentServerId == server.id }
          _uiState.value = SessionsUiState.Content(
            activeSessions = activeSessions,
            machineSessions = current?.machineSessions ?: baseline?.machineSessions ?: snapshot.machineSessions,
            globalSessions = current?.globalSessions ?: baseline?.globalSessions ?: snapshot.globalSessions,
            refreshing = true,
          )
          contentServerId = server.id
        }

        if (activeSuccess) {
          prefetchJob?.cancel()
          prefetchJob = viewModelScope.launch(Dispatchers.IO) {
            prefetchSessionHistory(server, activeSessions.take(2))
          }
          launchDiscoveryPublication(server, activeSessions, machineDeferred, globalDeferred)
        }
        val machineResult = machineDeferred.await()
        val globalResult = globalDeferred.await()
        if (activeServerId != server.id) return@launch

        val machineSessions = when (machineResult) {
          is HttpResult.Success -> visibleMachineSessions(activeSessions, machineResult.value.sessions)
          is HttpResult.Failure -> baseline?.machineSessions ?: emptyList()
        }
        val globalSessions = when (globalResult) {
          is HttpResult.Success -> visibleGlobalSessions(activeSessions, globalResult.value.sessions)
          is HttpResult.Failure -> baseline?.globalSessions ?: emptyList()
        }
        val finalSnapshot = SessionInventoryState.updateSnapshot(
          serverId = server.id,
          activeSessions = activeSessions.takeIf { activeSuccess },
          machineSessions = machineSessions.takeIf { machineResult is HttpResult.Success },
          globalSessions = globalSessions.takeIf { globalResult is HttpResult.Success },
        )
        if (activeSuccess || machineResult is HttpResult.Success || globalResult is HttpResult.Success) {
          saveDurableSnapshot(server.id, finalSnapshot)
        }

        val completeSuccess =
          activeSuccess &&
            machineResult is HttpResult.Success &&
            globalResult is HttpResult.Success
        if (!completeSuccess) SessionInventoryState.markStale(server.id)

        if (!activeSuccess && machineResult is HttpResult.Failure && baseline == null) {
          _uiState.value = SessionsUiState.Error("Could not load sessions")
          return@launch
        }

        val currentContent = (_uiState.value as? SessionsUiState.Content)
          ?.takeIf { contentServerId == server.id }
        _uiState.value = SessionsUiState.Content(
          activeSessions = activeSessions,
          machineSessions = currentContent?.machineSessions ?: machineSessions,
          globalSessions = currentContent?.globalSessions ?: globalSessions,
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

  fun createSession(
    cwd: String,
    prompt: String = "",
    count: Int = 1,
    title: String? = null,
    createWorktree: Boolean = false,
    workerId: String = "local",
  ) {
    if (_isCreating.value) return
    _isCreating.value = true
    viewModelScope.launch {
      try {
        val outcomes = repository.createSessions(
          cwd = cwd,
          prompt = prompt,
          count = count,
          title = title,
          createWorktree = createWorktree,
          workerId = workerId,
        )
        val sessionIds = outcomes.mapNotNull { it.sessionId }
        if (sessionIds.isNotEmpty()) {
          activeServerId?.let(SessionInventoryState::markStale)
          if (count == 1) _createdSessionId.value = sessionIds.last()
          else refresh(force = true)
        }
        outcomes.firstOrNull { it.error != null }?.error?.let { _actionError.value = it }
      } finally {
        _isCreating.value = false
      }
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

  private fun launchDiscoveryPublication(
    server: com.example.picompanion.data.settings.ServerEntry,
    activeSessions: List<ServerSession>,
    machineDeferred: kotlinx.coroutines.Deferred<HttpResult<com.example.picompanion.data.model.MachineSessionListResponse>>,
    globalDeferred: kotlinx.coroutines.Deferred<HttpResult<com.example.picompanion.data.model.GlobalSessionListResponse>>,
  ) {
    viewModelScope.launch {
      val result = try {
        machineDeferred.await()
      } catch (cancelled: kotlinx.coroutines.CancellationException) {
        throw cancelled
      } catch (_: Exception) {
        return@launch
      }
      val success = result as? HttpResult.Success
      if (activeServerId != server.id || contentServerId != server.id) return@launch
      val sessions = success?.let { visibleMachineSessions(activeSessions, it.value.sessions) }
      val snapshot = SessionInventoryState.updateSnapshot(server.id, machineSessions = sessions)
      val current = (_uiState.value as? SessionsUiState.Content)
        ?.takeIf { contentServerId == server.id } ?: return@launch
      _uiState.value = current.copy(
        machineSessions = snapshot.machineSessions,
        refreshing = current.refreshing || refreshJob?.isActive == true,
      )
    }
    viewModelScope.launch {
      val result = try {
        globalDeferred.await()
      } catch (cancelled: kotlinx.coroutines.CancellationException) {
        throw cancelled
      } catch (_: Exception) {
        return@launch
      }
      val success = result as? HttpResult.Success
      if (activeServerId != server.id || contentServerId != server.id) return@launch
      val sessions = success?.let { visibleGlobalSessions(activeSessions, it.value.sessions) }
      val snapshot = SessionInventoryState.updateSnapshot(server.id, globalSessions = sessions)
      val current = (_uiState.value as? SessionsUiState.Content)
        ?.takeIf { contentServerId == server.id } ?: return@launch
      _uiState.value = current.copy(
        globalSessions = snapshot.globalSessions,
        refreshing = current.refreshing || refreshJob?.isActive == true,
      )
    }
  }

  private suspend fun prefetchSessionHistory(
    server: com.example.picompanion.data.settings.ServerEntry,
    sessions: List<ServerSession>,
  ) {
    sessions.forEach { session ->
      val key = "${server.id}:${session.id}"
      if (com.example.picompanion.ui.sessiondetail.SessionStateCache.contains(key)) return@forEach
      val payload = (client.getSessionMessages(server, session.id, limit = 30) as? HttpResult.Success)?.value ?: return@forEach
      val data = payload["data"] as? kotlinx.serialization.json.JsonObject ?: return@forEach
      val messages = data["messages"] as? kotlinx.serialization.json.JsonArray ?: return@forEach
      val history = data["history"] as? kotlinx.serialization.json.JsonObject
      val total = history?.get("total")?.jsonPrimitive?.intOrNull ?: messages.size
      val start = com.example.picompanion.ui.sessiondetail.historyPageStartIndex(total, 0, messages.size)
      val parsed = com.example.picompanion.ui.sessiondetail.SessionHistoryParser.parse(messages, start)
      com.example.picompanion.ui.sessiondetail.SessionStateCache.put(key,
        com.example.picompanion.ui.sessiondetail.SessionStateCache.Entry(
          items = parsed, historicalItems = parsed,
          nextHistoryOffset = history?.get("nextOffset")?.jsonPrimitive?.intOrNull ?: parsed.size,
          hasOlder = history?.get("hasOlder")?.jsonPrimitive?.booleanOrNull == true,
          title = session.title.orEmpty(), project = session.project.orEmpty(), cwd = session.cwd.orEmpty(),
          lastEventId = 0, totalHistoryMessages = total,
        ),
      )
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
