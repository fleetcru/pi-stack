package com.example.picompanion.data.updater

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build
import android.util.Log
import android.widget.Toast
import kotlinx.coroutines.flow.MutableSharedFlow
import kotlinx.coroutines.flow.asSharedFlow

internal sealed interface InstallResultEvent {
  data object Success : InstallResultEvent
  data class Failure(val message: String) : InstallResultEvent
}

/** Process-local delivery from the durable PackageInstaller broadcast to UI. */
internal object InstallResultEvents {
  private val mutableEvents = MutableSharedFlow<InstallResultEvent>(extraBufferCapacity = 1)
  val events = mutableEvents.asSharedFlow()

  fun report(event: InstallResultEvent) {
    mutableEvents.tryEmit(event)
  }
}

/** Receives authoritative PackageInstaller status on every supported API. */
class InstallResultReceiver : BroadcastReceiver() {
  override fun onReceive(context: Context, intent: Intent) {
    val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)
    val statusMessage = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)
    val sessionId = intent.getIntExtra(PackageInstaller.EXTRA_SESSION_ID, -1)
    Log.i("AppUpdater", "Install result: status=$status session=$sessionId message=$statusMessage")

    when (status) {
      PackageInstaller.STATUS_SUCCESS -> InstallResultEvents.report(InstallResultEvent.Success)
      PackageInstaller.STATUS_PENDING_USER_ACTION -> {
        val confirmation = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
          intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java)
        } else {
          @Suppress("DEPRECATION")
          intent.getParcelableExtra(Intent.EXTRA_INTENT)
        }
        if (confirmation == null) {
          failPendingInstall(context, sessionId, "Android did not provide an install confirmation")
          return
        }
        confirmation.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        runCatching { context.startActivity(confirmation) }
          .onFailure {
            Log.e("AppUpdater", "Could not open install confirmation", it)
            failPendingInstall(
              context,
              sessionId,
              "Open Pi Companion and retry the update to confirm installation",
            )
          }
      }
      else -> {
        val message = statusMessage?.takeIf(String::isNotBlank) ?: "Android rejected the update"
        InstallResultEvents.report(InstallResultEvent.Failure(message))
        Toast.makeText(context, "Update could not be installed: $message", Toast.LENGTH_LONG).show()
      }
    }
  }

  private fun failPendingInstall(context: Context, sessionId: Int, message: String) {
    if (sessionId >= 0) {
      runCatching { context.packageManager.packageInstaller.abandonSession(sessionId) }
    }
    InstallResultEvents.report(InstallResultEvent.Failure(message))
    Toast.makeText(context, message, Toast.LENGTH_LONG).show()
  }
}
