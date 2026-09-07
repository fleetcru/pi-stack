package com.example.picompanion.data.updater

import com.example.picompanion.data.api.apiJson
import okhttp3.OkHttpClient
import okhttp3.Request
import kotlinx.serialization.json.Json
import java.time.Instant

/**
 * Queries the public GitHub Releases API for the pi-stack companion APK.
 * This is a separate, unauthenticated endpoint from the pi-server API and has
 * no bearer token. Only lookups are performed here; download/install live in
 * [AppUpdater].
 */
class ReleaseStore(
  private val repo: String = RELEASE_REPO,
  private val okHttpClient: OkHttpClient = OkHttpClient(),
  private val json: Json = apiJson,
) {
  /** Returns the newest companion release with an APK asset, or null. */
  fun newestCompanionRelease(): CompanionRelease? {
    val releases = fetchReleases() ?: return null
    return selectNewestCompanionRelease(releases)
  }

  /** Pure selection: newest companion APK by publish time across prerelease + stable. */
  internal fun selectNewestCompanionRelease(releases: List<GitHubRelease>): CompanionRelease? {
    return releases
      .asSequence()
      .flatMap { release ->
        val apk = release.assets.firstOrNull { asset ->
          asset.name.startsWith("pi-companion-") && asset.name.endsWith(".apk")
        } ?: return@flatMap emptySequence()
        sequenceOf(
          CompanionRelease(
            tagName = release.tagName,
            publishedAtMillis = parseIso(release.publishedAt),
            prerelease = release.prerelease,
            apkUrl = apk.browserDownloadUrl,
            apkName = apk.name,
          ),
        )
      }
      .maxByOrNull { it.publishedAtMillis }
  }

  private fun fetchReleases(): List<GitHubRelease>? {
    return try {
      val request = Request.Builder()
        .url("https://api.github.com/repos/$repo/releases?per_page=20")
        .header("Accept", "application/vnd.github+json")
        .build()
      okHttpClient.newCall(request).execute().use { response ->
        if (!response.isSuccessful) return null
        val body = response.body.string()
        json.decodeFromString<List<GitHubRelease>>(body)
      }
    } catch (_: Exception) {
      null
    }
  }

  private fun parseIso(iso: String): Long = try {
    Instant.parse(iso).toEpochMilli()
  } catch (_: Exception) {
    0L
  }

  companion object {
    const val RELEASE_REPO = "fleetcru/pi-stack"
  }
}
