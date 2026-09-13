package com.example.picompanion.ui.sessiondetail

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
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
  fun preservesTextAndToolBlockOrder() {
    val messages = Json.parseToJsonElement(
      """[
        {"role":"assistant","timestamp":"t1","content":[
          {"type":"text","text":"Before tool"},
          {"type":"toolCall","id":"call-ordered","name":"read","arguments":{"path":"README.md"}},
          {"type":"text","text":"After tool"}
        ]},
        {"role":"toolResult","toolCallId":"call-ordered","content":[{"type":"text","text":"result"}]}
      ]""",
    ).jsonArray

    val items = SessionHistoryParser.parse(messages)

    assertEquals(3, items.size)
    assertEquals("Before tool", (items[0] as SessionTimelineItem.Chat).text)
    assertEquals("call-ordered", (items[1] as SessionTimelineItem.Tool).callId)
    assertEquals("result", (items[1] as SessionTimelineItem.Tool).output)
    assertEquals("After tool", (items[2] as SessionTimelineItem.Chat).text)
  }

  @Test
  fun keepsIdenticalTextSegmentsSeparatedByTool() {
    val messages = Json.parseToJsonElement(
      """[{
        "role":"assistant","content":[
          {"type":"text","text":"Same text"},
          {"type":"toolCall","id":"call-between","name":"read","arguments":{}},
          {"type":"text","text":"Same text"}
        ]
      }]""",
    ).jsonArray

    val items = SessionHistoryParser.parse(messages, pageStartIndex = 20)

    assertEquals(3, items.size)
    assertEquals("history-20-segment-0", (items[0] as SessionTimelineItem.Chat).sourceId)
    assertEquals("history-20-segment-1", (items[2] as SessionTimelineItem.Chat).sourceId)
  }

  @Test
  fun stableSourceIdsUseAbsoluteHistoryPosition() {
    val messages = Json.parseToJsonElement(
      """[
        {"role":"user","content":"first"},
        {"role":"assistant","content":"second"}
      ]""",
    ).jsonArray

    val fullPage = SessionHistoryParser.parse(messages, pageStartIndex = 40)
    val shiftedPage = SessionHistoryParser.parse(JsonArray(listOf(messages[1])), pageStartIndex = 41)

    assertEquals(
      (fullPage[1] as SessionTimelineItem.Chat).sourceId,
      (shiftedPage.single() as SessionTimelineItem.Chat).sourceId,
    )
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
