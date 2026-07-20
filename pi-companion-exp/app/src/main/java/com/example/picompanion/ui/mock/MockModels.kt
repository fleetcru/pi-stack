package com.example.picompanion.ui.mock

import androidx.compose.runtime.Immutable

@Immutable
data class MockSession(
  val id: String,
  val title: String,
  val project: String,
  val cwd: String,
  val status: String,
  val lastMessage: String,
  val updatedAt: String,
)

@Immutable
data class MockChatMessage(
  val id: String,
  val author: String,
  val text: String,
  val time: String,
  val isUser: Boolean,
)

@Immutable
data class MockWorker(
  val id: String,
  val name: String,
  val status: String,
  val capacity: String,
)

val mockSessions = listOf(
  MockSession(
    id = "1",
    title = "Build Android companion",
    project = "pi-companion",
    cwd = "~/projects/pi-companion",
    status = "Running",
    lastMessage = "Updating Compose dashboard components",
    updatedAt = "now"
  ),
  MockSession(
    id = "2",
    title = "Pi server hardening",
    project = "pi-server",
    cwd = "~/projects/pi-server",
    status = "Idle",
    lastMessage = "go test ./... passed",
    updatedAt = "12m"
  ),
  MockSession(
    id = "3",
    title = "Browser test client",
    project = "pi-server-full-test",
    cwd = "~/projects/pi-server-full-test",
    status = "Stopped",
    lastMessage = "Waiting for next task",
    updatedAt = "1h"
  ),
)

val mockChatMessages = listOf(
  MockChatMessage(
    id = "1",
    author = "You",
    text = "Can you make the Android app look more like the mockup?",
    time = "9:14 PM",
    isUser = true
  ),
  MockChatMessage(
    id = "2",
    author = "Pi Agent",
    text = "Sure — I updated the dark dashboard, icons, and bottom navigation.",
    time = "9:15 PM",
    isUser = false
  ),
  MockChatMessage(
    id = "3",
    author = "You",
    text = "Now make a session sidebar and chat UI example.",
    time = "9:22 PM",
    isUser = true
  ),
  MockChatMessage(
    id = "4",
    author = "Pi Agent",
    text = "I can scaffold the mock session list, detail chat screen, and settings page.",
    time = "9:22 PM",
    isUser = false
  ),
)
