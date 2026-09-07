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
)
