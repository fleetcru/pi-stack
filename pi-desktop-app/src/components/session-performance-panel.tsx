import { useQuery } from "@tanstack/react-query"
import { Activity, Cpu, MemoryStick, RefreshCw } from "lucide-react"

import { type PerformanceSample, type PerformanceSeries, type PiServerClient } from "@/api/client"
import { usePiServerClient } from "@/api/hooks"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"

const DEFAULT_POLL_INTERVAL_MS = 15_000

function formatBytes(bytes?: number | null) {
  if (bytes === undefined || bytes === null || Number.isNaN(bytes)) return "—"
  const units = ["B", "KiB", "MiB", "GiB", "TiB"]
  let value = bytes
  let unit = 0
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024
    unit += 1
  }
  return `${value.toFixed(unit === 0 || value >= 10 ? 0 : 1)} ${units[unit]}`
}

function formatPercent(percent?: number | null) {
  if (percent === undefined || percent === null || Number.isNaN(percent)) return "—"
  return `${percent.toFixed(1)}%`
}

function metricValues(series: PerformanceSeries, key: "cpuPercent" | "rssBytes") {
  return series.samples.map((sample) => {
    const value = sample[key]
    return typeof value === "number" && Number.isFinite(value) ? value : null
  })
}

function peakValue(samples: PerformanceSample[], key: "cpuPercent" | "rssBytes") {
  const values = samples
    .map((sample) => sample[key])
    .filter((value): value is number => typeof value === "number" && Number.isFinite(value))
  return values.length > 0 ? Math.max(...values) : null
}

function MetricSparkline({
  values,
  label,
  format,
  zeroBased = false,
}: {
  values: Array<number | null>
  label: string
  format: (value: number) => string
  zeroBased?: boolean
}) {
  const points = values.filter((value): value is number => value !== null)
  if (points.length < 2) {
    return <p className="flex h-20 items-center justify-center text-xs text-muted-foreground">Waiting for more samples</p>
  }
  const width = 100
  const height = 28
  const maximum = Math.max(...points)
  const minimum = zeroBased ? Math.min(...points, 0) : Math.min(...points)
  const span = maximum - minimum
  const coordinates = points.map((value, index) => {
    const x = (index / (points.length - 1)) * width
    const y = span === 0 ? height / 2 : height - ((value - minimum) / span) * (height - 2) - 1
    return `${x.toFixed(2)},${y.toFixed(2)}`
  })
  const latest = points[points.length - 1]
  return (
    <svg
      role="img"
      aria-label={`${label}. Latest: ${format(latest)}`}
      viewBox={`0 0 ${width} ${height}`}
      preserveAspectRatio="none"
      className="h-20 w-full"
    >
      <polygon points={`0,${height} ${coordinates.join(" ")} ${width},${height}`} className="fill-primary/15" />
      <polyline points={coordinates.join(" ")} fill="none" strokeWidth={1} vectorEffect="non-scaling-stroke" className="stroke-primary" />
    </svg>
  )
}

function MetricCard({ label, value, detail, icon: Icon }: { label: string; value: string; detail: string; icon: typeof Cpu }) {
  return (
    <Card>
      <CardHeader className="gap-1 pb-2">
        <CardDescription className="flex items-center gap-1.5 text-[11px]"><Icon className="size-3.5" aria-hidden />{label}</CardDescription>
        <CardTitle className="text-xl tabular-nums">{value}</CardTitle>
      </CardHeader>
      <CardContent><p className="text-[11px] text-muted-foreground">{detail}</p></CardContent>
    </Card>
  )
}

export function SessionPerformancePanel({ sessionId, client: clientOverride }: { sessionId: string; client?: PiServerClient }) {
  const configuredClient = usePiServerClient()
  const client = clientOverride ?? configuredClient
  const query = useQuery({
    queryKey: ["server-performance", client.cacheScope],
    queryFn: () => client.performance(),
    refetchInterval: (currentQuery) =>
      Math.max(1_000, (currentQuery.state.data?.sampleIntervalSeconds ?? DEFAULT_POLL_INTERVAL_MS / 1_000) * 1_000),
    refetchIntervalInBackground: false,
  })

  if (query.isLoading) {
    return <div className="space-y-3 p-4" aria-busy="true" aria-label="Loading session performance"><div className="grid grid-cols-2 gap-2"><Skeleton className="h-24 rounded-xl" /><Skeleton className="h-24 rounded-xl" /></div><Skeleton className="h-32 rounded-xl" /><Skeleton className="h-32 rounded-xl" /></div>
  }

  if (query.error) {
    return (
      <div className="p-4">
        <Alert variant="destructive">
          <Activity />
          <AlertTitle>Metrics unavailable</AlertTitle>
          <AlertDescription className="space-y-2">
            <p>{query.error instanceof Error ? query.error.message : "The performance endpoint could not be reached."}</p>
            <Button size="sm" variant="outline" onClick={() => void query.refetch()}><RefreshCw data-icon="inline-start" />Retry</Button>
          </AlertDescription>
        </Alert>
      </div>
    )
  }

  const report = query.data
  const series = report?.sessions.find((item) => item.id === sessionId)
  if (!report || !series) {
    return (
      <div className="p-4">
        <Alert>
          <Activity />
          <AlertTitle>No process metrics</AlertTitle>
          <AlertDescription>Metrics appear after a managed local Pi process starts. Remote, relay, and discovered sessions do not report process CPU or memory here.</AlertDescription>
        </Alert>
      </div>
    )
  }

  const current = series.current
  const peakCPU = peakValue(series.samples, "cpuPercent")
  const peakMemory = peakValue(series.samples, "rssBytes")

  return (
    <ScrollArea className="h-full">
      <div className="space-y-3 p-4">
        <div className="flex items-center justify-between gap-2">
          <div>
            <p className="text-xs font-medium">Process performance</p>
            <p className="text-[11px] text-muted-foreground">Every {report.sampleIntervalSeconds}s · {series.samples.length} samples</p>
          </div>
          <Badge variant={series.running ? "secondary" : "outline"}>{series.running ? "Running" : "Stopped"}</Badge>
        </div>

        <div className="grid grid-cols-2 gap-2">
          <MetricCard label="CPU" value={series.running ? formatPercent(current?.cpuPercent) : "—"} detail={`Peak ${formatPercent(peakCPU)}`} icon={Cpu} />
          <MetricCard label="Memory" value={series.running ? formatBytes(current?.rssBytes) : "—"} detail={`Peak ${formatBytes(peakMemory)}`} icon={MemoryStick} />
        </div>

        <Card>
          <CardHeader className="pb-1"><CardTitle className="text-xs">CPU history</CardTitle><CardDescription className="text-[11px]">Share of total host CPU</CardDescription></CardHeader>
          <CardContent><MetricSparkline values={metricValues(series, "cpuPercent")} label="Session CPU history" format={formatPercent} zeroBased /></CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-1"><CardTitle className="text-xs">Memory history</CardTitle><CardDescription className="text-[11px]">Resident process memory</CardDescription></CardHeader>
          <CardContent><MetricSparkline values={metricValues(series, "rssBytes")} label="Session memory history" format={formatBytes} /></CardContent>
        </Card>

        <div className="flex items-center justify-between rounded-lg border border-border px-3 py-2 text-xs">
          <span className="text-muted-foreground">Process ID</span>
          <span className="font-mono tabular-nums">{series.pid || "—"}</span>
        </div>
      </div>
    </ScrollArea>
  )
}
