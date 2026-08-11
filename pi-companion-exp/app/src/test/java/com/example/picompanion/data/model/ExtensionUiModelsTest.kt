package com.example.picompanion.data.model

import com.example.picompanion.data.api.apiJson
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/**
 * Pure unit tests for [parseExtensionUiRequest] and the extension UI response
 * payload encoding. No Android dependencies.
 */
class ExtensionUiModelsTest {

  private fun parse(raw: String): ExtensionUiRequest? =
    parseExtensionUiRequest(apiJson.parseToJsonElement(raw).jsonObject)

  private fun request(marker: Boolean = true, extra: String = ""): String =
    """
    {"type":"extension_ui_request","id":"req-1","method":"ask_user",
     "question":"Which option?","context":"Optional detail",
     "options":[{"title":"Alpha","description":"First"},{"title":"Beta"}],
     "allowMultiple":true,"allowFreeform":true,"allowComment":true,
     "placeholder":"Type…","prefill":"draft",
     "_daemonExtensionUiRequiresResponse":$marker$extra}
    """.trimIndent()

  // ── Parser: marker / identity gates ────────────────────

  @Test
  fun ignoresFireAndForgetEventsWithoutMarker() {
    assertNull(parse(request(marker = false)))
  }

  @Test
  fun ignoresEventsWithMissingId() {
    val raw = """{"type":"extension_ui_request","_daemonExtensionUiRequiresResponse":true}"""
    assertNull(parse(raw))
  }

  @Test
  fun acceptsStringMarkerTrue() {
    val raw = """{"type":"extension_ui_request","id":"r1","_daemonExtensionUiRequiresResponse":"true"}"""
    assertEquals("r1", parse(raw)?.id)
  }

  // ── Parser: ask_user full payload ──────────────────────

  @Test
  fun parsesAskUserRequestFields() {
    val parsed = parse(request())!!
    assertEquals("req-1", parsed.id)
    assertEquals(ExtensionUiMethod.ASK_USER, parsed.method)
    assertEquals("Which option?", parsed.question)
    assertEquals("Optional detail", parsed.context)
    assertEquals("Type…", parsed.placeholder)
    assertEquals("draft", parsed.prefill)
    assertTrue(parsed.allowMultiple)
    assertTrue(parsed.allowFreeform)
    assertTrue(parsed.allowComment)
    assertTrue(parsed.isMultiSelect)
    assertTrue(parsed.supportsFreeform)
    assertTrue(parsed.supportsComment)
  }

  @Test
  fun parsesOptionsWithAndWithoutDescriptions() {
    val options = parse(request())!!.options
    assertEquals(2, options.size)
    assertEquals(ExtensionUiOption("Alpha", "First"), options[0])
    assertEquals(ExtensionUiOption("Beta", null), options[1])
  }

  @Test
  fun parsesPlainStringOptions() {
    val raw = """
      {"type":"extension_ui_request","id":"r1","method":"ask_user",
       "question":"Pick","options":["Allow","Block"],
       "_daemonExtensionUiRequiresResponse":true}
    """.trimIndent()
    val options = parse(raw)!!.options
    assertEquals(listOf("Allow", "Block"), options.map { it.title })
    assertTrue(options.all { it.description == null })
  }

  @Test
  fun defaultsFlagsAndOptionalFields() {
    val parsed = parse(
      """{"type":"extension_ui_request","id":"r1","method":"ask_user","question":"Q","_daemonExtensionUiRequiresResponse":true}"""
    )!!
    assertFalse(parsed.allowMultiple)
    assertFalse(parsed.allowFreeform)
    assertFalse(parsed.allowComment)
    assertTrue(parsed.options.isEmpty())
    assertNull(parsed.title)
    assertNull(parsed.message)
    assertNull(parsed.context)
    assertNull(parsed.placeholder)
    assertNull(parsed.prefill)
    assertEquals("Q", parsed.displayText)
    assertEquals("Pi extension request", parsed.displayTitle)
  }

  // ── Parser: generic dialog methods ─────────────────────

  @Test
  fun parsesSelectMethod() {
    val parsed = parse(
      """{"type":"extension_ui_request","id":"r1","method":"select","title":"Allow?","options":["Allow","Block"],"_daemonExtensionUiRequiresResponse":true}"""
    )!!
    assertEquals(ExtensionUiMethod.SELECT, parsed.method)
    assertEquals("Allow?", parsed.title)
    assertEquals("Allow?", parsed.displayText)
    assertFalse(parsed.isMultiSelect)
  }

  @Test
  fun parsesConfirmMethod() {
    val parsed = parse(
      """{"type":"extension_ui_request","id":"r1","method":"confirm","title":"Clear session?","message":"All messages lost.","_daemonExtensionUiRequiresResponse":true}"""
    )!!
    assertEquals(ExtensionUiMethod.CONFIRM, parsed.method)
    assertEquals("All messages lost.", parsed.message)
    assertEquals("All messages lost.", parsed.displayText)
    assertEquals("Clear session?", parsed.displayTitle)
  }

  @Test
  fun parsesInputMethod() {
    val parsed = parse(
      """{"type":"extension_ui_request","id":"r1","method":"input","title":"Enter a value","placeholder":"type something...","_daemonExtensionUiRequiresResponse":true}"""
    )!!
    assertEquals(ExtensionUiMethod.INPUT, parsed.method)
    assertEquals("type something...", parsed.placeholder)
  }

  @Test
  fun parsesEditorMethodWithPrefill() {
    val parsed = parse(
      """{"type":"extension_ui_request","id":"r1","method":"editor","title":"Edit text","prefill":"Line 1\nLine 2","_daemonExtensionUiRequiresResponse":true}"""
    )!!
    assertEquals(ExtensionUiMethod.EDITOR, parsed.method)
    assertEquals("Line 1\nLine 2", parsed.prefill)
  }

  @Test
  fun preservesUnknownMethodAsUnknown() {
    val parsed = parse(
      """{"type":"extension_ui_request","id":"r1","method":"notify","message":"hi","_daemonExtensionUiRequiresResponse":true}"""
    )!!
    assertEquals(ExtensionUiMethod.UNKNOWN, parsed.method)
    assertEquals("hi", parsed.message)
  }

  // ── Response payload encoding ──────────────────────────

  @Test
  fun responseOmitsUnsetFields() {
    val encoded = apiJson.encodeToString(ExtensionUiResponse(id = "req-1", cancelled = true))
    val json = apiJson.parseToJsonElement(encoded).jsonObject
    assertTrue(json.containsKey("id"))
    assertTrue(json["cancelled"]!!.toString() == "true")
    assertFalse(json.containsKey("value"))
    assertFalse(json.containsKey("confirmed"))
    assertFalse(json.containsKey("selections"))
    assertFalse(json.containsKey("comment"))
    assertFalse(json.containsKey("responseKind"))
  }

  @Test
  fun responseIncludesSetFields() {
    val encoded = apiJson.encodeToString(
      ExtensionUiResponse(
        id = "req-1",
        selections = listOf("Alpha", "Beta"),
        comment = "note",
        responseKind = "selection",
      )
    )
    val json = apiJson.parseToJsonElement(encoded).jsonObject
    assertEquals("req-1", json["id"]!!.toString().trim('"'))
    assertEquals(
      listOf("Alpha", "Beta"),
      json["selections"]!!.jsonArray.map { it.jsonPrimitive.content },
    )
    assertEquals("note", json["comment"]!!.toString().trim('"'))
    assertEquals("selection", json["responseKind"]!!.toString().trim('"'))
    assertFalse(json.containsKey("cancelled"))
    assertFalse(json.containsKey("value"))
  }

  @Test
  fun responseFreeformCarriesValueAndKind() {
    val encoded = apiJson.encodeToString(
      ExtensionUiResponse(id = "req-1", value = "custom", responseKind = "freeform")
    )
    val json = apiJson.parseToJsonElement(encoded).jsonObject
    assertEquals("custom", json["value"]!!.toString().trim('"'))
    assertEquals("freeform", json["responseKind"]!!.toString().trim('"'))
  }

  @Test
  fun responseConfirmedCarriesFlag() {
    val encoded = apiJson.encodeToString(ExtensionUiResponse(id = "req-1", confirmed = true))
    val json: JsonObject = apiJson.parseToJsonElement(encoded).jsonObject
    assertTrue(json["confirmed"]!!.toString() == "true")
  }
}
