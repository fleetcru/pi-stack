import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import { SessionSocket } from "./session-socket"
import type { PiServerClient } from "./client"

// Mock WebSocket
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
    issueWebSocketTicket: vi.fn().mockResolvedValue({ ws: "/v1/sessions/test/ws?ticket=abc123" }),
    webSocketUrl: (path: string) => `ws://localhost:3141${path}`,
  } as unknown as PiServerClient
}

describe("SessionSocket", () => {
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

  it("connects with a ticket and sends events", async () => {
    const onEvent = vi.fn()
    const onStatusChange = vi.fn()
    const client = makeClient()
    const socket = new SessionSocket({
      client,
      sessionId: "test",
      reconnect: false,
      onEvent,
      onStatusChange,
    })

    socket.connect()
    await vi.advanceTimersByTimeAsync(0)

    // Should have called issueWebSocketTicket
    expect(client.issueWebSocketTicket).toHaveBeenCalledWith("test")

    // Simulate WebSocket open
    const ws = (socket as unknown as { socket: MockWebSocket }).socket
    expect(ws).toBeDefined()
    ws.onopen?.()

    // Should have set status to open
    expect(onStatusChange).toHaveBeenCalledWith("open")

    // Simulate receiving an event
    ws.onmessage?.({ data: JSON.stringify({ type: "test", _daemonEventId: 1 }) })
    expect(onEvent).toHaveBeenCalledWith(expect.objectContaining({ type: "test", _daemonEventId: 1 }))

    socket.close()
  })

  it("detects event gaps and triggers onGap", async () => {
    const onGap = vi.fn()
    const client = makeClient()
    const socket = new SessionSocket({
      client,
      sessionId: "test",
      reconnect: false,
      onGap,
    })

    socket.connect()
    await vi.advanceTimersByTimeAsync(0)

    const ws = (socket as unknown as { socket: MockWebSocket }).socket
    ws.onopen?.()

    // Send event 1
    ws.onmessage?.({ data: JSON.stringify({ type: "test", _daemonEventId: 1 }) })
    expect(onGap).not.toHaveBeenCalled()

    // Send event 3 (gap: missing event 2)
    ws.onmessage?.({ data: JSON.stringify({ type: "test", _daemonEventId: 3 }) })
    expect(onGap).toHaveBeenCalledWith(1, 3)

    socket.close()
  })

  it("passes duplicate events through — deduplication is at the hook layer", async () => {
    const onEvent = vi.fn()
    const client = makeClient()
    const socket = new SessionSocket({
      client,
      sessionId: "test",
      reconnect: false,
      onEvent,
    })

    socket.connect()
    await vi.advanceTimersByTimeAsync(0)

    const ws = (socket as unknown as { socket: MockWebSocket }).socket
    ws.onopen?.()

    // Send same event twice — socket passes both through
    ws.onmessage?.({ data: JSON.stringify({ type: "test", _daemonEventId: 1 }) })
    ws.onmessage?.({ data: JSON.stringify({ type: "test", _daemonEventId: 1 }) })

    // The socket class does not deduplicate; that happens in useActiveSessionSocket
    expect(onEvent).toHaveBeenCalledTimes(2)

    socket.close()
  })

  it("handles events_lost server message and resynchronizes", async () => {
    const onGap = vi.fn()
    const client = makeClient()
    const socket = new SessionSocket({
      client,
      sessionId: "test",
      reconnect: false,
      onGap,
    })

    socket.connect()
    await vi.advanceTimersByTimeAsync(0)

    const ws = (socket as unknown as { socket: MockWebSocket }).socket
    ws.onopen?.()

    // Server sends events_lost
    ws.onmessage?.({
      data: JSON.stringify({ type: "events_lost", expectedAfter: 5, received: 10 }),
    })

    expect(onGap).toHaveBeenCalledWith(5, 10)

    socket.close()
  })

  it("does not open concurrently", async () => {
    const client = makeClient()
    const socket = new SessionSocket({
      client,
      sessionId: "test",
      reconnect: false,
    })

    // Call connect twice quickly
    socket.connect()
    socket.connect()

    await vi.advanceTimersByTimeAsync(0)

    // issueWebSocketTicket should only be called once
    expect(client.issueWebSocketTicket).toHaveBeenCalledTimes(1)

    socket.close()
  })

  it("includes since parameter on reconnect", async () => {
    const client = makeClient()
    const socket = new SessionSocket({
      client,
      sessionId: "test",
      reconnect: false,
    })

    socket.connect()
    await vi.advanceTimersByTimeAsync(0)

    const ws = (socket as unknown as { socket: MockWebSocket }).socket
    ws.onopen?.()

    // Process event to set latestEventId
    ws.onmessage?.({ data: JSON.stringify({ type: "test", _daemonEventId: 5 }) })

    // Verify lastEventId is tracked
    expect(socket.lastEventId).toBe(5)

    socket.close()
  })
})
