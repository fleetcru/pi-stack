package com.example.picompanion.data.model

import kotlinx.serialization.Serializable
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull

/**
 * Extension UI dialog methods that require a client response. Status, widget,
 * and notification methods share the `extension_ui_request` RPC type but are
 * fire-and-forget; the daemon marker distinguishes them (see
 * [parseExtensionUiRequest]).
 */
enum class ExtensionUiMethod(val wire: String) {
  ASK_USER("ask_user"),
  SELECT("select"),
  CONFIRM("confirm"),
  INPUT("input"),
  EDITOR("editor"),
  UNKNOWN("");

  companion object {
    fun fromWire(value: String?): ExtensionUiMethod =
      entries.firstOrNull { it.wire == value } ?: UNKNOWN
  }
}

/** A single selectable option in an ask_user/select request. */
data class ExtensionUiOption(val title: String, val description: String? = null)

/**
 * Parsed extension UI request from an `extension_ui_request` event.
 *
 * Only actionable requests become instances: the server stamps
 * `_daemonExtensionUiRequiresResponse: true` for dialog methods
 * (select/confirm/input/editor/ask_user); everything else is ignored by the
 * parser and never reaches the UI.
 */
data class ExtensionUiRequest(
  val id: String,
  val method: ExtensionUiMethod = ExtensionUiMethod.UNKNOWN,
  val title: String? = null,
  val message: String? = null,
  val question: String? = null,
  val context: String? = null,
  val options: List<ExtensionUiOption> = emptyList(),
  val allowMultiple: Boolean = false,
  val allowFreeform: Boolean = false,
  val allowComment: Boolean = false,
  val placeholder: String? = null,
  val prefill: String? = null,
) {
  /** Dialog title: explicit title, else a generic label. */
  val displayTitle: String get() = title ?: "Pi extension request"

  /** Dialog body text: question (ask_user), message, title, else generic. */
  val displayText: String
    get() = question ?: message ?: title ?: "Extension input requested"

  /** Whether the user may pick more than one option. */
  val isMultiSelect: Boolean get() = method == ExtensionUiMethod.ASK_USER && allowMultiple

  /** Whether a freeform answer field should be offered. */
  val supportsFreeform: Boolean get() = method == ExtensionUiMethod.ASK_USER && allowFreeform

  /** Whether an optional comment field should be offered. */
  val supportsComment: Boolean get() = method == ExtensionUiMethod.ASK_USER && allowComment
}

/**
 * Payload posted to `/v1/sessions/{id}/ui-response`.
 *
 * Nullable fields are omitted from the JSON body unless set (kotlinx
 * serialization skips defaults), so only the fields the answer needs travel.
 */
@Serializable
data class ExtensionUiResponse(
  val id: String,
  val cancelled: Boolean = false,
  val value: String? = null,
  val confirmed: Boolean? = null,
  val selections: List<String>? = null,
  val comment: String? = null,
  val responseKind: String? = null,
)

/**
 * Parses an `extension_ui_request` RPC event into a client model.
 *
 * Returns null when the event is not an actionable request: no `id`, or the
 * daemon marker `_daemonExtensionUiRequiresResponse` is not `true`
 * (fire-and-forget status/notification events share this RPC type).
 */
fun parseExtensionUiRequest(raw: JsonObject): ExtensionUiRequest? {
  val id = raw.string("id") ?: return null
  if (!raw.daemonMarker()) return null
  return ExtensionUiRequest(
    id = id,
    method = ExtensionUiMethod.fromWire(raw.string("method")),
    title = raw.string("title"),
    message = raw.string("message"),
    question = raw.string("question"),
    context = raw.string("context"),
    options = raw.parseOptions(),
    allowMultiple = raw.bool("allowMultiple"),
    allowFreeform = raw.bool("allowFreeform"),
    allowComment = raw.bool("allowComment"),
    placeholder = raw.string("placeholder"),
    prefill = raw.string("prefill"),
  )
}

/** True when the server stamped the event as requiring a client response. */
fun JsonObject.daemonMarker(): Boolean = when (val marker = this["_daemonExtensionUiRequiresResponse"]) {
  is JsonPrimitive -> marker.booleanOrNull ?: (marker.contentOrNull == "true")
  else -> false
}

private fun JsonObject.string(key: String): String? = (this[key] as? JsonPrimitive)?.contentOrNull

private fun JsonObject.bool(key: String): Boolean = when (val value = this[key]) {
  is JsonPrimitive -> value.booleanOrNull ?: (value.contentOrNull == "true")
  else -> false
}

/** Options may be plain strings or objects with `title`/`description`. */
private fun JsonObject.parseOptions(): List<ExtensionUiOption> {
  val array = this["options"] as? JsonArray ?: return emptyList()
  return array.mapNotNull { element ->
    when (element) {
      is JsonPrimitive ->
        element.contentOrNull?.takeIf { it.isNotBlank() }?.let { ExtensionUiOption(it) }
      is JsonObject -> {
        val title = (element["title"] as? JsonPrimitive)?.contentOrNull?.takeIf { it.isNotBlank() }
          ?: return@mapNotNull null
        ExtensionUiOption(
          title = title,
          description = (element["description"] as? JsonPrimitive)?.contentOrNull,
        )
      }
      else -> null
    }
  }
}
