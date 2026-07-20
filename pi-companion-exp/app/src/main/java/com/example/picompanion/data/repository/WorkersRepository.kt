package com.example.picompanion.data.repository

import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.ServerWorker
import com.example.picompanion.data.settings.SettingsDataStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withContext

class WorkersRepository(
  private val client: PiServerClient,
  private val settingsDataStore: SettingsDataStore,
) {

  suspend fun listWorkers(): HttpResult<List<ServerWorker>> {
    val settings = settingsDataStore.settingsFlow.first()
    val server = settings.activeServer ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      when (val result = client.listWorkers(server)) {
        is HttpResult.Success -> HttpResult.Success(result.value.workers)
        is HttpResult.Failure -> result
      }
    }
  }
}
