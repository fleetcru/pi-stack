import { memo, useCallback, useEffect, useMemo, useRef, useState } from "react"
import { ArrowUp, Bot, Brain, ChevronRight, Code, Copy, CopyCheck, Folder, ImagePlus, Search, Sparkles, Square, Terminal, X } from "lucide-react"
import { useImageAttachments } from "@/hooks/use-image-attachments"
import ReactMarkdown from "react-markdown"
import remarkGfm from "remark-gfm"
import rehypeSanitize from "rehype-sanitize"

import {
  useActiveSessionSocket,
  usePiServerClient,
  useSessionData,
  useSessionHistory,
  useSessionGitStatus,
} from "@/api/hooks"
import {
  type TimelineItem,
  type TextItem,
  type ToolItem,
  LARGE_RENDER_THRESHOLD,
  MARKDOWN_HARD_CAP,
  LARGE_TOOL_OUTPUT_THRESHOLD,
  buildTimeline,
  buildHistory,
  mergeTimeline,
  responseModels,
  groupModelsByProvider,
  findExtensionRequest,
  toolDisplayName,
  extractToolSummary,
  formatDuration,
  toolDuration,
} from "@pi-stack/webby-shared/session-workspace"
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
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Message, MessageContent, MessageGroup } from "@/components/ui/message"
import { ChangedFilesList } from "@/components/changed-files-list"
import {
  MessageScroller,
  MessageScrollerButton,
  MessageScrollerContent,
  MessageScrollerItem,
  MessageScrollerProvider,
  MessageScrollerViewport,
} from "@/components/ui/message-scroller"

import { useSessionNotifications } from "@/hooks/use-session-notifications"

export function SessionWorkspace({ sessionId }: { sessionId: string }) {
  const [prompt, setPrompt] = useState("")
  const [deliveryNotice, setDeliveryNotice] = useState<string | undefined>()
  const [deliveryCommandId, setDeliveryCommandId] = useState<string | undefined>()
  const imageInputRef = useRef<HTMLInputElement>(null)
  const imageAttachments = useImageAttachments(imageInputRef)
  const client = usePiServerClient()
  const socket = useActiveSessionSocket(sessionId)
  const historyQuery = useSessionHistory(sessionId)
  const gitStatusQuery = useSessionGitStatus(sessionId)
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
  // Derive live runtime state from WS events. The runtime_state event is
  // emitted by the server whenever the process state transitions. This is
  // more responsive than HTTP polling which freezes when WS is open.
  const wsRuntimeState = (() => {
    for (let i = socket.events.length - 1; i >= 0; i--) {
      const ev = socket.events[i]
      if (ev.type === "runtime_state") {
        return {
          state: ev.runtimeState as string | undefined,
          reason: ev.runtimeReason as string | undefined,
          detail: ev.runtimeDetail as string | undefined,
        }
      }
    }
    return undefined
  })()
  // Use WS runtime state when available (WS open), fall back to HTTP polling (WS closed).
  const isWorking = socket.status === "open"
    ? wsRuntimeState?.state === "working" || wsRuntimeState?.state === "starting" || wsRuntimeState?.state === "reconnecting"
    : state?.isStreaming === true

  // Fire OS notifications when session state transitions
  useSessionNotifications(sessionId, wsRuntimeState)
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
    if (!message && imageAttachments.images.length === 0) return
    try {
      setDeliveryNotice("Sending…")
      const images = await imageAttachments.toImageContent()
      const request = { message, images: images.length > 0 ? images : undefined }
      const response = mode === "steer" ? await client.steer(sessionId, request) : await client.prompt(sessionId, request)
      const commandId = (response as Record<string, unknown>).commandId
      setDeliveryCommandId(typeof commandId === "string" ? commandId : undefined)
      setPrompt("")
      imageAttachments.clearImages()
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
                  {gitStatusQuery.data?.status.changes?.length ? <ChangedFilesList changes={gitStatusQuery.data.status.changes} /> : null}
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
            <div
              className="rounded-2xl bg-muted/70 shadow-sm"
              onPaste={imageAttachments.handlePaste}
              onDragOver={imageAttachments.handleDragOver}
              onDrop={imageAttachments.handleDrop}
            >
              {imageAttachments.images.length > 0 && (
                <div className="flex flex-wrap gap-2 px-4 pt-3 pb-1">
                  {imageAttachments.images.map((img) => (
                    <div key={img.id} className="group relative shrink-0">
                      <div className="h-16 w-16 overflow-hidden rounded-lg border border-border">
                        <img
                          src={img.previewUrl}
                          alt="Attached image"
                          className="size-full object-cover"
                        />
                      </div>
                      <button
                        type="button"
                        onClick={() => imageAttachments.removeImage(img.id)}
                        className="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full border border-border bg-background text-muted-foreground opacity-0 transition-opacity hover:bg-destructive hover:text-white group-hover:opacity-100"
                        aria-label="Remove image"
                      >
                        <X className="size-3" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
              {imageAttachments.error && (
                <p className="px-4 pt-2 text-xs text-destructive">{imageAttachments.error}</p>
              )}
              <InputGroup className="rounded-2xl !border-0 !shadow-none !outline-none !ring-0 !ring-offset-0 has-[data-slot=input-group-control:focus-visible]:!ring-0 has-[data-slot=input-group-control:focus-visible]:!border-0">
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
                      {['off', 'low', 'medium', 'high'].map((level) => <SelectItem key={level} value={level}>{level}</SelectItem>)}
                    </SelectGroup>
                  </SelectContent>
                </Select>
                  </div>
                <input
                  ref={imageInputRef}
                  type="file"
                  accept="image/*"
                  multiple
                  className="hidden"
                  onChange={imageAttachments.handlePickerChange}
                />
                <InputGroupButton
                  type="button"
                  size="icon-xs"
                  variant="ghost"
                  className="rounded-xl text-muted-foreground"
                  onClick={imageAttachments.openPicker}
                  aria-label="Attach image"
                  title="Attach image (or paste/drop)"
                >
                  <ImagePlus />
                </InputGroupButton>
                <InputGroupButton
                  type="button"
                  size="sm"
                  variant="outline"
                  className="rounded-xl"
                  disabled={!prompt.trim() && imageAttachments.images.length === 0}
                  onClick={() => void sendPrompt("steer")}
                >
                  Steer
                </InputGroupButton>
                <InputGroupButton
                  type={isWorking ? "button" : "submit"}
                  size="icon-sm"
                  className={`ml-auto rounded-full text-white ${isWorking ? "bg-red-600 hover:bg-red-500" : "bg-blue-600 hover:bg-blue-500"}`}
                  disabled={isWorking ? false : !prompt.trim() && imageAttachments.images.length === 0}
                  onClick={isWorking ? () => void abortSession() : undefined}
                  aria-label={isWorking ? "Stop Pi" : "Send prompt"}
                  title={isWorking ? "Stop Pi" : "Send prompt"}
                >
                  {isWorking ? <Square /> : <ArrowUp />}
                </InputGroupButton>
                </div>
              </InputGroupAddon>
              </InputGroup>
            </div>
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

const toolIcons: Record<string, React.ComponentType<{ className?: string }>> = {
  terminal: Terminal,
  code: Code,
  search: Search,
  folder: Folder,
}

function DeferredMarkdown({ text }: { text: string }) {
  const [expanded, setExpanded] = useState(text.length <= LARGE_RENDER_THRESHOLD)
  if (text.length > MARKDOWN_HARD_CAP) {
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
  const { label, icon } = toolDisplayName(item.name)
  const IconComponent = toolIcons[icon] ?? Terminal
  const summary = extractToolSummary(item.name, item.args, item.output)
  const duration = toolDuration(item.startedAt, item.endedAt)
  const isRunning = !item.done
  const isFailed = item.failed === true
  const [copyState, setCopyState] = useState<"idle" | "copied">("idle")

  const statusColor = isFailed
    ? "border-l-red-500"
    : isRunning
      ? "border-l-blue-500"
      : "border-l-emerald-500"

  const statusBg = isFailed
    ? "bg-red-500/5"
    : isRunning
      ? "bg-blue-500/5"
      : ""

  const copyOutput = () => {
    if (!item.output) return
    navigator.clipboard.writeText(item.output).then(() => {
      setCopyState("copied")
      setTimeout(() => setCopyState("idle"), 2000)
    }).catch(() => {})
  }

  return (
    <Collapsible className="w-full">
      <CollapsibleTrigger className="group/tool w-full text-left">
        <div className={`flex items-center gap-2 rounded-lg border-l-[3px] px-3 py-2 text-xs transition-colors hover:bg-muted/40 ${statusColor} ${statusBg}`}>
          {isRunning ? (
            <span className="relative flex size-4 shrink-0 items-center justify-center">
              <span className="absolute inline-flex size-full animate-ping rounded-full bg-blue-400 opacity-40" />
              <IconComponent className="relative size-3.5 text-blue-500" />
            </span>
          ) : (
            <IconComponent className={`size-3.5 shrink-0 ${isFailed ? "text-red-500" : "text-emerald-500"}`} />
          )}
          <span className="min-w-0 flex-1 truncate">
            <span className="font-medium text-foreground/90">{label}</span>
            {summary && (
              <span className="ml-1.5 text-muted-foreground">{summary}</span>
            )}
          </span>
          {duration != null && !isRunning && (
            <span className="shrink-0 tabular-nums text-muted-foreground">
              {formatDuration(duration)}
            </span>
          )}
          {isRunning && item.startedAt && (
            <RunningTimer startedAt={item.startedAt} />
          )}
          <ChevronRight className="size-3.5 shrink-0 text-muted-foreground/60 transition-transform group-data-[state=open]/tool:rotate-90" />
        </div>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mx-3 mt-1 rounded-lg border border-border/60 bg-muted/20">
          {item.output ? (
            <>
              <div className="flex items-center justify-between border-b border-border/40 px-3 py-1.5">
                <span className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">
                  {isFailed ? "Error output" : "Output"}
                </span>
                <button
                  type="button"
                  onClick={copyOutput}
                  className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] text-muted-foreground transition-colors hover:bg-muted hover:text-foreground"
                >
                  {copyState === "copied" ? <><CopyCheck className="size-3" /> Copied</> : <><Copy className="size-3" /> Copy</>}
                </button>
              </div>
              <pre className="max-h-64 overflow-auto p-3 font-mono text-xs leading-5 whitespace-pre-wrap text-foreground/80">
                {item.output.slice(0, LARGE_TOOL_OUTPUT_THRESHOLD)}
              </pre>
              {item.output.length > LARGE_TOOL_OUTPUT_THRESHOLD && (
                <p className="border-t border-border/40 px-3 py-1.5 text-[10px] text-muted-foreground">
                  Output truncated — {Math.ceil(item.output.length / 1024)} KB total
                </p>
              )}
            </>
          ) : (
            <p className="px-3 py-3 text-xs text-muted-foreground">
              {isRunning ? "Waiting for output…" : "No output"}
            </p>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

/** Live elapsed timer for running tools. Updates every second. */
function RunningTimer({ startedAt }: { startedAt: string | number }) {
  const startMs = typeof startedAt === "string" ? new Date(startedAt).getTime() : startedAt
  const [elapsed, setElapsed] = useState(() => Date.now() - startMs)

  useEffect(() => {
    const interval = setInterval(() => {
      setElapsed(Date.now() - startMs)
    }, 1000)
    return () => clearInterval(interval)
  }, [startMs])

  return (
    <span className="shrink-0 tabular-nums text-blue-500">
      {formatDuration(elapsed)}
    </span>
  )
}



// The composer updates on every keystroke. Keep historical Markdown/tool rows
// out of that render path unless their own item object actually changed.
const TimelineRow = memo(function TimelineRow({ item }: { item: TimelineItem }) {
  if (item.kind === "tool") return <ToolTurn item={item} />
  if (item.kind === "system") return <div className="py-1 px-4 text-xs text-muted-foreground italic">{item.text}</div>
  return <ChatTurn item={item} />
})


