package com.example.picompanion.data.notifications

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.example.picompanion.R
import java.util.concurrent.ConcurrentHashMap

/**
 * Delivers lifecycle notifications only for transitions that matter when the
 * user is away from the session screen. State is kept per session so replayed
 * events after reconnect do not create duplicate alerts.
 */
object SessionNotificationManager {
  private const val channelId = "pi-session-updates"
  private val lastStates = ConcurrentHashMap<String, String>()

  fun initialize(context: Context) {
    if (Build.VERSION.SDK_INT < Build.VERSION_CODES.O) return
    val manager = context.getSystemService(NotificationManager::class.java)
    manager.createNotificationChannel(
      NotificationChannel(
        channelId,
        "Pi session updates",
        NotificationManager.IMPORTANCE_DEFAULT,
      ).apply {
        description = "Task completion, input requests, and session failures"
      },
    )
  }

  fun notifyRuntimeTransition(
    context: Context,
    sessionId: String,
    state: String?,
    detail: String?,
    appInForeground: Boolean,
  ) {
    if (state.isNullOrBlank()) return
    val previous = lastStates.put(sessionId, state)
    if (appInForeground || previous == state) return

    val (title, body) = when (state) {
      "idle", "created" -> "Pi finished" to "Session \${shortId(sessionId)} completed its task."
      "waiting_for_input" -> "Pi needs input" to (detail?.takeIf(String::isNotBlank)
        ?: "Session \${shortId(sessionId)} is waiting for your response.")
      "failed", "error", "stopped" -> "Pi session stopped" to "Session \${shortId(sessionId)} $state."
      else -> return
    }
    post(context, sessionId, title, body)
  }

  fun notifyInputRequest(context: Context, sessionId: String, message: String?, appInForeground: Boolean) {
    if (appInForeground) return
    post(
      context,
      sessionId,
      "Pi needs input",
      message?.takeIf(String::isNotBlank) ?: "Pi is waiting for your response.",
    )
  }

  private fun post(context: Context, sessionId: String, title: String, body: String) {
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
      context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
    ) return

    val notification = NotificationCompat.Builder(context, channelId)
      .setSmallIcon(R.drawable.pi_stack_logo)
      .setContentTitle(title)
      .setContentText(body)
      .setStyle(NotificationCompat.BigTextStyle().bigText(body))
      .setPriority(NotificationCompat.PRIORITY_DEFAULT)
      .setAutoCancel(true)
      .setOnlyAlertOnce(true)
      .build()
    NotificationManagerCompat.from(context).notify(sessionId.hashCode(), notification)
  }

  private fun shortId(sessionId: String): String = sessionId.take(8)
}
