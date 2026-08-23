import { describe, it, expect, vi, afterEach } from "vitest"
import { render, screen, within, cleanup, waitFor, fireEvent } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import "@testing-library/jest-dom/vitest"
import { CreateSessionDialog } from "./create-session-dialog"
import { useAppStore } from "@/state/app-store"

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

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
}))

vi.mock("@/api/hooks", () => ({
  useCreateSession: () => ({
    mutateAsync: hookMocks.createSession,
    isPending: false,
    error: null,
  }),
  usePiServerClient: () => ({
    baseUrl: "http://localhost:3141",
    listDirectories: vi.fn().mockResolvedValue({}),
    getWorkerHealth: vi.fn(),
    createWorkerSession: hookMocks.createWorkerSession,
  }),
  useWorkers: () => ({
    data: [
      { id: "local", url: "", status: "ok" },
      { id: "remote-1", url: "http://r1:3141", status: "ok" },
    ],
  }),
}))

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("CreateSessionDialog", () => {
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

  it("renders the dialog description", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByText(/Start a Pi agent in an allowed project folder/)).toBeInTheDocument()
  })

  it("renders the Worker label and Check health button", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByText("Worker")).toBeInTheDocument()
    expect(within(dialog).getByText("Check health")).toBeInTheDocument()
  })

  it("renders the Project folder input", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByPlaceholderText("/home/user/project")).toBeInTheDocument()
  })

  it("renders the Title input as optional", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByText("Title")).toBeInTheDocument()
    expect(within(dialog).getByText("(optional)")).toBeInTheDocument()
    expect(within(dialog).getByPlaceholderText("Refactor authentication")).toBeInTheDocument()
  })

  it("renders the Browse button", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByText("Browse")).toBeInTheDocument()
  })

  it("renders the Advanced options collapsible", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByText("Advanced options")).toBeInTheDocument()
  })

  it("create session button is disabled when cwd is empty", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    const submitBtn = within(dialog).getByText("Create session")
    expect(submitBtn).toBeDisabled()
  })

  it("renders Worker select placeholder", () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const dialog = getDialogContent()
    expect(within(dialog).getByText("Select a worker")).toBeInTheDocument()
  })

  async function fillRequiredFields(workerName = "local") {
    const user = userEvent.setup()
    const dialog = getDialogContent()
    await user.type(within(dialog).getByPlaceholderText("/home/user/project"), "/repo")
    await user.click(within(dialog).getByRole("combobox"))
    await user.click(await screen.findByRole("option", { name: new RegExp(workerName, "i") }))
    return user
  }

  it("creates one local session", async () => {
    hookMocks.createSession.mockResolvedValue({ id: "local-1" })
    const onOpenChange = vi.fn()
    render(<CreateSessionDialog open={true} onOpenChange={onOpenChange} />)
    const user = await fillRequiredFields()

    await user.click(within(getDialogContent()).getByRole("button", { name: "Create session" }))

    await waitFor(() => expect(hookMocks.createSession).toHaveBeenCalledTimes(1))
    expect(useAppStore.getState().selectedSessionId).toBe("local-1")
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("shows a total batch failure", async () => {
    hookMocks.createSession.mockRejectedValue(new Error("capacity full"))
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const user = await fillRequiredFields()
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
    const user = await fillRequiredFields()
    const count = within(getDialogContent()).getByRole("spinbutton")
    fireEvent.change(count, { target: { value: "2" } })
    await user.click(within(getDialogContent()).getByRole("button", { name: "Start 2 sessions" }))

    expect(await within(getDialogContent()).findByText("1 of 2 sessions started")).toBeInTheDocument()
    expect(useAppStore.getState().selectedSessionId).toBe("ok-1")
  })

  it("clamps the displayed batch count to twelve", async () => {
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const user = await fillRequiredFields()
    const count = within(getDialogContent()).getByRole("spinbutton")
    await user.clear(count)
    await user.type(count, "99")
    expect(count).toHaveValue(12)
    expect(within(getDialogContent()).getByRole("button", { name: "Start 12 sessions" })).toBeInTheDocument()
  })

  it("blocks duplicate submissions while creation is pending", async () => {
    let resolve!: (value: { id: string }) => void
    hookMocks.createSession.mockReturnValue(new Promise((done) => { resolve = done }))
    render(<CreateSessionDialog open={true} onOpenChange={vi.fn()} />)
    const user = await fillRequiredFields()
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
    const user = await fillRequiredFields("remote-1")
    await user.click(within(getDialogContent()).getByRole("button", { name: "Create session" }))
    await waitFor(() => expect(hookMocks.createWorkerSession).toHaveBeenCalledTimes(1))
    expect(hookMocks.createSession).not.toHaveBeenCalled()
  })

  it("does not render when open is false", () => {
    render(<CreateSessionDialog open={false} onOpenChange={vi.fn()} />)
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
  })
})
