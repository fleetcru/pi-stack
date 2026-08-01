import type {
  TimelineItem,
  TextItem,
  ToolItem,
  AvailableModel,
  ExtensionRequest,
} from "./types"

/**
 * Extracts plain text from a Pi message content value.
 * Content can be a string or an array of { text: string } parts.
 */
export function contentText(content: unknown): string {
  if (typeof content === "string") return content
  if (!Array.isArray(content)) return ""
  return content
    .map((part) => {
      if (
        typeof part === "object" &&
        part !== null &&
        "text" in part &&
        typeof (part as { text: unknown }).text === "string"
      )
        return (part as { text: string }).text
      return ""
    })
    .join("")
}

/**
 * Builds a timeline from live WebSocket events.
 * Processes message_start, message_update, message_end, tool_execution_*,
 * file_change, model_select, and thinking_level_select events.
 */
export function buildTimeline(
  events: Array<Record<string, unknown>>
): TimelineItem[] {
  const items: TimelineItem[] = []
  let activeAssistant: TextItem | undefined

  for (const [index, event] of events.entries()) {
    if (event.type === "message_start") {
      const message = event.message as
        | { role?: string; content?: unknown; timestamp?: number }
        | undefined
      const messageId = `${message?.role ?? "message"}-${message?.timestamp ?? event._daemonEventId ?? index}`
      if (message?.role === "user") {
        const text = contentText(message.content)
        if (text) items.push({ id: messageId, kind: "user", text })
      }
      if (message?.role === "assistant") {
        activeAssistant = { id: messageId, kind: "assistant", text: "" }
        items.push(activeAssistant)
      }
    }

    if (event.type === "message_update") {
      const delta = event.assistantMessageEvent as
        | { type?: string; delta?: string }
        | undefined
      if (delta?.type === "text_delta") {
        if (!activeAssistant) {
          activeAssistant = {
            id: `assistant-${event._daemonEventId ?? index}`,
            kind: "assistant",
            text: "",
          }
          items.push(activeAssistant)
        }
        activeAssistant.text += delta.delta ?? ""
      }
    }

    if (event.type === "message_end") activeAssistant = undefined

    // System events surfaced in the timeline for visibility.
    if (event.type === "file_change") {
      const path = event.path as string | undefined
      const change = event.change as string | undefined
      if (path)
        items.push({
          id: `file-${String(event._daemonEventId ?? index)}`,
          kind: "system",
          text: `File ${change ?? "changed"}: ${path}`,
        })
    }
    if (event.type === "model_select") {
      const model = event.model as
        | { provider?: string; id?: string }
        | undefined
      if (model?.id)
        items.push({
          id: `model-${String(event._daemonEventId ?? index)}`,
          kind: "system",
          text: `Model changed: ${model.provider ?? "?"}/${model.id}`,
        })
    }
    if (event.type === "thinking_level_select") {
      const level = event.level as string | undefined
      if (level)
        items.push({
          id: `think-${String(event._daemonEventId ?? index)}`,
          kind: "system",
          text: `Thinking level: ${level}`,
        })
    }

    if (event.type === "tool_execution_start") {
      items.push({
        id: `tool-${String(event.toolCallId ?? event._daemonEventId ?? index)}`,
        kind: "tool",
        name: String(event.toolName ?? "tool"),
        done: false,
        startedAt: typeof event.timestamp === "string" ? event.timestamp : typeof event.timestamp === "number" ? event.timestamp : undefined,
        args: typeof event.toolArgs === "object" && event.toolArgs !== null ? event.toolArgs as Record<string, unknown> : undefined,
      })
    }

    if (
      event.type === "tool_execution_update" ||
      event.type === "tool_execution_end"
    ) {
      const id = `tool-${String(event.toolCallId ?? event._daemonEventId ?? index)}`
      const tool = items.find(
        (item): item is ToolItem => item.id === id
      )
      const result = (event.partialResult ?? event.result) as
        | { content?: Array<{ text?: string }> }
        | undefined
      const output = result?.content
        ?.map((part) => part.text ?? "")
        .join("")
      if (tool) {
        tool.output = output ?? tool.output
        tool.done = event.type === "tool_execution_end"
        if (event.type === "tool_execution_end") {
          tool.endedAt = typeof event.timestamp === "string" ? event.timestamp : typeof event.timestamp === "number" ? event.timestamp : undefined
          tool.failed = event.status === "failed" || event.status === "error"
        }
      } else {
        items.push({
          id,
          kind: "tool",
          name: String(event.toolName ?? "tool"),
          output,
          done: event.type === "tool_execution_end",
        })
      }
    }
  }

  return items.filter((item) => item.kind !== "assistant" || item.text)
}

/**
 * Builds a timeline from paginated history responses.
 */
export function buildHistory(
  response: Record<string, unknown> | undefined
): TimelineItem[] {
  const data = response?.data as
    | { messages?: Array<Record<string, unknown>> }
    | undefined
  return (data?.messages ?? []).flatMap(
    (message, index): TimelineItem[] => {
      const role = String(message.role ?? "")
      const timestamp = message.timestamp ?? index
      if (role === "user" || role === "assistant") {
        const text = contentText(message.content)
        return text
          ? [{ id: `${role}-${String(timestamp)}`, kind: role, text }]
          : []
      }
      if (role === "toolResult") {
        return [
          {
            id: `tool-${String(message.toolCallId ?? timestamp)}`,
            kind: "tool" as const,
            name: String(message.toolName ?? "tool"),
            output: contentText(message.content),
            done: true,
          },
        ]
      }
      if (role === "bashExecution") {
        return [
          {
            id: `bash-${String(timestamp)}`,
            kind: "tool" as const,
            name: String(message.command ?? "bash"),
            output: String(message.output ?? ""),
            done: true,
          },
        ]
      }
      return []
    }
  )
}

/**
 * Merges historical and live timeline items, with live items taking precedence.
 */
export function mergeTimeline(
  history: TimelineItem[],
  live: TimelineItem[]
): TimelineItem[] {
  const merged = new Map(history.map((item) => [item.id, item]))
  for (const item of live) merged.set(item.id, item)
  return [...merged.values()]
}

/**
 * Extracts the available models list from a session data response.
 */
export function responseModels(
  response: Record<string, unknown> | undefined
): AvailableModel[] {
  const data = response?.data as { models?: unknown } | undefined
  return Array.isArray(data?.models)
    ? data.models.filter(
        (model): model is AvailableModel =>
          typeof model === "object" &&
          model !== null &&
          "provider" in model &&
          "id" in model &&
          typeof (model as AvailableModel).provider === "string" &&
          typeof (model as AvailableModel).id === "string"
      )
    : []
}

/**
 * Groups models by their provider name.
 */
export function groupModelsByProvider(models: AvailableModel[]) {
  const groups = new Map<string, AvailableModel[]>()
  for (const model of models)
    groups.set(model.provider, [
      ...(groups.get(model.provider) ?? []),
      model,
    ])
  return [...groups.entries()]
}

/**
 * Finds the most recent pending extension UI request from event stream.
 */
export function findExtensionRequest(
  events: Array<Record<string, unknown>>
): ExtensionRequest | undefined {
  const event = [...events].reverse().find(
    (item) =>
      item.type === "extension_ui_request" &&
      item._daemonExtensionUiRequiresResponse === true
  )
  if (!event || typeof event.id !== "string") return undefined
  return {
    id: event.id,
    message:
      typeof event.message === "string"
        ? event.message
        : typeof event.text === "string"
          ? event.text
          : "Extension input requested",
    placeholder:
      typeof event.placeholder === "string" ? event.placeholder : undefined,
  }
}

/**
 * Semantic tool name mapping — maps raw tool names to friendly labels and icon keys.
 * Used for consistent tool display across platforms.
 */
export function toolDisplayName(name: string): { label: string; icon: string; category: string } {
  const lower = name.toLowerCase()
  if (lower === "bash" || lower === "terminal")
    return { label: "Terminal", icon: "terminal", category: "shell" }
  if (lower === "read" || lower === "read_file")
    return { label: "Read file", icon: "code", category: "file" }
  if (lower === "write" || lower === "edit" || lower === "apply_patch")
    return { label: "Edit file", icon: "code", category: "file" }
  if (lower === "find" || lower === "grep" || lower === "search")
    return { label: "Search", icon: "search", category: "search" }
  if (lower === "ls" || lower === "list" || lower === "list_directory")
    return { label: "Browse files", icon: "folder", category: "file" }
  return { label: name, icon: "code", category: "other" }
}

/**
 * Extracts a one-line summary of what a tool call did, suitable for display
 * inline in a tool row without expanding.
 *
 * Examples:
 * - bash: `npm install`
 * - read: `src/components/Button.tsx`
 * - edit: `src/api/client.ts` (3 changes)
 * - search: `"useEffect" in src/` (12 results)
 * - ls: `src/`
 */
export function extractToolSummary(
  toolName: string,
  args?: Record<string, unknown>,
  output?: string
): string {
  const { category } = toolDisplayName(toolName)
  const lower = toolName.toLowerCase()

  if (category === "shell") {
    const cmd = args?.command ?? args?.cmd
    if (typeof cmd === "string") {
      const firstLine = cmd.split("\n")[0].trim()
      return firstLine.length > 60 ? firstLine.slice(0, 57) + "…" : firstLine
    }
    return ""
  }

  if (lower === "read" || lower === "read_file") {
    const path = args?.file_path ?? args?.path ?? args?.filePath
    if (typeof path === "string") return fileName(path)
    return ""
  }

  if (lower === "write" || lower === "edit" || lower === "apply_patch") {
    const path = args?.file_path ?? args?.path ?? args?.filePath
    const name = typeof path === "string" ? fileName(path) : ""
    // Try to count changes from the output/diff
    const changes = countDiffChanges(output)
    return name + (changes > 0 ? ` (${changes} change${changes > 1 ? "s" : ""})` : "")
  }

  if (category === "search") {
    const query = args?.query ?? args?.pattern ?? args?.regex
    const path = args?.path ?? args?.directory
    const q = typeof query === "string" ? `"${query}"` : ""
    const p = typeof path === "string" ? ` in ${fileName(path)}` : ""
    const results = countSearchResults(output)
    return q + p + (results > 0 ? ` (${results} result${results > 1 ? "s" : ""})` : "")
  }

  if (lower === "ls" || lower === "list" || lower === "list_directory") {
    const path = args?.path ?? args?.directory
    if (typeof path === "string") return fileName(path)
    return ""
  }

  return ""
}

/** Extracts the last path segment from a file path. */
function fileName(path: string): string {
  const parts = path.replace(/\\\\/g, "/").split("/").filter(Boolean)
  return parts.at(-1) ?? path
}

/** Counts changed lines from a diff/patch output. */
function countDiffChanges(output?: string): number {
  if (!output) return 0
  let count = 0
  for (const line of output.split("\n")) {
    if (line.startsWith("+") && !line.startsWith("+++")) count++
    if (line.startsWith("-") && !line.startsWith("---")) count++
  }
  return count
}

/** Counts search result lines from grep/search output. */
function countSearchResults(output?: string): number {
  if (!output) return 0
  // Count non-empty lines that look like file:line matches
  return output.split("\n").filter((line) => line.trim().length > 0 && /:.+/.test(line)).length
}

/**
 * Formats a duration in milliseconds to a human-readable string.
 * - Under 1s: "12ms"
 * - Under 60s: "4.2s"
 * - 60s+: "1:23"
 */
export function formatDuration(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  const minutes = Math.floor(ms / 60_000)
  const seconds = Math.floor((ms % 60_000) / 1000)
  return `${minutes}:${seconds.toString().padStart(2, "0")}`
}

/**
 * Calculates the duration of a tool execution from its timestamps.
 * Returns undefined if timestamps are missing.
 */
export function toolDuration(
  startedAt?: string | number,
  endedAt?: string | number
): number | undefined {
  if (!startedAt) return undefined
  const start = typeof startedAt === "string" ? new Date(startedAt).getTime() : startedAt
  const end = endedAt
    ? typeof endedAt === "string"
      ? new Date(endedAt).getTime()
      : endedAt
    : Date.now()
  const duration = end - start
  return duration >= 0 ? duration : undefined
}
