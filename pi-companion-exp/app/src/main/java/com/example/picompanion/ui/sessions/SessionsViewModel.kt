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

  init {
    refresh()
  }

  fun refresh() {
    viewModelScope.launch {
      _uiState.value = SessionsUiState.Loading
      val settings = settingsDataStore.settingsFlow.first()
      if (!settings.hasConfiguredServer) {
        _uiState.value = SessionsUiState.Empty
        return@launch
      }
      when (val result = repository.listSessions()) {
        is HttpResult.Success -> {
          _uiState.value = if (result.value.isEmpty()) {
            SessionsUiState.Empty
          } else {
            SessionsUiState.Content(result.value)
          }
        }
        is HttpResult.Failure -> {
          _uiState.value = SessionsUiState.Error(result.userMessage)
        }
      }
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

  fun clearCreatedSession() {
    _createdSessionId.value = null
  }
}

sealed interface SessionsUiState {
  data object Loading : SessionsUiState
  data object Empty : SessionsUiState
  data class Content(val sessions: List<ServerSession>) : SessionsUiState
  data class Error(val message: String) : SessionsUiState
}
