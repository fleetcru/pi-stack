package com.example.picompanion.data.updater

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller

/**
 * Receives the status broadcast emitted by PackageInstaller after a silent
 * self-update commit. It does not need to display anything; the platform
 * installer already surfaced its own confirmation. On a successful install the
 * app process is killed and the launcher can reopen the updated build.
 */
class InstallResultReceiver : BroadcastReceiver() {
  override fun onReceive(context: Context, intent: Intent) {
    val status = intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)
    // Nothing to do here beyond acknowledging the result; a relaunch intent is
    // not needed because Android terminates the app on successful upgrade.
  }
}
