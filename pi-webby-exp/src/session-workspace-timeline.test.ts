import { describe, expect, it } from "vitest"
import { buildHistory, findExtensionRequest, IncrementalTimeline, isNoiseFilePath } from "@pi-stack/webby-shared/session-workspace"

describe("shared history timeline", () => {
  it("parses Pi toolCall and toolResult records", () => {
    const timeline = buildHistory({ data: { messages: [
      { role: "assistant", timestamp: 1, content: [{ type: "toolCall", id: "call-1", name: "read", arguments: { path: "README.md" } }] },
      { role: "toolResult", timestamp: 2, toolCallId: "call-1", content: [{ type: "text", text: "hello" }] },
    ] } })
    expect(timeline).toEqual([
      expect.objectContaining({ id: "tool-call-1", name: "read", args: { path: "README.md" }, done: false }),
      expect.objectContaining({ id: "tool-call-1", name: "read", output: "hello", done: true }),
    ])
  })

  it("preserves images in historical user messages, including image-only messages", () => {
    const image = { type: "image", data: "abc123", mimeType: "image/png" }
    const timeline = buildHistory({ data: { messages: [
      { role: "user", timestamp: 1, content: [image, { type: "text", text: "Describe this" }] },
      { role: "user", timestamp: 2, content: [image] },
    ] } })
    expect(timeline).toEqual([
      expect.objectContaining({ kind: "user", text: "Describe this", images: [image] }),
      expect.objectContaining({ kind: "user", text: "", images: [image] }),
    ])
  })

  it("parses standalone fallback tool records without a role", () => {
    const timeline = buildHistory({ data: { messages: [
      { _historyType: "tool_use", id: "call-2", name: "bash", input: { command: "pwd" } },
      { _historyType: "tool_result", tool_use_id: "call-2", content: [{ type: "text", text: "/tmp" }] },
    ] } })
    expect(timeline.at(-1)).toEqual(expect.objectContaining({ id: "tool-call-2", name: "bash", output: "/tmp", done: true }))
  })
})

describe("IncrementalTimeline", () => {
  it("applies only new deltas and survives a shifted event window", () => {
    const reducer = new IncrementalTimeline()
    const start = { type: "message_start", _daemonEventId: 1, message: { role: "assistant", timestamp: 1 } }
    const first = { type: "message_update", _daemonEventId: 2, assistantMessageEvent: { type: "text_delta", delta: "hello" } }
    expect(reducer.update([start, first])).toMatchObject([{ kind: "assistant", text: "hello" }])
    const second = { type: "message_update", _daemonEventId: 3, assistantMessageEvent: { type: "text_delta", delta: " world" } }
    expect(reducer.update([first, second])).toMatchObject([{ kind: "assistant", text: "hello world" }])
  })

  it("preserves images in live user messages", () => {
    const reducer = new IncrementalTimeline()
    const image = { type: "image", data: "abc123", mimeType: "image/jpeg" }
    const items = reducer.update([{
      type: "message_start",
      _daemonEventId: 1,
      message: { role: "user", timestamp: 1, content: [image] },
    }])
    expect(items).toEqual([expect.objectContaining({ kind: "user", text: "", images: [image] })])
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
