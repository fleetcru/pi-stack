package com.example.picompanion.ui.sessiondetail

import com.example.picompanion.data.api.HttpResult
import com.example.picompanion.data.api.PiServerClient
import com.example.picompanion.data.settings.ServerEntry
import com.example.picompanion.data.websocket.SessionEventSocket
import com.example.picompanion.data.websocket.SocketEvent
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.withContext

/** Owns ticket acquisition and socket resources independently of UI state. */
internal class SessionTransportCoordinator(
  private val client: PiServerClient,
  private val sessionId: String,
) {
  private val socket = SessionEventSocket(client.okHttpClient)
  val events: Flow<SocketEvent> = socket.events

  suspend fun open(server: ServerEntry, since: Long?): String? {
    return when (val ticket = withContext(Dispatchers.IO) { client.issueWebSocketTicket(server, sessionId) }) {
      is HttpResult.Success -> { socket.connect(server, ticket.value.ws, since); null }
      is HttpResult.Failure -> ticket.message
    }
  }

  fun disconnect() = socket.disconnect()
  fun isConnected(): Boolean = socket.isConnected()
  fun close() = socket.close()
}
