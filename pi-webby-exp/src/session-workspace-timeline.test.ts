import { describe, expect, it } from "vitest"
import { findExtensionRequest, IncrementalTimeline } from "@pi-stack/webby-shared/session-workspace"

describe("IncrementalTimeline", () => {
  it("applies only new deltas and survives a shifted event window", () => {
    const reducer = new IncrementalTimeline()
    const start = { type: "message_start", _daemonEventId: 1, message: { role: "assistant", timestamp: 1 } }
    const first = { type: "message_update", _daemonEventId: 2, assistantMessageEvent: { type: "text_delta", delta: "hello" } }
    expect(reducer.update([start, first])).toMatchObject([{ kind: "assistant", text: "hello" }])
    const second = { type: "message_update", _daemonEventId: 3, assistantMessageEvent: { type: "text_delta", delta: " world" } }
    expect(reducer.update([first, second])).toMatchObject([{ kind: "assistant", text: "hello world" }])
  })

  it("updates tool entities in place", () => {
    const reducer = new IncrementalTimeline()
    const events = [
      { type: "tool_execution_start", _daemonEventId: 1, toolCallId: "call", toolName: "bash" },
      { type: "tool_execution_end", _daemonEventId: 2, toolCallId: "call", result: { content: [{ text: "done" }] } },
    ]
    expect(reducer.update(events)).toMatchObject([{ kind: "tool", done: true, output: "done" }])
  })

  it("shows ask_user questions and clears them after a close event", () => {
    const request = {
      type: "extension_ui_request",
      id: "ask-1",
      method: "ask_user",
      question: "Choose a release channel",
      _daemonExtensionUiRequiresResponse: true,
    }
    expect(findExtensionRequest([request])).toMatchObject({
      id: "ask-1",
      message: "Choose a release channel",
    })
    expect(findExtensionRequest([
      request,
      { type: "extension_ui_closed", id: "ask-1" },
    ])).toBeUndefined()
  })
})
