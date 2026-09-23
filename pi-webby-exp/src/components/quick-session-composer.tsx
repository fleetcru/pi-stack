import { useMemo, useState } from "react"
import { ArrowUp, BrainCircuit, Folder, LoaderCircle, Monitor, SlidersHorizontal, Sparkles } from "lucide-react"

import { useAvailableModels, useCreateSession, useDirectoryRoots, usePiServerClient, useWorkers } from "@/api/hooks"
import { Button } from "@/components/ui/button"
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from "@/components/ui/command"
import { Popover, PopoverContent, PopoverTrigger } from "@/components/ui/popover"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"

const THINKING_LEVELS = [
  { value: "__default", label: "Default effort" },
  { value: "off", label: "Off" },
  { value: "minimal", label: "Minimal" },
  { value: "low", label: "Low" },
  { value: "medium", label: "Medium" },
  { value: "high", label: "High" },
  { value: "max", label: "Maximum" },
  { value: "xhigh", label: "Extra high" },
  { value: "ultra", label: "Ultra" },
] as const

export function QuickSessionComposer({
  onCreated,
  onMoreOptions,
}: {
  onCreated: (sessionId: string) => void
  onMoreOptions: () => void
}) {
  const [prompt, setPrompt] = useState("")
  const [error, setError] = useState<string>()
  const [submitting, setSubmitting] = useState(false)
  const [draftSessionId, setDraftSessionId] = useState<string>()
  const [workerId, setWorkerId] = useState("local")
  const [cwd, setCwd] = useState("")
  const [provider, setProvider] = useState("")
  const [modelId, setModelId] = useState("")
  const [thinkingLevel, setThinkingLevel] = useState("__default")
  const [modelPickerOpen, setModelPickerOpen] = useState(false)
  const createSession = useCreateSession()
  const client = usePiServerClient()
  const workersQuery = useWorkers()
  const rootsQuery = useDirectoryRoots(workerId)
  const modelsQuery = useAvailableModels(workerId)
  const workers = workersQuery.data ?? []
  const roots = useMemo(() => rootsQuery.data ?? [], [rootsQuery.data])
  const models = useMemo(() => modelsQuery.data?.models ?? [], [modelsQuery.data?.models])
  const remoteWorkers = workers.filter((worker) => worker.id !== "local")
  const effectiveCwd = roots.some((root) => root.path === cwd) ? cwd : roots[0]?.path ?? ""
  const selectedRoot = roots.find((root) => root.path === effectiveCwd)
  const selectedWorker = workers.find((worker) => worker.id === workerId)
  const selectedModelKey = provider && modelId ? JSON.stringify([provider, modelId]) : "__default"
  const selectedModel = models.find((model) => model.provider === provider && model.id === modelId)
  const modelsByProvider = useMemo(() => {
    const groups = new Map<string, typeof models>()
    for (const model of models) groups.set(model.provider, [...(groups.get(model.provider) ?? []), model])
    return [...groups.entries()]
  }, [models])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    const message = prompt.trim()
    if (!message || !effectiveCwd || submitting || createSession.isPending) return

    setError(undefined)
    setSubmitting(true)
    try {
      const input = { cwd: effectiveCwd, start: true }
      const session = draftSessionId
        ? { id: draftSessionId }
        : workerId === "local"
          ? await createSession.mutateAsync(input)
          : await client.createWorkerSession(workerId, input)
      try {
        if (provider && modelId) {
          await client.sessionPost(session.id, "model", { provider, modelId })
        }
        if (thinkingLevel !== "__default") {
          await client.sessionPost(session.id, "thinking-level", { level: thinkingLevel })
        }
        await client.prompt(session.id, { message })
      } catch (cause) {
        setDraftSessionId(session.id)
        setError(`Session created, but setup was not completed. ${cause instanceof Error ? cause.message : "Request failed."} Retry will use the existing session.`)
        return
      }
      setPrompt("")
      setDraftSessionId(undefined)
      onCreated(session.id)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not start the session")
    } finally {
      setSubmitting(false)
    }
  }

  const busy = submitting || createSession.isPending
  const rootsUnavailable = rootsQuery.isLoading || roots.length === 0

  return (
    <div className="w-full max-w-4xl">
      <form
        onSubmit={submit}
        className="overflow-hidden rounded-[20px] border border-border/80 !bg-muted/70 shadow-[0_18px_60px_-36px_rgba(0,0,0,0.75)] ring-1 ring-foreground/[0.025] transition-[border-color,box-shadow] focus-within:border-foreground/30 focus-within:shadow-[0_22px_70px_-38px_rgba(0,0,0,0.9)]"
      >
        <Textarea
          autoFocus
          value={prompt}
          onChange={(event) => setPrompt(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
              event.preventDefault()
              event.currentTarget.form?.requestSubmit()
            }
          }}
          placeholder="Do anything..."
          aria-label="First message for the new session"
          className="min-h-20 resize-none rounded-none border-0 !bg-muted/70 px-5 pt-5 pb-3 text-sm leading-6 shadow-none focus-visible:ring-0"
        />
        <div className="flex items-center gap-1 overflow-x-auto !bg-muted/70 px-3 py-2">
          <Popover open={modelPickerOpen} onOpenChange={setModelPickerOpen}>
            <PopoverTrigger
              render={
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  className="max-w-64 justify-start px-2 text-muted-foreground"
                  aria-label="Choose model"
                  disabled={modelsQuery.isLoading || models.length === 0}
                />
              }
            >
              <Sparkles data-icon="inline-start" />
              <span className="truncate text-foreground">
                {modelsQuery.isLoading ? "Loading models..." : selectedModel?.name || selectedModel?.id || "Default model"}
              </span>
              {selectedModel && <span className="truncate text-xs">{selectedModel.provider}</span>}
            </PopoverTrigger>
            <PopoverContent align="start" className="w-96 max-w-[calc(100vw-2rem)] gap-0 p-0">
              <Command>
                <CommandInput placeholder="Search models..." />
                <CommandList>
                  <CommandEmpty>No matching models.</CommandEmpty>
                  <CommandGroup heading="Session default">
                    <CommandItem
                      value="default model"
                      data-checked={selectedModelKey === "__default"}
                      onSelect={() => {
                        setProvider("")
                        setModelId("")
                        setModelPickerOpen(false)
                      }}
                    >
                      <Sparkles />
                      <span>Default model</span>
                    </CommandItem>
                  </CommandGroup>
                  {modelsByProvider.map(([modelProvider, providerModels]) => (
                    <CommandGroup key={modelProvider} heading={modelProvider}>
                      {providerModels.map((model) => {
                        const key = JSON.stringify([model.provider, model.id])
                        return (
                          <CommandItem
                            key={key}
                            value={`${model.name || model.id} ${model.id} ${model.provider}`}
                            data-checked={selectedModelKey === key}
                            onSelect={() => {
                              setProvider(model.provider)
                              setModelId(model.id)
                              setModelPickerOpen(false)
                            }}
                          >
                            <span className="min-w-0 flex-1 truncate">{model.name || model.id}</span>
                            {model.name && model.name !== model.id && (
                              <span className="max-w-40 truncate text-xs text-muted-foreground">{model.id}</span>
                            )}
                          </CommandItem>
                        )
                      })}
                    </CommandGroup>
                  ))}
                </CommandList>
              </Command>
            </PopoverContent>
          </Popover>
          <Select value={thinkingLevel} onValueChange={(value) => setThinkingLevel(String(value))}>
            <SelectTrigger
              size="sm"
              className="max-w-44 shrink-0 border-transparent bg-transparent px-2 text-muted-foreground shadow-none hover:bg-muted"
              aria-label="Thinking effort"
              title="Thinking effort"
            >
              <BrainCircuit />
              <span className="truncate text-foreground">
                {THINKING_LEVELS.find((level) => level.value === thinkingLevel)?.label ?? "Default effort"}
              </span>
            </SelectTrigger>
            <SelectContent side="bottom" align="start" alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectLabel>Thinking / effort</SelectLabel>
                {THINKING_LEVELS.map((level) => (
                  <SelectItem key={level.value} value={level.value}>{level.label}</SelectItem>
                ))}
              </SelectGroup>
            </SelectContent>
          </Select>
          <div className="min-w-2 flex-1" />
          <span className="hidden shrink-0 px-2 text-[11px] text-muted-foreground sm:inline">Ctrl/⌘ + Enter</span>
          <Button
            type="submit"
            size="icon"
            className="rounded-full"
            aria-label="Create session and send message"
            disabled={!prompt.trim() || !effectiveCwd || busy}
          >
            {busy ? <LoaderCircle className="animate-spin" /> : <ArrowUp />}
          </Button>
        </div>
      </form>
      {error && <p role="alert" className="mt-2 px-3 text-sm text-destructive">{error}</p>}
      {rootsQuery.isError && <p role="alert" className="mt-2 px-3 text-sm text-destructive">Could not load allowed project folders.</p>}
      <div className="mt-2 flex min-w-0 items-center gap-1 overflow-x-auto px-2 py-1 text-xs text-muted-foreground">
        <Select value={effectiveCwd || null} onValueChange={(value) => setCwd(String(value))}>
          <SelectTrigger
            size="sm"
            className="max-w-72 shrink-0 border-transparent bg-transparent px-2 shadow-none hover:bg-muted"
            aria-label="Project folder"
            title={effectiveCwd || "No allowed project folders"}
            disabled={rootsUnavailable}
          >
            <Folder />
            <span className="truncate">{rootsQuery.isLoading ? "Loading projects..." : selectedRoot?.name || "No allowed projects"}</span>
          </SelectTrigger>
          <SelectContent side="bottom" align="start" alignItemWithTrigger={false} className="w-96 max-w-[calc(100vw-2rem)]">
            <SelectGroup>
              <SelectLabel>Allowed project folders</SelectLabel>
              {roots.map((root) => (
                <SelectItem key={root.path} value={root.path}>
                  <span className="min-w-0 flex-1 truncate">{root.name}</span>
                  <span className="max-w-64 truncate text-xs text-muted-foreground">{root.path}</span>
                </SelectItem>
              ))}
            </SelectGroup>
          </SelectContent>
        </Select>
        <Select
          value={workerId}
          onValueChange={(value) => {
            setWorkerId(String(value))
            setCwd("")
            setProvider("")
            setModelId("")
            setThinkingLevel("__default")
            setDraftSessionId(undefined)
          }}
        >
          <SelectTrigger size="sm" className="max-w-52 shrink-0 border-transparent bg-transparent px-2 shadow-none hover:bg-muted" aria-label="Worker">
            <Monitor />
            <span className="truncate">{workerId === "local" ? "Local" : selectedWorker?.id || workerId}</span>
          </SelectTrigger>
          <SelectContent side="bottom" align="start" alignItemWithTrigger={false} className="min-w-56">
            <SelectGroup>
              <SelectLabel>Session location</SelectLabel>
              <SelectItem value="local">Local</SelectItem>
            </SelectGroup>
            {remoteWorkers.length > 0 && (
              <SelectGroup>
                <SelectLabel>Remote workers</SelectLabel>
                {remoteWorkers.map((worker) => (
                  <SelectItem key={worker.id} value={worker.id} disabled={worker.status === "error"}>
                    <span className="min-w-0 flex-1 truncate">{worker.id}</span>
                    {worker.status && <span className="text-xs text-muted-foreground">{worker.status}</span>}
                  </SelectItem>
                ))}
              </SelectGroup>
            )}
          </SelectContent>
        </Select>
        <Button type="button" size="xs" variant="ghost" onClick={onMoreOptions}>
          <SlidersHorizontal data-icon="inline-start" />
          More options
        </Button>
      </div>
    </div>
  )
}
