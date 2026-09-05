import type {
  TimelineItem,
  TextItem,
  ToolItem,
  AvailableModel,
  ExtensionRequest,
  TimelineRun,
  TimelineImage,
} from "./types"

// Path segments that should never surface as "File changed:" system messages
// in the chat. These are git/editor/build/dependency internals that generate a
// flood of irrelevant file_change events (e.g. .git/*, .wrangler/tmp).
const NOISE_FILE_SEGMENTS = new Set([
  ".git",
  ".wrangler",
  "node_modules",
  ".next",
  ".output",
  ".turbo",
  ".cache",
  ".pnpm-store",
  ".venv",
  ".idea",
  ".vscode",
  "dist",
  "build",
  "coverage",
  "target",
  "vendor",
  "__pycache__",
])

// Exact filenames (any directory) that should never be reported as file changes.
const NOISE_FILE_NAMES = new Set([".DS_Store", "Thumbs.db", "desktop.ini"])

/** Returns true for file paths that should be hidden from the change feed. */
export function isNoiseFilePath(path: string): boolean {
  const segments = path.replace(/\\/g, "/").split("/")
  const leaf = segments[segments.length - 1]
  if (NOISE_FILE_NAMES.has(leaf)) return true
  return segments.some((segment) => NOISE_FILE_SEGMENTS.has(segment))
}

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

/** Extracts image blocks from a Pi message content array. */
export function contentImages(content: unknown): TimelineImage[] {
  if (!Array.isArray(content)) return []
  return content.flatMap((part): TimelineImage[] => {
    if (typeof part !== "object" || part === null) return []
    const image = part as { type?: unknown; data?: unknown; mimeType?: unknown }
    return image.type === "image" && typeof image.data === "string" && typeof image.mimeType === "string"
      ? [{ type: "image", data: image.data, mimeType: image.mimeType }]
      : []
  })
}

/**
 * Builds a timeline from live WebSocket events.
 * Processes message_start, message_update, message_end, tool_execution_*,
 * file_change, model_select, and thinking_level_select events.
 */
export class IncrementalTimeline {
  private items: TimelineItem[] = []
  private toolIndexes = new Map<string, number>()
  private activeAssistant: TextItem | undefined
  private activeAssistantIndex = -1
  private runs = new Map<string, TimelineRun>()
  private processed = 0
  private lastEventId: unknown
  private lastEvent: Record<string, unknown> | undefined

  update(events: Array<Record<string, unknown>>, maxItems = adaptiveTimelineLimit()): TimelineItem[] {
    // Callers append to the event array. A shorter array means it was replaced
    // during recovery, so rebuild; otherwise the processed cursor is O(1).
    let start = this.processed
    if (events.length < this.processed) { this.reset(); start = 0 }
    for (let index = start; index < events.length; index += 1) this.apply(events[index], index)
    this.processed = events.length
    this.lastEvent = events.at(-1)
    this.lastEventId = this.lastEvent?._daemonEventId
    if (this.items.length > maxItems) {
      const retained = this.items.slice(-maxItems)
      if (this.activeAssistant && !retained.includes(this.activeAssistant)) {
        retained.shift()
        retained.unshift(this.activeAssistant)
      }
      this.items = retained
      this.activeAssistantIndex = this.activeAssistant ? this.items.indexOf(this.activeAssistant) : -1
      this.reindexTools()
      this.pruneRuns()
    }
    return this.items.filter((item) => item.kind !== "assistant" || item.text)
  }

  getRuns(): ReadonlyMap<string, TimelineRun> { return this.runs }

  reset() {
    this.items = []
    this.toolIndexes.clear()
    this.runs.clear()
    this.activeAssistant = undefined
    this.activeAssistantIndex = -1
    this.processed = 0
    this.lastEventId = undefined
    this.lastEvent = undefined
  }

  private apply(event: Record<string, unknown>, index: number) {
    const taskId = typeof event._daemonTaskId === "string" ? event._daemonTaskId : undefined
    const runId = typeof event._daemonRunId === "string" ? event._daemonRunId : undefined
    if (runId && event.type === "agent_start") this.runs.set(runId, { id: runId, taskId, started: true, settled: false })
    if (runId && (event.type === "agent_end" || event.type === "agent_settled")) {
      const run = this.runs.get(runId) ?? { id: runId, taskId, started: true, settled: false }
      this.runs.set(runId, { ...run, settled: true })
    }
    if (event.type === "message_start") {
      const message = event.message as { role?: string; content?: unknown; images?: unknown; timestamp?: number } | undefined
      const id = `${message?.role ?? "message"}-${message?.timestamp ?? event._daemonEventId ?? index}`
      if (message?.role === "user") {
        const text = contentText(message.content)
        const images = contentImages(message.content).concat(contentImages(message.images))
        if (text || images.length > 0) this.items.push({ id, kind: "user", text, images: images.length > 0 ? images : undefined, taskId, runId })
      } else if (message?.role === "assistant") {
        this.activeAssistant = { id, kind: "assistant", text: "", streaming: true, taskId, runId }
        this.items.push(this.activeAssistant)
        this.activeAssistantIndex = this.items.length - 1
      }
    } else if (event.type === "message_update") {
      const delta = event.assistantMessageEvent as { type?: string; delta?: string } | undefined
      if (delta?.type === "text_delta") {
        if (!this.activeAssistant) {
          this.activeAssistant = { id: `assistant-${event._daemonEventId ?? index}`, kind: "assistant", text: "", streaming: true, taskId, runId }
          this.items.push(this.activeAssistant)
          this.activeAssistantIndex = this.items.length - 1
        } else {
          // Replace the object (new reference) so memoized row components
          // re-render on each delta instead of bailing out on the same ref.
          const updated = { ...this.activeAssistant, text: this.activeAssistant.text + (delta.delta ?? "") }
          this.activeAssistant = updated
          if (this.activeAssistantIndex >= 0) this.items[this.activeAssistantIndex] = updated
        }
      }
    } else if (event.type === "message_end") {
      if (this.activeAssistant) {
        const settled = { ...this.activeAssistant, streaming: false }
        if (this.activeAssistantIndex >= 0) this.items[this.activeAssistantIndex] = settled
        this.activeAssistant = undefined
        this.activeAssistantIndex = -1
      }
    }

    if (event.type === "file_change" && event.path && !isNoiseFilePath(event.path)) this.items.push({ id: `file-${String(event._daemonEventId ?? index)}`, kind: "system", text: `File ${event.change ?? "changed"}: ${event.path}`, taskId, runId })
    if (event.type === "tool_execution_start") {
      const id = `tool-${String(event.toolCallId ?? event._daemonEventId ?? index)}`
      this.items.push({ id, kind: "tool", name: String(event.toolName ?? "tool"), done: false, startedAt: typeof event.timestamp === "string" || typeof event.timestamp === "number" ? event.timestamp : undefined, args: typeof event.args === "object" && event.args !== null ? event.args as Record<string, unknown> : undefined, taskId, runId })
      this.toolIndexes.set(id, this.items.length - 1)
    }
    if (event.type === "tool_execution_update" || event.type === "tool_execution_end") {
      const id = `tool-${String(event.toolCallId ?? event._daemonEventId ?? index)}`
      const at = this.toolIndexes.get(id)
      const tool = at === undefined ? undefined : this.items[at] as ToolItem
      const result = (event.partialResult ?? event.result) as { content?: Array<{ text?: string }> } | undefined
      const output = result?.content?.map((part) => part.text ?? "").join("")
      if (tool) {
        const updated: ToolItem = {
          ...tool,
          output: output ?? tool.output,
          done: event.type === "tool_execution_end",
        }
        if (updated.done) {
          updated.endedAt = typeof event.timestamp === "string" || typeof event.timestamp === "number" ? event.timestamp : undefined
          updated.failed = event.status === "failed" || event.status === "error"
        }
        // Replace with a new reference so memoized rows re-render on stream.
        this.items[at] = updated
      } else {
        this.items.push({ id, kind: "tool", name: String(event.toolName ?? "tool"), output, done: event.type === "tool_execution_end", taskId, runId })
        this.toolIndexes.set(id, this.items.length - 1)
      }
    }
  }

  private reindexTools() {
    this.toolIndexes.clear()
    this.items.forEach((item, index) => { if (item.kind === "tool") this.toolIndexes.set(item.id, index) })
  }

  private pruneRuns() {
    const retained = new Set(this.items.map((item) => item.runId).filter((id): id is string => Boolean(id)))
    for (const [id, run] of this.runs) {
      if (run.settled && !retained.has(id)) this.runs.delete(id)
    }
  }
}

export class TimelineStore {
  private readonly reducer = new IncrementalTimeline()
  private snapshot: TimelineItem[] = []
  private listeners = new Set<() => void>()

  constructor(readonly sessionId: string) {}

  update(events: Array<Record<string, unknown>>): void {
    this.snapshot = this.reducer.update(events)
    for (const listener of this.listeners) listener()
  }

  getSnapshot = (): TimelineItem[] => this.snapshot
  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener)
    return () => this.listeners.delete(listener)
  }
}


export function adaptiveTimelineLimit(): number {
  const memory = typeof navigator === "undefined" ? undefined : (navigator as Navigator & { deviceMemory?: number }).deviceMemory
  if (memory !== undefined && memory <= 4) return 250
  if (memory !== undefined && memory >= 12) return 1000
  return 500
}

export function buildTimeline(events: Array<Record<string, unknown>>): TimelineItem[] {
  return new IncrementalTimeline().update(events, Number.MAX_SAFE_INTEGER)
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
  const toolCalls = new Map<string, { name?: string; args?: Record<string, unknown> }>()
  return (data?.messages ?? []).flatMap(
    (message, index): TimelineItem[] => {
      const role = String(message.role ?? "")
      const historyType = String(message._historyType ?? "")
      if (historyType === "tool_use") {
        const id = String(message.id ?? `history-${index}`)
        const name = String(message.name ?? "tool")
        const args = typeof message.input === "object" && message.input !== null
          ? message.input as Record<string, unknown>
          : undefined
        toolCalls.set(id, { name, args })
        return [{ id: `tool-${id}`, kind: "tool", name, args, done: false }]
      }
      if (historyType === "tool_result" || role === "tool") {
        const id = String(message.toolCallId ?? message.tool_use_id ?? message.id ?? `history-${index}`)
        const call = toolCalls.get(id)
        return [{
          id: `tool-${id}`,
          kind: "tool",
          name: String(message.toolName ?? call?.name ?? "tool"),
          args: call?.args,
          output: contentText(message.content),
          done: true,
        }]
      }
      const assistantToolCalls = role === "assistant" ? extractToolCalls(message.content) : []
      if (role === "assistant") rememberToolCalls(message.content, toolCalls)
      const timestamp = message.timestamp ?? index
      if (role === "user" || role === "assistant") {
        const text = contentText(message.content)
        const images = role === "user" ? contentImages(message.content).concat(contentImages(message.images)) : []
        const items: TimelineItem[] = text || images.length > 0
          ? [{ id: `${role}-${String(timestamp)}`, kind: role, text, images: images.length > 0 ? images : undefined }]
          : []
        if (role === "assistant") {
          items.push(...assistantToolCalls.map((call) => ({
            id: `tool-${call.id}`,
            kind: "tool" as const,
            name: call.name,
            args: call.args,
            done: false,
          })))
        }
        return items
      }
      if (role === "toolResult") {
        const toolCallId = String(message.toolCallId ?? "")
        const toolCall = toolCalls.get(toolCallId)
        return [
          {
            id: `tool-${String(message.toolCallId ?? timestamp)}`,
            kind: "tool" as const,
            name: String(message.toolName ?? toolCall?.name ?? "tool"),
            args: toolCall?.args,
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

type HistoryToolCall = { id: string; name: string; args?: Record<string, unknown> }

function extractToolCalls(content: unknown): HistoryToolCall[] {
  if (!Array.isArray(content)) return []
  return content.flatMap((part): HistoryToolCall[] => {
    if (typeof part !== "object" || part === null) return []
    const toolCall = part as { type?: unknown; id?: unknown; name?: unknown; arguments?: unknown; input?: unknown }
    if (toolCall.type !== "toolCall" && toolCall.type !== "tool_use") return []
    if (typeof toolCall.id !== "string") return []
    return [{
      id: toolCall.id,
      name: typeof toolCall.name === "string" ? toolCall.name : "tool",
      args: typeof toolCall.arguments === "object" && toolCall.arguments !== null
        ? toolCall.arguments as Record<string, unknown>
        : typeof toolCall.input === "object" && toolCall.input !== null
          ? toolCall.input as Record<string, unknown>
          : undefined,
    }]
  })
}

function rememberToolCalls(
  content: unknown,
  toolCalls: Map<string, { name?: string; args?: Record<string, unknown> }>
) {
  for (const toolCall of extractToolCalls(content)) {
    toolCalls.set(toolCall.id, { name: toolCall.name, args: toolCall.args })
  }
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
  const closedIds = new Set<string>()
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const candidate = events[index]
    if (candidate.type === "extension_ui_closed" && typeof candidate.id === "string") {
      closedIds.add(candidate.id)
      continue
    }
    if (
      candidate.type !== "extension_ui_request" ||
      candidate._daemonExtensionUiRequiresResponse !== true ||
      typeof candidate.id !== "string" ||
      closedIds.has(candidate.id)
    ) continue
    return {
      id: candidate.id,
      message:
        typeof candidate.question === "string"
          ? candidate.question
          : typeof candidate.message === "string"
            ? candidate.message
            : typeof candidate.text === "string"
              ? candidate.text
              : "Extension input requested",
      placeholder:
        typeof candidate.placeholder === "string" ? candidate.placeholder : undefined,
    }
  }
  return undefined
}

/**
 * Semantic tool name mapping — maps raw tool names to friendly labels and icon keys.
 * Used for consistent tool display across platforms.
 */
export function toolDisplayName(name: string): { label: string; icon: string; category: string } {
  const lower = name.toLowerCase()
  if (lower === "bash" || lower === "terminal")
    return { label: "bash", icon: "terminal", category: "shell" }
  if (lower === "read" || lower === "read_file")
    return { label: "readFile", icon: "code", category: "file" }
  if (lower === "write")
    return { label: "writeFile", icon: "code", category: "file" }
  if (lower === "edit" || lower === "apply_patch")
    return { label: "editFile", icon: "code", category: "file" }
  if (lower === "find" || lower === "grep" || lower === "search")
    return { label: "searchCode", icon: "search", category: "search" }
  if (lower === "ls" || lower === "list" || lower === "list_directory")
    return { label: "listFiles", icon: "folder", category: "file" }
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
