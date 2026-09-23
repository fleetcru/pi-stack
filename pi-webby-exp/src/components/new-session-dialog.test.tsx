import { describe, it, expect, vi, afterEach, beforeAll, afterAll } from "vitest"
import { render, screen, within, cleanup, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import "@testing-library/jest-dom/vitest"
import { NewSessionDialog } from "./new-session-dialog"
import { useAppStore } from "@/state/app-store"

const hookMocks = vi.hoisted(() => ({
  createSession: vi.fn(),
  createWorkerSession: vi.fn(),
  prompt: vi.fn(),
  sessionPost: vi.fn(),
  listDirectories: vi.fn().mockResolvedValue({ path: "/repo", directories: [], roots: [{ name: "repo", path: "/repo" }] }),
}))

vi.mock("@/api/hooks", () => ({
  useCreateSession: () => ({
    mutateAsync: hookMocks.createSession,
    isPending: false,
    error: null,
  }),
  useAvailableModels: () => ({ data: { models: [] }, isLoading: false }),
  useDirectoryRoots: () => ({
    data: [{ name: "repo", path: "/repo" }],
    isLoading: false,
    isError: false,
  }),
  usePiServerClient: () => ({
    baseUrl: "http://localhost:3141",
    listDirectories: hookMocks.listDirectories,
    getWorkerHealth: vi.fn(),
    createWorkerSession: hookMocks.createWorkerSession,
    prompt: hookMocks.prompt,
    sessionPost: hookMocks.sessionPost,
  }),
  useWorkers: () => ({
    data: [
      { id: "local", url: "", status: "ok" },
      { id: "remote-1", url: "http://r1:3141", status: "ok" },
    ],
  }),
}))

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

function getDialogContent(): HTMLElement {
  const dialogs = screen.getAllByRole("dialog")
  return dialogs[dialogs.length - 1]!
}

describe("NewSessionDialog", () => {
  beforeAll(() => {
    vi.stubGlobal("ResizeObserver", class {
      observe() {}
      unobserve() {}
      disconnect() {}
    })
    window.HTMLElement.prototype.scrollIntoView = vi.fn()
  })
  afterAll(() => vi.unstubAllGlobals())

  afterEach(() => {
    cleanup()
    resetStore()
    vi.clearAllMocks()
  })

  it("embeds the quick session composer, not a separate legacy form", () => {
    render(<NewSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByText("New session")).toBeInTheDocument()
    expect(within(dialog).getByRole("textbox", { name: "First message for the new session" })).toBeInTheDocument()
    expect(within(dialog).getByRole("combobox", { name: "Worker" })).toHaveTextContent("Local")
    // Advanced panel is collapsed by default (composer-first)
    expect(within(dialog).queryByText("Isolated git worktree")).not.toBeInTheDocument()
    expect(within(dialog).getByRole("button", { name: /Advanced session options/ })).toBeInTheDocument()
  })

  it("creates a session from the composer and closes", async () => {
    hookMocks.createSession.mockResolvedValue({ id: "local-1" })
    const onOpenChange = vi.fn()
    render(<NewSessionDialog open={true} onOpenChange={onOpenChange} />)
    const user = userEvent.setup()
    await user.click(within(getDialogContent()).getByRole("button", { name: "Create session" }))
    await waitFor(() => expect(hookMocks.createSession).toHaveBeenCalledTimes(1))
    expect(hookMocks.createSession).toHaveBeenCalledWith(expect.objectContaining({ cwd: "/repo", start: true }))
    expect(useAppStore.getState().selectedSessionId).toBe("local-1")
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("does not render when open is false", () => {
    render(<NewSessionDialog open={false} onOpenChange={vi.fn()} />)
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
  })
})
