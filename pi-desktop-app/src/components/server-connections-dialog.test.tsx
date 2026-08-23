import { cleanup, render, screen, within } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import { afterEach, describe, expect, it, vi } from "vitest"
import "@testing-library/jest-dom/vitest"
import { ServerConnectionsDialog } from "./server-connections-dialog"

const storeMocks = vi.hoisted(() => ({ addServer: vi.fn() }))

vi.mock("@/state/app-store", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) => selector({
    servers: [],
    addServer: storeMocks.addServer,
    removeServer: vi.fn(),
    selectServer: vi.fn(),
  }),
}))

describe("ServerConnectionsDialog", () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it("renders its connection form", () => {
    render(<ServerConnectionsDialog open onOpenChange={vi.fn()} />)
    const dialog = screen.getByRole("dialog")
    expect(within(dialog).getByPlaceholderText("https://pi-server.example:3141")).toBeInTheDocument()
    expect(within(dialog).getByRole("button", { name: "Add server" })).toBeDisabled()
  })

  it("warns before persisting a token", async () => {
    const user = userEvent.setup()
    render(<ServerConnectionsDialog open onOpenChange={vi.fn()} />)
    const dialog = screen.getByRole("dialog")
    await user.click(within(dialog).getByRole("checkbox"))
    expect(within(dialog).getByText(/stores the bearer token in local app storage/i)).toBeInTheDocument()
  })
})
