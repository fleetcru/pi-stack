import { useEffect, useMemo, useRef, useState } from "react"
import { ChevronDown, ChevronRight, MessageSquare, Wifi } from "lucide-react"

import { useSessions } from "@/api/hooks"
import type { MachineSession } from "@/api/client"

// ---------------------------------------------------------------------------
// Shared helpers (duplicated from sidebar-tree to avoid circular deps)
// ---------------------------------------------------------------------------

function folderName(path?: string): string | undefined {
  if (!path) return undefined
  const parts = path.replace(/\\/g, "/").split("/").filter(Boolean)
  return parts.at(-1)
}

function relativeTime(dateStr?: string): string {
  if (!dateStr) return ""
  const date = new Date(dateStr)
  const now = Date.now()
  const diffMs = now - date.getTime()
  if (diffMs < 0) return "just now"
  const diffMin = Math.floor(diffMs / 60_000)
  if (diffMin < 1) return "just now"
  if (diffMin < 60) return `${diffMin}m ago`
  const diffHr = Math.floor(diffMin / 60)
  if (diffHr < 24) return `${diffHr}h ago`
  const diffDay = Math.floor(diffHr / 24)
  if (diffDay === 1) return "yesterday"
  if (diffDay < 7) return `${diffDay}d ago`
  return date.toLocaleDateString()
}

// ---------------------------------------------------------------------------
// MachineSessionList
// ---------------------------------------------------------------------------

export function MachineSessionList({ sessions, onOpen }: { sessions: MachineSession[]; onOpen: (id: string) => Promise<void> }) {
  const [openError, setOpenError] = useState<string | null>(null)
  const errorTimerRef = useRef<ReturnType<typeof setTimeout>>()
  useEffect(() => () => { if (errorTimerRef.current) clearTimeout(errorTimerRef.current) }, [])

  const handleOpen = async (id: string) => {
    setOpenError(null)
    if (errorTimerRef.current) clearTimeout(errorTimerRef.current)
    try {
      await onOpen(id)
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to open session"
      setOpenError(message)
      errorTimerRef.current = setTimeout(() => setOpenError(null), 5000)
    }
  }

  // Check if any machine sessions are already open as live sessions
  const { data: sessionResult } = useSessions()
  const liveSessionCwds = useMemo(() => {
    const cwds = new Set<string>()
    for (const s of sessionResult?.sessions ?? []) {
      if (s.cwd) cwds.add(s.cwd)
    }
    return cwds
  }, [sessionResult?.sessions])

  const [expanded, setExpanded] = useState(false)

  // Filter out sessions with empty or missing CWD (defensive — server also filters)
  const validSessions = sessions.filter((s) => s.cwd && s.cwd.trim().length > 0)
  const invalidCount = sessions.length - validSessions.length

  // Avoid showing machine sessions that are already visible as live sessions
  const visibleSessions = validSessions.filter((s) => !liveSessionCwds.has(s.cwd))
  const alreadyOpenSessions = validSessions.filter((s) => liveSessionCwds.has(s.cwd))

  if (validSessions.length === 0 && invalidCount === 0) return null

  return (
    <div className="mt-3 border-t border-border/60 pt-2">
      <button
        type="button"
        onClick={() => setExpanded((prev) => !prev)}
        className="flex w-full items-center gap-1 px-2 py-1 text-left text-[10px] font-medium tracking-[0.14em] text-muted-foreground uppercase hover:text-sidebar-foreground"
      >
        {expanded ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        Local Pi sessions ({validSessions.length})
      </button>
      {expanded && <>
      <p className="px-2 pb-1 text-[10px] text-muted-foreground/60">From ~/.pi/agent/sessions</p>
      {invalidCount > 0 && (
        <p className="px-2 py-1 text-[10px] text-amber-500/80">{invalidCount} session{invalidCount > 1 ? "s" : ""} skipped (missing working directory)</p>
      )}
      {openError && (
        <p className="mx-2 my-1 rounded-md bg-destructive/10 px-2 py-1 text-[10px] text-destructive">{openError}</p>
      )}
      {visibleSessions.slice(0, 30).map((session) => (
        <button key={session.id} type="button" onClick={() => void handleOpen(session.id)} className="flex h-7 w-full items-center gap-2 rounded-md px-3 text-left text-xs hover:bg-sidebar-accent">
          <MessageSquare className="size-3 shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate">{folderName(session.cwd) || session.cwd}</span>
          <span className="shrink-0 text-[10px] text-muted-foreground">{relativeTime(session.updatedAt)}</span>
        </button>
      ))}
      {visibleSessions.length > 30 && (
        <p className="px-3 py-1 text-[10px] text-muted-foreground/60">...and {visibleSessions.length - 30} more</p>
      )}
      {alreadyOpenSessions.length > 0 && (
        <>
          <p className="mt-1 px-2 pt-1 text-[10px] font-medium tracking-[0.14em] text-muted-foreground/50 uppercase">Already open</p>
          {alreadyOpenSessions.map((session) => (
            <button key={session.id} type="button" onClick={() => void handleOpen(session.id)} className="flex h-7 w-full items-center gap-2 rounded-md px-3 text-left text-xs text-muted-foreground/50 hover:bg-sidebar-accent hover:text-sidebar-foreground">
              <MessageSquare className="size-3 shrink-0" />
              <span className="min-w-0 flex-1 truncate">{folderName(session.cwd) || session.cwd}</span>
              <span className="shrink-0 text-[10px]">open</span>
            </button>
          ))}
        </>
      )}
      </>}
    </div>
  )
}

// ---------------------------------------------------------------------------
// GlobalSessionList
// ---------------------------------------------------------------------------

export function GlobalSessionList({
  sessions,
  onOpen,
}: {
  sessions: Array<{ id: string; workerId: string; session: { id: string; title?: string; project?: string; status?: string } }>
  onOpen: (id: string) => void
}) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="mt-3 border-t border-border/60 pt-2">
      <button
        type="button"
        onClick={() => setExpanded((prev) => !prev)}
        className="flex w-full items-center gap-1 px-2 py-1 text-left text-[10px] font-medium tracking-[0.14em] text-muted-foreground uppercase hover:text-sidebar-foreground"
      >
        {expanded ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
        Global sessions ({sessions.length})
      </button>
      {expanded && sessions.map((global) => (
        <button
          key={global.id}
          type="button"
          onClick={() => onOpen(global.id)}
          className="flex h-7 w-full items-center gap-2 rounded-md px-3 text-left text-xs hover:bg-sidebar-accent"
        >
          <Wifi className="size-3 shrink-0 text-muted-foreground" />
          <span className="min-w-0 flex-1 truncate">{global.session.title || global.session.project || global.session.id}</span>
          <span className="shrink-0 text-[10px] text-muted-foreground">{global.workerId}</span>
        </button>
      ))}
    </div>
  )
}
