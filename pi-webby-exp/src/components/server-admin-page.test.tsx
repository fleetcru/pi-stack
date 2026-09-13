import { afterEach, beforeEach, describe, expect, it, vi } from "vitest"
import { cleanup, render, screen, waitFor } from "@testing-library/react"
import userEvent from "@testing-library/user-event"
import "@testing-library/jest-dom/vitest"
import { MemoryRouter } from "react-router"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"


import { ServerAdminPage } from "./server-admin-page"
import { useAppStore } from "@/state/app-store"

const settings = {
  addr: "0.0.0.0:3142",
  piBinary: "pi",
  extensions: [],
  cwd: "C:/work",
  dataDir: "C:/data",
  allowedOrigins: [],
  allowedRoots: ["C:/work"],
  allowedWorkerHosts: [],
  shutdownTimeout: "30s",
  requestTimeout: "30s",
  readTimeout: "30s",
  writeTimeout: "30s",
  idleTimeout: "2m",
  maxSessions: 8,
  maxActiveRuns: 8,
  maxRunsPerSession: 1,
  maxRunsPerWorker: 4,
  maxQueuedRuns: 32,
  distributedRunTimeout: "2h",
  restartMax: 3,
  restartBackoff: "1s",
  eventHistoryMax: 200,
  eventHistoryBytes: 8388608,
  maxWatches: 128,
  debug: false,
}

const adminState = {
  authenticationEnabled: true,
  overview: {
    apiVersion: "1",
    uptimeSeconds: 125,
    sessions: { active: 2, registered: 5, byTransport: { rpc: 5 }, max: 8 },
    workers: { total: 1, unhealthy: 0 },
    scheduler: { active: 2, queued: 0, globalLimit: 8, perSessionLimit: 1, perWorkerLimit: 4, queueLimit: 32, sessions: {}, workers: {} },
    warnings: [],
    configPath: "C:/data/admin-config.json",
  },
  settings,
  effectiveSettings: settings,
  sources: {},
  pairingEndpoints: [],
  runtimeFields: ["maxSessions", "maxActiveRuns", "maxRunsPerSession", "maxRunsPerWorker", "maxQueuedRuns"],
  restartRequired: false,
}

function jsonResponse(body: unknown, status = 200) {
  return new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } })
}

function configureServer() {
  const server = { baseUrl: "http://127.0.0.1:3142", name: "Local Pi", token: "secret" }
  useAppStore.setState({ servers: [server], connection: server })
}

function renderPage() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}><MemoryRouter><ServerAdminPage /></MemoryRouter></QueryClientProvider>)
}

describe("ServerAdminPage", () => {
  beforeEach(() => {
    localStorage.clear()
    useAppStore.setState({ servers: [], connection: undefined })
  })

  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it("shows an empty state without configured servers", () => {
    renderPage()
    expect(screen.getByText("No configured servers")).toBeTruthy()
  })

  it("loads the selected server through its bearer-auth admin API", async () => {
    const fetchMock = vi.fn().mockResolvedValue(jsonResponse(adminState))
    vi.stubGlobal("fetch", fetchMock)
    configureServer()

    renderPage()

    expect(await screen.findByText("2m")).toBeTruthy()
    expect(screen.getByText("5 registered")).toBeTruthy()
    expect(screen.getByText("Server healthy")).toBeTruthy()
    expect(fetchMock).toHaveBeenCalledWith(
      new URL("http://127.0.0.1:3142/v1/admin/state"),
      expect.objectContaining({ headers: expect.any(Headers) })
    )
    const headers = fetchMock.mock.calls[0][1].headers as Headers
    expect(headers.get("Authorization")).toBe("Bearer secret")
  })

  it("saves edited settings through the admin settings API", async () => {
    const fetchMock = vi.fn().mockImplementation((url: URL, init: RequestInit) => {
      if (url.pathname === "/v1/admin/settings" && init.method === "PUT") return Promise.resolve(jsonResponse({ ok: true, restartRequired: false }))
      return Promise.resolve(jsonResponse(adminState))
    })
    vi.stubGlobal("fetch", fetchMock)
    configureServer()
    const user = userEvent.setup()
    renderPage()

    const input = await screen.findByLabelText("Maximum sessions")
    await user.clear(input)
    await user.type(input, "12")
    await user.click(screen.getByRole("button", { name: "Save settings" }))

    await waitFor(() => expect(fetchMock.mock.calls.some((call) => {
      const [url, init] = call as [URL, RequestInit]
      return url.pathname === "/v1/admin/settings" && init.method === "PUT"
    })).toBe(true))
    const call = fetchMock.mock.calls.find((c) => (c as [URL])[0].pathname === "/v1/admin/settings")
    expect(JSON.parse(String(call?.[1] && (call[1] as RequestInit).body))).toMatchObject({ maxSessions: 12 })
    expect(await screen.findByText("Saved and applied to the running server.")).toBeTruthy()
  })

  it("confirms permanent deletion of a revoked device", async () => {
    const fetchMock = vi.fn().mockImplementation((url: URL, init: RequestInit) => {
      if (url.pathname === "/v1/devices") return Promise.resolve(jsonResponse({ devices: [{ id: "phone-1", name: "Phone", createdAt: "2026-01-01T00:00:00Z", lastSeen: "2026-01-02T00:00:00Z", revokedAt: "2026-01-03T00:00:00Z" }] }))
      if (url.pathname === "/v1/devices/phone-1/purge" && init.method === "DELETE") return Promise.resolve(jsonResponse({ deleted: "phone-1" }))
      return Promise.resolve(jsonResponse(adminState))
    })
    vi.stubGlobal("fetch", fetchMock)
    configureServer()
    const user = userEvent.setup()
    renderPage()

    await screen.findByText("Server healthy")
    await user.click(screen.getByRole("tab", { name: "Trusted devices" }))
    await screen.findByText("Phone")
    await user.click(screen.getByRole("button", { name: "Delete" }))
    await user.click(screen.getByRole("button", { name: "Delete" }))

    await waitFor(() => expect(fetchMock.mock.calls.some((call) => {
      const [url, init] = call as [URL, RequestInit]
      return url.pathname === "/v1/devices/phone-1/purge" && init.method === "DELETE"
    })).toBe(true))
  })
})
