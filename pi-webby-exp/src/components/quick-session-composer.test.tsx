import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest"
import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import "@testing-library/jest-dom/vitest"

import { QuickSessionComposer } from "./quick-session-composer"

const hooks = vi.hoisted(() => ({
  createSession: vi.fn(),
  createWorkerSession: vi.fn(),
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
  useDirectoryRoots: (workerId: string) => ({
    data: workerId === "remote-1"
      ? [{ name: "remote-app", path: "/srv/remote-app" }]
      : [
          { name: "workspace", path: "/workspace" },
          { name: "other", path: "/other" },
        ],
    isLoading: false,
    isError: false,
  }),
  useWorkers: () => ({
    data: [
      { id: "local", url: "local", status: "ready" },
      { id: "remote-1", url: "https://worker.example", status: "ready" },
    ],
  }),
  usePiServerClient: () => ({
    createWorkerSession: hooks.createWorkerSession,
    prompt: hooks.prompt,
    sessionPost: hooks.sessionPost,
  }),
}))

describe("QuickSessionComposer", () => {
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
    vi.clearAllMocks()
  })

  it("creates a local session in the selected allowed folder and sends the message", async () => {
    hooks.createSession.mockResolvedValue({ id: "session-1" })
    hooks.prompt.mockResolvedValue({ ok: true })
    const onCreated = vi.fn()
    const user = userEvent.setup()

    render(<QuickSessionComposer onCreated={onCreated} onMoreOptions={vi.fn()} />)
    await user.type(screen.getByRole("textbox", { name: "First message for the new session" }), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    await waitFor(() => expect(hooks.createSession).toHaveBeenCalledWith({ cwd: "/workspace", start: true }))
    expect(onCreated).toHaveBeenCalledWith("session-1")
    expect(hooks.prompt).toHaveBeenCalledWith("session-1", { message: "Inspect the API" })
  })

  it("lets the user choose another allowed project root", async () => {
    hooks.createSession.mockResolvedValue({ id: "session-1" })
    hooks.prompt.mockResolvedValue({ ok: true })
    const user = userEvent.setup()

    render(<QuickSessionComposer onCreated={vi.fn()} onMoreOptions={vi.fn()} />)
    await user.click(screen.getByRole("combobox", { name: "Project folder" }))
    await user.click(await screen.findByRole("option", { name: /other/ }))
    await user.type(screen.getByRole("textbox"), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    await waitFor(() => expect(hooks.createSession).toHaveBeenCalledWith({ cwd: "/other", start: true }))
  })

  it("creates the session on the selected worker with that worker's allowed root", async () => {
    hooks.createWorkerSession.mockResolvedValue({ id: "remote-1:session-1" })
    hooks.prompt.mockResolvedValue({ ok: true })
    const user = userEvent.setup()

    render(<QuickSessionComposer onCreated={vi.fn()} onMoreOptions={vi.fn()} />)
    await user.click(screen.getByRole("combobox", { name: "Worker" }))
    await user.click(await screen.findByRole("option", { name: /remote-1/ }))
    await waitFor(() => expect(screen.getByRole("combobox", { name: "Project folder" })).toHaveTextContent("remote-app"))
    await user.type(screen.getByRole("textbox"), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    await waitFor(() => expect(hooks.createWorkerSession).toHaveBeenCalledWith("remote-1", { cwd: "/srv/remote-app", start: true }))
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
    await user.click(screen.getByRole("button", { name: "Choose model" }))
    await user.click(await screen.findByRole("option", { name: /GPT-5/ }))
    await user.type(screen.getByRole("textbox"), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    await waitFor(() => expect(hooks.sessionPost).toHaveBeenCalledWith("session-1", "model", { provider: "openai", modelId: "gpt-5" }))
    expect(hooks.sessionPost.mock.invocationCallOrder[0]).toBeLessThan(hooks.prompt.mock.invocationCallOrder[0])
  })

  it("applies the selected thinking effort before sending", async () => {
    hooks.createSession.mockResolvedValue({ id: "session-1" })
    hooks.sessionPost.mockResolvedValue({ ok: true })
    hooks.prompt.mockResolvedValue({ ok: true })
    const user = userEvent.setup()

    render(<QuickSessionComposer onCreated={vi.fn()} onMoreOptions={vi.fn()} />)
    await user.click(screen.getByRole("combobox", { name: "Thinking effort" }))
    await user.click(await screen.findByRole("option", { name: "High" }))
    await user.type(screen.getByRole("textbox"), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    await waitFor(() => expect(hooks.sessionPost).toHaveBeenCalledWith("session-1", "thinking-level", { level: "high" }))
    expect(hooks.sessionPost.mock.invocationCallOrder[0]).toBeLessThan(hooks.prompt.mock.invocationCallOrder[0])
  })

  it("can return to the session default after selecting a model", async () => {
    hooks.createSession.mockResolvedValue({ id: "session-1" })
    hooks.prompt.mockResolvedValue({ ok: true })
    const user = userEvent.setup()

    render(<QuickSessionComposer onCreated={vi.fn()} onMoreOptions={vi.fn()} />)
    await user.click(screen.getByRole("button", { name: "Choose model" }))
    await user.click(await screen.findByRole("option", { name: /GPT-5/ }))
    await user.click(screen.getByRole("button", { name: "Choose model" }))
    await user.click(await screen.findByRole("option", { name: "Default model" }))
    await user.type(screen.getByRole("textbox"), "Inspect the API")
    await user.click(screen.getByRole("button", { name: "Create session and send message" }))

    await waitFor(() => expect(hooks.prompt).toHaveBeenCalledOnce())
    expect(hooks.sessionPost).not.toHaveBeenCalled()
  })

  it("opens the full session dialog from More options", async () => {
    const onMoreOptions = vi.fn()
    render(<QuickSessionComposer onCreated={vi.fn()} onMoreOptions={onMoreOptions} />)

    await userEvent.click(screen.getByRole("button", { name: "More options" }))
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
