package com.example.picompanion.data.model

import com.example.picompanion.data.api.apiJson
import kotlinx.serialization.decodeFromString
import kotlinx.serialization.encodeToString
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Round-trip serialization tests for all data models.
 * Ensures the API JSON config (lenient, ignoreUnknownKeys) works correctly
 * and that model defaults behave as expected.
 */
class ModelSerializationTest {

    private val json = apiJson

    // ── HealthResponse ──────────────────────────────────

    @Test
    fun healthResponseMinimalJson() {
        val parsed = json.decodeFromString<HealthResponse>("""{"ok":true}""")
        assertTrue(parsed.ok)
        assertTrue(parsed.sessions.isEmpty())
        org.junit.Assert.assertNull(parsed.capacity)
    }

    @Test
    fun healthResponseFullJson() {
        val body = """{"ok":true,"sessions":["s1","s2"],"capacity":{"activeSessions":2,"maxSessions":8}}"""
        val parsed = json.decodeFromString<HealthResponse>(body)
        assertEquals(2, parsed.sessions.size)
        assertEquals(2, parsed.capacity?.activeSessions)
        assertEquals(8, parsed.capacity?.maxSessions)
    }

    @Test
    fun healthResponseIgnoresUnknownFields() {
        val parsed = json.decodeFromString<HealthResponse>("""{"ok":true,"unknown":"value","extra":123}""")
        assertTrue(parsed.ok)
    }

    // ── ServerSession ───────────────────────────────────

    @Test
    fun serverSessionDefaults() {
        val session = ServerSession(id = "s1")
        assertEquals("s1", session.id)
        org.junit.Assert.assertNull(session.cwd)
        org.junit.Assert.assertNull(session.status)
        assertEquals(false, session.managed)
        assertTrue(session.args.isEmpty())
        assertTrue(session.labels.isEmpty())
    }

    @Test
    fun serverSessionRoundTrip() {
        val body = """{"id":"s1","cwd":"/tmp","status":"running","title":"My Task","project":"pi-stack"}"""
        val parsed = json.decodeFromString<ServerSession>(body)
        assertEquals("s1", parsed.id)
        assertEquals("/tmp", parsed.cwd)
        assertEquals("running", parsed.status)
        assertEquals("My Task", parsed.title)
        assertEquals("pi-stack", parsed.project)
    }

    @Test
    fun serverSessionWithState() {
        val body = """{"id":"s1","state":{"external":true}}"""
        val parsed = json.decodeFromString<ServerSession>(body)
        assertEquals("s1", parsed.id)
        assertTrue(parsed.state?.get("external")?.toString()?.contains("true") == true)
    }

    // ── CreateSessionRequest ────────────────────────────

    @Test
    fun createSessionRequestDefaults() {
        val req = CreateSessionRequest(cwd = "/home")
        assertEquals("/home", req.cwd)
        assertTrue(req.args.isEmpty())
        assertTrue(req.env.isEmpty())
        assertTrue(req.start)
        assertFalse(req.restart)
    }

    @Test
    fun createSessionRequestRoundTrip() {
        val req = CreateSessionRequest(
            cwd = "/tmp",
            args = listOf("--verbose"),
            start = false,
            restart = true,
            title = "Test",
            project = "proj",
        )
        val encoded = json.encodeToString(CreateSessionRequest.serializer(), req)
        val decoded = json.decodeFromString(CreateSessionRequest.serializer(), encoded)
        assertEquals(req, decoded)
    }

    // ── CreateSessionResponse ───────────────────────────

    @Test
    fun createSessionResponseParses() {
        val body = """{"id":"abc","cwd":"/tmp","args":["--flag"],"ws":"/ws/abc"}"""
        val parsed = json.decodeFromString<CreateSessionResponse>(body)
        assertEquals("abc", parsed.id)
        assertEquals("/tmp", parsed.cwd)
        assertEquals(listOf("--flag"), parsed.args)
        assertEquals("/ws/abc", parsed.ws)
    }

    @Test
    fun createSessionResponseNullWs() {
        val parsed = json.decodeFromString<CreateSessionResponse>("""{"id":"x"}""")
        assertEquals("x", parsed.id)
        org.junit.Assert.assertNull(parsed.ws)
    }

    // ── WebSocketTicketResponse ─────────────────────────

    @Test
    fun webSocketTicketResponseRoundTrip() {
        val resp = WebSocketTicketResponse(ticket = "t1", expiresAt = "2025-01-01", ws = "/ws/t1")
        val encoded = json.encodeToString(WebSocketTicketResponse.serializer(), resp)
        val decoded = json.decodeFromString(WebSocketTicketResponse.serializer(), encoded)
        assertEquals(resp, decoded)
    }

    // ── ServerWorker ────────────────────────────────────

    @Test
    fun serverWorkerDefaults() {
        val w = ServerWorker(id = "w1")
        assertEquals("w1", w.id)
        org.junit.Assert.assertNull(w.url)
        assertTrue(w.tags.isEmpty())
        assertEquals(0, w.activeSessions)
        assertEquals(0, w.maxSessions)
    }

    @Test
    fun serverWorkerRoundTrip() {
        val body = """{"id":"w1","url":"http://remote:3141","tags":["gpu"],"status":"online","activeSessions":2,"maxSessions":4}"""
        val parsed = json.decodeFromString<ServerWorker>(body)
        assertEquals("w1", parsed.id)
        assertEquals("http://remote:3141", parsed.url)
        assertEquals(listOf("gpu"), parsed.tags)
        assertEquals("online", parsed.status)
        assertEquals(2, parsed.activeSessions)
        assertEquals(4, parsed.maxSessions)
    }

    // ── WorkerWriteRequest ──────────────────────────────

    @Test
    fun workerWriteRequestRoundTrip() {
        val req = WorkerWriteRequest(id = "w1", url = "http://x:1", token = "secret", tags = listOf("gpu"))
        val encoded = json.encodeToString(WorkerWriteRequest.serializer(), req)
        val decoded = json.decodeFromString(WorkerWriteRequest.serializer(), encoded)
        assertEquals(req, decoded)
    }

    @Test
    fun workerWriteRequestNullToken() {
        val req = WorkerWriteRequest(id = "w1", url = "http://x:1")
        val encoded = json.encodeToString(WorkerWriteRequest.serializer(), req)
        val decoded = json.decodeFromString(WorkerWriteRequest.serializer(), encoded)
        org.junit.Assert.assertNull(decoded.token)
    }

    // ── DirectoryListResponse ───────────────────────────

    @Test
    fun directoryListResponseDefaults() {
        val resp = DirectoryListResponse()
        org.junit.Assert.assertNull(resp.path)
        assertTrue(resp.directories.isEmpty())
    }

    @Test
    fun directoryListResponseRoundTrip() {
        val body = """{"path":"/home","parent":"/","directories":[{"name":"docs","path":"/home/docs"}]}"""
        val parsed = json.decodeFromString<DirectoryListResponse>(body)
        assertEquals("/home", parsed.path)
        assertEquals("/", parsed.parent)
        assertEquals(1, parsed.directories.size)
        assertEquals("docs", parsed.directories[0].name)
    }

    // ── FileContentResponse ─────────────────────────────

    @Test
    fun fileContentResponseDefaults() {
        val resp = FileContentResponse(path = "/f.txt")
        assertEquals("/f.txt", resp.path)
        org.junit.Assert.assertNull(resp.content)
        assertFalse(resp.binary)
        assertFalse(resp.truncated)
    }

    // ── GlobalSession ───────────────────────────────────

    @Test
    fun globalSessionParses() {
        val body = """{"id":"g1","originId":"o1","workerId":"w1","session":{"id":"s1"},"reachable":true}"""
        val parsed = json.decodeFromString<GlobalSession>(body)
        assertEquals("g1", parsed.id)
        assertEquals("o1", parsed.originId)
        assertEquals("w1", parsed.workerId)
        assertEquals("s1", parsed.session.id)
        assertTrue(parsed.reachable)
    }

    // ── MachineSession ──────────────────────────────────

    @Test
    fun machineSessionParses() {
        val body = """{"id":"m1","path":"/sessions/m1","cwd":"/home","size":1024}"""
        val parsed = json.decodeFromString<MachineSession>(body)
        assertEquals("m1", parsed.id)
        assertEquals(1024L, parsed.size)
    }

    // ── ServerEntry serialization ───────────────────────

    @Test
    fun serverEntryRoundTrip() {
        val entry = com.example.picompanion.data.settings.ServerEntry(
            id = "abc",
            name = "My Server",
            url = "http://192.168.1.1:3141",
            authToken = "tok_123",
        )
        val encoded = json.encodeToString(
            com.example.picompanion.data.settings.ServerEntry.serializer(),
            entry,
        )
        val decoded = json.decodeFromString<com.example.picompanion.data.settings.ServerEntry>(encoded)
        assertEquals(entry.id, decoded.id)
        assertEquals(entry.name, decoded.name)
        assertEquals(entry.url, decoded.url)
        assertEquals(entry.authToken, decoded.authToken)
    }

    @Test
    fun serverEntryIgnoresUnknownFields() {
        val parsed = json.decodeFromString<com.example.picompanion.data.settings.ServerEntry>(
            """{"id":"x","url":"http://y","unknown":"field"}"""
        )
        assertEquals("x", parsed.id)
        assertEquals("http://y", parsed.url)
    }

    // ── AppSettings serialization ───────────────────────

    @Test
    fun appSettingsRoundTrip() {
        val settings = com.example.picompanion.data.settings.AppSettings(
            servers = listOf(
                com.example.picompanion.data.settings.ServerEntry(id = "s1", name = "Server 1", url = "http://localhost:3141"),
            ),
            activeServerId = "s1",
            reconnectOnLaunch = false,
            defaultProjectRoot = "/home/pi",
        )
        val encoded = json.encodeToString(
            com.example.picompanion.data.settings.AppSettings.serializer(),
            settings,
        )
        val decoded = json.decodeFromString<com.example.picompanion.data.settings.AppSettings>(encoded)
        assertEquals(1, decoded.servers.size)
        assertEquals("s1", decoded.activeServerId)
        assertFalse(decoded.reconnectOnLaunch)
        assertEquals("/home/pi", decoded.defaultProjectRoot)
    }
}
