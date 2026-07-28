import { describe, it, expect, vi, afterEach } from "vitest"
import { render, screen, within, cleanup } from "@testing-library/react"
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

vi.mock("@/api/hooks", () => ({
  useCreateSession: () => ({
    mutateAsync: vi.fn(),
    isPending: false,
    error: null,
  }),
  usePiServerClient: () => ({
    baseUrl: "http://localhost:3141",
    listDirectories: vi.fn().mockResolvedValue({}),
    getWorkerHealth: vi.fn(),
    createWorkerSession: vi.fn(),
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

  it("does not render when open is false", () => {
    render(<CreateSessionDialog open={false} onOpenChange={vi.fn()} />)
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument()
  })
})
