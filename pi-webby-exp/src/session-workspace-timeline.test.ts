import { describe, expect, it } from "vitest"
import { findExtensionRequest, IncrementalTimeline, isNoiseFilePath } from "@pi-stack/webby-shared/session-workspace"

describe("IncrementalTimeline", () => {
  it("applies only new deltas and survives a shifted event window", () => {
    const reducer = new IncrementalTimeline()
    const start = { type: "message_start", _daemonEventId: 1, message: { role: "assistant", timestamp: 1 } }
    const first = { type: "message_update", _daemonEventId: 2, assistantMessageEvent: { type: "text_delta", delta: "hello" } }
    expect(reducer.update([start, first])).toMatchObject([{ kind: "assistant", text: "hello" }])
    const second = { type: "message_update", _daemonEventId: 3, assistantMessageEvent: { type: "text_delta", delta: " world" } }
    expect(reducer.update([first, second])).toMatchObject([{ kind: "assistant", text: "hello world" }])
  })

  it("marks the assistant message as streaming until message_end", () => {
    const reducer = new IncrementalTimeline()
    const start = { type: "message_start", _daemonEventId: 1, message: { role: "assistant", timestamp: 1 } }
    const delta = { type: "message_update", _daemonEventId: 2, assistantMessageEvent: { type: "text_delta", delta: "hi" } }
    const end = { type: "message_end", _daemonEventId: 3 }
    // update() expects the cumulative window; passing the tail keeps the cursor.
    expect(reducer.update([start, delta])).toMatchObject([{ kind: "assistant", text: "hi", streaming: true }])
    expect(reducer.update([delta, end])).toMatchObject([{ kind: "assistant", text: "hi", streaming: false }])
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

  it("filters out noise file paths from the change feed", () => {
    // noise paths (should be hidden)
    for (const p of [
      ".git",
      ".git/index",
      ".git/config",
      ".wrangler",
      ".wrangler/tmp/abc",
      "node_modules/pkg/index.js",
      "dist/bundle.js",
      "build/.DS_Store",
    ]) {
      expect(isNoiseFilePath(p), p).toBe(true)
    }
    // real user paths (must stay visible)
    for (const p of [
      "src/app.tsx",
      "AGENTS.md",
      "pi-server-exp/internal/server/git_handlers.go",
      "README.md",
    ]) {
      expect(isNoiseFilePath(p), p).toBe(false)
    }
  })

  it("does not emit system items for noise file changes", () => {
    const reducer = new IncrementalTimeline()
    const events = [
      { type: "file_change", _daemonEventId: 1, path: ".git/index", change: "modified" },
      { type: "file_change", _daemonEventId: 2, path: ".wrangler/tmp/x", change: "modified" },
      { type: "file_change", _daemonEventId: 3, path: "src/app.tsx", change: "modified" },
    ]
    const items = reducer.update(events)
    expect(items).toHaveLength(1)
    expect(items[0]).toMatchObject({ kind: "system", text: "File modified: src/app.tsx" })
  })
})
