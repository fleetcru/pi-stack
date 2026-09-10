import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { StatusSocket } from "./status-socket"
import type { PiServerClient } from "./client"

class MockWebSocket {
  static CONNECTING = 0
  static OPEN = 1
  static CLOSING = 2
  static CLOSED = 3
  readyState = MockWebSocket.OPEN
  onopen: (() => void) | null = null
  onclose: (() => void) | null = null
  onmessage: ((event: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  send = vi.fn()
  close = vi.fn()
  url: string
  constructor(url: string) {
    this.url = url
  }
}

function makeClient(baseUrl = "http://localhost:3141"): PiServerClient {
  return {
    baseUrl,
    issueStatusTicket: vi.fn().mockResolvedValue({ ticket: "t1", expiresAt: "soon", ws: "/v1/status/ws?ticket=t1" }),
    webSocketUrl: (path: string) => `ws://localhost:3141${path}`,
  } as unknown as PiServerClient
}

async function connectSocket(socket: StatusSocket, client: PiServerClient) {
  socket.connect()
  await vi.advanceTimersByTimeAsync(0)
  expect(client.issueStatusTicket).toHaveBeenCalled()
  return (socket as unknown as { socket: MockWebSocket }).socket
}

describe("StatusSocket", () => {
  let originalWebSocket: typeof globalThis.WebSocket

  beforeEach(() => {
    originalWebSocket = globalThis.WebSocket
    // @ts-expect-error mock
    globalThis.WebSocket = MockWebSocket
    vi.useFakeTimers()
  })

  afterEach(() => {
    globalThis.WebSocket = originalWebSocket
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it("connects via a status ticket and applies snapshot events", async () => {
    const onEvent = vi.fn()
    const client = makeClient()
    const socket = new StatusSocket({ client, reconnect: false, onEvent })

    const ws = await connectSocket(socket, client)
    expect((client.issueStatusTicket as ReturnType<typeof vi.fn>).mock.calls[0]).toEqual([])

    ws.onopen?.()
    ws.onmessage?.({
      data: JSON.stringify({
        type: "status_snapshot",
        events: [
          { type: "session_status", sessionId: "s1", state: "working", updatedAt: "t" },
          { type: "session_status", workerId: "w1", state: "healthy", updatedAt: "t" },
        ],
        cursor: 7,
        generation: "g1",
      }),
    })

    expect(onEvent).toHaveBeenCalledTimes(2)
    expect(socket.lastCursor).toBe(7)
    expect(socket.hubGeneration).toBe("g1")
    socket.close()
  })

  it("tracks the cursor across live deltas", async () => {
    const onEvent = vi.fn()
    const client = makeClient()
    const socket = new StatusSocket({ client, reconnect: false, onEvent })

    const ws = await connectSocket(socket, client)
    ws.onopen?.()

    ws.onmessage?.({
      data: JSON.stringify({
        type: "status",
        event: { type: "session_status", sessionId: "s1", state: "idle", updatedAt: "t" },
        cursor: 9,
        generation: "g1",
      }),
    })
    expect(socket.lastCursor).toBe(9)

    ws.onmessage?.({
      data: JSON.stringify({
        type: "status",
        event: { type: "session_status", sessionId: "s1", state: "working", updatedAt: "t" },
        cursor: 10,
        generation: "g1",
      }),
    })
    expect(socket.lastCursor).toBe(10)
    socket.close()
  })

  it("drops messages that are not status envelopes", async () => {
    const onEvent = vi.fn()
    const onError = vi.fn()
    const client = makeClient()
    const socket = new StatusSocket({ client, reconnect: false, onEvent, onError })

    const ws = await connectSocket(socket, client)
    ws.onopen?.()

    // Detailed chat events must never surface from the status socket.
    ws.onmessage?.({ data: JSON.stringify({ type: "message_update", text_delta: "secret" }) })
    ws.onmessage?.({ data: JSON.stringify({ type: "tool_execution_start", toolName: "bash" }) })
    ws.onmessage?.({ data: "not json" })

    expect(onEvent).not.toHaveBeenCalled()
    expect(onError).toHaveBeenCalledTimes(1) // only the malformed frame
    socket.close()
  })

  it("reports a gap when the hub epoch changes", async () => {
    const onGap = vi.fn()
    const onEvent = vi.fn()
    const client = makeClient()
    const socket = new StatusSocket({ client, reconnect: false, onEvent, onGap })

    const ws = await connectSocket(socket, client)
    ws.onopen?.()

    ws.onmessage?.({
      data: JSON.stringify({
        type: "status_snapshot", events: [], cursor: 4, generation: "g1",
      }),
    })

    ws.onmessage?.({
      data: JSON.stringify({
        type: "status",
        event: { type: "session_status", sessionId: "s1", state: "working", updatedAt: "t" },
        cursor: 5,
        generation: "g2",
      }),
    })

    expect(onGap).toHaveBeenCalledWith("epoch")
    expect(socket.hubGeneration).toBe("g2")
    // Cursor cleared for a full-snapshot resync.
    expect(socket.lastCursor).toBeUndefined()
    expect(onEvent).not.toHaveBeenCalled()
    socket.close()
  })

  it("reconnects with since and epoch and accepts replay", async () => {
    const client = makeClient()
    const socket = new StatusSocket({
      client,
      reconnectMinDelayMs: 10,
      reconnectMaxDelayMs: 10,
    })

    const ws = await connectSocket(socket, client)
    ws.onopen?.()
    ws.onmessage?.({
      data: JSON.stringify({
        type: "status_snapshot", events: [], cursor: 5, generation: "g1",
      }),
    })

    // Simulate a drop and reconnect.
    ws.onclose?.()
    await vi.advanceTimersByTimeAsync(50)
    const reconnected = (socket as unknown as { socket: MockWebSocket }).socket
    expect(reconnected).toBeDefined()
    expect(String(reconnected.url)).toContain("since=5")
    expect(String(reconnected.url)).toContain("epoch=g1")

    reconnected.onopen?.()
    reconnected.onmessage?.({
      data: JSON.stringify({
        type: "status_replay",
        events: [{ type: "session_status", sessionId: "s1", state: "working", updatedAt: "t" }],
        cursor: 8,
        generation: "g1",
      }),
    })
    expect(socket.lastCursor).toBe(8)
    socket.close()
  })

  it("does not open concurrently", async () => {
    const client = makeClient()
    const socket = new StatusSocket({ client, reconnect: false })
    socket.connect()
    socket.connect()
    await vi.advanceTimersByTimeAsync(0)
    expect(client.issueStatusTicket).toHaveBeenCalledTimes(1)
    socket.close()
  })
})
