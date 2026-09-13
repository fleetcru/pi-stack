package com.example.picompanion.ui.sessiondetail

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull

/** CPU-only conversion of persisted Pi history. Safe to run off the UI thread. */
internal object SessionHistoryParser {
  fun parse(messages: JsonArray, pageStartIndex: Int = 0): List<SessionTimelineItem> {
    val results = mutableMapOf<String, String>()
    val parsed = mutableListOf<SessionTimelineItem>()

    messages.forEachIndexed { index, element ->
      val message = element as? JsonObject ?: return@forEachIndexed
      val historyType = message.string("_historyType")
      val role = message.string("role")
      val historyIndex = pageStartIndex + index
      val fallbackId = "tool-history-$historyIndex"

      if (historyType == "tool_use") {
        parsed += SessionTimelineItem.Tool(
          callId = message.string("id") ?: fallbackId,
          name = message.string("name") ?: "tool",
          status = "completed",
          args = message["input"]?.toString(),
        )
        return@forEachIndexed
      }
      if (historyType == "tool_result" || role == "tool" || role == "toolResult") {
        val id = message.string("toolCallId")
          ?: message.string("tool_use_id")
          ?: message.string("id")
        if (id != null) message.findText()?.let { results[id] = it }
        return@forEachIndexed
      }
      if (role != "user" && role != "assistant") return@forEachIndexed

      val author = if (role == "user") "You" else "Pi Agent"
      val isUser = role == "user"
      val timestamp = message.string("timestamp").orEmpty()
      val content = message["content"]
      if (content is JsonArray) {
        val text = StringBuilder()
        val images = mutableListOf<String>()
        var segmentIndex = 0
        fun flushChat() {
          val value = text.toString().trim()
          if (value.isNotEmpty() || images.isNotEmpty()) {
            parsed += SessionTimelineItem.Chat(
              author = author,
              text = value,
              time = timestamp,
              isUser = isUser,
              imageData = images.toList(),
              sourceId = "history-$historyIndex-segment-${segmentIndex++}",
            )
          }
          text.clear()
          images.clear()
        }

        content.forEachIndexed { toolIndex, block ->
          val objectBlock = block as? JsonObject
          if (objectBlock?.string("type") in setOf("toolCall", "tool_use")) {
            // Keep text before and after a tool as separate timeline bubbles.
            // Flattening all text after every tool made restored history differ
            // from the order shown by live events.
            flushChat()
            val id = objectBlock?.string("id") ?: "$fallbackId-$toolIndex"
            results.putIfAbsent(id, "")
            parsed += SessionTimelineItem.Tool(
              callId = id,
              name = objectBlock?.string("name") ?: "tool",
              status = "completed",
              args = objectBlock?.get("arguments")?.toString() ?: objectBlock?.get("input")?.toString(),
            )
          } else {
            block.findText()?.let(text::append)
            images += block.findImages()
          }
        }
        flushChat()
      } else {
        val text = message.findText()?.trim().orEmpty()
        val images = message.findImages()
        if (text.isNotEmpty() || images.isNotEmpty()) {
          parsed += SessionTimelineItem.Chat(
            author = author,
            text = text,
            time = timestamp,
            isUser = isUser,
            imageData = images,
            sourceId = "history-$historyIndex-segment-0",
          )
        }
      }
    }

    return parsed.map { item ->
      if (item is SessionTimelineItem.Tool) item.copy(output = results[item.callId]) else item
    }.distinctBy(::timelineItemId)
  }

  private fun JsonObject.string(key: String): String? =
    (this[key] as? JsonPrimitive)?.contentOrNull

  private fun JsonElement.findImages(): List<String> = when (this) {
    is JsonObject -> if (string("type") == "image") listOfNotNull(string("data")) else this["content"]?.findImages().orEmpty()
    is JsonArray -> flatMap { it.findImages() }
    else -> emptyList()
  }

  private fun JsonElement.findText(): String? = when (this) {
    is JsonPrimitive -> if (isString) contentOrNull else null
    is JsonObject -> string("text") ?: string("content") ?: string("delta")
      ?: string("message") ?: this["content"]?.findText()
    is JsonArray -> joinToString("") { it.findText().orEmpty() }.ifEmpty { null }
    else -> null
  }

  private fun timelineItemId(item: SessionTimelineItem): String = when (item) {
    is SessionTimelineItem.Chat -> "chat|${item.sourceId ?: "${item.isUser}|${item.time}|${item.text.length}|${item.text.hashCode()}|${item.text.take(50)}"}"
    is SessionTimelineItem.Tool -> "tool|${item.callId}"
    is SessionTimelineItem.FileChange -> "file|${item.operation}|${item.path}"
    is SessionTimelineItem.System -> "system|${item.text}"
  }
}
