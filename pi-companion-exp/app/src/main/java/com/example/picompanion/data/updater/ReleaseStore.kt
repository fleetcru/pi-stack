package com.example.picompanion.data.updater

import com.example.picompanion.data.api.apiJson
import kotlinx.serialization.json.Json
import okhttp3.OkHttpClient
import okhttp3.Request
import java.time.Instant
import java.util.concurrent.TimeUnit

/** Queries public GitHub Releases for validated Companion APK assets. */
class ReleaseStore(
  private val repo: String = RELEASE_REPO,
  private val okHttpClient: OkHttpClient = defaultHttpClient(),
  private val json: Json = apiJson,
) {
  /** Returns stable and rolling development channels, or null if GitHub is unavailable. */
  fun companionReleaseCatalog(): CompanionReleaseCatalog? {
    val releases = fetchReleases() ?: return null
    return CompanionReleaseCatalog(
      stable = selectNewestCompanionRelease(releases),
      development = fetchRelease(DEVELOPMENT_TAG)?.let(::selectDevelopmentRelease),
    )
  }

  /** Returns the highest valid Companion SemVer, or null on any fetch error. */
  fun newestCompanionRelease(includePrereleases: Boolean = false): CompanionRelease? {
    val releases = fetchReleases() ?: return null
    return selectNewestCompanionRelease(releases, includePrereleases)
  }

  /** Pure selection. Stable users do not receive prereleases without opt-in. */
  internal fun selectNewestCompanionRelease(
    releases: List<GitHubRelease>,
    includePrereleases: Boolean = false,
  ): CompanionRelease? = releases
    .asSequence()
    .filter { includePrereleases || !it.prerelease }
    .mapNotNull { release ->
      val apk = release.assets.singleOrNull { asset ->
        CompanionRelease.versionFromApkName(asset.name) != null
      } ?: return@mapNotNull null
      val version = CompanionRelease.versionFromApkName(apk.name) ?: return@mapNotNull null
      if (release.tagName != "v$version" || apk.size <= 0) return@mapNotNull null
      CompanionRelease(
        tagName = release.tagName,
        publishedAtMillis = parseIso(release.publishedAt),
        prerelease = release.prerelease,
        apkUrl = apk.browserDownloadUrl,
        apkName = apk.name,
        apkSize = apk.size,
      )
    }
    .filter { it.semanticVersion != null }
    .maxWithOrNull(
      compareBy<CompanionRelease> { requireNotNull(it.semanticVersion) }
        .thenBy { it.publishedAtMillis },
    )

  /** Validates the one mutable prerelease used for signed main-branch builds. */
  internal fun selectDevelopmentRelease(release: GitHubRelease): CompanionRelease? {
    if (release.tagName != DEVELOPMENT_TAG || !release.prerelease) return null
    val apk = release.assets.singleOrNull { asset ->
      CompanionRelease.versionFromApkName(asset.name) != null
    } ?: return null
    if (apk.size <= 0) return null
    val result = CompanionRelease(
      tagName = release.tagName,
      publishedAtMillis = parseIso(release.publishedAt),
      prerelease = true,
      apkUrl = apk.browserDownloadUrl,
      apkName = apk.name,
      apkSize = apk.size,
    )
    return result.takeIf { it.semanticVersion != null }
  }

  private fun fetchReleases(): List<GitHubRelease>? = try {
    val request = Request.Builder()
      .url("https://api.github.com/repos/$repo/releases?per_page=20")
      .header("Accept", "application/vnd.github+json")
      .header("X-GitHub-Api-Version", "2022-11-28")
      .build()
    okHttpClient.newCall(request).execute().use { response ->
      if (!response.isSuccessful) return null
      json.decodeFromString<List<GitHubRelease>>(response.body.string())
    }
  } catch (_: Exception) {
    null
  }

  private fun fetchRelease(tag: String): GitHubRelease? = try {
    val request = Request.Builder()
      .url("https://api.github.com/repos/$repo/releases/tags/$tag")
      .header("Accept", "application/vnd.github+json")
      .header("X-GitHub-Api-Version", "2022-11-28")
      .build()
    okHttpClient.newCall(request).execute().use { response ->
      if (!response.isSuccessful) return null
      json.decodeFromString<GitHubRelease>(response.body.string())
    }
  } catch (_: Exception) {
    null
  }

  private fun parseIso(iso: String): Long = try {
    Instant.parse(iso).toEpochMilli()
  } catch (_: Exception) {
    0L
  }

  companion object {
    const val RELEASE_REPO = "fleetcru/pi-stack"
    const val DEVELOPMENT_TAG = "companion-dev"

    private fun defaultHttpClient() = OkHttpClient.Builder()
      .connectTimeout(15, TimeUnit.SECONDS)
      .readTimeout(30, TimeUnit.SECONDS)
      .callTimeout(45, TimeUnit.SECONDS)
      .build()
  }
}
