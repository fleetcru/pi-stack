package com.example.picompanion.data.updater

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.util.Log
import android.widget.Toast

/**
 * Receives the status broadcast emitted by PackageInstaller after a silent
 * self-update commit. Reports failures so a signature mismatch or a declined
 * permission does not fail silently.
 */
class InstallResultReceiver : BroadcastReceiver() {
  override fun onReceive(context: Context, intent: Intent) {
    val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)
    val message = intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE)
    Log.i("AppUpdater", "Install result: status=$status message=$message")
    when (status) {
      PackageInstaller.STATUS_SUCCESS -> Unit
      PackageInstaller.STATUS_PENDING_USER_ACTION -> {
        val confirmation = if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.TIRAMISU) {
          intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java)
        } else {
          @Suppress("DEPRECATION")
          intent.getParcelableExtra(Intent.EXTRA_INTENT)
        }
        if (confirmation != null) {
          confirmation.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
          runCatching { context.startActivity(confirmation) }
            .onFailure {
              Log.e("AppUpdater", "Could not open install confirmation", it)
              Toast.makeText(
                context,
                "Open Pi Companion and retry the update to confirm installation",
                Toast.LENGTH_LONG,
              ).show()
            }
        } else {
          Toast.makeText(context, "Android did not provide an install confirmation", Toast.LENGTH_LONG).show()
        }
      }
      else -> Toast.makeText(
        context,
        "Update could not be installed: $message",
        Toast.LENGTH_LONG,
      ).show()
    }
  }
}
