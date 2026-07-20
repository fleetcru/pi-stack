package com.example.picompanion.data.api

import com.example.picompanion.data.model.CreateSessionRequest
import com.example.picompanion.data.model.CreateSessionResponse
import com.example.picompanion.data.model.DirectoryListResponse
import com.example.picompanion.data.model.HealthResponse
import com.example.picompanion.data.model.ServerCapacity
import com.example.picompanion.data.model.FileListResponse
import com.example.picompanion.data.model.FileContentResponse
import com.example.picompanion.data.model.SessionListResponse
import com.example.picompanion.data.model.GlobalSessionListResponse
import com.example.picompanion.data.model.MachineSessionListResponse
import com.example.picompanion.data.model.WorkerListResponse
import com.example.picompanion.data.model.ServerWorker
import com.example.picompanion.data.model.WorkerWriteRequest
import com.example.picompanion.data.model.WebSocketTicketResponse
import com.example.picompanion.data.settings.ServerEntry
import kotlinx.serialization.KSerializer
import kotlinx.serialization.Serializable
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import okhttp3.HttpUrl.Companion.toHttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.net.ConnectException
import java.net.SocketTimeoutException
import java.net.UnknownHostException

@Serializable
data class PromptImage(val base64: String, val mimeType: String)

@Serializable
private data class SendPromptRequest(val message: String, val images: List<PromptImage> = emptyList())

@Serializable
private data class SetModelRequest(val provider: String, val modelId: String)

@Serializable
private data class SetThinkingLevelRequest(val level: String)

@Serializable
private data class IssueTicketRequest(val sessionId: String)

@Serializable
private data class ExtensionUiResponse(
  val id: String,
  val cancelled: Boolean = false,
  val value: String? = null,
  val confirmed: Boolean? = null,
)

@Serializable
private data class UpdateMetadataRequest(val title: String, val project: String)

@Serializable
private data class SteerRequest(val message: String)

@Serializable
private data class UpdateCapacityRequest(val maxSessions: Int)

class PiServerClient(
  private val okHttpClient: OkHttpClient = OkHttpClient.Builder()
    .connectTimeout(10, java.util.concurrent.TimeUnit.SECONDS)
    .readTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
    .writeTimeout(30, java.util.concurrent.TimeUnit.SECONDS)
    .build(),
  private val json: Json = apiJson,
) {

  // ── Health ──────────────────────────────────────────────

  fun checkHealth(server: ServerEntry): HttpResult<HealthResponse> {
    return doGet(server, "/healthz", emptyMap(), HealthResponse.serializer())
  }

  fun updateCapacity(server: ServerEntry, maxSessions: Int): HttpResult<ServerCapacity> {
    val body = json.encodeToString(UpdateCapacityRequest(maxSessions))
    return doPatch(server, "/v1/capacity", body, ServerCapacity.serializer())
  }

  // ── Sessions ───────────────────────────────────────────

  /** Includes current runtime status and coordinator-mapped remote sessions. */
  fun listSessions(server: ServerEntry): HttpResult<SessionListResponse> {
    return doGet(server, "/v1/sessions?scope=all&include=state", emptyMap(), SessionListResponse.serializer())
  }

  fun listGlobalSessions(server: ServerEntry): HttpResult<GlobalSessionListResponse> =
    doGet(server, "/v1/global-sessions", emptyMap(), GlobalSessionListResponse.serializer())

  fun attachGlobalSession(server: ServerEntry, globalId: String): HttpResult<CreateSessionResponse> =
    doPost(server, "/v1/global-sessions/$globalId/attach", "{}", CreateSessionResponse.serializer())

  fun listMachineSessions(server: ServerEntry): HttpResult<MachineSessionListResponse> =
    doGet(server, "/v1/machine-sessions", emptyMap(), MachineSessionListResponse.serializer())

  fun openMachineSession(server: ServerEntry, machineSessionId: String): HttpResult<CreateSessionResponse> =
    doPost(server, "/v1/machine-sessions/$machineSessionId/open", "{}", CreateSessionResponse.serializer())

  fun createSession(server: ServerEntry, request: CreateSessionRequest): HttpResult<CreateSessionResponse> {
    val body = json.encodeToString(CreateSessionRequest.serializer(), request)
    return doPost(server, "/v1/sessions", body, CreateSessionResponse.serializer())
  }

  fun sendPrompt(
    server: ServerEntry,
    sessionId: String,
    message: String,
    images: List<PromptImage> = emptyList(),
  ): HttpResult<Unit> {
    val body = json.encodeToString(SendPromptRequest(message, images))
    return doPostUnit(server, "/v1/sessions/$sessionId/prompt", body)
  }

  /** Pi-server proxies Pi's get_messages RPC response for the active session. */
  fun getSessionMessages(server: ServerEntry, sessionId: String, offset: Int = 0): HttpResult<JsonObject> {
    return doGet(server, "/v1/sessions/$sessionId/messages?offset=$offset", emptyMap(), JsonObject.serializer())
  }

  fun getSessionGit(server: ServerEntry, sessionId: String, resource: String): HttpResult<JsonObject> {
    return doGet(server, "/v1/sessions/$sessionId/git/$resource", emptyMap(), JsonObject.serializer())
  }

  fun getSessionModels(server: ServerEntry, sessionId: String): HttpResult<JsonObject> =
    doGet(server, "/v1/sessions/$sessionId/models", emptyMap(), JsonObject.serializer())

  fun getSessionState(server: ServerEntry, sessionId: String): HttpResult<JsonObject> =
    doGet(server, "/v1/sessions/$sessionId/state", emptyMap(), JsonObject.serializer())

  fun setSessionModel(server: ServerEntry, sessionId: String, provider: String, modelId: String): HttpResult<Unit> =
    doPostUnit(server, "/v1/sessions/$sessionId/model", json.encodeToString(SetModelRequest(provider, modelId)))

  fun setThinkingLevel(server: ServerEntry, sessionId: String, level: String): HttpResult<Unit> =
    doPostUnit(server, "/v1/sessions/$sessionId/thinking-level", json.encodeToString(SetThinkingLevelRequest(level)))

  fun deleteSession(server: ServerEntry, sessionId: String): HttpResult<Unit> {
    return doDeleteUnit(server, "/v1/sessions/$sessionId")
  }

  /**
   * Browser WebSockets cannot carry an Authorization header, and tickets also
   * avoid exposing a server token during a socket upgrade. Use one fresh ticket
   * for every connection and reconnect.
   */
  fun issueWebSocketTicket(
    server: ServerEntry,
    sessionId: String,
  ): HttpResult<WebSocketTicketResponse> {
    val body = json.encodeToString(IssueTicketRequest(sessionId))
    return doPost(server, "/v1/ws-tickets", body, WebSocketTicketResponse.serializer())
  }

  /** Sends any existing pi-server session action without duplicating HTTP code. */
  fun steer(server: ServerEntry, sessionId: String, message: String): HttpResult<Unit> =
    doPostUnit(server, "/v1/sessions/$sessionId/steer", json.encodeToString(SteerRequest(message)))

  fun postSessionAction(
    server: ServerEntry,
    sessionId: String,
    action: String,
    body: String = "{}",
  ): HttpResult<Unit> = doPostUnit(server, "/v1/sessions/$sessionId/$action", body)

  fun respondToExtensionUi(server: ServerEntry, sessionId: String, id: String, value: String? = null, confirmed: Boolean? = null, cancelled: Boolean = false): HttpResult<Unit> {
    val body = json.encodeToString(ExtensionUiResponse(id, cancelled, value, confirmed))
    return doPostUnit(server, "/v1/sessions/$sessionId/ui-response", body)
  }

  fun updateSessionMetadata(
    server: ServerEntry,
    sessionId: String,
    title: String,
    project: String,
  ): HttpResult<Unit> {
    val body = json.encodeToString(UpdateMetadataRequest(title, project))
    return doPostUnit(server, "/v1/sessions/$sessionId/metadata", body)
  }

  // ── Workers ────────────────────────────────────────────

  fun listWorkers(server: ServerEntry): HttpResult<WorkerListResponse> {
    return doGet(server, "/v1/workers", emptyMap(), WorkerListResponse.serializer())
  }

  fun addWorker(server: ServerEntry, worker: WorkerWriteRequest): HttpResult<ServerWorker> =
    doPost(server, "/v1/workers", json.encodeToString(WorkerWriteRequest.serializer(), worker), ServerWorker.serializer())

  fun updateWorker(server: ServerEntry, worker: WorkerWriteRequest): HttpResult<ServerWorker> =
    doPut(server, "/v1/workers/${worker.id}", json.encodeToString(WorkerWriteRequest.serializer(), worker), ServerWorker.serializer())

  fun deleteWorker(server: ServerEntry, workerId: String): HttpResult<Unit> =
    doDeleteUnit(server, "/v1/workers/$workerId")

  fun checkWorkerHealth(server: ServerEntry, workerId: String): HttpResult<JsonObject> =
    doGet(server, "/v1/workers/$workerId/health", emptyMap(), JsonObject.serializer())

  // ── Directories ────────────────────────────────────────

  fun listDirectories(server: ServerEntry, path: String? = null): HttpResult<DirectoryListResponse> {
    val queryParams = if (path != null) mapOf("path" to path) else emptyMap()
    return doGet(server, "/v1/directories", queryParams, DirectoryListResponse.serializer())
  }

  fun listFiles(server: ServerEntry, cwd: String): HttpResult<FileListResponse> =
    doGet(server, "/v1/files", mapOf("cwd" to cwd), FileListResponse.serializer())

  fun getFileContent(server: ServerEntry, path: String): HttpResult<FileContentResponse> =
    doGet(server, "/v1/files/content", mapOf("path" to path), FileContentResponse.serializer())

  // ── Internal HTTP helpers ──────────────────────────────

  private fun <T> doGet(
    server: ServerEntry,
    path: String,
    queryParams: Map<String, String>,
    serializer: KSerializer<T>,
  ): HttpResult<T> {
    return try {
      val baseUrl = server.url.trimEnd('/')
      val urlBuilder = "$baseUrl$path".toHttpUrl().newBuilder()
      queryParams.forEach { (key, value) -> urlBuilder.addQueryParameter(key, value) }
      val url = urlBuilder.build()

      val requestBuilder = Request.Builder().url(url).get()
      addAuth(server, requestBuilder)

      val response = okHttpClient.newCall(requestBuilder.build()).execute()
      val body = response.body?.string() ?: ""

      if (!response.isSuccessful) {
        return HttpResult.Failure(
          message = "HTTP ${response.code}: ${body.take(200)}",
          code = response.code,
        )
      }

      val decoded = json.decodeFromString(serializer, body)
      HttpResult.Success(decoded)
    } catch (e: ConnectException) {
      HttpResult.Failure(message = "Connection refused — is pi-server running?", cause = e)
    } catch (e: UnknownHostException) {
      HttpResult.Failure(message = "Host not found — check server URL", cause = e)
    } catch (e: SocketTimeoutException) {
      HttpResult.Failure(message = "Connection timed out", cause = e)
    } catch (e: IOException) {
      HttpResult.Failure(message = "Network error: ${e.message ?: "unknown"}", cause = e)
    } catch (e: Exception) {
      HttpResult.Failure(message = "Unexpected error: ${e.message ?: "unknown"}", cause = e)
    }
  }

  private fun <T> doPost(
    server: ServerEntry,
    path: String,
    body: String,
    serializer: KSerializer<T>,
  ): HttpResult<T> {
    return try {
      val url = "${server.url.trimEnd('/')}$path"
      val requestBody = body.toRequestBody("application/json".toMediaType())

      val requestBuilder = Request.Builder().url(url).post(requestBody)
      addAuth(server, requestBuilder)

      val response = okHttpClient.newCall(requestBuilder.build()).execute()
      val responseBody = response.body?.string() ?: ""

      if (!response.isSuccessful) {
        return HttpResult.Failure(
          message = "HTTP ${response.code}: ${responseBody.take(200)}",
          code = response.code,
        )
      }

      val decoded = json.decodeFromString(serializer, responseBody)
      HttpResult.Success(decoded)
    } catch (e: ConnectException) {
      HttpResult.Failure(message = "Connection refused — is pi-server running?", cause = e)
    } catch (e: UnknownHostException) {
      HttpResult.Failure(message = "Host not found — check server URL", cause = e)
    } catch (e: SocketTimeoutException) {
      HttpResult.Failure(message = "Connection timed out", cause = e)
    } catch (e: IOException) {
      HttpResult.Failure(message = "Network error: ${e.message ?: "unknown"}", cause = e)
    } catch (e: Exception) {
      HttpResult.Failure(message = "Unexpected error: ${e.message ?: "unknown"}", cause = e)
    }
  }

  private fun <T> doPut(server: ServerEntry, path: String, body: String, serializer: KSerializer<T>): HttpResult<T> {
    return try {
      val request = Request.Builder().url("${server.url.trimEnd('/')}$path")
        .put(body.toRequestBody("application/json".toMediaType())).also { addAuth(server, it) }.build()
      val response = okHttpClient.newCall(request).execute()
      val responseBody = response.body?.string() ?: ""
      if (!response.isSuccessful) return HttpResult.Failure("HTTP ${response.code}: ${responseBody.take(200)}", response.code)
      HttpResult.Success(json.decodeFromString(serializer, responseBody))
    } catch (e: IOException) {
      HttpResult.Failure(message = "Network error: ${e.message ?: "unknown"}", cause = e)
    } catch (e: Exception) {
      HttpResult.Failure(message = "Unexpected error: ${e.message ?: "unknown"}", cause = e)
    }
  }

  private fun <T> doPatch(server: ServerEntry, path: String, body: String, serializer: KSerializer<T>): HttpResult<T> {
    return try {
      val request = Request.Builder().url("${server.url.trimEnd('/')}$path")
        .patch(body.toRequestBody("application/json".toMediaType())).also { addAuth(server, it) }.build()
      val response = okHttpClient.newCall(request).execute()
      val responseBody = response.body?.string() ?: ""
      if (!response.isSuccessful) return HttpResult.Failure("HTTP ${response.code}: ${responseBody.take(200)}", response.code)
      HttpResult.Success(json.decodeFromString(serializer, responseBody))
    } catch (e: IOException) {
      HttpResult.Failure(message = "Network error: ${e.message ?: "unknown"}", cause = e)
    } catch (e: Exception) {
      HttpResult.Failure(message = "Unexpected error: ${e.message ?: "unknown"}", cause = e)
    }
  }

  private fun doDeleteUnit(server: ServerEntry, path: String): HttpResult<Unit> {
    return try {
      val requestBuilder = Request.Builder().url("${server.url.trimEnd('/')}$path").delete()
      addAuth(server, requestBuilder)
      okHttpClient.newCall(requestBuilder.build()).execute().use { response ->
        if (!response.isSuccessful) {
          return HttpResult.Failure("HTTP ${response.code}: ${(response.body?.string() ?: "").take(200)}", response.code)
        }
        HttpResult.Success(Unit)
      }
    } catch (e: IOException) {
      HttpResult.Failure(message = "Network error: ${e.message ?: "unknown"}", cause = e)
    } catch (e: Exception) {
      HttpResult.Failure(message = "Unexpected error: ${e.message ?: "unknown"}", cause = e)
    }
  }

  private fun doPostUnit(server: ServerEntry, path: String, body: String): HttpResult<Unit> {
    return try {
      val url = "${server.url.trimEnd('/')}$path"
      val requestBody = body.toRequestBody("application/json".toMediaType())
      val requestBuilder = Request.Builder().url(url).post(requestBody)
      addAuth(server, requestBuilder)

      okHttpClient.newCall(requestBuilder.build()).execute().use { response ->
        if (!response.isSuccessful) {
          val responseBody = response.body?.string() ?: ""
          return HttpResult.Failure(
            message = "HTTP ${response.code}: ${responseBody.take(200)}",
            code = response.code,
          )
        }
        HttpResult.Success(Unit)
      }
    } catch (e: ConnectException) {
      HttpResult.Failure(message = "Connection refused — is pi-server running?", cause = e)
    } catch (e: UnknownHostException) {
      HttpResult.Failure(message = "Host not found — check server URL", cause = e)
    } catch (e: SocketTimeoutException) {
      HttpResult.Failure(message = "Connection timed out", cause = e)
    } catch (e: IOException) {
      HttpResult.Failure(message = "Network error: ${e.message ?: "unknown"}", cause = e)
    } catch (e: Exception) {
      HttpResult.Failure(message = "Unexpected error: ${e.message ?: "unknown"}", cause = e)
    }
  }

  private fun addAuth(server: ServerEntry, builder: Request.Builder) {
    if (server.authToken.isNotBlank()) {
      builder.addHeader("Authorization", "Bearer ${server.authToken}")
    }
  }

}
