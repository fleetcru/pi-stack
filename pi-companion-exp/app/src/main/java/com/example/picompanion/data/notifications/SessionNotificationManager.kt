package com.example.picompanion.data.notifications

import android.Manifest
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.NotificationManagerCompat
import com.example.picompanion.MainActivity
import com.example.picompanion.R
import java.util.concurrent.ConcurrentHashMap

internal data class SessionNotification(
  val title: String,
  val body: String,
)

/** Pure transition policy, kept separate so notification behavior is testable. */
internal fun runtimeNotificationForTransition(
  sessionId: String,
  previous: String?,
  current: String,
): SessionNotification? {
  // A replay or initial snapshot is baseline state, not a new transition.
  if (previous == null || previous == current) return null
  val shortId = sessionId.take(8)
  return when (current) {
    "idle", "created" -> if (previous == "working" || previous == "starting") {
      SessionNotification("Pi finished", "Session $shortId completed its task.")
    } else null
    "waiting_for_input" -> SessionNotification(
      "Pi needs input",
      "Session $shortId is waiting for your response.",
    )
    "failed", "error", "stopped" -> SessionNotification(
      "Pi session stopped",
      "Session $shortId $current.",
    )
    else -> null
  }
}

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
        lockscreenVisibility = NotificationCompat.VISIBILITY_PRIVATE
      },
    )
  }

  fun notifyRuntimeTransition(
    context: Context,
    sessionId: String,
    state: String?,
    appInForeground: Boolean,
  ) {
    if (state.isNullOrBlank()) return
    val previous = lastStates.put(sessionId, state)
    if (appInForeground) return
    val notification = runtimeNotificationForTransition(sessionId, previous, state) ?: return
    post(context, sessionId, notification)
  }

  private fun post(context: Context, sessionId: String, notification: SessionNotification) {
    if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU &&
      context.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
    ) return

    val openApp = PendingIntent.getActivity(
      context,
      sessionId.hashCode(),
      Intent(context, MainActivity::class.java).apply {
        flags = Intent.FLAG_ACTIVITY_CLEAR_TOP or Intent.FLAG_ACTIVITY_SINGLE_TOP
      },
      PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
    )
    val built = NotificationCompat.Builder(context, channelId)
      .setSmallIcon(R.drawable.pi_stack_logo)
      .setContentTitle(notification.title)
      .setContentText(notification.body)
      .setStyle(NotificationCompat.BigTextStyle().bigText(notification.body))
      .setPriority(NotificationCompat.PRIORITY_DEFAULT)
      .setVisibility(NotificationCompat.VISIBILITY_PRIVATE)
      .setContentIntent(openApp)
      .setAutoCancel(true)
      .setOnlyAlertOnce(true)
      .build()
    NotificationManagerCompat.from(context).notify(sessionId.hashCode(), built)
  }
}
