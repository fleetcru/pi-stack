import {
  useQuery,
} from "@tanstack/react-query"
import { useMemo } from "react"

import {
  PiServerClient,
} from "./client"
import {
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

export function useAppStore<T>(selector: (state: AppState) => T): T {
  if (!_useAppStore) throw new Error("setAppStoreHook() must be called before using hooks")
  return _useAppStore(selector)
}

export const piQueryKeys = {
  health: (baseUrl: string) => ["pi-server", baseUrl, "health"] as const,
  scheduler: (baseUrl: string) => ["pi-server", baseUrl, "scheduler"] as const,
  capabilities: (baseUrl: string) =>
    ["pi-server", baseUrl, "capabilities"] as const,
  models: (baseUrl: string, workerId: string) => ["pi-server", baseUrl, "workers", workerId, "models"] as const,
  directories: (baseUrl: string, workerId: string) => ["pi-server", baseUrl, "workers", workerId, "directories"] as const,
  workers: (baseUrl: string) => ["pi-server", baseUrl, "workers"] as const,
  sessions: (baseUrl: string) => ["pi-server", baseUrl, "sessions"] as const,
  globalSessions: (baseUrl: string) => ["pi-server", baseUrl, "global-sessions"] as const,
  machineSessions: (baseUrl: string) => ["pi-server", baseUrl, "machine-sessions"] as const,
  session: (baseUrl: string, id: string) =>
    ["pi-server", baseUrl, "sessions", id] as const,
  sessionHistory: (baseUrl: string, id: string) =>
    ["pi-server", baseUrl, "sessions", id, "history"] as const,
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

export function useAvailableModels(workerId = "local") {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({
    queryKey: piQueryKeys.models(client.cacheScope, workerId),
    queryFn: () => client.listAvailableModels(workerId),
    enabled: configured && Boolean(workerId),
    staleTime: 5 * 60_000,
  })
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

export function useDirectoryRoots(workerId = "local") {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({
    queryKey: piQueryKeys.directories(client.cacheScope, workerId),
    queryFn: () => client.listDirectories(undefined, workerId),
    enabled: configured && Boolean(workerId),
    select: (result) => result.roots?.length ? result.roots : result.directories ?? [],
    staleTime: 30_000,
  })
}

export function useWorkers(enabled = true) {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.workers(client.cacheScope), queryFn: () => client.listWorkers(), select: (result) => result.workers, refetchInterval: 30_000, enabled: configured && enabled })
}

export function useSessions() {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.sessions(client.cacheScope), queryFn: () => client.listSessions(), refetchInterval: 20_000, enabled: configured })
}

export function useGlobalSessions(enabled = true) {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.globalSessions(client.cacheScope), queryFn: () => client.listGlobalSessions(), refetchInterval: 20_000, enabled: configured && enabled })
}

export function useMachineSessions(enabled = true) {
  const client = usePiServerClient()
  const configured = useServerConfigured()
  return useQuery({ queryKey: piQueryKeys.machineSessions(client.cacheScope), queryFn: () => client.listMachineSessions(), refetchInterval: 30_000, enabled: configured && enabled })
}
