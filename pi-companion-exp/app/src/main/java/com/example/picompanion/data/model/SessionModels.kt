package com.example.picompanion.data.model

import kotlinx.serialization.Serializable

@Serializable
data class DirectoryListResponse(
  val path: String? = null,
  val parent: String? = null,
  val directories: List<DirectoryEntry> = emptyList(),
  val roots: List<DirectoryEntry> = emptyList(),
)

@Serializable
data class DirectoryEntry(
  val name: String,
  val path: String,
)

@Serializable
data class FileEntry(
  val name: String,
  val path: String,
  val isDir: Boolean,
  val size: Long = 0,
)

@Serializable
data class FileListResponse(
  val cwd: String,
  val files: List<FileEntry> = emptyList(),
)

@Serializable
data class FileContentResponse(
  val path: String,
  val content: String? = null,
  val binary: Boolean = false,
  val truncated: Boolean = false,
)

@Serializable
data class ServerSession(
  val id: String,
  val cwd: String? = null,
  val args: List<String> = emptyList(),
  val status: String? = null,
  val managed: Boolean = false,
  val project: String? = null,
  val title: String? = null,
  val taskType: String? = null,
  val owner: String? = null,
  val labels: List<String> = emptyList(),
  val metadata: Map<String, String> = emptyMap(),
  val createdAt: String? = null,
  val updatedAt: String? = null,
)

@Serializable
data class SessionListResponse(
  val sessions: List<ServerSession> = emptyList(),
)

@Serializable
data class GlobalSession(
  val id: String,
  val originId: String,
  val workerId: String,
  val session: ServerSession,
  val reachable: Boolean = true,
)

@Serializable
data class GlobalSessionListResponse(
  val sessions: List<GlobalSession> = emptyList(),
)

@Serializable
data class MachineSession(
  val id: String,
  val path: String,
  val cwd: String,
  val createdAt: String? = null,
  val updatedAt: String? = null,
  val size: Long = 0,
)

@Serializable
data class MachineSessionListResponse(
  val root: String? = null,
  val sessions: List<MachineSession> = emptyList(),
)

@Serializable
data class CreateSessionRequest(
  val cwd: String,
  val args: List<String> = emptyList(),
  val env: Map<String, String> = emptyMap(),
  val start: Boolean = true,
  val restart: Boolean = false,
  val project: String? = null,
  val title: String? = null,
  val taskType: String? = null,
  val owner: String? = null,
  val labels: List<String> = emptyList(),
  val metadata: Map<String, String> = emptyMap(),
  val sessionPath: String? = null,
  val id: String? = null,
)

@Serializable
data class CreateSessionResponse(
  val id: String,
  val cwd: String? = null,
  val args: List<String> = emptyList(),
  val ws: String? = null,
)

/** A short-lived, single-use server-issued ticket for a session WebSocket. */
@Serializable
data class WebSocketTicketResponse(
  val ticket: String,
  val expiresAt: String,
  val ws: String,
)
