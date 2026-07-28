package com.example.picompanion.data.api

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class HttpResultTest {

    // ── Success ─────────────────────────────────────────

    @Test
    fun successHoldsValue() {
        val result = HttpResult.Success(42)
        assertTrue(result is HttpResult.Success)
        assertEquals(42, (result as HttpResult.Success).value)
    }

    // ── Failure classification ──────────────────────────

    @Test
    fun networkErrorByDefaultCode() {
        val f = HttpResult.Failure("timeout")
        assertTrue(f.isNetworkError)
        assertFalse(f.isAuthError)
        assertFalse(f.isServerError)
        assertFalse(f.isNotFound)
    }

    @Test
    fun authError401() {
        val f = HttpResult.Failure("unauthorized", code = 401)
        assertTrue(f.isAuthError)
        assertFalse(f.isNetworkError)
    }

    @Test
    fun authError403() {
        val f = HttpResult.Failure("forbidden", code = 403)
        assertTrue(f.isAuthError)
    }

    @Test
    fun notFound404() {
        val f = HttpResult.Failure("missing", code = 404)
        assertTrue(f.isNotFound)
        assertFalse(f.isAuthError)
    }

    @Test
    fun serverError500() {
        val f = HttpResult.Failure("oops", code = 500)
        assertTrue(f.isServerError)
    }

    @Test
    fun serverError599() {
        val f = HttpResult.Failure("oops", code = 599)
        assertTrue(f.isServerError)
    }

    @Test
    fun clientError400IsNotServerError() {
        val f = HttpResult.Failure("bad request", code = 400)
        assertFalse(f.isServerError)
        assertFalse(f.isAuthError)
        assertFalse(f.isNotFound)
        assertFalse(f.isNetworkError)
    }

    // ── userMessage ─────────────────────────────────────

    @Test
    fun userMessageAuthError() {
        val f = HttpResult.Failure("raw", code = 401)
        assertEquals("Authentication failed — check your token", f.userMessage)
    }

    @Test
    fun userMessageNotFound() {
        val f = HttpResult.Failure("raw", code = 404)
        assertEquals("Resource not found", f.userMessage)
    }

    @Test
    fun userMessageServerError() {
        val f = HttpResult.Failure("raw", code = 502)
        assertEquals("Server error (502)", f.userMessage)
    }

    @Test
    fun userMessageNetworkErrorPassesThroughMessage() {
        val f = HttpResult.Failure("Connection refused", code = -1)
        assertEquals("Connection refused", f.userMessage)
    }

    @Test
    fun userMessageOtherCodePassesThroughMessage() {
        val f = HttpResult.Failure("some message", code = 422)
        assertEquals("some message", f.userMessage)
    }

    // ── map ─────────────────────────────────────────────

    @Test
    fun mapSuccessTransformsValue() {
        val result = HttpResult.Success("hello").map { it.length }
        assertTrue(result is HttpResult.Success)
        assertEquals(5, (result as HttpResult.Success).value)
    }

    @Test
    fun mapFailurePreservesError() {
        val original: HttpResult<String> = HttpResult.Failure("err", code = 500)
        val mapped: HttpResult<Int> = original.map { it.length }
        assertTrue(mapped is HttpResult.Failure)
        assertEquals("err", (mapped as HttpResult.Failure).message)
        assertEquals(500, mapped.code)
    }

    // ── getOrNull ───────────────────────────────────────

    @Test
    fun getOrNullReturnsValueForSuccess() {
        assertEquals("hello", HttpResult.Success("hello").getOrNull())
    }

    @Test
    fun getOrNullReturnsNullForFailure() {
        assertNull(HttpResult.Failure("err").getOrNull())
    }

    // ── onSuccess / onFailure ───────────────────────────

    @Test
    fun onSuccessInvokedForSuccess() {
        var captured: String? = null
        HttpResult.Success("hi").onSuccess { captured = it }
        assertEquals("hi", captured)
    }

    @Test
    fun onSuccessNotInvokedForFailure() {
        var invoked = false
        HttpResult.Failure("err").onSuccess { invoked = true }
        assertFalse(invoked)
    }

    @Test
    fun onFailureInvokedForFailure() {
        var captured: String? = null
        HttpResult.Failure("oops", code = 42).onFailure { captured = it.message }
        assertEquals("oops", captured)
    }

    @Test
    fun onFailureNotInvokedForSuccess() {
        var invoked = false
        HttpResult.Success(1).onFailure { invoked = true }
        assertFalse(invoked)
    }

    // ── cause propagation ───────────────────────────────

    @Test
    fun failurePreservesCause() {
        val ex = RuntimeException("root")
        val f = HttpResult.Failure("msg", cause = ex)
        assertEquals(ex, f.cause)
    }

    @Test
    fun failureDefaultCauseIsNull() {
        assertNull(HttpResult.Failure("msg").cause)
    }
}
