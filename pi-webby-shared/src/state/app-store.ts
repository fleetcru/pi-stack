import { create } from "zustand"
import { createJSONStorage, persist } from "zustand/middleware"

export interface ServerConnectionSettings {
  baseUrl: string
  name?: string
  /** Persisted only when rememberToken is enabled. */
  token?: string
  rememberToken?: boolean
}

export interface LiveSessionState {
  status: "idle" | "connecting" | "open" | "reconnecting" | "closed"
  latestEventId?: number
  lastEventAt?: number
  taskId?: string
  runId?: string
  runtimeState?: string
  runtimeReason?: string
  resynchronizing: boolean
}

interface AppState {
  connection?: ServerConnectionSettings
  servers: ServerConnectionSettings[]
  selectedSessionId?: string
  expandedTreeNodes: Record<string, true>
  pinnedSessionIds: Record<string, true>
  /** Volatile socket-derived state; intentionally excluded from persistence. */
  liveSessionState: Record<string, LiveSessionState>
  setConnection: (connection?: ServerConnectionSettings) => void
  addServer: (connection: ServerConnectionSettings) => void
  updateServer: (baseUrl: string, connection: ServerConnectionSettings) => void
  removeServer: (baseUrl: string) => void
  selectSession: (sessionId?: string) => void
  setTreeNodeExpanded: (nodeId: string, expanded: boolean) => void
  toggleTreeNode: (nodeId: string) => void
  togglePinSession: (sessionId: string) => void
  setLiveSessionState: (sessionId: string, state: LiveSessionState) => void
  clearLiveSessionState: (sessionId: string) => void
}

/**
 * Create a persisted Zustand store for app UI state.
 * Each app passes its own storage key to avoid namespace collisions.
 */
export function createAppStore(storageName: string) {
  return create<AppState>()(
    persist(
      (set) => ({
        connection: undefined,
        servers: [],
        selectedSessionId: undefined,
        expandedTreeNodes: {},
        pinnedSessionIds: {},
        liveSessionState: {},
        setConnection: (connection) =>
          set((state) => ({
            connection,
            selectedSessionId: undefined,
            servers: connection
              ? state.servers.map((server) => server.baseUrl === connection.baseUrl ? { ...server, ...connection } : server)
              : state.servers,
          })),
        addServer: (connection) =>
          set((state) => {
            const baseUrl = connection.baseUrl.replace(/\/+$/, "")
            const server = { ...connection, baseUrl }
            const exists = state.servers.some((item) => item.baseUrl === baseUrl)
            return {
              servers: exists
                ? state.servers.map((item) => item.baseUrl === baseUrl ? { ...item, ...server } : item)
                : [...state.servers, server],
              connection: state.connection?.baseUrl === baseUrl ? server : state.connection ?? server,
            }
          }),
        updateServer: (baseUrl, connection) =>
          set((state) => {
            const normalized = connection.baseUrl.replace(/\/+$/, "")
            const updated = { ...connection, baseUrl: normalized }
            return {
              servers: state.servers.map((server) => server.baseUrl === baseUrl ? updated : server),
              connection: state.connection?.baseUrl === baseUrl ? { ...state.connection, ...updated } : state.connection,
            }
          }),
        removeServer: (baseUrl) =>
          set((state) => {
            const servers = state.servers.filter((server) => server.baseUrl !== baseUrl)
            const removingActive = state.connection?.baseUrl === baseUrl
            return {
              servers,
              connection: removingActive ? servers[0] : state.connection,
              selectedSessionId: removingActive ? undefined : state.selectedSessionId,
            }
          }),
        selectSession: (selectedSessionId) => set({ selectedSessionId }),
        setTreeNodeExpanded: (nodeId, expanded) =>
          set((state) => {
            const expandedTreeNodes = { ...state.expandedTreeNodes }
            if (expanded) expandedTreeNodes[nodeId] = true
            else delete expandedTreeNodes[nodeId]
            return { expandedTreeNodes }
          }),
        toggleTreeNode: (nodeId) =>
          set((state) => {
            const expandedTreeNodes = { ...state.expandedTreeNodes }
            if (expandedTreeNodes[nodeId]) delete expandedTreeNodes[nodeId]
            else expandedTreeNodes[nodeId] = true
            return { expandedTreeNodes }
          }),
        togglePinSession: (sessionId) =>
          set((state) => {
            const pinnedSessionIds = { ...state.pinnedSessionIds }
            if (pinnedSessionIds[sessionId]) delete pinnedSessionIds[sessionId]
            else pinnedSessionIds[sessionId] = true
            return { pinnedSessionIds }
          }),
        setLiveSessionState: (sessionId, liveState) => set((state) => ({
          liveSessionState: { ...state.liveSessionState, [sessionId]: liveState },
        })),
        clearLiveSessionState: (sessionId) => set((state) => {
          const liveSessionState = { ...state.liveSessionState }
          delete liveSessionState[sessionId]
          return { liveSessionState }
        }),
      }),
      {
        name: storageName,
        version: 2,
        migrate: (persistedState, version) => {
          const persisted = persistedState as Partial<AppState>
          // v1 seeded a silent localhost "Default" server. Remove only that
          // generated entry; explicitly named user servers are preserved.
          if (version < 2 && persisted.connection?.name === "Default") {
            persisted.connection = undefined
            persisted.servers = (persisted.servers ?? []).filter((server) => server.name !== "Default")
          }
          return persisted
        },
        storage: createJSONStorage(() => localStorage),
        partialize: (state) => ({
          connection: state.connection
            ? {
                baseUrl: state.connection.baseUrl,
                name: state.connection.name,
                ...(state.connection.rememberToken && state.connection.token
                  ? { token: state.connection.token, rememberToken: true }
                  : {}),
              }
            : undefined,
          servers: state.servers.map((server) => ({
            baseUrl: server.baseUrl,
            name: server.name,
            ...(server.rememberToken && server.token
              ? { token: server.token, rememberToken: true }
              : {}),
          })),
          selectedSessionId: state.selectedSessionId,
          expandedTreeNodes: state.expandedTreeNodes,
          pinnedSessionIds: state.pinnedSessionIds,
        }),
        merge: (persistedState, currentState) => {
          const persisted = persistedState as Partial<AppState>
          return {
            ...currentState,
            ...persisted,
            connection: persisted.connection ? { ...persisted.connection } : undefined,
            servers: persisted.servers?.length ? persisted.servers : currentState.servers,
          }
        },
      }
    )
  )
}
