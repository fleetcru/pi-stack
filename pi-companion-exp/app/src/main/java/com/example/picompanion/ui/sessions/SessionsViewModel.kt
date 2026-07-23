package com.example.picompanion.ui.sessions

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.CreateSessionRequest
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.repository.SessionsRepository
import com.example.picompanion.data.settings.SettingsDataStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class SessionsViewModel(application: Application) : AndroidViewModel(application) {

  private val client = PiServerClient()
  private val settingsDataStore = SettingsDataStore(application)
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
      val machineSessions = (machineResult as? HttpResult.Success)?.value?.sessions ?: emptyList()
      val globalSessions = (globalResult as? HttpResult.Success)?.value?.sessions ?: emptyList()

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

  fun createSession(cwd: String) {
    viewModelScope.launch {
      when (val result = repository.createSession(CreateSessionRequest(cwd = cwd, start = true))) {
        is HttpResult.Success -> {
          _createdSessionId.value = result.value.id
          refresh()
        }
        is HttpResult.Failure -> {
          // Keep current state, could show a snackbar
        }
      }
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

  fun openMachineSession(machineId: String) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.openMachineSession(server, machineId) }) {
        is HttpResult.Success -> _createdSessionId.value = result.value.id
        is HttpResult.Failure -> _actionError.value = "Could not open session: ${result.userMessage}"
      }
    }
  }

  fun attachGlobalSession(globalId: String) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      when (val result = kotlinx.coroutines.withContext(Dispatchers.IO) { client.attachGlobalSession(server, globalId) }) {
        is HttpResult.Success -> _createdSessionId.value = result.value.id
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
