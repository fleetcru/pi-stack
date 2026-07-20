package com.example.picompanion.ui.workers

import android.app.Application
import androidx.lifecycle.AndroidViewModel
import androidx.lifecycle.viewModelScope
import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.ServerWorker
import com.example.picompanion.data.repository.WorkersRepository
import com.example.picompanion.data.settings.SettingsDataStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

class WorkersViewModel(application: Application) : AndroidViewModel(application) {

  private val client = PiServerClient()
  private val settingsDataStore = SettingsDataStore(application)
  private val repository = WorkersRepository(client, settingsDataStore)

  private val _uiState = MutableStateFlow<WorkersUiState>(WorkersUiState.Loading)
  val uiState: StateFlow<WorkersUiState> = _uiState.asStateFlow()

  init {
    refresh()
  }

  fun saveWorker(id: String, url: String, token: String, tags: List<String>) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      val request = com.example.picompanion.data.model.WorkerWriteRequest(id, url, token.ifBlank { null }, tags)
      kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) {
        if ((_uiState.value as? WorkersUiState.Content)?.workers?.any { it.id == id } == true) client.updateWorker(server, request) else client.addWorker(server, request)
      }
      refresh()
    }
  }

  fun deleteWorker(id: String) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) { client.deleteWorker(server, id) }
      refresh()
    }
  }

  fun checkHealth(id: String) {
    viewModelScope.launch {
      val server = settingsDataStore.settingsFlow.first().activeServer ?: return@launch
      kotlinx.coroutines.withContext(kotlinx.coroutines.Dispatchers.IO) { client.checkWorkerHealth(server, id) }
      refresh()
    }
  }

  fun refresh() {
    viewModelScope.launch {
      _uiState.value = WorkersUiState.Loading
      val settings = settingsDataStore.settingsFlow.first()
      if (!settings.hasConfiguredServer) {
        _uiState.value = WorkersUiState.Empty
        return@launch
      }
      when (val result = repository.listWorkers()) {
        is HttpResult.Success -> {
          _uiState.value = if (result.value.isEmpty()) {
            WorkersUiState.Empty
          } else {
            WorkersUiState.Content(result.value)
          }
        }
        is HttpResult.Failure -> {
          _uiState.value = WorkersUiState.Error(result.userMessage)
        }
      }
    }
  }
}

sealed interface WorkersUiState {
  data object Loading : WorkersUiState
  data object Empty : WorkersUiState
  data class Content(val workers: List<ServerWorker>) : WorkersUiState
  data class Error(val message: String) : WorkersUiState
}
