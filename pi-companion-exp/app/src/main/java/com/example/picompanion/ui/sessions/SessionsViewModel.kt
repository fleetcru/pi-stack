package com.example.picompanion.ui.sessions

import android.app.Application
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

  init {
    refresh()
  }

  fun selectTab(tab: SessionTab) {
    _selectedTab.value = tab
  }

  fun refresh() {
    viewModelScope.launch {
      _uiState.value = SessionsUiState.Loading
      val settings = settingsDataStore.settingsFlow.first()
      if (!settings.hasConfiguredServer) {
        _uiState.value = SessionsUiState.Empty
        return@launch
      }
      val server = settings.activeServer ?: return@launch

      // Fetch all session types in parallel
      val activeDeferred = async(Dispatchers.IO) { client.listSessions(server) }
      val machineDeferred = async(Dispatchers.IO) { client.listMachineSessions(server) }
      val globalDeferred = async(Dispatchers.IO) { client.listGlobalSessions(server) }

      val activeResult = activeDeferred.await()
      val machineResult = machineDeferred.await()
      val globalResult = globalDeferred.await()

      val activeSessions = (activeResult as? HttpResult.Success)?.value?.sessions
        ?.sortedByDescending { it.updatedAt ?: it.createdAt ?: "" }
        ?: emptyList()
      val machineSessions = visibleMachineSessions(
        activeSessions,
        (machineResult as? HttpResult.Success)?.value?.sessions ?: emptyList(),
      )
      val globalSessions = visibleGlobalSessions(
        activeSessions,
        (globalResult as? HttpResult.Success)?.value?.sessions ?: emptyList(),
      )

      if (activeResult is HttpResult.Failure && machineResult is HttpResult.Failure) {
        _uiState.value = SessionsUiState.Error((activeResult as? HttpResult.Failure)?.userMessage ?: "Failed to load sessions")
        return@launch
      }

      _uiState.value = SessionsUiState.Content(
        activeSessions = activeSessions,
        machineSessions = machineSessions,
        globalSessions = globalSessions,
      )
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
        // Keep the session list visible when creating a batch. The user can
        // choose which thread to open instead of being moved to the last one.
        if (count == 1) _createdSessionId.value = sessionIds.last()
        refresh()
      }
      outcomes.firstOrNull { it.error != null }?.error?.let { _actionError.value = it }
    }
  }

  fun deleteSession(sessionId: String) {
    viewModelScope.launch {
      when (val result = repository.deleteSession(sessionId)) {
        is HttpResult.Success -> refresh()
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
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: run {
        _actionError.value = "No server is configured"
        return@launch
      }
      when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.openMachineSession(server, machineId) }) {
        is HttpResult.Success -> {
          val sessionId = result.value.id.trim()
          if (sessionId.isEmpty()) {
            _actionError.value = "The server returned an invalid session ID"
          } else {
            // Do not route through _createdSessionId: a second tap could replace
            // the first result before the UI's LaunchedEffect consumes it.
            onOpened(sessionId)
          }
        }
        is HttpResult.Failure -> _actionError.value = "Could not open session: ${result.userMessage}"
      }
    }
  }

  fun attachGlobalSession(globalId: String, onAttached: (String) -> Unit) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: run {
        _actionError.value = "No server is configured"
        return@launch
      }
      when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.attachGlobalSession(server, globalId) }) {
        is HttpResult.Success -> {
          val sessionId = result.value.id.trim()
          if (sessionId.isEmpty()) {
            _actionError.value = "The server returned an invalid session ID"
          } else {
            onAttached(sessionId)
          }
        }
        is HttpResult.Failure -> _actionError.value = "Could not attach session: ${result.userMessage}"
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
  ) : SessionsUiState {
    val hasAnySessions: Boolean get() = activeSessions.isNotEmpty() || machineSessions.isNotEmpty() || globalSessions.isNotEmpty()
  }
  data class Error(val message: String) : SessionsUiState
}
