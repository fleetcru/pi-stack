package com.example.picompanion.data.repository

import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.CreateSessionRequest
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.settings.ServerEntry
import com.example.picompanion.data.settings.SettingsDataStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject

class SessionsRepository(
  private val client: PiServerClient,
  private val settingsDataStore: SettingsDataStore,
) {

  suspend fun getActiveServer(): ServerEntry? {
    val settings = settingsDataStore.settingsFlow.first()
    return settings.activeServer
  }

  suspend fun listSessions(server: ServerEntry? = null): HttpResult<List<ServerSession>> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      when (val result = client.listSessions(target)) {
        is HttpResult.Success -> HttpResult.Success(result.value.sessions)
        is HttpResult.Failure -> result
      }
    }
  }

  suspend fun createSession(
    request: CreateSessionRequest,
    server: ServerEntry? = null,
  ): HttpResult<ServerSession> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      when (val result = client.createSession(target, request)) {
        is HttpResult.Success -> {
          // Return a local model from the create response — no second fetch needed
          HttpResult.Success(
            ServerSession(
              id = result.value.id,
              cwd = result.value.cwd ?: request.cwd,
              status = "created",
              title = request.title,
              project = request.project,
            )
          )
        }
        is HttpResult.Failure -> result
      }
    }
  }

  suspend fun sendPrompt(sessionId: String, message: String, server: ServerEntry? = null): HttpResult<Unit> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      client.sendPrompt(target, sessionId, message)
    }
  }

  suspend fun sendSteer(sessionId: String, message: String, server: ServerEntry? = null): HttpResult<Unit> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      client.steer(target, sessionId, message)
    }
  }

  suspend fun deleteSession(sessionId: String, server: ServerEntry? = null): HttpResult<Unit> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      client.deleteSession(target, sessionId)
    }
  }

  suspend fun getSessionMessages(sessionId: String, server: ServerEntry? = null, offset: Int = 0): HttpResult<JsonObject> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      client.getSessionMessages(target, sessionId, offset)
    }
  }
}
