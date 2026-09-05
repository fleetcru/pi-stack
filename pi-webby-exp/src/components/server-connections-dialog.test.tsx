import { describe, it, expect, vi, afterEach } from "vitest"
import { render, screen, within, cleanup } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import "@testing-library/jest-dom/vitest"
import { ServerConnectionsDialog } from "./server-connections-dialog"
import { useAppStore } from "@/state/app-store"

// ---------------------------------------------------------------------------
// Mocks
// ---------------------------------------------------------------------------

vi.mock("lucide-react", async (importOriginal) => {
  const actual: Record<string, unknown> = await importOriginal()
  return {
    ...actual,
    Trash2: () => <span data-testid="icon-trash" />,
  }
})

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

/** Get the dialog content element (the last-rendered dialog). */
function getDialogContent(): HTMLElement {
  const dialogs = screen.getAllByRole("dialog")
  return dialogs[dialogs.length - 1]!
}

function renderDialog(
  props: { open?: boolean; onOpenChange?: (open: boolean) => void } = {}
) {
  const onOpenChange = props.onOpenChange ?? vi.fn()
  const result = render(
    <ServerConnectionsDialog
      open={props.open ?? true}
      onOpenChange={onOpenChange}
    />
  )
  return { ...result, onOpenChange }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("ServerConnectionsDialog", () => {
  afterEach(() => {
    cleanup()
    resetStore()
  })

  it("renders the dialog title and description", () => {
    renderDialog()
    const dialog = getDialogContent()
    expect(within(dialog).getByText("Pi servers")).toBeInTheDocument()
    expect(within(dialog).getByText(/Switch between trusted pi-server instances/)).toBeInTheDocument()
  })

  it("renders add server button", () => {
    renderDialog()
    const dialog = getDialogContent()
    expect(within(dialog).getByText("Add server")).toBeInTheDocument()
  })

  it("add button is disabled when baseUrl is empty", () => {
    renderDialog()
    const dialog = getDialogContent()
    expect(within(dialog).getByText("Add server")).toBeDisabled()
  })

  it("shows existing servers in the list", () => {
    useAppStore.getState().addServer({ baseUrl: "http://a.example:3141", name: "My Server" })
    useAppStore.getState().addServer({ baseUrl: "http://b.example:3141", name: "Remote" })
    renderDialog()
    const dialog = getDialogContent()
    expect(within(dialog).getByText("My Server")).toBeInTheDocument()
    expect(within(dialog).getByText("Remote")).toBeInTheDocument()
  })

  it("shows remove button for each server", () => {
    useAppStore.getState().addServer({ baseUrl: "http://a.example:3141", name: "Test" })
    renderDialog()
    expect(screen.getByTestId("icon-trash")).toBeInTheDocument()
  })

  it("removes a server when trash is clicked", async () => {
    useAppStore.getState().addServer({ baseUrl: "http://a.example:3141", name: "Remove Me" })
    renderDialog()
    const trashBtn = screen.getByLabelText("Remove Remove Me")
    await userEvent.click(trashBtn)
    await userEvent.click(screen.getByLabelText("Confirm remove Remove Me"))
    expect(useAppStore.getState().servers).toHaveLength(0)
    expect(screen.queryByText("Remove Me")).not.toBeInTheDocument()
  })

  it("sets active connection when a server is clicked", async () => {
    useAppStore.getState().addServer({ baseUrl: "http://a.example:3141", name: "Click Me" })
    const { onOpenChange } = renderDialog()
    const serverBtn = screen.getByText("Click Me")
    await userEvent.click(serverBtn)
    expect(useAppStore.getState().connection?.baseUrl).toBe("http://a.example:3141")
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })

  it("accepts user input for name and URL", async () => {
    renderDialog()
    const dialog = getDialogContent()
    const nameInput = within(dialog).getByPlaceholderText("Server name (optional)")
    const urlInput = within(dialog).getByPlaceholderText("https://pi-server.example:3141")
    await userEvent.type(nameInput, "New Server")
    await userEvent.type(urlInput, "http://new.example:3141")
    expect(nameInput).toHaveValue("New Server")
    expect(urlInput).toHaveValue("http://new.example:3141")
  })

  it("shows HTTP warning for non-localhost HTTP URLs", async () => {
    renderDialog()
    const dialog = getDialogContent()
    const urlInput = within(dialog).getByPlaceholderText("https://pi-server.example:3141")
    await userEvent.type(urlInput, "http://remote.example:3141")
    expect(within(dialog).getByText(/HTTP sends your token in plaintext/)).toBeInTheDocument()
  })

  it("does not show HTTP warning for localhost", async () => {
    renderDialog()
    const dialog = getDialogContent()
    const urlInput = within(dialog).getByPlaceholderText("https://pi-server.example:3141")
    await userEvent.type(urlInput, "http://localhost:3141")
    expect(within(dialog).queryByText(/HTTP sends your token in plaintext/)).not.toBeInTheDocument()
  })

  it("add server button becomes enabled with valid URL", async () => {
    renderDialog()
    const dialog = getDialogContent()
    const urlInput = within(dialog).getByPlaceholderText("https://pi-server.example:3141")
    const addBtn = within(dialog).getByText("Add server")
    expect(addBtn).toBeDisabled()
    await userEvent.type(urlInput, "http://new.example:3141")
    expect(addBtn).not.toBeDisabled()
  })

  it("adds a server via the add button", async () => {
    renderDialog()
    const dialog = getDialogContent()
    const urlInput = within(dialog).getByPlaceholderText("https://pi-server.example:3141")
    await userEvent.type(urlInput, "http://new.example:3141")
    await userEvent.click(within(dialog).getByText("Add server"))
    expect(useAppStore.getState().servers).toHaveLength(1)
    expect(useAppStore.getState().servers[0]!.baseUrl).toBe("http://new.example:3141")
  })

  it("calls onOpenChange(false) when dialog requests close", () => {
    const { onOpenChange } = renderDialog()
    onOpenChange(false)
    expect(onOpenChange).toHaveBeenCalledWith(false)
  })
})
