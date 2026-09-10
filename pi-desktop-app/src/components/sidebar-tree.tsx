import { useMemo, useState, type ReactNode } from "react"
import {
  ChevronDown,
  ChevronRight,
  FolderGit2,
  MessageSquare,
  Star,
  Wifi,
} from "lucide-react"

import { cn } from "@/lib/utils"
import { useAppStore } from "@/state/app-store"

type SessionStatus = "working" | "waiting" | "active" | "idle" | "reconnecting" | "error"

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

function shortSessionID(id: string) {
  return id.length > 18 ? `${id.slice(0, 8)}…${id.slice(-4)}` : id
}

function folderName(path?: string): string | undefined {
  if (!path) return undefined
  const parts = path.replace(/\\/g, "/").split("/").filter(Boolean)
  return parts.at(-1)
}

function sessionStatus(status?: string, runtimeState?: string): SessionStatus {
  // Use runtimeState for more granular status when available
  const state = runtimeState ?? status
  switch (state) {
    case "working": return "working"
    case "waiting_for_input":
    case "queued": return "waiting"
    case "starting":
    case "reconnecting": return "reconnecting"
    case "error":
    case "failed":
    case "stopped": return "error"
    case "running": return "active"
    case "idle":
    case "created":
    case "cancelled": return "idle"
    default:
      if (state) console.warn(`[sessionStatus] unknown state: ${state}`)
      return "idle"
  }
}

function matchesFilter(session: { id: string; title?: string; project?: string; cwd?: string; workerId?: string }, filter: string): boolean {
  if (!filter) return true
  const lower = filter.toLowerCase()
  return (
    (session.title?.toLowerCase().includes(lower) ?? false) ||
    (session.project?.toLowerCase().includes(lower) ?? false) ||
    (session.cwd?.toLowerCase().includes(lower) ?? false) ||
    session.id.toLowerCase().includes(lower) ||
    (session.workerId?.toLowerCase().includes(lower) ?? false)
  )
}

function groupByProject<T extends { cwd?: string; project?: string }>(
  sessions: T[]
) {
  const groups = new Map<string, T[]>()
  for (const session of sessions) {
    const project = session.project || folderName(session.cwd) || "Unassigned"
    const group = groups.get(project) ?? []
    group.push(session)
    groups.set(project, group)
  }
  return groups
}

// ---------------------------------------------------------------------------
// StatusDot
// ---------------------------------------------------------------------------

export function StatusDot({ status }: { status: SessionStatus }) {
  return (
    <span
      aria-label={status}
      title={status}
      className={cn(
        "size-[7px] shrink-0 rounded-full ring-1 ring-inset ring-background",
        status === "working" && "bg-emerald-500 animate-pulse",
        status === "waiting" && "bg-amber-400 animate-pulse",
        status === "active" && "bg-emerald-500",
        status === "idle" && "bg-muted-foreground/40",
        status === "reconnecting" && "bg-blue-400 animate-pulse",
        status === "error" && "bg-destructive"
      )}
    />
  )
}

// ---------------------------------------------------------------------------
// EmptyTreeState
// ---------------------------------------------------------------------------

export function EmptyTreeState({ compact = false }: { compact?: boolean }) {
  return (
    <p
      className={cn(
        "px-3 py-2 text-xs text-muted-foreground",
        compact && "py-1.5 pl-10"
      )}
    >
      No sessions
    </p>
  )
}

// ---------------------------------------------------------------------------
// TreeNode
// ---------------------------------------------------------------------------

type TreeNodeProps = {
  children: ReactNode
  depth?: number
  expanded?: boolean
  icon: ReactNode
  label: string
  onClick?: () => void
  selected?: boolean
  status?: SessionStatus
  badge?: string
}

export function TreeNode({
  children,
  depth = 0,
  expanded = false,
  icon,
  label,
  onClick,
  selected = false,
  status,
  badge,
}: TreeNodeProps) {
  const hasChildren = Boolean(children)
  return (
    <div>
      <button
        type="button"
        onClick={onClick}
        aria-expanded={hasChildren ? expanded : undefined}
        title={label}
        className={cn(
          "group flex h-8 w-full items-center gap-2 rounded-lg pr-2 text-left text-xs font-medium text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
          selected && "bg-sidebar-accent text-sidebar-accent-foreground",
          !onClick && "cursor-default hover:bg-transparent"
        )}
        style={{ paddingLeft: `${8 + depth * 16}px` }}
      >
        {hasChildren ? (
          expanded ? (
            <ChevronDown className="size-3 shrink-0" />
          ) : (
            <ChevronRight className="size-3 shrink-0" />
          )
        ) : (
          <span className="w-3 shrink-0" />
        )}
        <span className="text-muted-foreground">{icon}</span>
        <span className="min-w-0 flex-1 truncate">{label}</span>
        {badge && <span className="shrink-0 rounded-md bg-muted px-1.5 py-0.5 text-[10px] font-normal tabular-nums text-muted-foreground">{badge}</span>}
        {status && <StatusDot status={status} />}
      </button>
      {hasChildren && expanded && <div>{children}</div>}
    </div>
  )
}

// ---------------------------------------------------------------------------
// SessionLeaf
// ---------------------------------------------------------------------------

function SessionLeaf({
  session,
  source,
  selected,
  pinned,
  onSelect,
}: {
  session: { id: string; status?: string; title?: string; updatedAt?: string; transport?: string; state?: Record<string, unknown> }
  source?: string
  selected: boolean
  pinned: boolean
  onSelect: (sessionId?: string) => void
}) {
  const togglePin = useAppStore((state) => state.togglePinSession)
  // Select only this session's entry so unrelated status deltas do not
  // re-render every row in the tree.
  const liveStatus = useAppStore((state) => state.runtimeSessions[session.id])
  // Runtime-only inventory omits state.external for relays, so also check transport.
  const isRelay = source === "external" || session.state?.external === true || session.transport === "relay"
  const label = session.title || shortSessionID(session.id)

  return (
    <div className="group/leaf relative">
      <button
        type="button"
        onClick={() => onSelect(session.id)}
        title={label}
        className={cn(
          "flex h-8 w-full items-center gap-2 rounded-lg pr-2 pl-11 text-left text-xs text-sidebar-foreground transition-colors hover:bg-sidebar-accent hover:text-sidebar-accent-foreground",
          selected && "bg-sidebar-accent font-medium text-sidebar-accent-foreground"
        )}
      >
        <span className="w-3 shrink-0" />
        <MessageSquare className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate">
          {isRelay && <span className="mr-0.5 text-muted-foreground opacity-50">↗</span>}
          {pinned && <Star className="mr-0.5 inline size-3 shrink-0 fill-amber-400 text-amber-400" />}
          {label}
        </span>
        <span className="group-hover/leaf:opacity-0">
          <StatusDot status={sessionStatus(session.status, liveStatus?.state ?? (session.state?.runtimeStatus as { state?: string } | undefined)?.state)} />
        </span>
      </button>
      {/* Status detail label */}
      {(() => {
        const rt = session.state?.runtimeStatus as { state?: string; detail?: string } | undefined
        const state = liveStatus?.state ?? rt?.state
        if (!state || state === "idle" || state === "created") return null
        const detail = liveStatus?.detail || rt?.detail || state
        return (
          <div className="flex h-5 w-full items-center pl-14 pr-2">
            <span className="truncate text-[10px] text-muted-foreground/70">{detail}</span>
          </div>
        )
      })()}
      {/* Pin/unpin on hover */}
      <button
        type="button"
        onClick={(event) => { event.stopPropagation(); togglePin(session.id) }}
        className="absolute top-1/2 right-1 -translate-y-1/2 rounded-md p-1 opacity-0 transition-opacity hover:bg-muted group-hover/leaf:opacity-100 focus-visible:opacity-100"
        aria-label={pinned ? "Unpin session" : "Pin session"}
        title={pinned ? "Unpin" : "Pin"}
      >
        <Star className={cn("size-3", pinned ? "fill-amber-400 text-amber-400" : "text-muted-foreground")} />
      </button>
    </div>
  )
}

// ---------------------------------------------------------------------------
// ProjectBranch
// ---------------------------------------------------------------------------

const STALE_THRESHOLD_MS = 7 * 24 * 60 * 60 * 1000 // 7 days

function ProjectBranch({
  project,
  sessions,
  selectedSessionId,
  onSelectSession,
  pinnedSessionIds,
  source,
  forceExpanded = false,
}: {
  project: string
  sessions: Array<{ id: string; status?: string; title?: string; updatedAt?: string; state?: Record<string, unknown> }>
  selectedSessionId?: string
  onSelectSession: (sessionId?: string) => void
  pinnedSessionIds: Record<string, true>
  source?: string
  forceExpanded?: boolean
}) {
  const nodeId = `project:${source ?? "unknown"}:${project}`
  const expanded = useAppStore((state) =>
    Boolean(state.expandedTreeNodes[nodeId])
  )
  const toggle = useAppStore((state) => state.toggleTreeNode)

  const [olderExpanded, setOlderExpanded] = useState(false)
  // eslint-disable-next-line react-hooks/purity -- stale threshold is intentionally computed once per mount
  const nowRef = useMemo(() => Date.now(), [])

  // Split into pinned, recent, and older
  const { recent, older } = useMemo(() => {
    const now = nowRef
    const sorted = [...sessions].sort((a, b) => {
      const aPin = pinnedSessionIds[a.id] ? 1 : 0
      const bPin = pinnedSessionIds[b.id] ? 1 : 0
      if (aPin !== bPin) return bPin - aPin
      const aTime = a.updatedAt ? new Date(a.updatedAt).getTime() : 0
      const bTime = b.updatedAt ? new Date(b.updatedAt).getTime() : 0
      if (aTime !== bTime) return bTime - aTime
      return a.id.localeCompare(b.id)
    })
    const staleThreshold = now - STALE_THRESHOLD_MS
    const recent: typeof sorted = []
    const older: typeof sorted = []
    for (const session of sorted) {
      const lastActive = session.updatedAt ? new Date(session.updatedAt).getTime() : 0
      if (lastActive > 0 && lastActive < staleThreshold) {
        older.push(session)
      } else {
        recent.push(session)
      }
    }
    return { recent, older }
  }, [sessions, pinnedSessionIds, nowRef])

  return (
    <TreeNode
      depth={1}
      expanded={forceExpanded || expanded}
      icon={<FolderGit2 className="size-3.5" />}
      label={`${project} (${sessions.length})`}
      onClick={() => toggle(nodeId)}
    >
      {recent.map((session) => (
        <SessionLeaf
          key={session.id}
          session={session}
          source={source}
          selected={selectedSessionId === session.id}
          pinned={Boolean(pinnedSessionIds[session.id])}
          onSelect={onSelectSession}
        />
      ))}
      {older.length > 0 && (
        <>
          <button
            type="button"
            onClick={() => setOlderExpanded((open) => !open)}
            className="flex h-7 w-full items-center gap-1.5 rounded-lg px-3 text-[10px] text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-foreground"
            style={{ paddingLeft: `${8 + 2 * 16}px` }}
          >
            {olderExpanded ? <ChevronDown className="size-3" /> : <ChevronRight className="size-3" />}
            <span className="opacity-60">Older ({older.length})</span>
          </button>
          {olderExpanded && older.map((session) => (
            <SessionLeaf
              key={session.id}
              session={session}
              source={source}
              selected={selectedSessionId === session.id}
              pinned={Boolean(pinnedSessionIds[session.id])}
              onSelect={onSelectSession}
            />
          ))}
        </>
      )}
    </TreeNode>
  )
}

// ---------------------------------------------------------------------------
// WorkerBranch
// ---------------------------------------------------------------------------

function WorkerBranch({
  workerId,
  sessions,
  selectedSessionId,
  onSelectSession,
  pinnedSessionIds,
  filterActive = false,
}: {
  workerId: string
  sessions: Array<{
    id: string
    cwd?: string
    project?: string
    status?: string
    title?: string
    updatedAt?: string
    state?: Record<string, unknown>
  }>
  selectedSessionId?: string
  onSelectSession: (sessionId?: string) => void
  pinnedSessionIds: Record<string, true>
  filterActive?: boolean
}) {
  const nodeId = `worker:${workerId}`
  const expanded = useAppStore((state) =>
    Boolean(state.expandedTreeNodes[nodeId])
  )
  const toggle = useAppStore((state) => state.toggleTreeNode)
  const projects = [...groupByProject(sessions).entries()].sort(([a], [b]) => a.localeCompare(b))

  return (
    <TreeNode
      expanded={expanded || filterActive}
      icon={<Wifi className="size-3.5" />}
      label={workerId === "local" ? "Local" : workerId === "external" ? "Live TUI bridge" : workerId}
      onClick={() => toggle(nodeId)}
    >
      {projects.map(([project, projectSessions]) => (
        <ProjectBranch
          key={project}
          project={project}
          sessions={projectSessions}
          selectedSessionId={selectedSessionId}
          onSelectSession={onSelectSession}
          pinnedSessionIds={pinnedSessionIds}
          source={workerId}
          forceExpanded={filterActive}
        />
      ))}
      {sessions.length === 0 && <EmptyTreeState compact />}
    </TreeNode>
  )
}

// ---------------------------------------------------------------------------
// SessionTree (public)
// ---------------------------------------------------------------------------

export function SessionTree({
  sessions,
  workerIds,
  selectedSessionId,
  onSelectSession,
  filterText,
}: {
  sessions: Array<{
    id: string
    workerId: string
    cwd?: string
    project?: string
    status?: string
    title?: string
    updatedAt?: string
    state?: Record<string, unknown>
  }>
  workerIds: string[]
  selectedSessionId?: string
  onSelectSession: (sessionId?: string) => void
  filterText?: string
}) {
  const pinnedSessionIds = useAppStore((state) => state.pinnedSessionIds)
  const filteredSessions = filterText ? sessions.filter((s) => matchesFilter(s, filterText)) : sessions

  const workers = new Map<string, typeof filteredSessions>()
  for (const workerId of workerIds) workers.set(workerId, [])
  for (const session of filteredSessions) {
    const workerSessions = workers.get(session.workerId) ?? []
    workerSessions.push(session)
    workers.set(session.workerId, workerSessions)
  }

  if (workers.size === 0) {
    return filterText ? <p className="px-3 py-4 text-center text-xs text-muted-foreground">No matching sessions</p> : <EmptyTreeState />
  }

  const sortedWorkers = [...workers.entries()].sort(([a], [b]) => {
    if (a === "local") return -1
    if (b === "local") return 1
    return a.localeCompare(b)
  })

  return sortedWorkers.map(([workerId, workerSessions]) => (
    <WorkerBranch
      key={workerId}
      workerId={workerId}
      sessions={workerSessions}
      selectedSessionId={selectedSessionId}
      onSelectSession={onSelectSession}
      pinnedSessionIds={pinnedSessionIds}
      filterActive={Boolean(filterText)}
    />
  ))
}
