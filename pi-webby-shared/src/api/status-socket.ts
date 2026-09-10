import { PiServerApiError, type PiServerClient } from "./client"

export type StatusSocketStatus =
  "idle" | "connecting" | "open" | "reconnecting" | "closed"

/**
 * A server-wide status delta. Carries only routing and lifecycle fields —
 * never prompt contents or detailed chat events.
 */
export interface ServerStatusEvent {
  type: "session_status"
  sessionId?: string
  workerId?: string
  state: string
  reason?: string
  detail?: string
  runId?: string
  /** 1-based admission queue position; set on "queued" events. */
  position?: number
  queuedAt?: string
  updatedAt: string
}

/** The three envelope shapes pi-server writes to /v1/status/ws. */
interface StatusSnapshotMessage {
  type: "status_snapshot"
  events: ServerStatusEvent[]
  cursor: number
  generation: string
  gap?: boolean
}
interface StatusReplayMessage {
  type: "status_replay"
  events: ServerStatusEvent[]
  cursor: number
  generation: string
}
interface StatusDeltaMessage {
  type: "status"
  event: ServerStatusEvent
  cursor: number
  generation: string
}
type StatusWireMessage = StatusSnapshotMessage | StatusReplayMessage | StatusDeltaMessage

const STATUS_MESSAGE_TYPES = new Set(["status_snapshot", "status_replay", "status"])

export interface StatusSocketOptions {
  client: PiServerClient
  reconnect?: boolean
  reconnectMinDelayMs?: number
  reconnectMaxDelayMs?: number
  onStatusChange?: (status: StatusSocketStatus) => void
  /** Applied for every snapshot, replay, and live delta event. */
  onEvent?: (event: ServerStatusEvent) => void
  /** Called when the cursor is unusable and a full snapshot resync follows. */
  onGap?: (reason: "epoch" | "ring") => void
  onError?: (error: Error) => void
}

/**
 * A ticket-authenticated, read-only server-wide status WebSocket.
 *
 * Owns the cursor/generation handshake with pi-server: reconnects send
 * `since=<cursor>&epoch=<generation>` so the server replays bounded status
 * deltas, and a server restart (generation mismatch) or a ring overflow
 * falls back to a full snapshot. This socket never subscribes to detailed
 * session events — messages outside the three status envelope types are
 * dropped.
 */
export class StatusSocket {
  private readonly client: PiServerClient
  private readonly reconnect: boolean
  private readonly minDelay: number
  private readonly maxDelay: number
  private readonly onStatusChange?: (status: StatusSocketStatus) => void
  private readonly onEvent?: (event: ServerStatusEvent) => void
  private readonly onGap?: (reason: "epoch" | "ring") => void
  private readonly onError?: (error: Error) => void

  private socket?: WebSocket
  private retryTimer?: number
  private retryAttempt = 0
  private manuallyClosed = false
  private latestCursor?: number
  private generation?: string
  private opening = false
  private status: StatusSocketStatus = "idle"

  constructor(options: StatusSocketOptions) {
    this.client = options.client
    this.reconnect = options.reconnect ?? true
    this.minDelay = options.reconnectMinDelayMs ?? 500
    this.maxDelay = options.reconnectMaxDelayMs ?? 10_000
    this.onStatusChange = options.onStatusChange
    this.onEvent = options.onEvent
    this.onGap = options.onGap
    this.onError = options.onError
  }

  get connectionStatus(): StatusSocketStatus {
    return this.status
  }

  get lastCursor(): number | undefined {
    return this.latestCursor
  }

  get hubGeneration(): string | undefined {
    return this.generation
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

  private async open(): Promise<void> {
    if (this.opening) return
    this.opening = true
    this.setStatus(this.retryAttempt === 0 ? "connecting" : "reconnecting")
    try {
      const ticket = await this.client.issueStatusTicket()
      if (this.manuallyClosed) return

      const url = new URL(this.client.webSocketUrl(ticket.ws))
      if (this.latestCursor !== undefined) {
        url.searchParams.set("since", String(this.latestCursor))
        if (this.generation !== undefined)
          url.searchParams.set("epoch", this.generation)
      }

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
        this.onError?.(new Error("pi-server status WebSocket error"))
      }
      socket.onclose = () => {
        if (this.socket === socket) this.socket = undefined
        if (this.manuallyClosed) return
        this.scheduleReconnect()
      }
    } catch (error) {
      if (!this.manuallyClosed) {
        const normalized = toError(error)
        this.onError?.(normalized)
        if (error instanceof PiServerApiError && error.status === 401) {
          this.setStatus("closed")
          return
        }
        this.scheduleReconnect()
      }
    } finally {
      this.opening = false
    }
  }

  private handleMessage(message: MessageEvent<string>): void {
    let parsed: unknown
    try {
      parsed = JSON.parse(message.data)
    } catch {
      this.onError?.(new Error("pi-server sent a malformed status message"))
      return
    }
    // Hard filter: anything that is not one of the three status envelope
    // types (including any detailed session/chat event) is dropped.
    if (
      typeof parsed !== "object" ||
      parsed === null ||
      !("type" in parsed) ||
      typeof (parsed as { type: unknown }).type !== "string" ||
      !STATUS_MESSAGE_TYPES.has((parsed as { type: string }).type)
    ) {
      return
    }
    const msg = parsed as StatusWireMessage
    if (typeof msg.generation === "string") {
      if (this.generation !== undefined && this.generation !== msg.generation) {
        // The hub restarted: cursor is meaningless, take a full snapshot.
        this.generation = msg.generation
        this.latestCursor = undefined
        this.onGap?.("epoch")
        this.socket?.close(1012, "Status hub generation changed; resynchronizing")
        return
      }
      this.generation = msg.generation
    }
    if (typeof msg.cursor === "number" && Number.isSafeInteger(msg.cursor)) {
      this.latestCursor = Math.max(this.latestCursor ?? 0, msg.cursor)
    }
    if (msg.type === "status_snapshot" && msg.gap) {
      this.onGap?.("ring")
    }
    if (msg.type === "status") {
      this.onEvent?.(msg.event)
    } else {
      for (const event of msg.events) this.onEvent?.(event)
    }
  }

  private scheduleReconnect(): void {
    if (!this.reconnect || this.manuallyClosed || this.retryTimer !== undefined) {
      this.setStatus("closed")
      return
    }
    const cappedDelay = Math.min(
      this.maxDelay,
      this.minDelay * 2 ** this.retryAttempt
    )
    const delay = Math.round(cappedDelay * (0.75 + Math.random() * 0.5))
    this.retryAttempt += 1
    this.setStatus("reconnecting")
    this.retryTimer = window.setTimeout(() => {
      this.retryTimer = undefined
      void this.open()
    }, delay)
  }

  private setStatus(status: StatusSocketStatus): void {
    if (this.status === status) return
    this.status = status
    this.onStatusChange?.(status)
  }
}

function toError(error: unknown): Error {
  return error instanceof Error
    ? error
    : new Error("Unable to connect to pi-server status stream")
}
