package com.example.picompanion.data.model

import com.example.picompanion.data.settings.AppSettings
import com.example.picompanion.data.settings.ServerEntry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class AppSettingsTest {

    // ── ServerEntry ─────────────────────────────────────

    @Test
    fun serverIsConfiguredWhenUrlNotBlank() {
        val server = ServerEntry(id = "1", url = "http://localhost:3141")
        assertTrue(server.isConfigured)
    }

    @Test
    fun serverNotConfiguredWhenUrlBlank() {
        val server = ServerEntry(id = "1", url = "")
        assertFalse(server.isConfigured)
    }

    @Test
    fun serverNotConfiguredWhenUrlWhitespaceOnly() {
        val server = ServerEntry(id = "1", url = "   ")
        // isNotBlank() returns false for whitespace-only strings
        assertFalse(server.isConfigured)
    }

    @Test
    fun serverEntryDefaults() {
        val server = ServerEntry(id = "1")
        assertEquals("", server.name)
        assertEquals("", server.url)
        assertEquals("", server.authToken)
    }

    // ── AppSettings.activeServer ────────────────────────

    @Test
    fun activeServerReturnsMatchingId() {
        val s1 = ServerEntry(id = "a", name = "A")
        val s2 = ServerEntry(id = "b", name = "B")
        val settings = AppSettings(servers = listOf(s1, s2), activeServerId = "b")
        assertEquals("B", settings.activeServer?.name)
    }

    @Test
    fun activeServerFallsBackToFirstWhenIdNotFound() {
        val s1 = ServerEntry(id = "a", name = "A")
        val s2 = ServerEntry(id = "b", name = "B")
        val settings = AppSettings(servers = listOf(s1, s2), activeServerId = "z")
        assertEquals("A", settings.activeServer?.name)
    }

    @Test
    fun activeServerReturnsNullWhenNoServers() {
        val settings = AppSettings(servers = emptyList())
        assertNull(settings.activeServer)
    }

    // ── AppSettings computed properties ─────────────────

    @Test
    fun hasServersTrueWhenNonEmpty() {
        val settings = AppSettings(servers = listOf(ServerEntry(id = "1")))
        assertTrue(settings.hasServers)
    }

    @Test
    fun hasServersFalseWhenEmpty() {
        assertFalse(AppSettings().hasServers)
    }

    @Test
    fun hasConfiguredServerTrueWhenAtLeastOneUrlSet() {
        val settings = AppSettings(
            servers = listOf(
                ServerEntry(id = "1", url = ""),
                ServerEntry(id = "2", url = "http://x:1"),
            )
        )
        assertTrue(settings.hasConfiguredServer)
    }

    @Test
    fun hasConfiguredServerFalseWhenAllUrlsBlank() {
        val settings = AppSettings(
            servers = listOf(ServerEntry(id = "1", url = ""), ServerEntry(id = "2"))
        )
        assertFalse(settings.hasConfiguredServer)
    }

    // ── AppSettings defaults ────────────────────────────

    @Test
    fun defaultBooleansAreTrue() {
        val s = AppSettings()
        assertTrue(s.reconnectOnLaunch)
        assertTrue(s.rememberLastSession)
        assertTrue(s.replayEventsSinceLastSeen)
        assertTrue(s.showFileChangeEvents)
        assertTrue(s.showToolEvents)
        assertTrue(s.showDaemonEvents)
    }

    @Test
    fun defaultProjectRootIsEmpty() {
        assertEquals("", AppSettings().defaultProjectRoot)
    }
}
