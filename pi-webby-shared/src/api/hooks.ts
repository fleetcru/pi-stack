import {
  useMutation,
  useInfiniteQuery,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query"
import { useCallback, useEffect, useMemo, useRef, useState } from "react"

import {
  PiServerClient,
  type ApiSession,
  type ApiWorker,
  type CreateSessionRequest,
  type GitBranchesResponse,
  type GitStatusResponse,
  type GitFileDiffResponse,
  type GitWorktreesResponse,
  type PromptRequest,
  type RpcCommand,
  type RpcResponse,
} from "./client"
import {
  SessionSocket,
  type SessionEvent,
  type SessionSocketStatus,
} from "./session-socket"
// Each app injects its own store hook at startup via setAppStoreHook().
import type { ServerConnectionSettings } from "../state/app-store"

type AppState = {
  connection?: ServerConnectionSettings
  servers: ServerConnectionSettings[]
  selectedSessionId?: string
  selectSession: (sessionId?: string) => void
  setLiveSessionState: (sessionId: string, state: {
    status: SessionSocketStatus
    latestEventId?: number
    lastEventAt?: number
    taskId?: string
    runId?: string
    runtimeState?: string
    runtimeReason?: string
    resynchronizing: boolean
  }) => void
  clearLiveSessionState: (sessionId: string) => void
}
type UseAppStore = <T>(selector: (state: AppState) => T) => T

let _useAppStore: UseAppStore | undefined

/** Inject the app-specific store hook. Must be called before any hooks are used. */
export function setAppStoreHook(hook: UseAppStore) {
  _useAppStore = hook
}

function useAppStore<T>(selector: (state: AppState) => T): T {
  if (!_useAppStore) throw new Error("setAppStoreHook() must be called before using hooks")
  return _useAppStore(selector)
}

export const piQueryKeys = {
  health: (baseUrl: string) => ["pi-server", baseUrl, "health"] as const,
  scheduler: (baseUrl: string) => ["pi-server", baseUrl, "scheduler"] as const,
  capabilities: (baseUrl: string) =>
    ["pi-server", baseUrl, "capabilities"] as const,
  workers: (baseUrl: string) => ["pi-server", baseUrl, "workers"] as const,
  sessions: (baseUrl: string) => ["pi-server", baseUrl, "sessions"] as const,
  globalSessions: (baseUrl: string) => ["pi-server", baseUrl, "global-sessions"] as const,
  machineSessions: (baseUrl: string) => ["pi-server", baseUrl, "machine-sessions"] as const,
  session: (baseUrl: string, id: string) =>
    ["pi-server", baseUrl, "sessions", id] as const,
  sessionData: (baseUrl: string, id: string, resource: string) =>
    ["pi-server", baseUrl, "sessions", id, resource] as const,
  git: (baseUrl: string, id: string, resource: string) =>
    ["pi-server", baseUrl, "sessions", id, "git", resource] as const,
  files: (baseUrl: string, cwd: string) =>
    ["pi-server", baseUrl, "files", cwd] as const,
  fileContent: (baseUrl: string, sessionId: string, path: string) =>
    ["pi-server", baseUrl, "sessions", sessionId, "file-content", path] as const,
}

/** A stable client that changes only when the configured server changes. */
export function usePiServerClient(): PiServerClient {
  const connection = useAppStore((state) => state.connection)
  return useMemo(() => new PiServerClient(connection), [connection])
}

export function useServerConfigured() {
  return useAppStore((state) => Boolean(state.connection?.baseUrl))
}

export function useServerHealth() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.health(client.cacheScope), queryFn: () => client.health(), refetchInterval: 30_000, enabled: configured })
}

export function useSchedulerStatus(enabled = true) {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({
    queryKey: piQueryKeys.scheduler(client.cacheScope),
    queryFn: () => client.schedulerStatus(),
    refetchInterval: 2_000,
    enabled: configured && enabled,
  })
}

export function useServerCapabilities() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.capabilities(client.cacheScope), queryFn: () => client.capabilities(), staleTime: Infinity, enabled: configured })
}

export function useWorkers() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.workers(client.cacheScope), queryFn: () => client.listWorkers(), select: (result) => result.workers, refetchInterval: 30_000, enabled: configured })
}

export function useSessions() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.sessions(client.cacheScope), queryFn: () => client.listSessions(), refetchInterval: 20_000, enabled: configured })
}

export function useGlobalSessions() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.globalSessions(client.cacheScope), queryFn: () => client.listGlobalSessions(), refetchInterval: 20_000, enabled: configured })
}

export function useMachineSessions() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.machineSessions(client.cacheScope), queryFn: () => client.listMachineSessions(), refetchInterval: 30_000, enabled: configured })
}

export function useSessionHistory(sessionId?: string) {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useInfiniteQuery({
    queryKey: ["pi-server", client.cacheScope, "sessions", sessionId ?? "none", "history"],
    queryFn: ({ pageParam }) => client.getSessionMessages(sessionId!, pageParam),
    initialPageParam: 0,
    getNextPageParam: (page) => {
      const history = (page.data as { history?: { hasOlder?: boolean; nextOffset?: number } } | undefined)?.history
      return history?.hasOlder ? history.nextOffset : undefined
    },
    enabled: configured && Boolean(sessionId),
    staleTime: 30_000,
    maxPages: 10,
  })
}

export function useSession(sessionId?: string) {
  const client = usePiServerClient()
  return useQuery({
    queryKey: piQueryKeys.session(client.cacheScope, sessionId ?? "none"),
    queryFn: () => client.getSession(sessionId!),
    enabled: Boolean(sessionId),
    refetchInterval: 10_000,
  })
}

type SessionResource = Parameters<PiServerClient["getSessionData"]>[1]

export function useSessionData(
  sessionId: string | undefined,
  resource: SessionResource,
  options?: Pick<UseQueryOptions<RpcResponse>, "enabled" | "refetchInterval">
) {
  const client = usePiServerClient()
  return useQuery({
    queryKey: piQueryKeys.sessionData(
      client.cacheScope,
      sessionId ?? "none",
      resource
    ),
    queryFn: () => client.getSessionData(sessionId!, resource),
    enabled: Boolean(sessionId) && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval,
  })
}

export function useSessionEvents(sessionId?: string) {
  const client = usePiServerClient()
  return useQuery({
    queryKey: piQueryKeys.sessionData(
      client.cacheScope,
      sessionId ?? "none",
      "events"
    ),
    queryFn: () => client.getSessionEvents(sessionId!, undefined, 100),
    enabled: Boolean(sessionId),
    // Polling disabled — the active WebSocket stream provides live events.
    // The inspector's Activity tab can manually refetch if needed.
  })
}

export function useSessionGit(
  sessionId: string | undefined,
  resource: "status" | "diff" | "log" | "head"
) {
  const client = usePiServerClient()
  return useQuery({
    queryKey: piQueryKeys.git(client.cacheScope, sessionId ?? "none", resource),
    queryFn: () => client.getSessionGit(sessionId!, resource),
    enabled: Boolean(sessionId),
    refetchInterval: resource === "status" ? 5_000 : false,
  })
}

export function useSessionGitStatus(sessionId: string | undefined) {
  const client = usePiServerClient()
  return useQuery<GitStatusResponse>({
    queryKey: piQueryKeys.git(client.cacheScope, sessionId ?? "none", "status-json"),
    queryFn: () => client.getSessionGitStatus(sessionId!),
    enabled: Boolean(sessionId),
    refetchInterval: 5_000,
  })
}

export function useSessionGitFileDiff(sessionId: string | undefined, path: string, enabled = true) {
  const client = usePiServerClient()
  return useQuery<GitFileDiffResponse>({
    queryKey: [...piQueryKeys.git(client.cacheScope, sessionId ?? "none", "file-diff"), path],
    queryFn: () => client.getSessionGitFileDiff(sessionId!, path),
    enabled: Boolean(sessionId) && Boolean(path) && enabled,
    staleTime: 5_000,
  })
}

export function useSessionGitBranches(sessionId: string | undefined) {
  const client = usePiServerClient()
  return useQuery<GitBranchesResponse>({
    queryKey: piQueryKeys.git(client.cacheScope, sessionId ?? "none", "branches"),
    queryFn: () => client.getSessionGitBranches(sessionId!),
    enabled: Boolean(sessionId),
    staleTime: 10_000,
  })
}

export function useSessionGitWorktrees(sessionId: string | undefined) {
  const client = usePiServerClient()
  return useQuery<GitWorktreesResponse>({
    queryKey: piQueryKeys.git(client.cacheScope, sessionId ?? "none", "worktrees"),
    queryFn: () => client.getSessionGitWorktrees(sessionId!),
    enabled: Boolean(sessionId),
    staleTime: 5_000,
  })
}

export function useFileTree(cwd?: string) {
  const client = usePiServerClient()
  return useQuery({
    queryKey: piQueryKeys.files(client.cacheScope, cwd ?? "none"),
    queryFn: () => client.getFileTree(cwd!),
    enabled: Boolean(cwd),
    staleTime: 10_000,
  })
}

export function useSessionFileContent(sessionId?: string, path?: string) {
  const client = usePiServerClient()
  return useQuery({
    queryKey: piQueryKeys.fileContent(client.cacheScope, sessionId ?? "none", path ?? "none"),
    queryFn: () => client.getSessionFileContent(sessionId!, path!),
    enabled: Boolean(sessionId && path),
    staleTime: 5_000,
  })
}

export function useCreateSession() {
  const client = usePiServerClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (input: CreateSessionRequest) => client.createSession(input),
    onSuccess: () => invalidateSessionInventory(queryClient, client.cacheScope),
  })
}

export function useDeleteSession() {
  const client = usePiServerClient()
  const queryClient = useQueryClient()
  const selectedSessionId = useAppStore((state) => state.selectedSessionId)
  const selectSession = useAppStore((state) => state.selectSession)
  return useMutation({
    mutationFn: (sessionId: string) => client.deleteSession(sessionId),
    onSuccess: (_, sessionId) => {
      if (selectedSessionId === sessionId) selectSession()
      invalidateSessionInventory(queryClient, client.cacheScope)
    },
  })
}

export function useSendPrompt(
  mode: "prompt" | "steer" | "follow-up" = "prompt"
) {
  const client = usePiServerClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({
      sessionId,
      request,
    }: {
      sessionId: string
      request: PromptRequest
    }) => {
      if (mode === "steer") return client.steer(sessionId, request)
      if (mode === "follow-up") return client.followUp(sessionId, request)
      return client.prompt(sessionId, request)
    },
    onSuccess: (_, { sessionId }) =>
      invalidateSession(queryClient, client.cacheScope, sessionId),
  })
}

export function useSessionCommand() {
  const client = usePiServerClient()
  return useMutation({
    mutationFn: ({
      sessionId,
      command,
    }: {
      sessionId: string
      command: RpcCommand
    }) => client.command(sessionId, command),
  })
}

export interface SessionEventHealth {
  latestEventId?: number
  lastEventAt?: number
  taskId?: string
  runId?: string
  runtime?: { state?: string; reason?: string; detail?: string }
  gap?: { expectedAfter: number; received: number }
  resynchronizing: boolean
}

export interface ActiveSessionSocket {
  events: SessionEvent[]
  error?: Error
  send: (command: RpcCommand) => void
  status: SessionSocketStatus
  health: SessionEventHealth
}

/**
 * Opens exactly one ticket-authenticated socket for the selected session.
 * Rendering this hook with another session ID closes the previous socket.
 */
export interface ActiveSessionSocketOptions {
  /** Maximum number of events buffered between socket delivery and UI flush. */
  maxBufferedEvents?: number
}

export function useActiveSessionSocket(
  sessionId?: string,
  watchFiles = false,
  options: ActiveSessionSocketOptions = {}
): ActiveSessionSocket {
  const client = usePiServerClient()
  const selectedSessionId = useAppStore((state) => state.selectedSessionId)
  const activeSessionId = sessionId ?? selectedSessionId
  const setLiveSessionState = useAppStore((state) => state.setLiveSessionState)
  const clearLiveSessionState = useAppStore((state) => state.clearLiveSessionState)
  const [status, setStatus] = useState<SessionSocketStatus>("idle")
  const [events, setEvents] = useState<SessionEvent[]>([])
  const [error, setError] = useState<Error>()
  const [health, setHealth] = useState<SessionEventHealth>({ resynchronizing: false })
  const socketRef = useRef<SessionSocket | undefined>(undefined)
  const queryClient = useQueryClient()
  const bufferedEventsRef = useRef<SessionEvent[]>([])
  const maxBufferedEvents = Math.max(100, options.maxBufferedEvents ?? 4_000)
  // Use a bounded Map rather than copying/slicing a Set. Keep the eviction
  // batch small so a busy event stream never causes a long handler pause.
  const seenEventIdsRef = useRef<Map<number, boolean>>(new Map())
  const flushTimerRef = useRef<number | undefined>(undefined)
  const bufferOverflowRef = useRef(false)
  const resyncingRef = useRef(false)

  useEffect(() => {
    let disposed = false
    queueMicrotask(() => {
      if (disposed) return
      setEvents([])
      bufferedEventsRef.current = []
      seenEventIdsRef.current.clear()
      setError(undefined)
      setHealth({ resynchronizing: false })
      if (!activeSessionId) setStatus("idle")
    })
    if (!activeSessionId) {
      socketRef.current = undefined
      return () => {
        disposed = true
      }
    }

    const socket = new SessionSocket({
      client,
      sessionId: activeSessionId,
      watchFiles,
      onStatusChange: setStatus,
      onEvent: (event) => {
        if (disposed) return
        // A successfully delivered event means the gap/overflow has recovered;
        // clear the stale resync error so the UI stops warning after recovery.
        if (resyncingRef.current) {
          resyncingRef.current = false
          setError(undefined)
        }
        const id = event._daemonEventId
        if (typeof id === "number") {
          if (seenEventIdsRef.current.has(id)) return
          seenEventIdsRef.current.set(id, true)
          if (seenEventIdsRef.current.size > 5_000) {
            // Map iteration order is insertion order, so remove a small
            // oldest batch instead of doing thousands of deletes at once.
            let toRemove = 1_000
            for (const key of seenEventIdsRef.current.keys()) {
              if (toRemove-- <= 0) break
              seenEventIdsRef.current.delete(key)
            }
          }
        }
        setHealth((current) => ({
          ...current,
          latestEventId: typeof id === "number" ? id : current.latestEventId,
          lastEventAt: Date.now(),
          taskId: typeof event._daemonTaskId === "string" ? event._daemonTaskId : current.taskId,
          runId: typeof event._daemonRunId === "string" ? event._daemonRunId : current.runId,
          runtime: event.type === "runtime_state"
            ? {
                state: event.runtimeState as string | undefined,
                reason: event.runtimeReason as string | undefined,
                detail: event.runtimeDetail as string | undefined,
              }
            : current.runtime,
          gap: undefined,
          resynchronizing: false,
        }))
        // Keep burst buffering bounded. If a producer outpaces rendering, do
        // not silently discard events: mark the stream for durable-history
        // reconciliation while retaining lifecycle events in the queue.
        if (bufferedEventsRef.current.length >= maxBufferedEvents) {
          const discardIndex = bufferedEventsRef.current.findIndex((queued) =>
            queued.type === "message_update" ||
            queued.type === "tool_execution_update" ||
            queued.type === "file_change"
          )
          if (discardIndex >= 0) {
            bufferedEventsRef.current.splice(discardIndex, 1)
          } else {
            // An all-lifecycle burst is exceptional. Retain the newest state
            // and reconcile history rather than allowing unbounded memory.
            bufferedEventsRef.current.shift()
          }
          if (!bufferOverflowRef.current) {
            bufferOverflowRef.current = true
            resyncingRef.current = true
            setError(new Error("Session event buffer overflow; restoring conversation history"))
            setHealth((current) => ({ ...current, resynchronizing: true }))
            void queryClient.invalidateQueries({
              queryKey: ["pi-server", client.cacheScope, "sessions", activeSessionId, "history"],
            })
          }
        }
        bufferedEventsRef.current.push(event)
        if (flushTimerRef.current !== undefined) return
        // A 30fps cap keeps streamed output responsive while avoiding full
        // timeline reconstruction and Markdown work on every display frame.
        flushTimerRef.current = window.setTimeout(() => {
          flushTimerRef.current = undefined
          const incoming = bufferedEventsRef.current.splice(0)
          bufferOverflowRef.current = false
          setEvents((current) => {
            const next = current.concat(incoming)
            return next.length > 500 ? next.slice(next.length - 500) : next
          })
        }, 33)
      },
      onGap: (expectedAfter, received) => {
        if (disposed) return
        setError(new Error(`Session event history gap detected (${expectedAfter} → ${received}); resynchronizing conversation`))
        resyncingRef.current = true
        setHealth((current) => ({ ...current, gap: { expectedAfter, received }, resynchronizing: true }))
        void queryClient.invalidateQueries({
          queryKey: ["pi-server", client.cacheScope, "sessions", activeSessionId, "history"],
        })
      },
      onError: (err) => { if (!disposed) setError(err) },
    })
    socketRef.current = socket
    socket.connect()
    return () => {
      disposed = true
      socket.close()
      if (flushTimerRef.current !== undefined) window.clearTimeout(flushTimerRef.current)
      flushTimerRef.current = undefined
      if (socketRef.current === socket) socketRef.current = undefined
    }
  }, [activeSessionId, client, maxBufferedEvents, queryClient, watchFiles])

  useEffect(() => {
    if (!activeSessionId) return
    setLiveSessionState(activeSessionId, {
      status,
      latestEventId: health.latestEventId,
      lastEventAt: health.lastEventAt,
      taskId: health.taskId,
      runId: health.runId,
      runtimeState: health.runtime?.state,
      runtimeReason: health.runtime?.reason,
      resynchronizing: health.resynchronizing,
    })
  }, [activeSessionId, health, setLiveSessionState, status])

  useEffect(() => {
    if (!activeSessionId) return
    return () => clearLiveSessionState(activeSessionId)
  }, [activeSessionId, clearLiveSessionState])

  const send = useCallback((command: RpcCommand) => {
    const socket = socketRef.current
    if (!socket) throw new Error("session socket is not connected")
    socket.send(command)
  }, [])

  return {
    events,
    error,
    send,
    status,
    health,
  }
}

export function getSessionDisplayName(session: ApiSession): string {
  return session.title || session.project || session.cwd || session.id
}

export function workerSessionGroups(
  sessions: ApiSession[],
  workers: ApiWorker[]
) {
  const workerNames = new Map(workers.map((worker) => [worker.id, worker.id]))
  return sessions.reduce<Map<string, ApiSession[]>>((groups, session) => {
    const worker = workerNames.get(session.workerId) ?? session.workerId
    const group = groups.get(worker) ?? []
    group.push(session)
    groups.set(worker, group)
    return groups
  }, new Map())
}

function invalidateSessionInventory(
  queryClient: ReturnType<typeof useQueryClient>,
  baseUrl: string
) {
  return queryClient.invalidateQueries({
    queryKey: ["pi-server", baseUrl, "sessions"],
  })
}

function invalidateSession(
  queryClient: ReturnType<typeof useQueryClient>,
  baseUrl: string,
  sessionId: string
) {
  return queryClient.invalidateQueries({
    queryKey: ["pi-server", baseUrl, "sessions", sessionId],
  })
}
