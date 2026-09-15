import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { cleanup, render, screen } from "@testing-library/react"
import "@testing-library/jest-dom/vitest"
import { afterEach, describe, expect, it, vi } from "vitest"

import { type PerformanceReport, type PiServerClient } from "@/api/client"
import { SessionPerformancePanel } from "./session-performance-panel"

const report: PerformanceReport = {
  sampleIntervalSeconds: 15,
  historyLimit: 240,
  server: { id: "server", name: "pi-server", kind: "server", pid: 10, running: true, current: null, samples: [] },
  sessions: [
    {
      id: "session-1",
      name: "Memory audit",
      kind: "session",
      pid: 4242,
      running: true,
      current: { at: "2026-01-01T00:00:15Z", cpuPercent: 12.5, rssBytes: 64 * 1024 * 1024 },
      samples: [
        { at: "2026-01-01T00:00:00Z", cpuPercent: 5, rssBytes: 32 * 1024 * 1024 },
        { at: "2026-01-01T00:00:15Z", cpuPercent: 12.5, rssBytes: 64 * 1024 * 1024 },
      ],
    },
  ],
  requests: [],
}

function renderPanel(performance: () => Promise<PerformanceReport>, sessionId = "session-1") {
  const client = { cacheScope: "sidebar-test", performance } as unknown as PiServerClient
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={queryClient}>
      <SessionPerformancePanel sessionId={sessionId} client={client} />
    </QueryClientProvider>
  )
}

describe("SessionPerformancePanel", () => {
  afterEach(cleanup)

  it("shows current and historical process metrics", async () => {
    renderPanel(vi.fn().mockResolvedValue(report))

    expect(await screen.findByText("12.5%")).toBeTruthy()
    expect(screen.getByText("64 MiB")).toBeTruthy()
    expect(screen.getByText("Running")).toBeTruthy()
    expect(screen.getByText("4242")).toBeTruthy()
    expect(screen.getByRole("img", { name: /Session CPU history/ })).toBeTruthy()
    expect(screen.getByRole("img", { name: /Session memory history/ })).toBeTruthy()
  })

  it("explains when the selected session has no local process metrics", async () => {
    renderPanel(vi.fn().mockResolvedValue(report), "remote-session")

    expect(await screen.findByText("No process metrics")).toBeTruthy()
    expect(screen.getByText(/Remote, relay, and discovered sessions/)).toBeTruthy()
  })
})
