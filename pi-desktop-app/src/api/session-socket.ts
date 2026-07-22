import { PiServerClient, type RpcCommand } from "@/api/client"

export type SessionSocketStatus =
  "idle" | "connecting" | "open" | "reconnecting" | "closed"
export type SessionEvent = Record<string, unknown> & { _daemonEventId?: number }

export interface SessionSocketOptions {
  client: PiServerClient
  sessionId: string
  watchFiles?: boolean
  reconnect?: boolean
  reconnectMinDelayMs?: number
  reconnectMaxDelayMs?: number
  onStatusChange?: (status: SessionSocketStatus) => void
  onEvent?: (event: SessionEvent) => void
  /** Called when the server event cursor jumps, requiring history resync. */
  onGap?: (expectedAfter: number, received: number) => void
  onError?: (error: Error) => void
}

/**
 * A ticket-authenticated browser WebSocket for one Pi session.
 *
 * Each reconnect requests a new single-use ticket and includes the last daemon
 * event ID as `since`, so pi-server replays buffered events that arrived while
 * the browser was disconnected.
 */
export class SessionSocket {
  private readonly client: PiServerClient
  private readonly sessionId: string
  private readonly watchFiles: boolean
  private readonly reconnect: boolean
  private readonly minDelay: number
  private readonly maxDelay: number
  private readonly onStatusChange?: (status: SessionSocketStatus) => void
  private readonly onEvent?: (event: SessionEvent) => void
  private readonly onGap?: (expectedAfter: number, received: number) => void
  private readonly onError?: (error: Error) => void

  private socket?: WebSocket
  private retryTimer?: number
  private retryAttempt = 0
  private manuallyClosed = false
  private latestEventId?: number
  private resynchronizing = false
  private opening = false
  private status: SessionSocketStatus = "idle"

  constructor(options: SessionSocketOptions) {
    this.client = options.client
    this.sessionId = options.sessionId
    this.watchFiles = options.watchFiles ?? false
    this.reconnect = options.reconnect ?? true
    this.minDelay = options.reconnectMinDelayMs ?? 500
    this.maxDelay = options.reconnectMaxDelayMs ?? 10_000
    this.onStatusChange = options.onStatusChange
    this.onEvent = options.onEvent
    this.onGap = options.onGap
    this.onError = options.onError
  }

  get connectionStatus(): SessionSocketStatus {
    return this.status
  }

  get lastEventId(): number | undefined {
    return this.latestEventId
  }

  connect(): void {
    this.manuallyClosed = false
    if (
      this.socket?.readyState === WebSocket.OPEN ||
      this.socket?.readyState === WebSocket.CONNECTING
    )
      return
    void this.open()
  }

  close(): void {
    this.manuallyClosed = true
    if (this.retryTimer !== undefined) window.clearTimeout(this.retryTimer)
    this.retryTimer = undefined
    this.socket?.close(1000, "Client closed connection")
    this.socket = undefined
    this.setStatus("closed")
  }

  send(command: RpcCommand): void {
    if (this.socket?.readyState !== WebSocket.OPEN) {
      throw new Error("Pi session WebSocket is not connected")
    }
    this.socket.send(JSON.stringify(command))
  }

  private async open(): Promise<void> {
    if (this.opening) return
    this.opening = true
    this.setStatus(this.retryAttempt === 0 ? "connecting" : "reconnecting")
    try {
      const ticket = await this.client.issueWebSocketTicket(this.sessionId)
      if (this.manuallyClosed) return

      const url = new URL(this.client.webSocketUrl(ticket.ws))
      if (this.latestEventId !== undefined)
        url.searchParams.set("since", String(this.latestEventId))
      if (this.watchFiles) url.searchParams.set("watch", "files")

      const socket = new WebSocket(url)
      this.socket = socket
      socket.onopen = () => {
        if (this.socket !== socket) return
        this.retryAttempt = 0
        this.setStatus("open")
      }
      socket.onmessage = (message) => this.handleMessage(message)
      socket.onerror = () => {
        if (this.socket !== socket) return
        this.onError?.(new Error("Pi session WebSocket error"))
      }
      socket.onclose = () => {
        if (this.socket === socket) this.socket = undefined
        if (this.manuallyClosed) return
        this.scheduleReconnect()
      }
    } catch (error) {
      // Guard against stale errors from a previous socket that resolved
      // after the user switched to a new session.
      if (!this.manuallyClosed) {
        this.onError?.(toError(error))
        this.scheduleReconnect()
      }
    } finally {
      this.opening = false
    }
  }

  private handleMessage(message: MessageEvent<string>): void {
    try {
      const event = JSON.parse(message.data) as SessionEvent
      if (event.type === "events_lost") {
        const expectedAfter = typeof event.expectedAfter === "number" ? event.expectedAfter : this.latestEventId ?? 0
        const received = typeof event.received === "number" ? event.received : expectedAfter + 1
        this.onGap?.(expectedAfter, received)
        // Reset cursor and enter resynchronizing mode: the next reconnect
        // will request full replay (no since). Suppress gap detection until
        // we've established a new consecutive baseline from the replay.
        this.latestEventId = undefined
        this.resynchronizing = true
        this.socket?.close(1012, "Event history gap; resynchronizing")
        return
      }
      const eventId = event._daemonEventId
      if (typeof eventId === "number" && Number.isSafeInteger(eventId)) {
        const previous = this.latestEventId
        // IDs are consecutive for both PiProcess and relay registries. A gap
        // means the bounded ring or a slow-subscriber drop lost events.
        // After an events_lost-triggered resync, suppress gap detection until
        // we've seen the first event to establish the new baseline.
        if (this.resynchronizing) {
          // Accept the first event from replay as the new baseline.
          this.latestEventId = eventId
          this.resynchronizing = false
        } else if (previous !== undefined && eventId > previous + 1) {
          this.onGap?.(previous, eventId)
          this.latestEventId = undefined
          this.resynchronizing = true
          this.socket?.close(1012, "Event history gap; resynchronizing")
          return
        } else {
          this.latestEventId = Math.max(previous ?? 0, eventId)
        }
      }
      this.onEvent?.(event)
    } catch {
      this.onError?.(new Error("pi-server sent a malformed WebSocket event"))
    }
  }

  private scheduleReconnect(): void {
    if (
      !this.reconnect ||
      this.manuallyClosed ||
      this.retryTimer !== undefined
    ) {
      this.setStatus("closed")
      return
    }
    const delay = Math.min(
      this.maxDelay,
      this.minDelay * 2 ** this.retryAttempt
    )
    this.retryAttempt += 1
    this.setStatus("reconnecting")
    this.retryTimer = window.setTimeout(() => {
      this.retryTimer = undefined
      void this.open()
    }, delay)
  }

  private setStatus(status: SessionSocketStatus): void {
    if (this.status === status) return
    this.status = status
    this.onStatusChange?.(status)
  }
}

function toError(error: unknown): Error {
  return error instanceof Error
    ? error
    : new Error("Unable to connect to pi-server")
}
