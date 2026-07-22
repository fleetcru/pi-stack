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
  type PromptRequest,
  type RpcCommand,
  type RpcResponse,
} from "@/api/client"
import {
  SessionSocket,
  type SessionEvent,
  type SessionSocketStatus,
} from "@/api/session-socket"
import { useAppStore } from "@/state/app-store"

export const piQueryKeys = {
  health: (baseUrl: string) => ["pi-server", baseUrl, "health"] as const,
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
  return useQuery({ queryKey: piQueryKeys.health(client.baseUrl), queryFn: () => client.health(), refetchInterval: 15_000, retry: 1, enabled: configured })
}

export function useServerCapabilities() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.capabilities(client.baseUrl), queryFn: () => client.capabilities(), staleTime: Infinity, retry: 1, enabled: configured })
}

export function useWorkers() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.workers(client.baseUrl), queryFn: () => client.listWorkers(), select: (result) => result.workers, refetchInterval: 15_000, enabled: configured })
}

export function useSessions() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.sessions(client.baseUrl), queryFn: () => client.listSessions(), refetchInterval: 10_000, enabled: configured })
}

export function useGlobalSessions() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.globalSessions(client.baseUrl), queryFn: () => client.listGlobalSessions(), refetchInterval: 10_000, enabled: configured })
}

export function useMachineSessions() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.machineSessions(client.baseUrl), queryFn: () => client.listMachineSessions(), refetchInterval: 30_000, enabled: configured })
}

export function useSessionHistory(sessionId?: string) {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useInfiniteQuery({
    queryKey: ["pi-server", client.baseUrl, "sessions", sessionId ?? "none", "history"],
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
    queryKey: piQueryKeys.session(client.baseUrl, sessionId ?? "none"),
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
      client.baseUrl,
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
      client.baseUrl,
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
    queryKey: piQueryKeys.git(client.baseUrl, sessionId ?? "none", resource),
    queryFn: () => client.getSessionGit(sessionId!, resource),
    enabled: Boolean(sessionId),
    refetchInterval: resource === "status" ? 5_000 : false,
  })
}

export function useFileTree(cwd?: string) {
  const client = usePiServerClient()
  return useQuery({
    queryKey: piQueryKeys.files(client.baseUrl, cwd ?? "none"),
    queryFn: () => client.getFileTree(cwd!),
    enabled: Boolean(cwd),
    staleTime: 10_000,
  })
}

export function useSessionFileContent(sessionId?: string, path?: string) {
  const client = usePiServerClient()
  return useQuery({
    queryKey: piQueryKeys.fileContent(client.baseUrl, sessionId ?? "none", path ?? "none"),
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
    onSuccess: () => invalidateSessionInventory(queryClient, client.baseUrl),
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
      invalidateSessionInventory(queryClient, client.baseUrl)
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
      invalidateSession(queryClient, client.baseUrl, sessionId),
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

export interface ActiveSessionSocket {
  events: SessionEvent[]
  error?: Error
  send: (command: RpcCommand) => void
  status: SessionSocketStatus
}

/**
 * Opens exactly one ticket-authenticated socket for the selected session.
 * Rendering this hook with another session ID closes the previous socket.
 */
export function useActiveSessionSocket(
  sessionId?: string,
  watchFiles = false
): ActiveSessionSocket {
  const client = usePiServerClient()
  const selectedSessionId = useAppStore((state) => state.selectedSessionId)
  const activeSessionId = sessionId ?? selectedSessionId
  const [status, setStatus] = useState<SessionSocketStatus>("idle")
  const [events, setEvents] = useState<SessionEvent[]>([])
  const [error, setError] = useState<Error>()
  const socketRef = useRef<SessionSocket | undefined>(undefined)
  const queryClient = useQueryClient()
  const bufferedEventsRef = useRef<SessionEvent[]>([])
  // Use a Map<number, boolean> instead of Set for O(1) eviction: when the
  // map exceeds the cap, delete the oldest entries by insertion order rather
  // than spreading/slicing the entire collection (which pauses for ~200ms).
  const seenEventIdsRef = useRef<Map<number, boolean>>(new Map())
  const flushFrameRef = useRef<number | undefined>(undefined)

  useEffect(() => {
    let disposed = false
    queueMicrotask(() => {
      if (disposed) return
      setEvents([])
      bufferedEventsRef.current = []
      seenEventIdsRef.current.clear()
      setError(undefined)
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
        const id = event._daemonEventId
        if (typeof id === "number") {
          if (seenEventIdsRef.current.has(id)) return
          seenEventIdsRef.current.set(id, true)
          if (seenEventIdsRef.current.size > 10_000) {
            // Evict the oldest 5000 entries by insertion order. Map iteration
            // order is insertion order, so the first 5000 keys are the oldest.
            let toRemove = 5_000
            for (const key of seenEventIdsRef.current.keys()) {
              if (toRemove-- <= 0) break
              seenEventIdsRef.current.delete(key)
            }
          }
        }
        bufferedEventsRef.current.push(event)
        if (flushFrameRef.current !== undefined) return
        // Batch at ~60fps using requestAnimationFrame for smooth streaming.
        // The streaming assistant row re-parses its full Markdown on each
        // flush, but requestAnimationFrame ensures we only update once per
        // frame, preventing excessive recomposition.
        flushFrameRef.current = requestAnimationFrame(() => {
          flushFrameRef.current = undefined
          const incoming = bufferedEventsRef.current.splice(0)
          setEvents((current) => [...current, ...incoming].slice(-500))
        })
      },
      onGap: (expectedAfter, received) => {
        if (disposed) return
        setError(new Error(`Session event history gap detected (${expectedAfter} → ${received}); resynchronizing conversation`))
        void queryClient.invalidateQueries({
          queryKey: ["pi-server", client.baseUrl, "sessions", activeSessionId, "history"],
        })
      },
      onError: (err) => { if (!disposed) setError(err) },
    })
    socketRef.current = socket
    socket.connect()
    return () => {
      disposed = true
      socket.close()
      if (flushFrameRef.current !== undefined) window.cancelAnimationFrame(flushFrameRef.current)
      flushFrameRef.current = undefined
      if (socketRef.current === socket) socketRef.current = undefined
    }
  }, [activeSessionId, client, queryClient, watchFiles])

  const send = useCallback(
    (command: RpcCommand) => socketRef.current?.send(command),
    []
  )

  return {
    events,
    error,
    send,
    status,
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
