package com.example.picompanion.data.updater

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ReleaseStoreTest {

  private val store = ReleaseStore()

  private fun release(
    tag: String,
    publishedAt: String,
    prerelease: Boolean = false,
    assetNames: List<String> = listOf("pi-companion-1.0.0.apk"),
  ): GitHubRelease = GitHubRelease(
    tagName = tag,
    prerelease = prerelease,
    publishedAt = publishedAt,
    assets = assetNames.map { name ->
      GitHubAsset(name = name, browserDownloadUrl = "https://example/$name", size = 100)
    },
  )

  @Test
  fun picksNewestByPublishTimeAcrossPrereleaseAndStable() {
    val releases = listOf(
      release("v1.4.8", "2026-08-26T08:42:02Z", prerelease = false),
      release("v1.5.0-46", "2026-09-07T16:20:23Z", prerelease = true),
      release("v1.5.0-45", "2026-09-07T15:07:42Z", prerelease = true),
    )
    val newest = store.selectNewestCompanionRelease(releases)
    assertEquals("v1.5.0-46", newest?.tagName)
    assertEquals(true, newest?.prerelease)
  }

  @Test
  fun ignoresReleasesWithoutCompanionApkAsset() {
    val releases = listOf(
      release("server-v1.5.0-125", "2026-09-07T15:05:27Z", prerelease = true, assetNames = listOf("pi-server-windows-amd64.exe")),
      release("v1.4.8", "2026-08-26T08:42:02Z", prerelease = false),
    )
    val newest = store.selectNewestCompanionRelease(releases)
    assertEquals("v1.4.8", newest?.tagName)
  }

  @Test
  fun returnsNullWhenNoCompanionReleases() {
    val releases = listOf(
      release("server-v1.5.0-125", "2026-09-07T15:05:27Z", assetNames = listOf("pi-server-windows-amd64.exe")),
    )
    assertNull(store.selectNewestCompanionRelease(releases))
  }
}
