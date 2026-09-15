import { useQuery } from "@tanstack/react-query"
import { Activity, AlertTriangle, Cpu, Gauge, MemoryStick } from "lucide-react"

import {
  PiServerClient,
  type PerformanceReport,
  type PerformanceSample,
  type PerformanceSeries,
} from "@/api/client"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Skeleton } from "@/components/ui/skeleton"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

const DEFAULT_POLL_INTERVAL_MS = 15_000

function formatBytes(bytes?: number | null) {
  if (bytes === undefined || bytes === null || Number.isNaN(bytes)) return "—"
  if (bytes < 1024) return `${bytes} B`
  const units = ["KiB", "MiB", "GiB", "TiB"]
  let value = bytes / 1024
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(value >= 10 || unit === 0 ? 0 : 1)} ${units[unit]}`
}

function formatPercent(percent?: number | null) {
  if (percent === undefined || percent === null || Number.isNaN(percent)) return "—"
  return `${percent.toFixed(1)}%`
}

function formatCount(value?: number | null) {
  if (value === undefined || value === null || Number.isNaN(value)) return "—"
  return value.toLocaleString()
}

function formatRate(value?: number | null) {
  if (value === undefined || value === null || Number.isNaN(value)) return "—"
  return `${value.toFixed(value < 10 ? 1 : 0)}/s`
}

function formatMs(value?: number | null) {
  if (value === undefined || value === null || Number.isNaN(value)) return "—"
  if (value >= 1000) return `${(value / 1000).toFixed(2)} s`
  return `${value.toFixed(value < 10 ? 1 : 0)} ms`
}

function sampleValue(sample: PerformanceSample | null | undefined, key: keyof PerformanceSample): number | null {
  if (!sample) return null
  const value = sample[key]
  return typeof value === "number" && Number.isFinite(value) ? value : null
}

/** Lightweight inline SVG sparkline. Renders an area under a polyline; no chart library required. */
export function Sparkline({
  values,
  label,
  format = (value) => String(value),
  className = "h-16",
  zeroBased = false,
}: {
  values: Array<number | null>
  label: string
  format?: (value: number) => string
  className?: string
  zeroBased?: boolean
}) {
  const points = values.filter((value): value is number => value !== null)
  if (points.length < 2) {
    return <p className="flex h-16 items-center justify-center text-xs text-muted-foreground">Not enough history yet</p>
  }
  const width = 100
  const height = 24
  const max = Math.max(...points)
  const min = zeroBased ? Math.min(...points, 0) : Math.min(...points)
  const span = max - min
  const coordinates = points.map((value, index) => {
    const x = (index / (points.length - 1)) * width
    const y = span === 0 ? height / 2 : height - ((value - min) / span) * (height - 2) - 1
    return `${x.toFixed(2)},${y.toFixed(2)}`
  })
  const last = points[points.length - 1]
  return (
    <svg
      role="img"
      aria-label={`${label}. Latest: ${format(last)}`}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className={`w-full ${className}`}
    >
      <polygon points={`0,${height} ${coordinates.join(" ")} ${width},${height}`} className="fill-primary/15" />
      <polyline points={coordinates.join(" ")} fill="none" stroke="currentColor" strokeWidth={1} className="stroke-primary" vectorEffect="non-scaling-stroke" />
    </svg>
  )
}

function seriesMetric(series: PerformanceSeries, key: keyof PerformanceSample) {
  return series.samples.map((sample) => sampleValue(sample, key))
}

function StatCard({ label, value, detail, icon: Icon }: { label: string; value: string; detail: string; icon: typeof Cpu }) {
  return (
    <Card>
      <CardHeader className="pb-2">
        <CardDescription className="flex items-center gap-2">
          <Icon className="size-3.5" aria-hidden />
          {label}
        </CardDescription>
        <CardTitle className="text-2xl tabular-nums">{value}</CardTitle>
      </CardHeader>
      <CardContent>
        <p className="text-xs text-muted-foreground">{detail}</p>
      </CardContent>
    </Card>
  )
}

export function PerformancePanel({ client }: { client: PiServerClient }) {
  const query = useQuery({
    queryKey: ["server-performance", client.cacheScope],
    queryFn: () => client.performance(),
    refetchInterval: (currentQuery) =>
      Math.max(1_000, (currentQuery.state.data?.sampleIntervalSeconds ?? DEFAULT_POLL_INTERVAL_MS / 1_000) * 1_000),
    // Pause background polling when the tab or window is hidden.
    refetchIntervalInBackground: false,
  })
  return <PerformancePanelView report={query.data} isLoading={query.isLoading} error={query.error} onRetry={() => void query.refetch()} />
}

export function PerformancePanelView({
  report,
  isLoading,
  error,
  onRetry,
}: {
  report?: PerformanceReport
  isLoading: boolean
  error: unknown
  onRetry: () => void
}) {
  if (isLoading) {
    return (
      <div className="flex flex-col gap-4 pt-4" aria-busy="true" aria-label="Loading performance data">
        <div className="grid grid-cols-4 gap-3 max-lg:grid-cols-2 max-sm:grid-cols-1">
          {Array.from({ length: 4 }, (_, index) => (
            <Skeleton key={index} className="h-24 rounded-xl" />
          ))}
        </div>
        <Skeleton className="h-48 rounded-xl" />
        <Skeleton className="h-48 rounded-xl" />
      </div>
    )
  }
  if (error) {
    return (
      <div className="pt-4">
        <Alert variant="destructive">
          <AlertTriangle />
          <AlertTitle>Could not load performance data</AlertTitle>
          <AlertDescription>
            {error instanceof Error ? error.message : "The request failed."}{" "}
            <button type="button" className="underline underline-offset-2" onClick={onRetry}>
              Retry
            </button>
          </AlertDescription>
        </Alert>
      </div>
    )
  }
  if (!report) return null

  const server = report.server
  const current = server.current
  const sessionsWithSamples = report.sessions.filter((session) => session.samples.length > 0 || session.current)
  const sortedRequests = [...report.requests].sort((a, b) => b.count - a.count)

  return (
    <div className="flex flex-col gap-4 pt-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h3 className="text-sm font-semibold">Performance</h3>
          <p className="text-xs text-muted-foreground">
            Sampled every {report.sampleIntervalSeconds}s, keeping {report.historyLimit} points per series. Refreshes automatically while visible.
          </p>
        </div>
        <Badge variant="secondary">{server.running === false ? "Idle" : "Sampling"}</Badge>
      </div>

      {current ? (
        <div className="grid grid-cols-4 gap-3 max-lg:grid-cols-2 max-sm:grid-cols-1">
          <StatCard label="Server CPU" value={formatPercent(current.cpuPercent)} detail={server.pid ? `PID ${server.pid}` : server.name} icon={Cpu} />
          <StatCard label="Resident memory" value={formatBytes(current.rssBytes)} detail={current.heapAllocBytes !== undefined ? `Go heap ${formatBytes(current.heapAllocBytes)}` : "Process RSS"} icon={MemoryStick} />
          <StatCard label="Request rate" value={formatRate(current.requestRate)} detail={`${formatMs(current.averageLatencyMs)} average latency · ${formatRate(current.errorRate)} errors`} icon={Activity} />
          <StatCard label="Scheduler" value={`${formatCount(current.activeRuns)} active`} detail={`${formatCount(current.activeSessions)} sessions · ${formatCount(current.queuedRuns)} queued`} icon={Gauge} />
        </div>
      ) : (
        <Alert>
          <Activity />
          <AlertTitle>Waiting for the first sample</AlertTitle>
          <AlertDescription>This server has not reported a performance sample yet. Charts appear once data arrives.</AlertDescription>
        </Alert>
      )}

      <div className="grid grid-cols-2 gap-3 max-sm:grid-cols-1">
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Server CPU history</CardTitle>
            <CardDescription>Share of total host CPU, last {server.samples.length} samples</CardDescription>
          </CardHeader>
          <CardContent>
            <Sparkline values={seriesMetric(server, "cpuPercent")} label="Server CPU history" format={formatPercent} zeroBased />
          </CardContent>
        </Card>
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-sm">Server memory history</CardTitle>
            <CardDescription>Resident memory, last {server.samples.length} samples</CardDescription>
          </CardHeader>
          <CardContent>
            <Sparkline values={seriesMetric(server, "rssBytes")} label="Server memory history" format={formatBytes} />
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Sessions</CardTitle>
          <CardDescription>CPU and memory for managed local Pi processes.</CardDescription>
        </CardHeader>
        <CardContent>
          {sessionsWithSamples.length === 0 ? (
            <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              No session performance samples yet. Sessions appear here once they report.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table aria-label="Session performance">
                <TableHeader>
                  <TableRow>
                    <TableHead scope="col">Session</TableHead>
                    <TableHead scope="col">Status</TableHead>
                    <TableHead scope="col" className="text-right">CPU</TableHead>
                    <TableHead scope="col" className="text-right">Memory</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sessionsWithSamples.map((session) => (
                    <TableRow key={session.id}>
                      <TableCell className="max-w-64 truncate font-medium" title={session.name}>{session.name}</TableCell>
                      <TableCell>
                        <Badge variant={session.running ? "secondary" : "outline"}>{session.running ? "Running" : "Stopped"}</Badge>
                      </TableCell>
                      <TableCell className="text-right tabular-nums">{session.running ? formatPercent(session.current?.cpuPercent) : "—"}</TableCell>
                      <TableCell className="text-right tabular-nums">{session.running ? formatBytes(session.current?.rssBytes) : "—"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-sm">Request routes</CardTitle>
          <CardDescription>HTTP handler traffic since this server process started.</CardDescription>
        </CardHeader>
        <CardContent>
          {sortedRequests.length === 0 ? (
            <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">
              No requests have been recorded on this server yet.
            </p>
          ) : (
            <div className="overflow-x-auto">
              <Table aria-label="Route request performance">
                <TableHeader>
                  <TableRow>
                    <TableHead scope="col">Route</TableHead>
                    <TableHead scope="col" className="text-right">Requests</TableHead>
                    <TableHead scope="col" className="text-right">Errors</TableHead>
                    <TableHead scope="col" className="text-right">Average</TableHead>
                    <TableHead scope="col" className="text-right">Slowest</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {sortedRequests.map((route) => (
                    <TableRow key={route.route}>
                      <TableCell className="max-w-80 truncate font-mono text-xs" title={route.route}>{route.route}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatCount(route.count)}</TableCell>
                      <TableCell className={`text-right tabular-nums ${route.errors > 0 ? "text-destructive" : ""}`}>{formatCount(route.errors)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatMs(route.averageMs)}</TableCell>
                      <TableCell className="text-right tabular-nums">{formatMs(route.maxMs)}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
