import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { afterEach, describe, expect, it, vi } from "vitest"
import { cleanup, render, screen } from "@testing-library/react"
import "@testing-library/jest-dom/vitest"

import { PiServerClient, type PerformanceReport } from "@/api/client"
import { PerformancePanel, PerformancePanelView, Sparkline } from "./performance-panel"

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
    { id: "s2", name: "beta", kind: "session", pid: 0, running: false, current: { at: "2026-01-01T00:00:00Z", cpuPercent: 0, rssBytes: 512 * 1024 }, samples: [] },
  ],
  requests: [
    { route: "/v1/sessions/x/prompt", count: 10, errors: 2, averageMs: 12, maxMs: 90 },
    { route: "/healthz", count: 40, errors: 0, averageMs: 1, maxMs: 3 },
  ],
}

function stubClient(performance: () => Promise<PerformanceReport>) {
  return { cacheScope: "test", performance } as unknown as PiServerClient
}

function renderPanel(client: PiServerClient) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}><PerformancePanel client={client} /></QueryClientProvider>)
}

describe("PerformancePanel", () => {
  afterEach(() => {
    cleanup()
  })

  it("renders server stat cards, session rows, and route rows", async () => {
    const performance = vi.fn().mockResolvedValue(report)
    renderPanel(stubClient(performance))

    expect(await screen.findByText("12.5%")).toBeTruthy()
    expect(screen.getByText("64 MiB")).toBeTruthy()
    expect(screen.getByText("Go heap 8.0 MiB")).toBeTruthy()
    expect(screen.getByText("alpha")).toBeTruthy()
    expect(screen.getByText("Running")).toBeTruthy()
    expect(screen.getByText("Stopped")).toBeTruthy()
    expect(screen.getByText("/v1/sessions/x/prompt")).toBeTruthy()
    expect(screen.getByRole("img", { name: /Server CPU history/ })).toBeTruthy()
    expect(screen.getByRole("img", { name: /Server memory history/ })).toBeTruthy()
  })

  it("shows an error state with retry when the query fails", async () => {
    const performance = vi.fn().mockRejectedValue(new Error("boom"))
    renderPanel(stubClient(performance))

    expect(await screen.findByText("Could not load performance data")).toBeTruthy()
    expect(screen.getByText("boom")).toBeTruthy()
    expect(screen.getByRole("button", { name: "Retry" })).toBeTruthy()
  })

  it("shows an empty state when the server has no samples yet", () => {
    const empty: PerformanceReport = { ...report, server: { ...report.server, current: null }, sessions: [] }
    render(<QueryClientProvider client={new QueryClient()}><PerformancePanelView report={empty} isLoading={false} error={undefined} onRetry={() => {}} /></QueryClientProvider>)

    expect(screen.getByText("Waiting for the first sample")).toBeTruthy()
  })
})

describe("Sparkline", () => {
  it("renders an accessible chart when there is enough data", () => {
    const { container } = render(<Sparkline values={[1, 2, 3]} label="CPU" format={(value) => `${value}%`} />)
    const svg = container.querySelector("svg")
    expect(svg).toBeTruthy()
    expect(svg?.getAttribute("aria-label")).toBe("CPU. Latest: 3%")
  })

  it("explains that there is not enough history with fewer than two points", () => {
    render(<Sparkline values={[1]} label="CPU" />)
    expect(screen.getByText("Not enough history yet")).toBeTruthy()
  })
})


