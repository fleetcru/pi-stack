import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, render, screen } from "@testing-library/react"
import "@testing-library/jest-dom/vitest"

import { PiServerClient, type PerformanceReport } from "@/api/client"
import { PerformancePanel } from "./performance-panel"

const report: PerformanceReport = {
  sampleIntervalSeconds: 15,
  historyLimit: 240,
  server: {
    id: "server",
    name: "pi-server",
    kind: "server",
    pid: 4321,
    running: true,
    current: { at: "2026-01-01T00:00:00Z", cpuPercent: 12.5, rssBytes: 64 * 1024 * 1024, heapAllocBytes: 8 * 1024 * 1024, requestRate: 3, errorRate: 0.5, averageLatencyMs: 4, activeSessions: 2, activeRuns: 1, queuedRuns: 0, goroutines: 42 },
    samples: [
      { at: "2026-01-01T00:00:00Z", cpuPercent: 5, rssBytes: 32 * 1024 * 1024 },
      { at: "2026-01-01T00:00:15Z", cpuPercent: 12.5, rssBytes: 64 * 1024 * 1024 },
    ],
  },
  sessions: [
    { id: "s1", name: "alpha", kind: "session", pid: 1001, running: true, current: { at: "2026-01-01T00:00:00Z", cpuPercent: 30, rssBytes: 1024 * 1024 }, samples: [] },
  ],
  requests: [{ route: "/v1/sessions/x/prompt", count: 10, errors: 2, averageMs: 12, maxMs: 90 }],
}

describe("PerformancePanel (desktop)", () => {
  afterEach(() => {
    cleanup()
  })

  it("renders server stat cards, sessions, and routes", async () => {
    const performance = vi.fn().mockResolvedValue(report)
    const client = { cacheScope: "test", performance } as unknown as PiServerClient
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={queryClient}><PerformancePanel client={client} /></QueryClientProvider>)

    expect(await screen.findByText("12.5%")).toBeTruthy()
    expect(screen.getByText("Go heap 8.0 MiB")).toBeTruthy()
    expect(screen.getByText("alpha")).toBeTruthy()
    expect(screen.getByText("/v1/sessions/x/prompt")).toBeTruthy()
  })

  it("shows an error state with retry when the query fails", async () => {
    const performance = vi.fn().mockRejectedValue(new Error("boom"))
    const client = { cacheScope: "test", performance } as unknown as PiServerClient
    const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(<QueryClientProvider client={queryClient}><PerformancePanel client={client} /></QueryClientProvider>)

    expect(await screen.findByText("Could not load performance data")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy()
  })
})
