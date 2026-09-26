package com.example.picompanion.data.repository

import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.model.CreateSessionRequest
import com.example.picompanion.data.model.CreateWorktreeOptions
import com.example.picompanion.data.model.ServerSession
import com.example.picompanion.data.settings.ServerEntry
import com.example.picompanion.data.settings.SettingsDataStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.async
import kotlinx.coroutines.awaitAll
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.JsonObject

class SessionsRepository(
  private val client: PiServerClient,
  private val settingsDataStore: SettingsDataStore,
) {

  data class CreateOutcome(
    val sessionId: String?,
    val error: String?,
  )

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
    workerId: String = "local",
  ): HttpResult<ServerSession> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      val result = if (workerId == "local") {
        client.createSession(target, request)
      } else {
        client.createWorkerSession(target, workerId, request)
      }
      when (result) {
        is HttpResult.Success -> {
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

  /**
   * Shared create path for SessionsScreen and ShellScreen drawer.
   * Creates 1–12 sessions, optionally seeding each with [prompt].
   */
  suspend fun createSessions(
    cwd: String,
    prompt: String = "",
    count: Int = 1,
    title: String? = null,
    createWorktree: Boolean = false,
    workerId: String = "local",
    server: ServerEntry? = null,
  ): List<CreateOutcome> {
    val target = server ?: getActiveServer() ?: return listOf(CreateOutcome(null, "No server configured"))
    val sessionCount = count.coerceIn(1, 12)
    return coroutineScope {
      (1..sessionCount).map { index ->
        async(Dispatchers.IO) {
          val baseTitle = title?.trim().orEmpty()
          val sessionTitle = when {
            sessionCount > 1 -> "${baseTitle.ifBlank { "New session" }} $index"
            baseTitle.isNotBlank() -> baseTitle
            else -> null
          }
          val request = CreateSessionRequest(
            cwd = cwd,
            title = sessionTitle,
            start = true,
            createWorktree = if (createWorktree) CreateWorktreeOptions(enabled = true) else null,
          )
          when (val created = createSession(request, target, workerId)) {
            is HttpResult.Success -> {
              if (prompt.isBlank()) {
                CreateOutcome(created.value.id, null)
              } else {
                when (val sent = sendPrompt(created.value.id, prompt, target)) {
                  is HttpResult.Success -> CreateOutcome(created.value.id, null)
                  is HttpResult.Failure -> CreateOutcome(
                    created.value.id,
                    "Session created, but task was not sent: ${sent.userMessage}",
                  )
                }
              }
            }
            is HttpResult.Failure -> CreateOutcome(null, created.userMessage)
          }
        }
      }.awaitAll()
    }
  }

  suspend fun sendPrompt(sessionId: String, message: String, server: ServerEntry? = null, idempotencyKey: String? = null): HttpResult<Unit> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      client.sendPrompt(target, sessionId, message, idempotencyKey = idempotencyKey)
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

  suspend fun getSessionMessages(sessionId: String, server: ServerEntry? = null, offset: Int = 0, limit: Int = 50): HttpResult<JsonObject> {
    val target = server ?: getActiveServer() ?: return HttpResult.Failure("No server configured")
    return withContext(Dispatchers.IO) {
      client.getSessionMessages(target, sessionId, offset, limit)
    }
  }
}
