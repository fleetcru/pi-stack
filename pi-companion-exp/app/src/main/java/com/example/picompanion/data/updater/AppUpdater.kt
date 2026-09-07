package com.example.picompanion.data.updater

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInfo
import android.content.pm.PackageInstaller
import android.content.pm.PackageManager
import android.content.pm.Signature
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.provider.Settings
import android.util.Log
import androidx.core.content.pm.PackageInfoCompat
import androidx.core.net.toUri
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import okhttp3.HttpUrl
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.io.FileOutputStream
import java.security.MessageDigest
import java.util.concurrent.TimeUnit

/** Downloads and installs signed Companion APKs published on GitHub Releases. */
class AppUpdater(
  private val context: Context,
  private val releaseStore: ReleaseStore = ReleaseStore(),
  private val okHttpClient: OkHttpClient = defaultHttpClient(),
) {
  data class DownloadedApk(
    val versionCode: Long,
    val packageName: String,
  )

  fun currentVersionCode(): Long = PackageInfoCompat.getLongVersionCode(
    context.packageManager.getPackageInfo(context.packageName, 0),
  )

  fun currentVersionName(): String = context.packageManager
    .getPackageInfo(context.packageName, 0).versionName ?: ""

  fun canRequestInstall(): Boolean = context.packageManager.canRequestPackageInstalls()

  fun installPermissionIntent(): Intent = Intent(
    Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES,
    "package:${context.packageName}".toUri(),
  )

  /** Stable releases only. Prerelease updates require an explicit future opt-in. */
  suspend fun newestRelease(): CompanionRelease? = withContext(Dispatchers.IO) {
    releaseStore.newestCompanionRelease(includePrereleases = false)
  }

  /** Downloads to a temporary private file and atomically promotes it on success. */
  suspend fun download(
    release: CompanionRelease,
    onProgress: (downloadedBytes: Long, totalBytes: Long) -> Unit = { _, _ -> },
  ): File = withContext(Dispatchers.IO) {
    require(CompanionRelease.versionFromApkName(release.apkName) != null) {
      "Release has an invalid APK filename"
    }
    require(release.apkSize in 1..MAX_APK_BYTES) {
      "Release APK size is invalid"
    }

    val request = Request.Builder().url(release.apkUrl).build()
    validateDownloadUrl(request.url)
    val finalFile = File(context.cacheDir, release.apkName)
    val partFile = File(context.cacheDir, "${release.apkName}.part")
    cleanupOldDownloads(finalFile, partFile)

    try {
      okHttpClient.newCall(request).execute().use { response ->
        if (!response.isSuccessful) {
          throw IllegalStateException("Download failed: HTTP ${response.code}")
        }
        validateDownloadUrl(response.request.url)
        val body = response.body
        val responseLength = body.contentLength()
        if (responseLength > MAX_APK_BYTES) throw IllegalStateException("Update APK is too large")
        if (responseLength >= 0 && responseLength != release.apkSize) {
          throw IllegalStateException("Update APK size does not match the GitHub release")
        }

        var copied = 0L
        var lastPercent = -1
        FileOutputStream(partFile).use { output ->
          body.byteStream().use { input ->
            val buffer = ByteArray(DEFAULT_BUFFER_SIZE)
            while (true) {
              val count = input.read(buffer)
              if (count < 0) break
              copied += count
              if (copied > release.apkSize || copied > MAX_APK_BYTES) {
                throw IllegalStateException("Update APK exceeded its advertised size")
              }
              output.write(buffer, 0, count)
              val percent = ((copied * 100) / release.apkSize).toInt()
              if (percent != lastPercent) {
                lastPercent = percent
                onProgress(copied, release.apkSize)
              }
            }
          }
          output.fd.sync()
        }
        if (copied != release.apkSize) {
          throw IllegalStateException("Update APK download was incomplete")
        }
      }
      if (finalFile.exists() && !finalFile.delete()) {
        throw IllegalStateException("Could not replace the cached update")
      }
      if (!partFile.renameTo(finalFile)) {
        throw IllegalStateException("Could not finalize the downloaded update")
      }
      finalFile
    } catch (error: Exception) {
      partFile.delete()
      throw error
    }
  }

  /**
   * Verifies package identity, signing certificate, and version before the APK
   * is copied into PackageInstaller. Android verifies these again at commit.
   */
  fun validateDownloadedApk(apkFile: File): DownloadedApk {
    val packageManager = context.packageManager
    val flags = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
      PackageManager.GET_SIGNING_CERTIFICATES
    } else {
      @Suppress("DEPRECATION")
      PackageManager.GET_SIGNATURES
    }
    val downloaded = packageManager.getPackageArchiveInfo(apkFile.absolutePath, flags)
      ?: throw IllegalStateException("Downloaded file is not a readable APK")
    if (downloaded.packageName != context.packageName) {
      throw IllegalStateException("Downloaded APK belongs to a different app")
    }

    val installed = packageManager.getPackageInfo(context.packageName, flags)
    val installedSigners = signerDigests(installed)
    val downloadedSigners = signerDigests(downloaded)
    if (installedSigners.isEmpty() || downloadedSigners.isEmpty() || installedSigners.intersect(downloadedSigners).isEmpty()) {
      throw IllegalStateException("Downloaded APK signing certificate does not match this app")
    }

    val versionCode = PackageInfoCompat.getLongVersionCode(downloaded)
    if (versionCode <= currentVersionCode()) {
      throw IllegalStateException("Downloaded APK is not newer than the installed app")
    }
    return DownloadedApk(versionCode = versionCode, packageName = downloaded.packageName)
  }

  /** Stages an APK in PackageInstaller and reports the final session result. */
  suspend fun install(
    apkFile: File,
    onFinished: (success: Boolean, message: String?) -> Unit,
  ) {
    withContext(Dispatchers.IO) {
      val installer = context.packageManager.packageInstaller
      val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL).apply {
        setAppPackageName(context.packageName)
      }
      val sessionId = installer.createSession(params)
      val mainHandler = Handler(Looper.getMainLooper())
      val callback = object : PackageInstaller.SessionCallback() {
        override fun onCreated(sessionId: Int) {}
        override fun onBadgingChanged(sessionId: Int) {}
        override fun onActiveChanged(sessionId: Int, active: Boolean) {}
        override fun onProgressChanged(sessionId: Int, progress: Float) {}
        override fun onFinished(sessionId: Int, success: Boolean) {
          Log.i("AppUpdater", "Install finished success=$success session=$sessionId")
          runCatching { installer.unregisterSessionCallback(this) }
          apkFile.delete()
          mainHandler.post {
            onFinished(success, if (success) null else "Android rejected the update")
          }
        }
      }
      installer.registerSessionCallback(callback)
      try {
        installer.openSession(sessionId).use { session ->
          session.openWrite("pi-companion", 0, apkFile.length()).use { output ->
            apkFile.inputStream().use { input -> input.copyTo(output) }
            session.fsync(output)
          }
          val statusPending = PendingIntent.getBroadcast(
            context,
            sessionId,
            Intent(context, InstallResultReceiver::class.java),
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
          )
          session.commit(statusPending.intentSender)
        }
      } catch (error: Exception) {
        apkFile.delete()
        runCatching { installer.abandonSession(sessionId) }
        runCatching { installer.unregisterSessionCallback(callback) }
        mainHandler.post { onFinished(false, error.message ?: "Install failed") }
      }
    }
  }

  private fun cleanupOldDownloads(vararg keep: File) {
    val keepPaths = keep.mapTo(mutableSetOf()) { it.absolutePath }
    context.cacheDir.listFiles()?.forEach { file ->
      if (
        file.absolutePath !in keepPaths &&
        file.name.startsWith(CompanionRelease.APK_PREFIX) &&
        (file.name.endsWith(CompanionRelease.APK_SUFFIX) || file.name.endsWith(".apk.part"))
      ) {
        file.delete()
      }
    }
    keep.forEach { it.delete() }
  }

  private fun validateDownloadUrl(url: HttpUrl) {
    if (url.scheme != "https" || url.host !in ALLOWED_DOWNLOAD_HOSTS) {
      throw IllegalStateException("Update download URL is not an approved GitHub host")
    }
  }

  @Suppress("DEPRECATION")
  private fun signerDigests(info: PackageInfo): Set<String> {
    val signatures: Array<Signature> = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
      val signingInfo = info.signingInfo ?: return emptySet()
      if (signingInfo.hasMultipleSigners()) {
        signingInfo.apkContentsSigners
      } else {
        signingInfo.signingCertificateHistory
      }
    } else {
      info.signatures ?: emptyArray()
    }
    return signatures.mapTo(mutableSetOf()) { signature ->
      MessageDigest.getInstance("SHA-256")
        .digest(signature.toByteArray())
        .joinToString("") { byte -> "%02x".format(byte) }
    }
  }

  companion object {
    private const val MAX_APK_BYTES = 200L * 1024L * 1024L
    private val ALLOWED_DOWNLOAD_HOSTS = setOf(
      "github.com",
      "objects.githubusercontent.com",
      "release-assets.githubusercontent.com",
    )

    private fun defaultHttpClient() = OkHttpClient.Builder()
      .connectTimeout(15, TimeUnit.SECONDS)
      .readTimeout(60, TimeUnit.SECONDS)
      .callTimeout(5, TimeUnit.MINUTES)
      .build()
  }
}
