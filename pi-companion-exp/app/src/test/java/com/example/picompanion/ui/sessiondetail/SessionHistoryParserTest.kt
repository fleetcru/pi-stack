package com.example.picompanion.ui.sessiondetail

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.jsonArray

class SessionHistoryParserTest {
  @Test
  fun parsesStandaloneToolRecordsWithoutRole() {
    val messages = Json.parseToJsonElement(
      """[
        {"_historyType":"tool_use","id":"call-1","name":"bash","input":{"command":"pwd"}},
        {"_historyType":"tool_result","toolCallId":"call-1","content":[{"type":"text","text":"/tmp"}]}
      ]""",
    ).jsonArray

    val item = SessionHistoryParser.parse(messages).single()
    assertTrue(item is SessionTimelineItem.Tool)
    val tool = item as SessionTimelineItem.Tool
    assertEquals("call-1", tool.callId)
    assertEquals("bash", tool.name)
    assertEquals("/tmp", tool.output)
  }

  @Test
  fun associatesPiToolResultWithOpenAiToolCall() {
    val messages = Json.parseToJsonElement(
      """[
        {"role":"assistant","content":[{"type":"toolCall","id":"call-2","name":"read","arguments":{"path":"README.md"}}]},
        {"role":"toolResult","toolCallId":"call-2","content":[{"type":"text","text":"hello"}]}
      ]""",
    ).jsonArray

    val item = SessionHistoryParser.parse(messages).single()
    assertTrue(item is SessionTimelineItem.Tool)
    val tool = item as SessionTimelineItem.Tool
    assertEquals("hello", tool.output)
  }
}
