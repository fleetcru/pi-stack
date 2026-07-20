import { memo, useCallback, useMemo, useState } from "react"
import { ArrowUp, Bot, Brain, ChevronRight, Sparkles, Square, Terminal } from "lucide-react"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import rehypeSanitize from "rehype-sanitize"

import {
  useActiveSessionSocket,
  usePiServerClient,
  useSessionData,
  useSessionHistory,
} from "@/api/hooks"
import { Bubble, BubbleContent, BubbleGroup } from "@/components/ui/bubble"
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible"
import {
  Empty,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupTextarea,
} from "@/components/ui/input-group"
import { Marker, MarkerContent, MarkerIcon } from "@/components/ui/marker"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Message, MessageContent, MessageGroup } from "@/components/ui/message"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"

export function SessionWorkspace({ sessionId }: { sessionId: string }) {
  const [prompt, setPrompt] = useState("")
  const [deliveryNotice, setDeliveryNotice] = useState<string | undefined>()
  const [deliveryCommandId, setDeliveryCommandId] = useState<string | undefined>()
  const client = usePiServerClient()
  const socket = useActiveSessionSocket(sessionId)
  const historyQuery = useSessionHistory(sessionId)
  const modelsQuery = useSessionData(sessionId, "models")
  const stateQuery = useSessionData(sessionId, "state", {
    // Poll only when the WebSocket is not open — the stream provides live state.
    refetchInterval: socket.status === "open" ? false : 5_000,
  })
  const [extensionValue, setExtensionValue] = useState("")
  const [ignoredExtensionIds, setIgnoredExtensionIds] = useState<string[]>([])
  const ignoreExtension = useCallback((id: string) => {
    setIgnoredExtensionIds((prev) => {
      const next = [...prev, id]
      // Cap at 100 to prevent unbounded growth in long sessions.
      return next.length > 100 ? next.slice(-100) : next
    })
  }, [])
  const [extensionDialogOpen, setExtensionDialogOpen] = useState(false)
  const extension = useMemo(() => findExtensionRequest(socket.events), [socket.events])
  const visibleExtension = extension && !ignoredExtensionIds.includes(extension.id) ? extension : undefined
  const timeline = useMemo(() => {
    const history = historyQuery.data?.pages
      .slice()
      .reverse()
      .flatMap((page) => buildHistory(page)) ?? []
    const live = buildTimeline(socket.events)
    return mergeTimeline(history, live)
  }, [historyQuery.data, socket.events])

  const deliveryStage = useMemo(() => {
    if (!deliveryCommandId) return deliveryNotice
    const receiptIndex = socket.events.findLastIndex((event) => event.type === "bridge_receipt" && event.commandId === deliveryCommandId)
    if (receiptIndex < 0) return deliveryNotice
    const responding = socket.events.slice(receiptIndex + 1).some((event) => event.type === "message_start" && (event.message as { role?: string } | undefined)?.role === "assistant")
    return responding ? "Pi responding…" : "Delivered to Pi"
  }, [deliveryCommandId, deliveryNotice, socket.events])

  const models = responseModels(modelsQuery.data)
  const state = stateQuery.data?.data as { model?: { provider?: string; id?: string }; thinkingLevel?: string; isStreaming?: boolean; external?: boolean; relayConnected?: boolean; relayLatencyMs?: number } | undefined
  const isWorking = state?.isStreaming === true
  const relayStatus = state?.external
    ? state.relayConnected
      ? `Relay connected${typeof state.relayLatencyMs === "number" ? ` · ${state.relayLatencyMs} ms` : ""}`
      : "Relay disconnected — commands queue on the server"
    : state ? "Local RPC" : undefined

  async function abortSession() {
    try {
      await client.abort(sessionId)
      setDeliveryNotice("Stopping Pi…")
    } catch (error) {
      setDeliveryNotice(error instanceof Error ? error.message : "Could not stop Pi")
    }
  }

  async function sendPrompt(mode: "prompt" | "steer" = "prompt") {
    const message = prompt.trim()
    if (!message) return
    try {
      setDeliveryNotice("Sending…")
      const response = mode === "steer" ? await client.steer(sessionId, { message }) : await client.prompt(sessionId, { message })
      const commandId = (response as Record<string, unknown>).commandId
      setDeliveryCommandId(typeof commandId === "string" ? commandId : undefined)
      setPrompt("")
      setDeliveryNotice(mode === "steer" ? "Steering Pi now…" : "Sent to bridged Pi. It will arrive at the next safe turn boundary.")
    } catch (error) {
      setDeliveryNotice(error instanceof Error ? error.message : "Could not send message to Pi")
    }
  }

  return (
    <MessageScrollerProvider
      autoScroll
      defaultScrollPosition="last-anchor"
      scrollPreviousItemPeek={32}
      scrollMargin={24}
    >
      {visibleExtension && (
        <div role="status" className="mx-5 mt-3 flex items-center gap-2 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-xs text-foreground">
          <span className="min-w-0 flex-1 truncate">Pi extension request waiting: {visibleExtension.message}</span>
          <Button size="xs" variant="outline" onClick={() => setExtensionDialogOpen(true)}>Review</Button>
          <Button size="xs" variant="ghost" onClick={() => ignoreExtension(visibleExtension.id)}>Ignore</Button>
        </div>
      )}
      {visibleExtension && (
        <Dialog open={extensionDialogOpen} onOpenChange={setExtensionDialogOpen}>
          <DialogContent>
            <DialogHeader><DialogTitle>Pi extension request</DialogTitle><DialogDescription>{visibleExtension.message}</DialogDescription></DialogHeader>
            <p className="text-xs leading-5 text-muted-foreground">Request ID: {visibleExtension.id}. Only respond if you expected this extension prompt. A reconnect can replay an older request; Ignore hides it locally without approving or cancelling it.</p>
            <InputGroup><InputGroupTextarea value={extensionValue} onChange={(event) => setExtensionValue(event.target.value)} placeholder={visibleExtension.placeholder || "Response"} /></InputGroup>
            <DialogFooter>
              <Button variant="ghost" onClick={() => { ignoreExtension(visibleExtension.id); setExtensionDialogOpen(false) }}>Ignore</Button>
              <Button variant="outline" onClick={() => { void client.sessionPost(sessionId, "ui-response", { id: visibleExtension.id, cancelled: true }); ignoreExtension(visibleExtension.id); setExtensionDialogOpen(false) }}>Cancel</Button>
              <Button onClick={() => { void client.sessionPost(sessionId, "ui-response", { id: visibleExtension.id, value: extensionValue || undefined, confirmed: true }); ignoreExtension(visibleExtension.id); setExtensionDialogOpen(false); setExtensionValue("") }}>Confirm</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
      <div className="flex min-h-0 flex-1 flex-col">
        <MessageScroller>
          <MessageScrollerViewport>
            <MessageScrollerContent
              aria-busy={socket.status !== "open"}
              className="mx-auto w-full max-w-3xl gap-8 px-8 py-10 select-text"
            >
              {timeline.length === 0 ? (
                <Empty className="border-0">
                  <EmptyHeader>
                    <EmptyMedia variant="icon">
                      <Bot />
                    </EmptyMedia>
                    <EmptyTitle>
                      {historyQuery.isLoading
                        ? "Loading conversation…"
                        : "Ready when you are"}
                    </EmptyTitle>
                    <EmptyDescription>
                      {historyQuery.isLoading
                        ? "Restoring this session's message history."
                        : "Give Pi a task, question, or file path to begin."}
                    </EmptyDescription>
                  </EmptyHeader>
                </Empty>
              ) : (
                <MessageGroup className="w-full gap-5">
                  {historyQuery.hasNextPage && (
                    <div className="flex justify-center">
                      <Button variant="outline" size="sm" disabled={historyQuery.isFetchingNextPage} onClick={() => void historyQuery.fetchNextPage()}>
                        {historyQuery.isFetchingNextPage ? "Loading older messages…" : "Load older messages"}
                      </Button>
                    </div>
                  )}
                  {timeline.map((item) => (
                    <MessageScrollerItem
                      className="w-full"
                      key={item.id}
                      messageId={item.id}
                      scrollAnchor={item.kind === "assistant"}
                    >
                      <TimelineRow item={item} />
                    </MessageScrollerItem>
                  ))}
                </MessageGroup>
              )}
            </MessageScrollerContent>
          </MessageScrollerViewport>
          <MessageScrollerButton />
        </MessageScroller>

        <div className="px-5 pt-2 pb-5">
          <form
            className="mx-auto w-full max-w-3xl"
            onSubmit={(event) => {
              event.preventDefault()
              void sendPrompt()
            }}
          >
            <InputGroup className="rounded-2xl bg-muted/70 shadow-sm">
              <InputGroupTextarea
                className="max-h-32 min-h-14 px-4 pt-3 text-sm leading-6 select-text"
                value={prompt}
                onChange={(event) => setPrompt(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter" && !event.shiftKey) {
                    event.preventDefault()
                    void sendPrompt()
                  }
                }}
                placeholder="Ask the agent anything…"
              />
              <InputGroupAddon align="block-end" className="w-full px-3 pt-0 pb-2.5">
                <div className="flex w-full items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2 overflow-x-auto">
                <Select
                  value={state?.model?.provider && state?.model?.id ? `${state.model.provider}:${state.model.id}` : null}
                  onValueChange={(value) => {
                    const [provider, modelId] = String(value).split(":")
                    if (provider && modelId) void client.sessionPost(sessionId, "model", { provider, modelId }).then(() => void stateQuery.refetch()).catch((error: unknown) => setDeliveryNotice(error instanceof Error ? error.message : "Could not change model"))
                  }}
                >
                  <SelectTrigger size="sm" className="min-w-40 max-w-56 rounded-xl border-border/70 bg-muted/70 text-foreground shadow-none hover:bg-muted">
                    <Sparkles className="size-3.5 text-primary" />
                    <SelectValue placeholder="Choose model" />
                  </SelectTrigger>
                  <SelectContent align="start">
                    {groupModelsByProvider(models).map(([provider, providerModels]) => (
                      <SelectGroup key={provider}>
                        <SelectLabel className="font-semibold tracking-wide uppercase">{provider}</SelectLabel>
                        {providerModels.map((model) => <SelectItem key={`${model.provider}:${model.id}`} value={`${model.provider}:${model.id}`}>{model.name || model.id}</SelectItem>)}
                      </SelectGroup>
                    ))}
                  </SelectContent>
                </Select>
                <Select
                  value={state?.thinkingLevel ?? null}
                  onValueChange={(value) => void client.sessionPost(sessionId, "thinking-level", { level: String(value) }).then(() => void stateQuery.refetch()).catch((error: unknown) => setDeliveryNotice(error instanceof Error ? error.message : "Could not change thinking level"))}
                >
                  <SelectTrigger size="sm" className="min-w-32 rounded-xl border-border/70 bg-muted/70 text-foreground shadow-none hover:bg-muted">
                    <Brain className="size-3.5 text-primary" />
                    <SelectValue placeholder="Thinking / effort" />
                  </SelectTrigger>
                  <SelectContent align="start">
                    <SelectGroup>
                      <SelectLabel>Thinking / effort</SelectLabel>
                      {['off', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max'].map((level) => <SelectItem key={level} value={level}>{level}</SelectItem>)}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                  </div>
                <InputGroupButton
                  type="button"
                  size="sm"
                  variant="outline"
                  className="rounded-xl"
                  disabled={!prompt.trim()}
                  onClick={() => void sendPrompt("steer")}
                >
                  Steer
                </InputGroupButton>
                <InputGroupButton
                  type={isWorking ? "button" : "submit"}
                  size="icon-sm"
                  className={`ml-auto rounded-full text-white ${isWorking ? "bg-red-600 hover:bg-red-500" : "bg-blue-600 hover:bg-blue-500"}`}
                  disabled={isWorking ? false : !prompt.trim()}
                  onClick={isWorking ? () => void abortSession() : undefined}
                  aria-label={isWorking ? "Stop Pi" : "Send prompt"}
                  title={isWorking ? "Stop Pi" : "Send prompt"}
                >
                  {isWorking ? <Square /> : <ArrowUp />}
                </InputGroupButton>
                </div>
              </InputGroupAddon>
            </InputGroup>
          </form>
          {relayStatus && (
            <p className={`mx-auto mt-2 max-w-3xl text-xs ${!state?.external || state?.relayConnected ? "text-muted-foreground" : "text-amber-600 dark:text-amber-400"}`} role="status">
              {relayStatus}
            </p>
          )}
          {deliveryStage && <p className="mx-auto mt-2 max-w-3xl text-xs text-muted-foreground" role="status">{deliveryStage}</p>}
          {socket.error && (
            <p className="mx-auto mt-2 max-w-3xl text-xs text-destructive">
              {socket.error.message}
            </p>
          )}
        </div>
      </div>
    </MessageScrollerProvider>
  )
}

export default SessionWorkspace

function ChatTurn({ item }: { item: TextItem }) {
  const user = item.kind === "user"
  return (
    <Message align={user ? "end" : "start"} className="w-full">
      <MessageContent className="w-full">
        <BubbleGroup
          className={user ? "w-full items-end" : "w-full items-start"}
        >
          <Bubble
            align={user ? "end" : "start"}
            variant={user ? "secondary" : "ghost"}
            className={user ? "w-auto max-w-[78%] min-w-fit" : "w-full max-w-full"}
          >
            <BubbleContent
              className={`max-w-full text-sm leading-6 [overflow-wrap:normal] ${user ? "w-auto whitespace-pre-wrap bg-muted/80" : "w-full text-foreground"}`}
            >
              {user ? (
                item.text
              ) : (
                <DeferredMarkdown text={item.text || "Thinking…"} />
              )}
            </BubbleContent>
          </Bubble>
        </BubbleGroup>
      </MessageContent>
    </Message>
  )
}

const largeRenderThreshold = 60_000
const markdownHardCap = 256_000
const largeToolOutputThreshold = 80_000

function DeferredMarkdown({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(text.length <= largeRenderThreshold)
  if (text.length > markdownHardCap) {
    // Beyond the hard cap, render as plain <pre> to avoid freezing the browser
    // during Markdown parsing of very large tool outputs (1MB+).
    return <pre className="max-h-96 overflow-auto whitespace-pre-wrap text-xs text-muted-foreground">{text}</pre>
  }
  if (!expanded) {
    return <div className="space-y-2"><pre className="max-h-48 overflow-hidden whitespace-pre-wrap text-xs text-muted-foreground">{text.slice(0, 4_000)}…</pre><Button size="sm" variant="outline" onClick={() => setExpanded(true)}>Render full response ({Math.ceil(text.length / 1024)} KB)</Button></div>
  }
  return <MarkdownResponse>{text}</MarkdownResponse>
}

function MarkdownResponse({ children }: { children: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeSanitize]}
      components={{
        a: (props) => (
          <a
            className="underline underline-offset-4 hover:text-primary"
            target="_blank"
            rel="noreferrer"
            {...props}
          />
        ),
        h1: (props) => <h1 className="mt-6 mb-3 text-xl font-semibold tracking-tight first:mt-0" {...props} />,
        h2: (props) => <h2 className="mt-5 mb-2 text-lg font-semibold tracking-tight first:mt-0" {...props} />,
        h3: (props) => <h3 className="mt-4 mb-2 text-base font-semibold first:mt-0" {...props} />,
        code: (props) => (
          <code
            className="rounded-md border border-border/60 bg-muted/60 px-1.5 py-0.5 font-mono text-[0.88em] text-foreground"
            {...props}
          />
        ),
        pre: (props) => (
          <pre
            className="my-4 max-w-full overflow-auto rounded-xl border border-border/70 bg-muted/45 p-4 font-mono text-xs leading-5 shadow-sm"
            {...props}
          />
        ),
        ul: (props) => (
          <ul className="my-2 list-disc space-y-1 pl-5" {...props} />
        ),
        ol: (props) => (
          <ol className="my-2 list-decimal space-y-1 pl-5" {...props} />
        ),
        blockquote: (props) => <blockquote className="my-3 border-l-2 border-primary/35 pl-3 text-muted-foreground italic" {...props} />,
        table: (props) => <div className="my-3 overflow-x-auto"><table className="w-full border-collapse text-sm" {...props} /></div>,
        th: (props) => <th className="border-b border-border px-3 py-2 text-left font-medium" {...props} />,
        td: (props) => <td className="border-b border-border/60 px-3 py-2 align-top" {...props} />,
        p: (props) => <p className="my-2.5 first:mt-0 last:mb-0" {...props} />,
      }}
    >
      {children}
    </ReactMarkdown>
  )
}

function ToolTurn({ item }: { item: ToolItem }) {
  return (
    <Message className="w-full">
      <MessageContent className="w-full">
        <Collapsible className="w-full">
          <CollapsibleTrigger className="group w-fit max-w-full py-1">
            <Marker className="w-fit max-w-full gap-2 text-xs">
              <MarkerIcon>
                <Terminal />
              </MarkerIcon>
              <MarkerContent className="flex items-center gap-2">
                <span className="font-medium text-foreground/80">
                  {item.name}
                </span>
                <span>{item.done ? "Complete" : "Running"}</span>
              </MarkerContent>
              <ChevronRight className="size-3.5 shrink-0 transition-transform group-data-open:rotate-90" />
            </Marker>
          </CollapsibleTrigger>
          <CollapsibleContent>
            <div className="mx-5 mt-2 rounded-lg border border-border bg-muted/30 p-3">
              {item.output ? (
                <div className="space-y-2">
                  <pre className="max-h-48 overflow-auto font-mono text-xs whitespace-pre-wrap text-foreground/80">
                    {item.output.slice(0, largeToolOutputThreshold)}
                  </pre>
                  {item.output.length > largeToolOutputThreshold && <p className="text-xs text-muted-foreground">Output preview limited to {Math.ceil(largeToolOutputThreshold / 1024)} KB.</p>}
                </div>
              ) : (
                <p className="text-xs text-muted-foreground">
                  Waiting for output…
                </p>
              )}
            </div>
          </CollapsibleContent>
        </Collapsible>
      </MessageContent>
    </Message>
  )
}

type TextItem = { id: string; kind: "user" | "assistant"; text: string }
type ToolItem = {
  id: string
  kind: "tool"
  name: string
  output?: string
  done: boolean
}
type TimelineItem = TextItem | ToolItem

// The composer updates on every keystroke. Keep historical Markdown/tool rows
// out of that render path unless their own item object actually changed.
const TimelineRow = memo(function TimelineRow({ item }: { item: TimelineItem }) {
  return item.kind === "tool" ? <ToolTurn item={item} /> : <ChatTurn item={item} />
})

function buildTimeline(events: Array<Record<string, unknown>>): TimelineItem[] {
  const items: TimelineItem[] = []
  let activeAssistant: TextItem | undefined

  for (const [index, event] of events.entries()) {
    if (event.type === "message_start") {
      const message = event.message as
        { role?: string; content?: unknown; timestamp?: number } | undefined
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
        { type?: string; delta?: string } | undefined
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

    if (event.type === "tool_execution_start") {
      items.push({
        id: `tool-${String(event.toolCallId ?? event._daemonEventId ?? index)}`,
        kind: "tool",
        name: String(event.toolName ?? "tool"),
        done: false,
      })
    }

    if (
      event.type === "tool_execution_update" ||
      event.type === "tool_execution_end"
    ) {
      const id = `tool-${String(event.toolCallId ?? event._daemonEventId ?? index)}`
      const tool = items.find((item): item is ToolItem => item.id === id)
      const result = (event.partialResult ?? event.result) as
        { content?: Array<{ text?: string }> } | undefined
      const output = result?.content?.map((part) => part.text ?? "").join("")
      if (tool) {
        tool.output = output ?? tool.output
        tool.done = event.type === "tool_execution_end"
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

function buildHistory(
  response: Record<string, unknown> | undefined
): TimelineItem[] {
  const data = response?.data as
    { messages?: Array<Record<string, unknown>> } | undefined
  return (data?.messages ?? []).flatMap((message, index): TimelineItem[] => {
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
          kind: "tool",
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
          kind: "tool",
          name: String(message.command ?? "bash"),
          output: String(message.output ?? ""),
          done: true,
        },
      ]
    }
    return []
  })
}

function mergeTimeline(
  history: TimelineItem[],
  live: TimelineItem[]
): TimelineItem[] {
  const merged = new Map(history.map((item) => [item.id, item]))
  for (const item of live) merged.set(item.id, item)
  return [...merged.values()]
}

type AvailableModel = { provider: string; id: string; name?: string }

function responseModels(response: Record<string, unknown> | undefined): AvailableModel[] {
  const data = response?.data as { models?: unknown } | undefined
  return Array.isArray(data?.models)
    ? data.models.filter((model): model is AvailableModel => typeof model === "object" && model !== null && "provider" in model && "id" in model && typeof model.provider === "string" && typeof model.id === "string")
    : []
}

function groupModelsByProvider(models: AvailableModel[]) {
  const groups = new Map<string, AvailableModel[]>()
  for (const model of models) groups.set(model.provider, [...(groups.get(model.provider) ?? []), model])
  return [...groups.entries()]
}

function findExtensionRequest(events: Array<Record<string, unknown>>) {
  const event = [...events].reverse().find(
    (item) => item.type === "extension_ui_request" && item._daemonExtensionUiRequiresResponse === true
  )
  if (!event || typeof event.id !== "string") return undefined
  return {
    id: event.id,
    message: typeof event.message === "string" ? event.message : typeof event.text === "string" ? event.text : "Extension input requested",
    placeholder: typeof event.placeholder === "string" ? event.placeholder : undefined,
  }
}

function contentText(content: unknown): string {
  if (typeof content === "string") return content
  if (!Array.isArray(content)) return ""
  return content
    .map((part) => {
      if (
        typeof part === "object" &&
        part !== null &&
        "text" in part &&
        typeof part.text === "string"
      )
        return part.text
      return ""
    })
    .join("")
}
