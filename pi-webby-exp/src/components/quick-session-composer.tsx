import { useMemo, useState } from "react"
import { ArrowUp, Folder, LoaderCircle, Monitor } from "lucide-react"

import { useAvailableModels, useCreateSession, usePiServerClient } from "@/api/hooks"
import { Button } from "@/components/ui/button"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Textarea } from "@/components/ui/textarea"

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
  const [provider, setProvider] = useState("")
  const [modelId, setModelId] = useState("")
  const createSession = useCreateSession()
  const client = usePiServerClient()
  const modelsQuery = useAvailableModels()
  const models = useMemo(() => modelsQuery.data?.models ?? [], [modelsQuery.data?.models])
  const providers = useMemo(() => [...new Set(models.map((model) => model.provider))], [models])
  const providerModels = useMemo(() => models.filter((model) => model.provider === provider), [models, provider])

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    const message = prompt.trim()
    if (!message || submitting || createSession.isPending) return

    setError(undefined)
    setSubmitting(true)
    try {
      // An empty cwd tells pi-server to use its configured default folder.
      const session = draftSessionId
        ? { id: draftSessionId }
        : await createSession.mutateAsync({ cwd: "", start: true })
      try {
        if (provider && modelId) {
          await client.sessionPost(session.id, "model", { provider, modelId })
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

  return (
    <div className="w-full max-w-2xl">
      <form
        onSubmit={submit}
        className="overflow-hidden rounded-2xl border border-border/80 bg-card shadow-lg shadow-black/5 transition-shadow focus-within:border-foreground/20 focus-within:shadow-xl dark:shadow-black/30"
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
          placeholder="Ask Pi to make changes, inspect code, or fix an issue"
          aria-label="First message for the new session"
          className="min-h-28 resize-none border-0 bg-transparent px-4 py-4 text-sm shadow-none focus-visible:ring-0 sm:min-h-32 sm:px-5 sm:py-5"
        />
        <div className="flex flex-wrap items-center gap-2 border-t border-border/60 px-3 py-2.5 sm:px-4">
          <Button type="button" size="sm" variant="outline" className="h-8 rounded-lg" onClick={onMoreOptions}>
            <Folder className="size-3.5" />
            Choose project
          </Button>
          <span className="inline-flex h-8 items-center gap-1.5 rounded-lg border border-border/70 bg-muted/30 px-2.5 text-xs text-muted-foreground">
            <Monitor className="size-3.5" />
            Local default
          </span>
          <Select value={provider || null} onValueChange={(value) => { const next = String(value); setProvider(next === "__default" ? "" : next); setModelId("") }}>
            <SelectTrigger className="h-8 w-32 rounded-lg text-xs" aria-label="Model provider" disabled={modelsQuery.isLoading || providers.length === 0}>
              <SelectValue placeholder={modelsQuery.isLoading ? "Loading…" : "Provider"} />
            </SelectTrigger>
            <SelectContent side="bottom" align="start" alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectLabel>Provider</SelectLabel>
                <SelectItem value="__default">Use session default</SelectItem>
                {providers.map((item) => <SelectItem key={item} value={item}>{item}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
          <Select value={modelId || null} onValueChange={(value) => setModelId(String(value))}>
            <SelectTrigger className="h-8 min-w-36 max-w-56 flex-1 rounded-lg text-xs sm:flex-none" aria-label="Model" disabled={!provider || providerModels.length === 0}>
              <SelectValue placeholder="Model" />
            </SelectTrigger>
            <SelectContent side="bottom" align="start" alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectLabel>{provider || "Model"}</SelectLabel>
                {providerModels.map((model) => <SelectItem key={model.id} value={model.id}>{model.name || model.id}</SelectItem>)}
              </SelectGroup>
            </SelectContent>
          </Select>
          <div className="min-w-2 flex-1" />
          <span className="hidden text-[11px] text-muted-foreground sm:inline">Ctrl/⌘ + Enter</span>
          <Button
            type="submit"
            size="icon-sm"
            className="rounded-lg"
            aria-label="Create session and send message"
            disabled={!prompt.trim() || busy}
          >
            {busy ? <LoaderCircle className="animate-spin" /> : <ArrowUp />}
          </Button>
        </div>
      </form>
      {error && <p role="alert" className="mt-3 text-center text-sm text-destructive">{error}</p>}
      <p className="mt-3 text-center text-xs text-muted-foreground">
        Starts locally in the server default folder. Choose project for workers, worktrees, and advanced options.
      </p>
    </div>
  )
}
