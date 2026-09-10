import { useEffect, useState } from "react"
import { ExternalLink, LoaderCircle, X } from "lucide-react"

import type { ApiSession } from "@/api/client"
import { useCancelRun, useSchedulerStatus } from "@/api/hooks"
import { Button } from "@/components/ui/button"
import { useAppStore } from "@/state/app-store"

/** Re-renders once per second so elapsed times stay current. */
function useNow(enabled: boolean) {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!enabled) return
    const timer = window.setInterval(() => setNow(Date.now()), 1000)
    return () => window.clearInterval(timer)
  }, [enabled])
  return now
}

function formatElapsed(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000))
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60
  if (minutes >= 60) {
    const hours = Math.floor(minutes / 60)
    return `${hours}h ${String(minutes % 60).padStart(2, "0")}m`
  }
  return `${String(minutes).padStart(2, "0")}:${String(seconds).padStart(2, "0")}`
}

export interface ActiveRunsListProps {
  sessions: ApiSession[]
  onOpenSession: (sessionId: string) => void
}

/**
 * Compact sidebar section for admission-scheduler runs. Session metadata
 * (title, worker) comes from the HTTP inventory; live state/detail come from
 * the status stream's runtime entries when available.
 */
export function ActiveRunsList({ sessions, onOpenSession }: ActiveRunsListProps) {
  const { data: scheduler } = useSchedulerStatus()
  const cancelRun = useCancelRun()
  const runtimeSessions = useAppStore((state) => state.runtimeSessions)
  const runs = scheduler?.runs ?? []
  const now = useNow(runs.length > 0)
  const titles = new Map(sessions.map((session) => [session.id, session.title || session.project || session.id]))

  if (runs.length === 0) return null

  return (
    <div className="px-3 py-1" data-testid="active-runs">
      <div className="flex items-center justify-between">
        <span className="text-[11px] font-medium uppercase tracking-wide text-muted-foreground">
          Active runs
        </span>
        <span className="text-[11px] text-muted-foreground">
          {runs.filter((run) => run.phase === "active").length} active · {runs.filter((run) => run.phase === "queued").length} queued
        </span>
      </div>
      <ul className="mt-1 space-y-1">
        {runs.map((run) => {
          const live = runtimeSessions[run.sessionId]
          const status = live?.state ?? (run.phase === "queued" ? "queued" : "working")
          const detail = live?.detail
          const queuedAtMs = Date.parse(run.queuedAt)
          const elapsed = Number.isFinite(queuedAtMs) ? now - queuedAtMs : 0
          // Only active runs on locally managed sessions can be aborted from
          // the hub; queued runs are always cancellable via admission.
          const canCancel = run.phase === "queued" || run.workerId === "local"
          const hint = run.phase === "queued"
            ? "Cancel queued run"
            : canCancel
              ? "Abort active run"
              : "Active runs on remote workers or relays cannot be cancelled from here"
          return (
            <li
              key={run.runId}
              className="rounded-md border border-border/60 bg-background/60 px-2 py-1.5 text-xs"
            >
              <div className="flex items-center gap-1.5">
                <Button
                  size="icon-xs"
                  variant="ghost"
                  className="shrink-0"
                  aria-label="Open session"
                  title="Open session"
                  onClick={() => onOpenSession(run.sessionId)}
                >
                  <ExternalLink />
                </Button>
                <button
                  type="button"
                  className="min-w-0 flex-1 truncate text-left font-medium hover:underline"
                  onClick={() => onOpenSession(run.sessionId)}
                  title={titles.get(run.sessionId) ?? run.sessionId}
                >
                  {titles.get(run.sessionId) ?? run.sessionId}
                </button>
                <span className="shrink-0 tabular-nums text-muted-foreground">{formatElapsed(elapsed)}</span>
                <Button
                  size="icon-xs"
                  variant="ghost"
                  className="shrink-0"
                  disabled={!canCancel || (cancelRun.isPending && cancelRun.variables === run.runId)}
                  aria-label={hint}
                  title={hint}
                  onClick={() => cancelRun.mutate(run.runId)}
                >
                  {cancelRun.isPending && cancelRun.variables === run.runId
                    ? <LoaderCircle className="animate-spin" />
                    : <X />}
                </Button>
              </div>
              <div className="mt-0.5 flex items-center gap-1.5 pl-6 text-[11px] text-muted-foreground">
                <span className="shrink-0">{run.workerId}</span>
                <span aria-hidden>·</span>
                <span className="min-w-0 flex-1 truncate">
                  {run.phase === "queued" && run.position
                    ? `Queue position ${run.position}`
                    : detail
                      ? detail
                      : status}
                </span>
              </div>
            </li>
          )
        })}
      </ul>
    </div>
  )
}
