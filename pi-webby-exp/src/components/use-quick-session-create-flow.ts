import { useEffect, useMemo, useRef, useState } from "react"

import { useAvailableModels, useCreateSession, useDirectoryRoots, usePiServerClient, useWorkers } from "@/api/hooks"
import { initialComposerState } from "@/components/quick-session-composer-state"

export function useQuickSessionCreateFlow({
  variant,
  defaultMoreOptionsOpen,
  expandMoreOptionsToken,
  focusToken,
  onCreated,
  onRequestClose,
}: {
  variant: "inline" | "dialog"
  defaultMoreOptionsOpen: boolean
  expandMoreOptionsToken?: number
  focusToken?: number
  onCreated: (sessionId: string) => void
  onRequestClose?: () => void
}) {
  const promptRef = useRef<HTMLTextAreaElement | null>(null)
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
  const [moreOptionsOpen, setMoreOptionsOpen] = useState(defaultMoreOptionsOpen)
  const [title, setTitle] = useState("")
  const [isolated, setIsolated] = useState(false)
  const [sessionCount, setSessionCount] = useState(1)
  const [args, setArgs] = useState("")
  const [labels, setLabels] = useState("")
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [browsing, setBrowsing] = useState(false)
  const [browserLoading, setBrowserLoading] = useState(false)
  const [browserPath, setBrowserPath] = useState<string>()
  const [browserParent, setBrowserParent] = useState<string>()
  const [directories, setDirectories] = useState<Array<{ name: string; path: string }>>([])
  const [browserError, setBrowserError] = useState<string>()

  const createSession = useCreateSession()
  const client = usePiServerClient()
  const workersQuery = useWorkers()
  const rootsQuery = useDirectoryRoots(workerId)
  const modelsQuery = useAvailableModels(workerId)
  const workers = workersQuery.data ?? []
  const roots = useMemo(() => rootsQuery.data ?? [], [rootsQuery.data])
  const models = useMemo(() => modelsQuery.data?.models ?? [], [modelsQuery.data?.models])
  const remoteWorkers = workers.filter((worker) => worker.id !== "local")
  const effectiveCwd = cwd || roots[0]?.path || ""
  const selectedRoot = roots.find((root) => root.path === effectiveCwd)
  const selectedWorker = workers.find((worker) => worker.id === workerId)
  const selectedModelKey = provider && modelId ? JSON.stringify([provider, modelId]) : "__default"
  const selectedModel = models.find((model) => model.provider === provider && model.id === modelId)
  const modelsByProvider = useMemo(() => {
    const groups = new Map<string, typeof models>()
    for (const model of models) groups.set(model.provider, [...(groups.get(model.provider) ?? []), model])
    return [...groups.entries()]
  }, [models])

  useEffect(() => {
    if (expandMoreOptionsToken === undefined || expandMoreOptionsToken <= 0) return
    setMoreOptionsOpen(true)
  }, [expandMoreOptionsToken])

  useEffect(() => {
    if (focusToken === undefined || focusToken <= 0) return
    promptRef.current?.focus()
  }, [focusToken])

  function resetBrowser() {
    setBrowsing(false)
    setBrowserLoading(false)
    setBrowserPath(undefined)
    setBrowserParent(undefined)
    setDirectories([])
    setBrowserError(undefined)
  }

  function resetForm() {
    const next = initialComposerState()
    setPrompt(next.prompt)
    setError(next.error)
    setSubmitting(next.submitting)
    setDraftSessionId(next.draftSessionId)
    setWorkerId(next.workerId)
    setCwd(next.cwd)
    setProvider(next.provider)
    setModelId(next.modelId)
    setThinkingLevel(next.thinkingLevel)
    setTitle(next.title)
    setIsolated(next.isolated)
    setSessionCount(next.sessionCount)
    setArgs(next.args)
    setLabels(next.labels)
    setAdvancedOpen(next.advancedOpen)
    resetBrowser()
    setMoreOptionsOpen(defaultMoreOptionsOpen)
  }

  async function loadDirectories(path?: string) {
    setBrowserError(undefined)
    setBrowserLoading(true)
    try {
      const result = await client.listDirectories(path, workerId)
      setBrowserPath(result.path)
      setBrowserParent(result.parent)
      setDirectories(path ? (result.directories ?? []) : (result.roots?.length ? result.roots : result.directories ?? []))
    } catch (cause) {
      setBrowserError(cause instanceof Error ? cause.message : "Could not load directories")
    } finally {
      setBrowserLoading(false)
    }
  }

  function changeWorker(nextWorkerId: string) {
    setWorkerId(nextWorkerId)
    setCwd("")
    setProvider("")
    setModelId("")
    setThinkingLevel("__default")
    setDraftSessionId(undefined)
    resetBrowser()
  }

  async function createOne(input: {
    cwd: string
    start: true
    title?: string
    createWorktree?: { enabled: boolean }
    args: string[]
    labels: string[]
  }) {
    if (workerId === "local") return createSession.mutateAsync(input)
    return client.createWorkerSession(workerId, input)
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    const message = prompt.trim()
    if (!effectiveCwd || submitting || createSession.isPending) return
    if (!message && variant === "inline" && !moreOptionsOpen) return

    setError(undefined)
    setSubmitting(true)
    const count = Math.min(12, Math.max(1, Math.trunc(sessionCount)))
    const parsedArgs = args.split(/\s+/).filter(Boolean)
    const parsedLabels = labels.split(",").map((label) => label.trim()).filter(Boolean)
    const baseInput = {
      cwd: effectiveCwd,
      start: true as const,
      createWorktree: isolated ? { enabled: true } : undefined,
      args: parsedArgs,
      labels: parsedLabels,
    }

    try {
      if (draftSessionId && count === 1 && message) {
        try {
          if (provider && modelId) {
            await client.sessionPost(draftSessionId, "model", { provider, modelId })
          }
          if (thinkingLevel !== "__default") {
            await client.sessionPost(draftSessionId, "thinking-level", { level: thinkingLevel })
          }
          await client.prompt(draftSessionId, { message })
        } catch (cause) {
          setError(`Session created, but setup was not completed. ${cause instanceof Error ? cause.message : "Request failed."} Retry will use the existing session.`)
          return
        }
        const id = draftSessionId
        resetForm()
        onCreated(id)
        onRequestClose?.()
        return
      }

      const results = await Promise.allSettled(Array.from({ length: count }, (_, index) => {
        const baseTitle = title.trim()
        const sessionTitle = count > 1
          ? `${baseTitle || "New session"} ${index + 1}`
          : baseTitle || undefined
        return createOne({ ...baseInput, title: sessionTitle })
      }))
      const sessions = results.flatMap((result) => result.status === "fulfilled" ? [result.value] : [])
      if (sessions.length === 0) {
        const failure = results.find((result) => result.status === "rejected")
        setError(failure?.status === "rejected" && failure.reason instanceof Error ? failure.reason.message : "Could not create sessions")
        return
      }

      const primary = sessions[sessions.length - 1]
      if (message) {
        try {
          if (count === 1) {
            if (provider && modelId) {
              await client.sessionPost(primary.id, "model", { provider, modelId })
            }
            if (thinkingLevel !== "__default") {
              await client.sessionPost(primary.id, "thinking-level", { level: thinkingLevel })
            }
            await client.prompt(primary.id, { message })
          } else {
            await Promise.allSettled(sessions.map((session) => client.prompt(session.id, { message })))
          }
        } catch (cause) {
          if (count === 1) {
            setDraftSessionId(primary.id)
            setError(`Session created, but setup was not completed. ${cause instanceof Error ? cause.message : "Request failed."} Retry will use the existing session.`)
            return
          }
          setError(`Created ${sessions.length} session(s), but seeding the prompt failed for some.`)
        }
      }

      const partial = sessions.length !== count
      if (partial) {
        setError(`${sessions.length} of ${count} sessions started`)
        onCreated(primary.id)
        return
      }

      resetForm()
      onCreated(primary.id)
      onRequestClose?.()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : "Could not start the session")
    } finally {
      setSubmitting(false)
    }
  }

  const busy = submitting || createSession.isPending
  const rootsUnavailable = rootsQuery.isLoading || (roots.length === 0 && !cwd)
  const canSubmit = Boolean(effectiveCwd) && !busy && (Boolean(prompt.trim()) || moreOptionsOpen || variant === "dialog")
  const submitLabel = sessionCount > 1
    ? `Start ${sessionCount} sessions`
    : prompt.trim()
      ? "Create session and send message"
      : "Create session"

  return {
    promptRef,
    prompt, setPrompt,
    error,
    workerId,
    cwd, setCwd,
    thinkingLevel, setThinkingLevel,
    modelPickerOpen, setModelPickerOpen,
    moreOptionsOpen, setMoreOptionsOpen,
    title, setTitle,
    isolated, setIsolated,
    sessionCount, setSessionCount,
    args, setArgs,
    labels, setLabels,
    advancedOpen, setAdvancedOpen,
    browsing, setBrowsing,
    browserLoading,
    browserPath,
    browserParent,
    directories,
    browserError,
    rootsQuery,
    modelsQuery,
    workers,
    roots,
    models,
    remoteWorkers,
    effectiveCwd,
    selectedRoot,
    selectedWorker,
    selectedModelKey,
    selectedModel,
    modelsByProvider,
    provider, setProvider,
    modelId, setModelId,
    busy,
    rootsUnavailable,
    canSubmit,
    submitLabel,
    loadDirectories,
    changeWorker,
    submit,
  }
}
