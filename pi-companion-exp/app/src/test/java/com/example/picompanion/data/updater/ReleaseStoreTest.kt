package com.example.picompanion.data.updater

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ReleaseStoreTest {

  private val store = ReleaseStore()

  private fun release(
    tag: String,
    publishedAt: String,
    prerelease: Boolean = false,
    assetNames: List<String> = listOf("pi-companion-${tag.removePrefix("v")}.apk"),
    assetSize: Long = 100,
  ): GitHubRelease = GitHubRelease(
    tagName = tag,
    prerelease = prerelease,
    publishedAt = publishedAt,
    assets = assetNames.map { name ->
      GitHubAsset(
        name = name,
        browserDownloadUrl = "https://github.com/fleetcru/pi-stack/releases/download/$tag/$name",
        size = assetSize,
      )
    },
  )

  @Test
  fun picksHighestStableSemVerInsteadOfLatestPublishTime() {
    val releases = listOf(
      release("v0.2.0", "2026-08-26T08:42:02Z"),
      release("v0.1.9", "2026-09-07T16:20:23Z"),
    )
    assertEquals("v0.2.0", store.selectNewestCompanionRelease(releases)?.tagName)
  }

  @Test
  fun ignoresPrereleasesByDefault() {
    val releases = listOf(
      release("v0.1.0", "2026-09-07T15:00:00Z"),
      release("v0.2.0-beta.1", "2026-09-07T16:00:00Z", prerelease = true),
    )
    assertEquals("v0.1.0", store.selectNewestCompanionRelease(releases)?.tagName)
    assertEquals(
      "v0.2.0-beta.1",
      store.selectNewestCompanionRelease(releases, includePrereleases = true)?.tagName,
    )
  }

  @Test
  fun ignoresMalformedAmbiguousAndMismatchedAssets() {
    val releases = listOf(
      release("server-v1.0.0", "2026-09-07T15:00:00Z", assetNames = listOf("pi-server.exe")),
      release("v0.2.0", "2026-09-07T16:00:00Z", assetNames = listOf("pi-companion-latest.apk")),
      release("v0.3.0", "2026-09-07T17:00:00Z", assetNames = listOf("pi-companion-0.2.0.apk")),
      release(
        "v0.4.0",
        "2026-09-07T18:00:00Z",
        assetNames = listOf("pi-companion-0.4.0.apk", "pi-companion-0.4.0-beta.1.apk"),
      ),
      release("v0.1.0", "2026-09-07T14:00:00Z"),
    )
    assertEquals("v0.1.0", store.selectNewestCompanionRelease(releases)?.tagName)
  }

  @Test
  fun ignoresEmptyAssetsAndInvalidSize() {
    assertNull(store.selectNewestCompanionRelease(listOf(release("v0.1.0", "2026-09-07T15:00:00Z", assetNames = emptyList()))))
    assertNull(store.selectNewestCompanionRelease(listOf(release("v0.1.0", "2026-09-07T15:00:00Z", assetSize = 0))))
  }

  @Test
  fun semVerComparisonHandlesStableAndPrereleasePrecedence() {
    val stable = companionRelease("1.0.0")
    val beta1 = companionRelease("1.0.0-beta.1", prerelease = true)
    val beta2 = companionRelease("1.0.0-beta.2", prerelease = true)

    assertTrue(stable.isNewerThan("0.9.9"))
    assertTrue(stable.isNewerThan("1.0.0-beta.2"))
    assertTrue(beta2.isNewerThan("1.0.0-beta.1"))
    assertFalse(beta1.isNewerThan("1.0.0"))
    assertFalse(stable.isNewerThan("1.0.0"))
    assertFalse(stable.isNewerThan("not-a-version"))
  }

  @Test
  fun numericPrereleaseIdentifiersSortBeforeTextIdentifiers() {
    val numeric = SemanticVersion.parse("1.0.0-1")
    val text = SemanticVersion.parse("1.0.0-alpha")
    assertTrue(requireNotNull(numeric) < requireNotNull(text))
  }

  @Test
  fun rejectsInvalidSemanticVersions() {
    assertNull(SemanticVersion.parse("1.0"))
    assertNull(SemanticVersion.parse("01.0.0"))
    assertNull(SemanticVersion.parse("1.0.0-beta.01"))
    assertNull(SemanticVersion.parse("latest"))
  }

  @Test
  fun extractsVersionOnlyFromStrictApkNames() {
    assertEquals("0.1.0", CompanionRelease.versionFromApkName("pi-companion-0.1.0.apk"))
    assertEquals("0.2.0-beta.1", CompanionRelease.versionFromApkName("pi-companion-0.2.0-beta.1.apk"))
    assertNull(CompanionRelease.versionFromApkName("../pi-companion-0.1.0.apk"))
    assertNull(CompanionRelease.versionFromApkName("pi-companion-latest.apk"))
  }

  private fun companionRelease(version: String, prerelease: Boolean = false) = CompanionRelease(
    tagName = "v$version",
    publishedAtMillis = 0,
    prerelease = prerelease,
    apkUrl = "https://github.com/example.apk",
    apkName = "pi-companion-$version.apk",
    apkSize = 100,
  )
}
