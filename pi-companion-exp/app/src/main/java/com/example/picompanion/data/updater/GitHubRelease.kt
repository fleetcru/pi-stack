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

data class CompanionReleaseCatalog(
  val stable: CompanionRelease?,
  val development: CompanionRelease?,
)

/** A validated Companion APK release. */
data class CompanionRelease(
  val tagName: String,
  val publishedAtMillis: Long,
  val prerelease: Boolean,
  val apkUrl: String,
  val apkName: String,
  val apkSize: Long,
) {
  /** Version name embedded in the asset filename (pi-companion-<version>.apk). */
  val versionName: String
    get() = apkName
      .removePrefix(APK_PREFIX)
      .removeSuffix(APK_SUFFIX)

  internal val semanticVersion: SemanticVersion?
    get() = SemanticVersion.parse(versionName)

  /** True only when both values are valid SemVer and this release is newer. */
  fun isNewerThan(current: String): Boolean {
    val available = semanticVersion ?: return false
    val installed = SemanticVersion.parse(current) ?: return false
    return available > installed
  }

  companion object {
    const val APK_PREFIX = "pi-companion-"
    const val APK_SUFFIX = ".apk"

    private val apkNamePattern = Regex(
      "^pi-companion-((?:0|[1-9]\\d*)\\.(?:0|[1-9]\\d*)\\.(?:0|[1-9]\\d*)(?:-[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?)\\.apk$",
    )

    fun versionFromApkName(name: String): String? =
      apkNamePattern.matchEntire(name)?.groupValues?.get(1)
  }
}

/** Strict SemVer precedence without build metadata, which APK filenames omit. */
internal data class SemanticVersion(
  val major: Int,
  val minor: Int,
  val patch: Int,
  val prerelease: List<String>,
) : Comparable<SemanticVersion> {
  override fun compareTo(other: SemanticVersion): Int {
    major.compareTo(other.major).takeIf { it != 0 }?.let { return it }
    minor.compareTo(other.minor).takeIf { it != 0 }?.let { return it }
    patch.compareTo(other.patch).takeIf { it != 0 }?.let { return it }
    if (prerelease.isEmpty() && other.prerelease.isNotEmpty()) return 1
    if (prerelease.isNotEmpty() && other.prerelease.isEmpty()) return -1
    for (index in 0 until maxOf(prerelease.size, other.prerelease.size)) {
      val left = prerelease.getOrNull(index) ?: return -1
      val right = other.prerelease.getOrNull(index) ?: return 1
      val leftNumber = left.toIntOrNull()
      val rightNumber = right.toIntOrNull()
      val compared = when {
        leftNumber != null && rightNumber != null -> leftNumber.compareTo(rightNumber)
        leftNumber != null -> -1
        rightNumber != null -> 1
        else -> left.compareTo(right)
      }
      if (compared != 0) return compared
    }
    return 0
  }

  companion object {
    private val pattern = Regex(
      "^v?(0|[1-9]\\d*)\\.(0|[1-9]\\d*)\\.(0|[1-9]\\d*)(?:-([0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*))?(?:\\+[0-9A-Za-z-]+(?:\\.[0-9A-Za-z-]+)*)?$",
    )

    fun parse(value: String): SemanticVersion? {
      val match = pattern.matchEntire(value.trim()) ?: return null
      val prerelease = match.groupValues[4]
        .takeIf(String::isNotEmpty)
        ?.split('.')
        ?: emptyList()
      if (prerelease.any { it.length > 1 && it.all(Char::isDigit) && it.startsWith('0') }) {
        return null
      }
      return SemanticVersion(
        major = match.groupValues[1].toIntOrNull() ?: return null,
        minor = match.groupValues[2].toIntOrNull() ?: return null,
        patch = match.groupValues[3].toIntOrNull() ?: return null,
        prerelease = prerelease,
      )
    }
  }
}
