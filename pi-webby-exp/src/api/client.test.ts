import { describe, it, expect, vi, beforeEach, afterEach } from "vitest"
import {
  PiServerClient,
  PiServerApiError,
  type CreateSessionRequest,
} from "./client"

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function mockFetch(responses: Array<{ status: number; body?: unknown }>) {
  let call = 0
  return vi.fn().mockImplementation(async (...args: [RequestInfo | URL, RequestInit?]) => {
    void args
    const { status, body } = responses[Math.min(call++, responses.length - 1)] ?? {
      status: 200,
      body: {},
    }
    return new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    })
  })
}

function makeClient(
  fetchFn: typeof globalThis.fetch,
  options?: { baseUrl?: string; token?: string }
): PiServerClient {
  return new PiServerClient({
    baseUrl: options?.baseUrl ?? "http://localhost:3141",
    token: options?.token,
    fetch: fetchFn,
  })
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe("PiServerClient", () => {
  let originalFetch: typeof globalThis.fetch

  beforeEach(() => {
    originalFetch = globalThis.fetch
  })

  afterEach(() => {
    globalThis.fetch = originalFetch
    vi.restoreAllMocks()
  })

  // --- Constructor / base URL ---

  it("strips trailing slashes from baseUrl", () => {
    const fetchFn = vi.fn()
    const client = makeClient(fetchFn, { baseUrl: "http://localhost:3141///" })
    expect(client.baseUrl).toBe("http://localhost:3141")
  })

  it("uses default baseUrl when none provided", () => {
    // Reset import.meta.env so fallback applies
    const fetchFn = vi.fn()
    const client = new PiServerClient({ fetch: fetchFn })
    expect(client.baseUrl).toBeDefined()
  })

  // --- Auth headers ---

  it("sends Authorization header when token is provided", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn, { token: "secret123" })

    await client.health()

    expect(fetchFn).toHaveBeenCalledTimes(1)
    const [, init] = fetchFn.mock.calls[0]!
    const headers = init!.headers as Headers
    expect(headers.get("Authorization")).toBe("Bearer secret123")
  })

  it("omits Authorization header when no token", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn)

    await client.health()

    const [, init] = fetchFn.mock.calls[0]!
    const headers = init!.headers as Headers
    expect(headers.get("Authorization")).toBeNull()
  })

  it("always sends Accept: application/json", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn)

    await client.health()

    const [, init] = fetchFn.mock.calls[0]!
    const headers = init!.headers as Headers
    expect(headers.get("Accept")).toBe("application/json")
  })

  // --- Request routing ---

  it("health() hits /healthz with GET", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true, apiVersion: "1.0" } }])
    const client = makeClient(fetchFn)

    const result = await client.health()

    expect(fetchFn).toHaveBeenCalledTimes(1)
    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/healthz")
    expect(init!.method).toBe("GET")
    expect(result.ok).toBe(true)
  })

  it("capabilities() hits /v1/capabilities", async () => {
    const caps = { sessions: true, workers: false }
    const fetchFn = mockFetch([{ status: 200, body: caps }])
    const client = makeClient(fetchFn)

    const result = await client.capabilities()

    const [url] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/capabilities")
    expect(result).toEqual(caps)
  })

  it("listSessions() hits /v1/sessions?scope=all&include=state", async () => {
    const body = { sessions: [], partialFailures: [] }
    const fetchFn = mockFetch([{ status: 200, body }])
    const client = makeClient(fetchFn)

    await client.listSessions()

    const [url] = fetchFn.mock.calls[0]!
    const u = new URL(String(url))
    expect(u.pathname).toBe("/v1/sessions")
    expect(u.searchParams.get("scope")).toBe("all")
    expect(u.searchParams.get("include")).toBe("state")
  })

  it("createSession() sends POST with JSON body", async () => {
    const resp = { id: "abc", cwd: "/tmp", args: [], ws: "/v1/sessions/abc/ws" }
    const fetchFn = mockFetch([{ status: 201, body: resp }])
    const client = makeClient(fetchFn)

    const input: CreateSessionRequest = { cwd: "/tmp", start: true }
    const result = await client.createSession(input)

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/sessions")
    expect(init!.method).toBe("POST")
    expect(JSON.parse(init!.body as string)).toEqual(input)
    expect(result.id).toBe("abc")
  })

  it("deleteSession() sends DELETE to correct path", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { deleted: "abc" } }])
    const client = makeClient(fetchFn)

    await client.deleteSession("abc")

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/sessions/abc")
    expect(init!.method).toBe("DELETE")
  })

  it("URL-encodes special characters in session IDs", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn)

    await client.getSession("a/b=c&d")

    const [url] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain(encodeURIComponent("a/b=c&d"))
  })

  it("prompt() sends POST to session prompt endpoint", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn)

    await client.prompt("s1", { message: "hello" } as never)

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/sessions/s1/prompt")
    expect(init!.method).toBe("POST")
  })

  it("steer() sends POST to session steer endpoint", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn)

    await client.steer("s1", { message: "hi" } as never)

    const [url] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/sessions/s1/steer")
  })

  it("abort() sends POST with empty body", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn)

    await client.abort("s1")

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/sessions/s1/abort")
    expect(JSON.parse(init!.body as string)).toEqual({})
  })

  it("listWorkers() hits /v1/workers", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { workers: [] } }])
    const client = makeClient(fetchFn)

    const result = await client.listWorkers()

    const [url] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/workers")
    expect(result.workers).toEqual([])
  })

  it("addWorker() sends POST with worker body", async () => {
    const worker = { id: "w1", url: "http://remote:3141" }
    const fetchFn = mockFetch([{ status: 200, body: worker }])
    const client = makeClient(fetchFn)

    await client.addWorker(worker)

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/workers")
    expect(init!.method).toBe("POST")
    expect(JSON.parse(init!.body as string)).toEqual(worker)
  })

  it("deleteWorker() sends DELETE", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn)

    await client.deleteWorker("w1")

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/workers/w1")
    expect(init!.method).toBe("DELETE")
  })

  it("deleteWorker(force=true) appends ?force=true", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ok: true } }])
    const client = makeClient(fetchFn)

    await client.deleteWorker("w1", true)

    const [url] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("force=true")
  })

  it("issueWebSocketTicket() sends POST", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { ws: "/ws?ticket=abc" } }])
    const client = makeClient(fetchFn)

    const ticket = await client.issueWebSocketTicket("s1")

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/ws-tickets")
    expect(init!.method).toBe("POST")
    expect(ticket.ws).toBe("/ws?ticket=abc")
  })

  it("webSocketUrl() converts http to ws", () => {
    const client = makeClient(vi.fn(), { baseUrl: "http://localhost:3141" })
    const url = client.webSocketUrl("/v1/sessions/s1/ws")
    expect(url).toContain("ws://localhost:3141")
  })

  it("webSocketUrl() converts https to wss", () => {
    const client = makeClient(vi.fn(), { baseUrl: "https://pi.example.com:3141" })
    const url = client.webSocketUrl("/v1/sessions/s1/ws")
    expect(url).toContain("wss://pi.example.com:3141")
  })

  // --- Error handling ---

  it("throws PiServerApiError on HTTP error with structured body", async () => {
    const fetchFn = mockFetch([
      {
        status: 404,
        body: { error: "Session not found", code: "NOT_FOUND", requestId: "r1" },
      },
    ])
    const client = makeClient(fetchFn)

    await expect(client.getSession("missing")).rejects.toThrow(PiServerApiError)

    try {
      await client.getSession("missing")
    } catch (err) {
      expect(err).toBeInstanceOf(PiServerApiError)
      const apiErr = err as PiServerApiError
      expect(apiErr.status).toBe(404)
      expect(apiErr.message).toBe("Session not found")
      expect(apiErr.code).toBe("NOT_FOUND")
      expect(apiErr.requestId).toBe("r1")
    }
  })

  it("throws PiServerApiError with statusText on unstructured error", async () => {
    const fetchFn = mockFetch([{ status: 500, body: "Internal Server Error" }])
    const client = makeClient(fetchFn)

    try {
      await client.health()
      expect.fail("Should have thrown")
    } catch (err) {
      expect(err).toBeInstanceOf(PiServerApiError)
      expect((err as PiServerApiError).status).toBe(500)
    }
  })

  it("throws PiServerApiError(0) on network failure", async () => {
    const fetchFn = vi.fn().mockRejectedValue(new Error("fetch failed"))
    const client = makeClient(fetchFn)

    try {
      await client.health()
      expect.fail("Should have thrown")
    } catch (err) {
      expect(err).toBeInstanceOf(PiServerApiError)
      expect((err as PiServerApiError).status).toBe(0)
      expect((err as PiServerApiError).message).toBe("fetch failed")
    }
  })

  it("throws on null response payload", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      new Response("null", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    )
    const client = makeClient(fetchFn)

    try {
      await client.health()
      expect.fail("Should have thrown")
    } catch (err) {
      expect(err).toBeInstanceOf(PiServerApiError)
      expect((err as PiServerApiError).message).toContain("Unexpected response format")
    }
  })

  it("throws on non-object response (string)", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      new Response('"ok"', {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    )
    const client = makeClient(fetchFn)

    try {
      await client.health()
      expect.fail("Should have thrown")
    } catch (err) {
      expect(err).toBeInstanceOf(PiServerApiError)
      expect((err as PiServerApiError).message).toContain("Unexpected response format")
    }
  })

  // --- Query parameter construction ---

  it("getSessionEvents() builds query with since and limit", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { events: [], since: 0 } }])
    const client = makeClient(fetchFn)

    await client.getSessionEvents("s1", 42, 50)

    const [url] = fetchFn.mock.calls[0]!
    const u = new URL(String(url))
    expect(u.searchParams.get("since")).toBe("42")
    expect(u.searchParams.get("limit")).toBe("50")
  })

  it("getSessionEvents() omits since when undefined", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { events: [], since: 0 } }])
    const client = makeClient(fetchFn)

    await client.getSessionEvents("s1", undefined, 100)

    const [url] = fetchFn.mock.calls[0]!
    const u = new URL(String(url))
    expect(u.searchParams.has("since")).toBe(false)
    expect(u.searchParams.get("limit")).toBe("100")
  })

  it("listDirectories() includes path query param when provided", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { path: "/home" } }])
    const client = makeClient(fetchFn)

    await client.listDirectories("/home/user")

    const [url] = fetchFn.mock.calls[0]!
    const u = new URL(String(url))
    expect(u.searchParams.get("path")).toBe("/home/user")
  })

  it("listDirectories() omits query when path is undefined", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { roots: [] } }])
    const client = makeClient(fetchFn)

    await client.listDirectories()

    const [url] = fetchFn.mock.calls[0]!
    const u = new URL(String(url))
    expect(u.search).toBe("")
  })

  it("getFileTree() passes cwd as query param", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { cwd: "/src", files: [] } }])
    const client = makeClient(fetchFn)

    await client.getFileTree("/src")

    const [url] = fetchFn.mock.calls[0]!
    const u = new URL(String(url))
    expect(u.searchParams.get("cwd")).toBe("/src")
  })

  it("getSessionMessages() builds offset and limit query params", async () => {
    const fetchFn = mockFetch([{ status: 200, body: {} }])
    const client = makeClient(fetchFn)

    await client.getSessionMessages("s1", 25, 50)

    const [url] = fetchFn.mock.calls[0]!
    const u = new URL(String(url))
    expect(u.searchParams.get("offset")).toBe("25")
    expect(u.searchParams.get("limit")).toBe("50")
  })

  // --- PATCH method ---

  it("updateCapacity() sends PATCH with body", async () => {
    const fetchFn = mockFetch([{ status: 200, body: { activeSessions: 0, maxSessions: 4 } }])
    const client = makeClient(fetchFn)

    await client.updateCapacity(4)

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/capacity")
    expect(init!.method).toBe("PATCH")
    expect(typeof init!.body).toBe("string")
  })
})
