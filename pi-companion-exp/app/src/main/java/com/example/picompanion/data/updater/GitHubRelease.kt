package com.example.picompanion.data.updater

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

@Serializable
data class GitHubRelease(
  @SerialName("tag_name") val tagName: String,
  @SerialName("prerelease") val prerelease: Boolean,
  @SerialName("published_at") val publishedAt: String,
  @SerialName("assets") val assets: List<GitHubAsset> = emptyList(),
)

@Serializable
data class GitHubAsset(
  @SerialName("name") val name: String,
  @SerialName("browser_download_url") val browserDownloadUrl: String,
  @SerialName("size") val size: Long,
)

/** A companion APK release, sorted by publish time descending. */
data class CompanionRelease(
  val tagName: String,
  val publishedAtMillis: Long,
  val prerelease: Boolean,
  val apkUrl: String,
  val apkName: String,
) {
  /** Version name embedded in the asset filename (pi-companion-<version>.apk). */
  val versionName: String
    get() = apkName
      .removePrefix("pi-companion-")
      .removeSuffix(".apk")

  /** True when this release's version is judged newer than the given one. */
  fun isNewerThan(current: String): Boolean =
    compareVersions(versionName, current) > 0

  private fun compareVersions(a: String, b: String): Int {
    val left = parseParts(a)
    val right = parseParts(b)
    for (i in 0 until maxOf(left.size, right.size)) {
      val l = left.getOrElse(i) { 0 }
      val r = right.getOrElse(i) { 0 }
      if (l != r) return l.compareTo(r)
    }
    return 0
  }

  private fun parseParts(value: String): List<Int> =
    value.split('.', '-', '+', '_')
      .mapNotNull { part -> part.filter(Char::isDigit).toIntOrNull() }
      .ifEmpty { listOf(0) }
}
