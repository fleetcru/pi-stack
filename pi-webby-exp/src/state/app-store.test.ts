import { describe, it, expect, beforeEach } from "vitest"
import { useAppStore, type ServerConnectionSettings } from "./app-store"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Reset store to initial state between tests. */
function resetStore() {
  useAppStore.setState({
    connection: undefined,
    servers: [],
    selectedSessionId: undefined,
    expandedTreeNodes: {},
    pinnedSessionIds: {},
  })
  localStorage.clear()
}

const serverA: ServerConnectionSettings = {
  baseUrl: "http://a.example:3141",
  name: "Server A",
}
const serverB: ServerConnectionSettings = {
  baseUrl: "http://b.example:3141",
  name: "Server B",
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("useAppStore", () => {
  beforeEach(() => resetStore())

  // --- Connection ---

  it("setConnection() sets the active connection", () => {
    useAppStore.getState().setConnection(serverA)
    expect(useAppStore.getState().connection).toEqual(serverA)
  })

  it("setConnection(undefined) clears the connection", () => {
    useAppStore.getState().setConnection(serverA)
    useAppStore.getState().setConnection(undefined)
    expect(useAppStore.getState().connection).toBeUndefined()
  })

  it("setConnection() clears selectedSessionId", () => {
    useAppStore.getState().setConnection(serverA)
    useAppStore.getState().selectSession("s1")
    expect(useAppStore.getState().selectedSessionId).toBe("s1")

    useAppStore.getState().setConnection(serverB)
    expect(useAppStore.getState().selectedSessionId).toBeUndefined()
  })

  it("setConnection() updates matching server in servers list", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().setConnection({ ...serverA, token: "tok123" })

    const updated = useAppStore
      .getState()
      .servers.find((s) => s.baseUrl === serverA.baseUrl)
    expect(updated?.token).toBe("tok123")
  })

  // --- addServer ---

  it("addServer() adds a new server to the list", () => {
    useAppStore.getState().addServer(serverA)
    expect(useAppStore.getState().servers).toHaveLength(1)
    expect(useAppStore.getState().servers[0]).toEqual(serverA)
  })

  it("addServer() sets connection when list was empty", () => {
    useAppStore.getState().addServer(serverA)
    expect(useAppStore.getState().connection?.baseUrl).toBe(serverA.baseUrl)
  })

  it("addServer() normalizes trailing slashes in baseUrl", () => {
    useAppStore.getState().addServer({ ...serverA, baseUrl: "http://a.example:3141///" })
    expect(useAppStore.getState().servers[0]!.baseUrl).toBe("http://a.example:3141")
  })

  it("addServer() updates existing server with same baseUrl", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().addServer({ ...serverA, name: "Updated" })

    expect(useAppStore.getState().servers).toHaveLength(1)
    expect(useAppStore.getState().servers[0]!.name).toBe("Updated")
  })

  it("addServer() updates connection when adding the active server", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().addServer({ ...serverA, name: "V2" })

    expect(useAppStore.getState().connection?.name).toBe("V2")
  })

  it("addServer() does not replace connection with different server", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().addServer(serverB)

    expect(useAppStore.getState().connection?.baseUrl).toBe(serverA.baseUrl)
    expect(useAppStore.getState().servers).toHaveLength(2)
  })

  // --- removeServer ---

  it("removeServer() removes a server", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().addServer(serverB)
    useAppStore.getState().removeServer(serverA.baseUrl)

    expect(useAppStore.getState().servers).toHaveLength(1)
    expect(useAppStore.getState().servers[0]!.baseUrl).toBe(serverB.baseUrl)
  })

  it("removeServer() switches connection to next server if removing active", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().addServer(serverB)
    useAppStore.getState().setConnection(serverA)

    useAppStore.getState().removeServer(serverA.baseUrl)

    expect(useAppStore.getState().connection?.baseUrl).toBe(serverB.baseUrl)
  })

  it("removeServer() clears connection when removing the only server", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().removeServer(serverA.baseUrl)

    expect(useAppStore.getState().connection).toBeUndefined()
    expect(useAppStore.getState().servers).toHaveLength(0)
  })

  it("removeServer() clears selectedSessionId when removing active server", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().setConnection(serverA)
    useAppStore.getState().selectSession("s1")

    useAppStore.getState().removeServer(serverA.baseUrl)

    expect(useAppStore.getState().selectedSessionId).toBeUndefined()
  })

  it("removeServer() does not clear selectedSessionId for non-active server", () => {
    useAppStore.getState().addServer(serverA)
    useAppStore.getState().addServer(serverB)
    useAppStore.getState().setConnection(serverA)
    useAppStore.getState().selectSession("s1")

    useAppStore.getState().removeServer(serverB.baseUrl)

    expect(useAppStore.getState().selectedSessionId).toBe("s1")
  })

  // --- selectSession ---

  it("selectSession() sets selectedSessionId", () => {
    useAppStore.getState().selectSession("s1")
    expect(useAppStore.getState().selectedSessionId).toBe("s1")
  })

  it("selectSession(undefined) clears selectedSessionId", () => {
    useAppStore.getState().selectSession("s1")
    useAppStore.getState().selectSession(undefined)
    expect(useAppStore.getState().selectedSessionId).toBeUndefined()
  })

  // --- Tree nodes ---

  it("setTreeNodeExpanded(true) adds node to expandedTreeNodes", () => {
    useAppStore.getState().setTreeNodeExpanded("node1", true)
    expect(useAppStore.getState().expandedTreeNodes.node1).toBe(true)
  })

  it("setTreeNodeExpanded(false) removes node from expandedTreeNodes", () => {
    useAppStore.getState().setTreeNodeExpanded("node1", true)
    useAppStore.getState().setTreeNodeExpanded("node1", false)
    expect(useAppStore.getState().expandedTreeNodes.node1).toBeUndefined()
  })

  it("toggleTreeNode() toggles node expansion", () => {
    useAppStore.getState().toggleTreeNode("node1")
    expect(useAppStore.getState().expandedTreeNodes.node1).toBe(true)

    useAppStore.getState().toggleTreeNode("node1")
    expect(useAppStore.getState().expandedTreeNodes.node1).toBeUndefined()
  })

  it("toggleTreeNode() handles expanding multiple independent nodes", () => {
    useAppStore.getState().toggleTreeNode("a")
    useAppStore.getState().toggleTreeNode("b")

    expect(useAppStore.getState().expandedTreeNodes).toEqual({ a: true, b: true })
  })

  // --- Pin sessions ---

  it("togglePinSession() pins a session", () => {
    useAppStore.getState().togglePinSession("s1")
    expect(useAppStore.getState().pinnedSessionIds.s1).toBe(true)
  })

  it("togglePinSession() unpins a pinned session", () => {
    useAppStore.getState().togglePinSession("s1")
    useAppStore.getState().togglePinSession("s1")
    expect(useAppStore.getState().pinnedSessionIds.s1).toBeUndefined()
  })

  it("togglePinSession() handles multiple sessions independently", () => {
    useAppStore.getState().togglePinSession("a")
    useAppStore.getState().togglePinSession("b")
    useAppStore.getState().togglePinSession("a")

    expect(useAppStore.getState().pinnedSessionIds).toEqual({ b: true })
  })
})
