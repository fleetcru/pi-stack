import { describe, it, expect, vi, afterEach, beforeAll, afterAll } from "vitest"
import { render, screen, within, cleanup, waitFor, fireEvent } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import "@testing-library/jest-dom/vitest"
import { CreateSessionDialog } from "./create-session-dialog"
import { useAppStore } from "@/state/app-store"

vi.mock("lucide-react", async (importOriginal) => {
  const actual: Record<string, unknown> = await importOriginal()
  return {
    ...actual,
    Folder: () => <span data-testid="icon-folder" />,
    FolderOpen: () => <span data-testid="icon-folder-open" />,
    House: () => <span data-testid="icon-house" />,
    LoaderCircle: () => <span data-testid="icon-loader" />,
  }
})

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

describe("CreateSessionDialog", () => {
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

  it("renders the dialog title", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByText("New session")).toBeInTheDocument()
  })

  it("defaults worker to local and shows More options content", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByRole("combobox", { name: "Worker" })).toHaveTextContent("Local")
    expect(within(dialog).getByText("Isolated git worktree")).toBeInTheDocument()
    expect(within(dialog).getByText("Title")).toBeInTheDocument()
    expect(within(dialog).getByText("Browse")).toBeInTheDocument()
    expect(within(dialog).getByText("Advanced options")).toBeInTheDocument()
  })

  it("creates one local session without a prompt", async () => {
    hookMocks.createSession.mockResolvedValue({ id: "local-1" })
    const onOpenChange = vi.fn()
    render(<CreateSessionDialog open={true} onOpenChange={onOpenChange} />)
    const user = userEvent.setup()
    await user.click(within(getDialogContent()).getByRole("button", { name: "Create session" }))
    await waitFor(() => expect(hookMocks.createSession).toHaveBeenCalledTimes(1))
    expect(hookMocks.createSession).toHaveBeenCalledWith(expect.objectContaining({ cwd: "/repo", start: true }))
    expect(useAppStore.getState().selectedSessionId).toBe("local-1")
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("shows a total batch failure", async () => {
    hookMocks.createSession.mockRejectedValue(new Error("capacity full"))
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const user = userEvent.setup()
    const count = within(getDialogContent()).getByRole("spinbutton")
    fireEvent.change(count, { target: { value: "2" } })
    await user.click(within(getDialogContent()).getByRole("button", { name: "Start 2 sessions" }))
    expect(await within(getDialogContent()).findByText("capacity full")).toBeInTheDocument()
    expect(hookMocks.createSession).toHaveBeenCalledTimes(2)
  })

  it("reports partial batch failure and selects the successful session", async () => {
    hookMocks.createSession
      .mockResolvedValueOnce({ id: "ok-1" })
      .mockRejectedValueOnce(new Error("failed"))
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const user = userEvent.setup()
    const count = within(getDialogContent()).getByRole("spinbutton")
    fireEvent.change(count, { target: { value: "2" } })
    await user.click(within(getDialogContent()).getByRole("button", { name: "Start 2 sessions" }))
    expect(await within(getDialogContent()).findByText("1 of 2 sessions started")).toBeInTheDocument()
    expect(useAppStore.getState().selectedSessionId).toBe("ok-1")
  })

  it("clamps the displayed batch count to twelve", async () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const count = within(getDialogContent()).getByRole("spinbutton")
    fireEvent.change(count, { target: { value: "99" } })
    expect(count).toHaveValue(12)
    expect(within(getDialogContent()).getByRole("button", { name: "Start 12 sessions" })).toBeInTheDocument()
  })

  it("blocks duplicate submissions while creation is pending", async () => {
    let resolve!: (value: { id: string }) => void
    hookMocks.createSession.mockReturnValue(new Promise((done) => { resolve = done }))
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const user = userEvent.setup()
    const submit = within(getDialogContent()).getByRole("button", { name: "Create session" })
    await user.click(submit)
    await user.click(submit)
    expect(hookMocks.createSession).toHaveBeenCalledTimes(1)
    resolve({ id: "later" })
    await waitFor(() => expect(useAppStore.getState().selectedSessionId).toBe("later"))
  })

  it("creates a remote worker session", async () => {
    hookMocks.createWorkerSession.mockResolvedValue({ id: "remote-session" })
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const user = userEvent.setup()
    await user.click(within(getDialogContent()).getByRole("combobox", { name: "Worker" }))
    await user.click(await screen.findByRole("option", { name: /remote-1/ }))
    await user.click(within(getDialogContent()).getByRole("button", { name: "Create session" }))
    await waitFor(() => expect(hookMocks.createWorkerSession).toHaveBeenCalledTimes(1))
    expect(hookMocks.createSession).not.toHaveBeenCalled()
  })

  it("passes workerId when browsing directories", async () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const user = userEvent.setup()
    await user.click(within(getDialogContent()).getByRole("combobox", { name: "Worker" }))
    await user.click(await screen.findByRole("option", { name: /remote-1/ }))
    await user.click(within(getDialogContent()).getByText("Browse"))
    await waitFor(() => expect(hookMocks.listDirectories).toHaveBeenCalledWith(undefined, "remote-1"))
  })

  it("does not render when open is false", () => {
    render(<CreateSessionDialog open={false} onOpenChange={vi.fn()} />)
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
  })
})
