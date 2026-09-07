package com.example.picompanion.data.updater

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.net.Uri
import android.os.Build
import android.provider.Settings
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.io.FileOutputStream

/**
 * Downloads the newest companion APK from GitHub Releases and installs it
 * through Android's [PackageInstaller]. Installation requires the app be
 * granted REQUEST_INSTALL_PACKAGES ("install unknown apps"); the caller should
 * route the user to the system settings intent when that permission is absent.
 */
class AppUpdater(
  private val context: Context,
  private val releaseStore: ReleaseStore = ReleaseStore(),
  private val okHttpClient: OkHttpClient = OkHttpClient(),
) {
  fun currentVersionCode(): Long = context.packageManager
    .getPackageInfo(context.packageName, 0).longVersionCode

  fun currentVersionName(): String = context.packageManager
    .getPackageInfo(context.packageName, 0).versionName ?: ""

  fun canRequestInstall(): Boolean = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
    context.packageManager.canRequestPackageInstalls()
  } else {
    true
  }

  fun installPermissionIntent(): Intent = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
    Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:${context.packageName}"))
  } else {
    Intent(Settings.ACTION_SECURITY_SETTINGS)
  }

  /** The newest companion release by publish time, or null on any error. */
  suspend fun newestRelease(): CompanionRelease? = withContext(Dispatchers.IO) {
    releaseStore.newestCompanionRelease()
  }

  /** Downloads the release APK to a private cache file. */
  suspend fun download(release: CompanionRelease): File = withContext(Dispatchers.IO) {
    val file = File(context.cacheDir, release.apkName)
    val request = Request.Builder().url(release.apkUrl).build()
    okHttpClient.newCall(request).execute().use { response ->
      if (!response.isSuccessful) throw IllegalStateException("Download failed: HTTP ${response.code}")
      val body = response.body
      FileOutputStream(file).use { out -> body.byteStream().copyTo(out) }
    }
    file
  }

  /**
   * Reads the downloaded APK's versionCode. Returns null if it cannot be read,
   * which the caller should treat as "not an update" to avoid a downgrade.
   */
  fun downloadedVersionCode(apkFile: File): Long? {
    val info = context.packageManager.getPackageArchiveInfo(apkFile.absolutePath, 0)
    return info?.longVersionCode
  }

  /**
   * Stages an APK for a full install and commits it. The system shows its own
   * confirmation UI. The commit status is delivered to a broadcast receiver,
   * whose PendingIntent must be mutable so the framework can attach
   * PackageInstaller.EXTRA_STATUS.
   */
  fun install(apkFile: File) {
    val installer = context.packageManager.packageInstaller
    val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
    val sessionId = installer.createSession(params)
    installer.openSession(sessionId).use { session ->
      session.openWrite("pi-companion", 0, apkFile.length()).use { out ->
        apkFile.inputStream().use { input -> input.copyTo(out) }
        session.fsync(out)
      }
      val statusReceiver = PendingIntent.getBroadcast(
        context,
        0,
        Intent(context, InstallResultReceiver::class.java),
        PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
      )
      session.commit(statusReceiver.intentSender)
    }
  }
}

