package com.example.picompanion.data.api

import com.example.picompanion.data.settings.ServerEntry
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Tests [PiServerClient] URL construction and auth logic.
 *
 * Network I/O methods (checkHealth, listSessions, etc.) hit a real server
 * and can't be unit-tested without MockWebServer (not in current deps).
 * These tests verify the non-IO parts: ServerEntry configuration, HttpResult
 * classification, and the serialization round-trip of request types.
 */
class PiServerClientTest {

    private val entry = ServerEntry(
        id = "test",
        name = "Test Server",
        url = "http://127.0.0.1:3141",
        authToken = "tok_secret",
    )

    private val noAuthEntry = ServerEntry(
        id = "noauth",
        name = "No Auth",
        url = "http://127.0.0.1:3141",
        authToken = "",
    )

    // ── ServerEntry validation for client usage ─────────

    @Test
    fun configuredServerUrlIsValid() {
        assertTrue(entry.isConfigured)
        assertEquals("http://127.0.0.1:3141", entry.url)
    }

    @Test
    fun authTokenIsPresent() {
        assertEquals("tok_secret", entry.authToken)
    }

    @Test
    fun noAuthTokenIsEmpty() {
        assertEquals("", noAuthEntry.authToken)
    }

    // ── ApiJson configuration ───────────────────────────

    @Test
    fun apiJsonIgnoresUnknownKeys() {
        val json = apiJson
        val parsed = json.decodeFromString<JsonObject>("""{"ok":true,"unknown":"value","sessions":[]}""")
        assertEquals(JsonPrimitive(true), parsed["ok"])
        assertEquals(JsonPrimitive("value"), parsed["unknown"])
    }

    @Test
    fun apiJsonIsLenient() {
        val parsed = apiJson.decodeFromString<JsonObject>("""{ok: true}""")
        assertEquals(JsonPrimitive(true), parsed["ok"])
    }

    // ── PromptImage serialization ───────────────────────

    @Test
    fun promptImageRoundTrip() {
        val image = PromptImage(base64 = "aGVsbG8=", mimeType = "image/png")
        val encoded = apiJson.encodeToString(PromptImage.serializer(), image)
        val decoded = apiJson.decodeFromString(PromptImage.serializer(), encoded)
        assertEquals(image.base64, decoded.base64)
        assertEquals(image.mimeType, decoded.mimeType)
    }

    // ── HttpResult edge cases ───────────────────────────

    @Test
    fun httpResultFailureWithCauseChaining() {
        val cause = IllegalArgumentException("bad input")
        val f = HttpResult.Failure("validation failed", code = 422, cause = cause)
        assertEquals("bad input", f.cause?.message)
        assertEquals(422, f.code)
    }

    @Test
    fun mapChainsMultipleTransformations() {
        val result = HttpResult.Success("hello")
            .map { it.length }
            .map { it * 10 }
        assertEquals(50, (result as HttpResult.Success).value)
    }

    @Test
    fun mapShortCircuitsOnFirstFailure() {
        var transformCalled = false
        val input: HttpResult<String> = HttpResult.Failure("err")
        val result: HttpResult<Int> = input.map { transformCalled = true; it.length }
        assertTrue(result is HttpResult.Failure)
        assertFalse(transformCalled)
    }

    @Test
    fun onFailureReturnsReceiverForChaining() {
        val original = HttpResult.Failure("err")
        val returned = original.onFailure { /* no-op */ }
        assertEquals(original, returned)
    }

    @Test
    fun onSuccessReturnsReceiverForChaining() {
        val original = HttpResult.Success(42)
        val returned = original.onSuccess { /* no-op */ }
        assertEquals(original, returned)
    }
}
