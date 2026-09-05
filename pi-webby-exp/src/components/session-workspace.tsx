import { memo, useCallback, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react"
import { ArrowUp, Bot, Brain, ChevronRight, CircleCheck, CircleX, Clock3, Copy, CopyCheck, ImagePlus, LoaderCircle, Sparkles, Square, Terminal, X } from "lucide-react"
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
  useSchedulerStatus,
} from "@/api/hooks"
import {
  type TimelineItem,
  type TextItem,
  type ToolItem,
  LARGE_RENDER_THRESHOLD,
  MARKDOWN_HARD_CAP,
  LARGE_TOOL_OUTPUT_THRESHOLD,
  TimelineStore,
  buildHistory,
  mergeTimeline,
  responseModels,
  responseThinkingLevels,
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
import { Input } from "@/components/ui/input"
import { Separator } from "@/components/ui/separator"
import { ScrollArea } from "@/components/ui/scroll-area"
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

export function SessionWorkspace({ sessionId }: { sessionId: string }) {
  const [prompt, setPrompt] = useState("")
  const [deliveryNotice, setDeliveryNotice] = useState<string | undefined>()
  const [deliveryCommandId, setDeliveryCommandId] = useState<string | undefined>()
  const imageInputRef = useRef<HTMLInputElement>(null)
  const { images: pendingImages, error: imageError, ...imageActions } = useImageAttachments(imageInputRef)
  const client = usePiServerClient()
  // Subscribe to filesystem watcher events so file changes are rendered in
  // the chat timeline as well as in the changed-files summary.
  const socket = useActiveSessionSocket(sessionId, true)
  const timelineStore = useMemo(() => new TimelineStore(sessionId), [sessionId])
  useEffect(() => timelineStore.update(socket.events), [timelineStore, socket.events])
  const liveTimelineItems = useSyncExternalStore(timelineStore.subscribe, timelineStore.getSnapshot, timelineStore.getSnapshot)
  const historyQuery = useSessionHistory(sessionId)
  const gitStatusQuery = useSessionGitStatus(sessionId)
  const schedulerQuery = useSchedulerStatus()
  const modelsQuery = useSessionData(sessionId, "models", { refetchInterval: 5_000 })
  const stateQuery = useSessionData(sessionId, "state", {
    // Poll only when the WebSocket is not open — the stream provides live state.
    refetchInterval: socket.status === "open" ? false : 5_000,
  })
  const [extensionValue, setExtensionValue] = useState("")
  const [extensionResponseError, setExtensionResponseError] = useState<string>()
  const [extensionResponding, setExtensionResponding] = useState(false)
  const [ignoredExtensionIds, setIgnoredExtensionIds] = useState<string[]>([])
  const ignoreExtension = useCallback((id: string) => {
    setIgnoredExtensionIds((prev) => {
      const next = [...prev, id]
      // Cap at 100 to prevent unbounded growth in long sessions.
      return next.length > 100 ? next.slice(-100) : next
    })
  }, [])
  const [extensionDialogOpen, setExtensionDialogOpen] = useState(false)
  const respondToExtension = useCallback(async (id: string, response: Record<string, unknown>) => {
    setExtensionResponding(true)
    setExtensionResponseError(undefined)
    try {
      await client.sessionPost(sessionId, "ui-response", { id, ...response })
      ignoreExtension(id)
      setExtensionDialogOpen(false)
      setExtensionValue("")
    } catch (error) {
      setExtensionResponseError(error instanceof Error ? error.message : "Could not send extension response")
    } finally {
      setExtensionResponding(false)
    }
  }, [client, ignoreExtension, sessionId])
  const [modelPickerOpen, setModelPickerOpen] = useState(false)
  const [selectedProvider, setSelectedProvider] = useState<string | undefined>()
  const [modelSearch, setModelSearch] = useState("")
  const extension = useMemo(() => findExtensionRequest(socket.events), [socket.events])
  const visibleExtension = extension && !ignoredExtensionIds.includes(extension.id) ? extension : undefined
  const historyItems = useMemo(() =>
    historyQuery.data?.pages
      .slice()
      .reverse()
      .flatMap((page) => buildHistory(page)) ?? []
  , [historyQuery.data])
  const timeline = useMemo(() => mergeTimeline(historyItems, liveTimelineItems), [historyItems, liveTimelineItems])

  const deliveryStage = useMemo(() => {
    if (!deliveryCommandId) return deliveryNotice
    let receiptIndex = -1
    for (let index = socket.events.length - 1; index >= 0; index -= 1) {
      const event = socket.events[index]
      if (event.type === "bridge_receipt" && event.commandId === deliveryCommandId) {
        receiptIndex = index
        break
      }
    }
    if (receiptIndex < 0) return deliveryNotice
    for (let index = receiptIndex + 1; index < socket.events.length; index += 1) {
      const event = socket.events[index]
      if (
        event.type === "message_start" &&
        (event.message as { role?: string } | undefined)?.role === "assistant"
      ) {
        return "Pi responding…"
      }
    }
    return "Delivered to Pi"
  }, [deliveryCommandId, deliveryNotice, socket.events])

  const models = responseModels(modelsQuery.data)
  const state = stateQuery.data?.data as { model?: { provider?: string; id?: string }; thinkingLevel?: string; isStreaming?: boolean; external?: boolean; relayConnected?: boolean; relayLatencyMs?: number } | undefined
  const modelGroups = useMemo(() => groupModelsByProvider(models), [models])
  const visibleProvider = selectedProvider && modelGroups.some(([provider]) => provider === selectedProvider)
    ? selectedProvider
    : state?.model?.provider && modelGroups.some(([provider]) => provider === state.model?.provider)
      ? state.model.provider
      : modelGroups[0]?.[0]
  const modelSearchLower = modelSearch.trim().toLowerCase()
  const visibleModels = (modelGroups.find(([provider]) => provider === visibleProvider)?.[1] ?? [])
    .filter((model) => !modelSearchLower || (model.name || "").toLowerCase().includes(modelSearchLower) || model.id.toLowerCase().includes(modelSearchLower))
  const visibleProviderGroups = modelSearchLower
    ? modelGroups.filter(([provider]) => provider.toLowerCase().includes(modelSearchLower))
    : modelGroups
  const selectedModel = models.find((model) => model.provider === state?.model?.provider && model.id === state?.model?.id)
  const thinkingLevels = responseThinkingLevels(modelsQuery.data, selectedModel)
  const modelLabel = selectedModel?.name || state?.model?.id || "Choose model"
  // The shared socket hook derives this incrementally from the event stream.
  const wsRuntimeState = socket.health.runtime
  // Use WS runtime state when available (WS open), fall back to HTTP polling (WS closed).
  const isWorking = socket.status === "open"
    ? wsRuntimeState?.state === "working" || wsRuntimeState?.state === "starting" || wsRuntimeState?.state === "reconnecting"
    : state?.isStreaming === true
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

  async function selectModel(provider: string, modelId: string) {
    try {
      await client.sessionPost(sessionId, "model", { provider, modelId })
      setModelPickerOpen(false)
      void stateQuery.refetch()
    } catch (error) {
      setDeliveryNotice(error instanceof Error ? error.message : "Could not change model")
    }
  }

  async function sendPrompt(mode: "prompt" | "steer" = "prompt") {
    const message = prompt.trim()
    if (!message && pendingImages.length === 0) return
    try {
      setDeliveryNotice("Sending…")
      const images = await imageActions.toImageContent()
      const request = { message, images: images.length > 0 ? images : undefined }
      const response = mode === "steer" ? await client.steer(sessionId, request) : await client.prompt(sessionId, request)
      const commandId = (response as Record<string, unknown>).commandId
      setDeliveryCommandId(typeof commandId === "string" ? commandId : undefined)
      setPrompt("")
      imageActions.clearImages()
      setDeliveryNotice(mode === "steer"
        ? "Steering Pi now…"
        : state?.external ? "Sent to bridged Pi. It will arrive at the next safe turn boundary." : "Sent to local Pi. Waiting for response…")
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
            {extensionResponseError && <p role="alert" className="text-sm text-destructive">{extensionResponseError}</p>}
            <DialogFooter>
              <Button variant="ghost" disabled={extensionResponding} onClick={() => { ignoreExtension(visibleExtension.id); setExtensionDialogOpen(false) }}>Ignore</Button>
              <Button variant="outline" disabled={extensionResponding} onClick={() => { void respondToExtension(visibleExtension.id, { cancelled: true }) }}>Cancel</Button>
              <Button disabled={extensionResponding} onClick={() => { void respondToExtension(visibleExtension.id, { value: extensionValue || undefined, confirmed: true }) }}>{extensionResponding ? "Sending…" : "Confirm"}</Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}
      <Dialog
        open={modelPickerOpen}
        onOpenChange={(open) => {
          setModelPickerOpen(open)
          if (open) { setSelectedProvider(state?.model?.provider); setModelSearch("") }
        }}
      >
        <DialogContent className="max-w-3xl sm:max-w-3xl gap-0 overflow-hidden p-0">
          <DialogHeader className="px-6 pt-6 pb-4">
            <DialogTitle>Choose a model</DialogTitle>
            <DialogDescription>Select a provider, then choose one of its available models.</DialogDescription>
          </DialogHeader>
          <Separator />
          <div className="flex h-[32rem] min-h-0">
            <aside className="flex w-52 shrink-0 flex-col border-r">
              <p className="px-4 pt-4 pb-2 text-xs font-medium text-muted-foreground">Providers</p>
              <ScrollArea className="min-h-0 flex-1 px-2 pb-3">
                <div className="flex flex-col gap-1">
                  {visibleProviderGroups.map(([provider, providerModels]) => (
                    <Button
                      key={provider}
                      type="button"
                      variant={provider === visibleProvider ? "secondary" : "ghost"}
                      size="sm"
                      className="w-full justify-between"
                      onClick={() => setSelectedProvider(provider)}
                    >
                      <span className="truncate">{provider}</span>
                      <span>{providerModels.length}</span>
                    </Button>
                  ))}
                </div>
              </ScrollArea>
            </aside>
            <section className="flex min-w-0 flex-1 flex-col">
              <div className="flex items-center gap-3 px-5 pt-4 pb-2">
                <p className="shrink-0 text-xs font-medium text-muted-foreground">{visibleProvider || "Models"}</p>
                <Input
                  value={modelSearch}
                  onChange={(event) => setModelSearch(event.target.value)}
                  placeholder="Search models…"
                  className="h-8 min-w-0 flex-1 text-sm"
                />
              </div>
              <ScrollArea className="min-h-0 flex-1 px-3 pb-4">
                <div className="flex flex-col gap-1">
                  {visibleModels.map((model) => {
                    const active = model.provider === state?.model?.provider && model.id === state?.model?.id
                    return (
                      <Button
                        key={`${model.provider}:${model.id}`}
                        type="button"
                        variant={active ? "secondary" : "ghost"}
                        className="w-full justify-between"
                        onClick={() => void selectModel(model.provider, model.id)}
                      >
                        <span className="min-w-0 truncate text-left">{model.name || model.id}</span>
                        {active && <CircleCheck data-icon="inline-end" />}
                      </Button>
                    )
                  })}
                  {visibleProvider && visibleModels.length === 0 && (
                    <p className="px-2 py-4 text-sm text-muted-foreground">
                      {modelSearchLower ? "No models match your search." : "No models are available for this provider."}
                    </p>
                  )}
                </div>
              </ScrollArea>
            </section>
          </div>
        </DialogContent>
      </Dialog>
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
                  {timeline.map((item, index) => {
                    const nextItem = timeline[index + 1]
                    const isFollowedByTool = item.kind === "tool" && nextItem?.kind === "tool"
                    return (
                      <MessageScrollerItem
                        className={isFollowedByTool ? "w-full -mb-6" : "w-full"}
                        key={item.id}
                        messageId={item.id}
                        scrollAnchor={item.kind === "assistant"}
                      >
                        <TimelineRow item={item} />
                      </MessageScrollerItem>
                    )
                  })}
                  {gitStatusQuery.data?.status.changes?.length ? <ChangedFilesList sessionId={sessionId} changes={gitStatusQuery.data.status.changes} /> : null}
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
              onPaste={imageActions.handlePaste}
              onDragOver={imageActions.handleDragOver}
              onDrop={imageActions.handleDrop}
            >
              {pendingImages.length > 0 && (
                <div className="flex flex-wrap gap-2 px-4 pt-3 pb-1">
                  {pendingImages.map((img) => (
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
                        onClick={() => imageActions.removeImage(img.id)}
                        className="absolute -right-1.5 -top-1.5 flex size-5 items-center justify-center rounded-full border border-border bg-background text-muted-foreground opacity-0 transition-opacity hover:bg-destructive hover:text-white group-hover:opacity-100"
                        aria-label="Remove image"
                      >
                        <X className="size-3" />
                      </button>
                    </div>
                  ))}
                </div>
              )}
              {imageError && (
                <p className="px-4 pt-2 text-xs text-destructive">{imageError}</p>
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
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  className="min-w-40 max-w-56 rounded-xl border-border/70 bg-muted/70 text-foreground shadow-none hover:bg-muted"
                  onClick={() => {
                    setSelectedProvider(state?.model?.provider)
                    setModelPickerOpen(true)
                  }}
                >
                  <Sparkles data-icon="inline-start" />
                  <span className="truncate">{modelLabel}</span>
                </Button>
                {thinkingLevels.length > 0 ? <Select
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
                      {thinkingLevels.map((level) => <SelectItem key={level} value={level}>{level}</SelectItem>)}
                    </SelectGroup>
                  </SelectContent>
                </Select> : <Button size="sm" variant="outline" disabled className="min-w-32 rounded-xl border-border/70 bg-muted/70 text-foreground opacity-100"><Brain className="size-3.5 text-primary" /><span className="truncate">{state?.thinkingLevel ? `Thinking: ${state.thinkingLevel}` : "Thinking unavailable"}</span></Button>}
                  </div>
                <input
                  ref={imageInputRef}
                  type="file"
                  accept="image/*"
                  multiple
                  className="hidden"
                  onChange={imageActions.handlePickerChange}
                />
                <InputGroupButton
                  type="button"
                  size="icon-xs"
                  variant="ghost"
                  className="rounded-xl text-muted-foreground"
                  onClick={imageActions.openPicker}
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
                  disabled={!prompt.trim() && pendingImages.length === 0}
                  onClick={() => void sendPrompt("steer")}
                >
                  Steer
                </InputGroupButton>
                <InputGroupButton
                  type={isWorking ? "button" : "submit"}
                  size="icon-sm"
                  className={`ml-auto rounded-full text-white ${isWorking ? "bg-red-600 hover:bg-red-500" : "bg-blue-600 hover:bg-blue-500"}`}
                  disabled={isWorking ? false : !prompt.trim() && pendingImages.length === 0}
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
          <p className="mx-auto mt-2 max-w-3xl text-xs text-muted-foreground" role="status">
            Event stream: {socket.status}{socket.health.runtime?.state ? ` · ${socket.health.runtime.state}` : ""}{socket.health.resynchronizing ? " · restoring history" : ""}
            {(schedulerQuery.data?.admission.queued ?? 0) > 0 ? ` · queue: ${schedulerQuery.data?.admission.queued} waiting, ${schedulerQuery.data?.admission.active} active` : ""}
          </p>
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
                <>
                  {item.images?.map((image, index) => (
                    <img
                      key={`${image.mimeType}-${index}`}
                      src={image.data.startsWith("data:") ? image.data : `data:${image.mimeType};base64,${image.data}`}
                      alt="Attached image"
                      className="mb-2 max-h-96 max-w-full rounded-lg object-contain last:mb-0"
                    />
                  ))}
                  {item.text}
                </>
              ) : (
                <DeferredMarkdown text={item.text || "Thinking…"} streaming={item.streaming} />
              )}
            </BubbleContent>
          </Bubble>
        </BubbleGroup>
      </MessageContent>
    </Message>
  )
}

function DeferredMarkdown({ text, streaming }: { text: string; streaming?: boolean }) {
  // Hooks must run unconditionally before any early return.
  const [expanded, setExpanded] = useState(() => text.length <= LARGE_RENDER_THRESHOLD)
  // While a message is still streaming, render plain text (cheap) instead of
  // running ReactMarkdown + gfm + rehype-sanitize on the whole growing message
  // every 33ms flush. Markdown is rendered once after message_end.
  if (streaming) {
    return <pre className="whitespace-pre-wrap font-sans text-sm leading-6 text-foreground">{text}</pre>
  }
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
  const { label } = toolDisplayName(item.name)
  const command = toolCommand(item.args)
  const argumentPreview = formatToolArguments(item.args)
  const argumentDetails = formatToolArgumentDetails(item.args, command ? ["command", "cmd"] : [])
  const summary = extractToolSummary(item.name, item.args, item.output)
  const collapsedDetail = command ? formatInlineCommand(command) : argumentPreview || summary
  const duration = toolDuration(item.startedAt, item.endedAt)
  const isRunning = !item.done
  const isFailed = item.failed === true
  const [copyState, setCopyState] = useState<"idle" | "copied">("idle")

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
        <div className="flex h-9 items-center gap-3 border-b border-border/50 px-2 font-mono text-xs text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring">
          <Terminal className="size-4 shrink-0" aria-hidden="true" />
          <span className="shrink-0 text-sm font-semibold text-foreground">{label}</span>
          {collapsedDetail && (
            <span className="min-w-0 flex-1 truncate">{collapsedDetail}</span>
          )}
          <span className="flex shrink-0 items-center gap-3">
            {(duration != null || (isRunning && item.startedAt)) && (
              <span className="flex items-center gap-1 tabular-nums">
                <Clock3 className="size-3" aria-hidden="true" />
                {isRunning && item.startedAt ? <RunningTimer startedAt={item.startedAt} /> : formatDuration(duration!)}
              </span>
            )}
            {isRunning ? (
              <LoaderCircle className="size-4 animate-spin" aria-label="Running" />
            ) : isFailed ? (
              <CircleX className="size-4 text-destructive" aria-label="Failed" />
            ) : (
              <CircleCheck className="size-4" aria-label="Completed" />
            )}
            <ChevronRight className="size-3.5 transition-transform group-data-[state=open]/tool:rotate-90" aria-hidden="true" />
          </span>
        </div>
      </CollapsibleTrigger>
      <CollapsibleContent>
        <div className="mt-2 rounded-xl border border-border/60 px-4 py-3 font-mono text-xs">
          {command && (
            <section>
              <p className="mb-2 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">Command</p>
              <pre className="max-h-40 overflow-auto leading-5 whitespace-pre-wrap text-foreground/80">{command}</pre>
            </section>
          )}
          {command && (argumentDetails || item.output) && <Separator className="my-3" />}
          {argumentDetails && (
            <section>
              <p className="mb-2 text-[10px] font-medium uppercase tracking-wider text-muted-foreground">Arguments</p>
              <pre className="max-h-40 overflow-auto leading-5 whitespace-pre-wrap text-foreground/80">{argumentDetails}</pre>
            </section>
          )}
          {argumentDetails && item.output && <Separator className="my-3" />}
          {item.output ? (
            <section>
              <div className="mb-2 flex items-center justify-between">
                <p className="text-[10px] font-medium uppercase tracking-wider text-muted-foreground">{isFailed ? "Error output" : "Output"}</p>
                <Button type="button" variant="ghost" size="xs" onClick={copyOutput}>
                  {copyState === "copied" ? <><CopyCheck data-icon="inline-start" />Copied</> : <><Copy data-icon="inline-start" />Copy</>}
                </Button>
              </div>
              <pre className="max-h-64 overflow-auto leading-5 whitespace-pre-wrap text-foreground/80">
                {item.output.slice(0, LARGE_TOOL_OUTPUT_THRESHOLD)}
              </pre>
              {item.output.length > LARGE_TOOL_OUTPUT_THRESHOLD && (
                <p className="mt-2 text-[10px] text-muted-foreground">Output truncated — {Math.ceil(item.output.length / 1024)} KB total</p>
              )}
            </section>
          ) : !argumentDetails && (
            <p className="text-muted-foreground">{isRunning ? "Waiting for output…" : "No output"}</p>
          )}
        </div>
      </CollapsibleContent>
    </Collapsible>
  )
}

function formatToolArguments(args?: Record<string, unknown>): string {
  if (!args) return ""
  return Object.entries(args)
    .filter(([, value]) => value !== undefined)
    .slice(0, 3)
    .map(([key, value]) => `${key}=${formatToolArgumentValue(value)}`)
    .join("  ")
}

function formatToolArgumentDetails(args?: Record<string, unknown>, omitKeys: string[] = []): string {
  if (!args) return ""
  return Object.entries(args)
    .filter(([key, value]) => value !== undefined && !omitKeys.includes(key))
    .map(([key, value]) => `${key}=${formatToolArgumentDetailValue(value)}`)
    .join("\n")
}

function toolCommand(args?: Record<string, unknown>): string | undefined {
  const command = args?.command ?? args?.cmd
  return typeof command === "string" && command.trim() ? command : undefined
}

function formatInlineCommand(command: string): string {
  const singleLine = command.replace(/\s+/g, " ").trim()
  return singleLine.length > 96 ? `${singleLine.slice(0, 93)}…` : singleLine
}

function formatToolArgumentDetailValue(value: unknown): string {
  if (typeof value === "string") return JSON.stringify(value)
  return JSON.stringify(value) ?? String(value)
}

function formatToolArgumentValue(value: unknown): string {
  if (typeof value === "string") {
    const abbreviated = value.replace(/\s+/g, " ").trim()
    return JSON.stringify(abbreviated.length > 64 ? `${abbreviated.slice(0, 61)}…` : abbreviated)
  }
  const serialized = JSON.stringify(value)
  return serialized && serialized.length > 64 ? `${serialized.slice(0, 61)}…` : serialized ?? String(value)
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
    <span className="tabular-nums">
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


