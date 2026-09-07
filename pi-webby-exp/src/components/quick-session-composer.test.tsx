import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import "@testing-library/jest-dom/vitest"

import { QuickSessionComposer } from "./quick-session-composer"

const hooks = vi.hoisted(() => ({
  createSession: vi.fn(),
  prompt: vi.fn(),
  sessionPost: vi.fn(),
}))

vi.mock("@/api/hooks", () => ({
  useAvailableModels: () => ({
    data: { models: [
      { provider: "openai", id: "gpt-5", name: "GPT-5" },
      { provider: "google", id: "gemini-pro", name: "Gemini Pro" },
    ] },
    isLoading: false,
  }),
  useCreateSession: () => ({ mutateAsync: hooks.createSession, isPending: false }),
  usePiServerClient: () => ({ prompt: hooks.prompt, sessionPost: hooks.sessionPost }),
}))

describe("QuickSessionComposer", () => {
  afterEach(() => {
    cleanup()
    vi.clearAllMocks()
  })

  it("creates a local session in the server default folder and sends the message", async () => {
    hooks.createSession.mockResolvedValue({ id: "session-1" })
    hooks.prompt.mockResolvedValue({ ok: true })
    const onCreated = vi.fn()
    const user = userEvent.setup()

    render(<QuickSessionComposer onCreated={onCreated} onMoreOptions={vi.fn()} />)
    await user.type(screen.getByRole("textbox", { name: "First message for the new session" }), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    await waitFor(() => expect(hooks.createSession).toHaveBeenCalledWith({ cwd: "", start: true }))
    expect(onCreated).toHaveBeenCalledWith("session-1")
    expect(hooks.prompt).toHaveBeenCalledWith("session-1", { message: "Inspect the API" })
  })

  it("keeps the composer open when the session starts but the message fails", async () => {
    hooks.createSession.mockResolvedValue({ id: "session-1" })
    hooks.prompt.mockRejectedValue(new Error("capacity full"))
    const onCreated = vi.fn()
    const user = userEvent.setup()

    render(<QuickSessionComposer onCreated={onCreated} onMoreOptions={vi.fn()} />)
    await user.type(screen.getByRole("textbox"), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    expect(await screen.findByRole("alert")).toHaveTextContent("Retry will use the existing session")
    expect(onCreated).not.toHaveBeenCalled()

    hooks.prompt.mockResolvedValue({ ok: true })
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))
    await waitFor(() => expect(onCreated).toHaveBeenCalledWith("session-1"))
    expect(hooks.createSession).toHaveBeenCalledOnce()
    expect(hooks.prompt).toHaveBeenCalledTimes(2)
  })

  it("applies the selected provider and model before sending", async () => {
    hooks.createSession.mockResolvedValue({ id: "session-1" })
    hooks.sessionPost.mockResolvedValue({ ok: true })
    hooks.prompt.mockResolvedValue({ ok: true })
    const user = userEvent.setup()

    render(<QuickSessionComposer onCreated={vi.fn()} onMoreOptions={vi.fn()} />)
    await user.click(screen.getByRole("combobox", { name: "Model provider" }))
    await user.click(await screen.findByRole("option", { name: "openai" }))
    await user.click(screen.getByRole("combobox", { name: "Model" }))
    await user.click(await screen.findByRole("option", { name: "GPT-5" }))
    await user.type(screen.getByRole("textbox"), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    await waitFor(() => expect(hooks.sessionPost).toHaveBeenCalledWith("session-1", "model", { provider: "openai", modelId: "gpt-5" }))
    expect(hooks.sessionPost.mock.invocationCallOrder[0]).toBeLessThan(hooks.prompt.mock.invocationCallOrder[0])
  })

  it("opens the full session dialog from Choose project", async () => {
    const onMoreOptions = vi.fn()
    render(<QuickSessionComposer onCreated={vi.fn()} onMoreOptions={onMoreOptions} />)

    await userEvent.click(screen.getByRole("button", { name: "Choose project" }))
    expect(onMoreOptions).toHaveBeenCalledOnce()
  })

  it("submits with Control+Enter", async () => {
    hooks.createSession.mockResolvedValue({ id: "session-2" })
    hooks.prompt.mockResolvedValue({ ok: true })
    const user = userEvent.setup()
    render(<QuickSessionComposer onCreated={vi.fn()} onMoreOptions={vi.fn()} />)

    const input = screen.getByRole("textbox", { name: "First message for the new session" })
    await user.type(input, "Run tests")
    await user.keyboard("{Control>}{Enter}{/Control}")

    await waitFor(() => expect(hooks.createSession).toHaveBeenCalledOnce())
  })
})
