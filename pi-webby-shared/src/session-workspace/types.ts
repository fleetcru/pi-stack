/** Timeline item types shared across all Pi chat clients. */

export type TextItem = {
  id: string
  kind: "user" | "assistant"
  text: string
}

export type ToolItem = {
  id: string
  kind: "tool"
  name: string
  output?: string
  done: boolean
  /** When the tool started (ISO timestamp or ms). */
  startedAt?: string | number
  /** When the tool finished (ISO timestamp or ms). */
  endedAt?: string | number
  /** Tool arguments as received from the event. */
  args?: Record<string, unknown>
  /** Whether the tool failed. */
  failed?: boolean
}

export type SystemItem = {
  id: string
  kind: "system"
  text: string
}

export type TimelineItem = TextItem | ToolItem | SystemItem

/** Model available on a Pi server session. */
export type AvailableModel = {
  provider: string
  id: string
  name?: string
}

/** Extension UI request from a Pi extension. */
export type ExtensionRequest = {
  id: string
  message: string
  placeholder?: string
}
