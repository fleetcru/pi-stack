import { describe, expect, it, vi } from "vitest"
import { PiServerClient } from "./client"

describe("PiServerClient.performance (desktop re-export of shared client)", () => {
  it("hits /v1/performance with GET and the bearer token", async () => {
    const fetchFn = vi.fn().mockResolvedValue(
      new Response(JSON.stringify({ sampleIntervalSeconds: 15, historyLimit: 240, server: { id: "server", name: "pi-server", kind: "server", pid: 1234, running: true, current: null, samples: [] }, sessions: [], requests: [] }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      })
    )
    const client = new PiServerClient({ baseUrl: "http://localhost:3142", token: "secret", fetch: fetchFn })

    const result = await client.performance()

    const [url, init] = fetchFn.mock.calls[0]!
    expect(String(url)).toContain("/v1/performance")
    expect(init!.method).toBe("GET")
    const headers = new Headers(init!.headers)
    expect(headers.get("Authorization")).toBe("Bearer secret")
    expect(result.sampleIntervalSeconds).toBe(15)
  })
})
