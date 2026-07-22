import { create } from "zustand"
import { createJSONStorage, persist } from "zustand/middleware"

export interface ServerConnectionSettings {
  baseUrl: string
  name?: string
  /** Kept in memory only; never written to localStorage. */
  token?: string
}

interface AppState {
  connection?: ServerConnectionSettings
  servers: ServerConnectionSettings[]
  selectedSessionId?: string
  expandedTreeNodes: Record<string, true>
  pinnedSessionIds: Record<string, true>
  setConnection: (connection?: ServerConnectionSettings) => void
  addServer: (connection: ServerConnectionSettings) => void
  removeServer: (baseUrl: string) => void
  selectSession: (sessionId?: string) => void
  setTreeNodeExpanded: (nodeId: string, expanded: boolean) => void
  toggleTreeNode: (nodeId: string) => void
  togglePinSession: (sessionId: string) => void
}


/**
 * UI state shared by future tree, workspace, and inspector components.
 * Connection tokens deliberately remain outside persisted storage.
 */
export const useAppStore = create<AppState>()(
  persist(
    (set) => ({
      connection: undefined,
      servers: [],
      selectedSessionId: undefined,
      expandedTreeNodes: {},
      pinnedSessionIds: {},
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
            connection: state.connection ?? server,
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
    }),
    {
      name: "pi-webby-ui",
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
        connection: state.connection ? { baseUrl: state.connection.baseUrl, name: state.connection.name } : undefined,
        servers: state.servers.map(({ baseUrl, name }) => ({ baseUrl, name })),
        selectedSessionId: state.selectedSessionId,
        expandedTreeNodes: state.expandedTreeNodes,
        pinnedSessionIds: state.pinnedSessionIds,
      }),
      merge: (persistedState, currentState) => {
        const persisted = persistedState as Partial<AppState>
        return {
          ...currentState,
          ...persisted,
          connection: persisted.connection
            ? { ...persisted.connection, token: undefined }
            : undefined,
          servers: persisted.servers?.length ? persisted.servers : currentState.servers,
        }
      },
    }
  )
)
