import { useEffect, useMemo, useState, type ReactNode } from "react"
import { useQueryClient } from "@tanstack/react-query"
import { useNavigate } from "react-router"
import { usePanelRef } from "react-resizable-panels"
import {
  ChevronDown,
  ChevronRight,
  FolderGit2,
  LoaderCircle,
  MessageSquare,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
  Plus,
  Menu,
  PanelRight,
  Search,
  Server,
  Star,
  Sun,
  Wifi,
} from "lucide-react"

import { useGlobalSessions, useMachineSessions, usePiServerClient, useServerHealth, useSessions, useWorkers, piQueryKeys } from "@/api/hooks"
import type { ApiSession, ApiWorker, GlobalSession, MachineSession } from "@/api/client"
import type { PiServerClient } from "@/api/client"
import { CreateSessionDialog } from "@/components/create-session-dialog"
import { ServerConnectionsDialog } from "@/components/server-connections-dialog"
import { SessionInspector } from "@/components/session-inspector"
import SessionWorkspace from "@/components/session-workspace"
import { Button } from "@/components/ui/button"
import { useTheme } from "@/components/theme-provider"
import {
  ResizableHandle,
  ResizablePanel,
  ResizablePanelGroup,
} from "@/components/ui/resizable"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"
import { cn } from "@/lib/utils"
import { useAppStore } from "@/state/app-store"

type SessionStatus = "working" | "waiting" | "active" | "idle" | "reconnecting" | "error"

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

export function WorkspaceShell() {
  const [inspectorOpen, setInspectorOpen] = useState(true)
  const [createSessionOpen, setCreateSessionOpen] = useState(false)
  const [serverConnectionsOpen, setServerConnectionsOpen] = useState(false)
  const [leftSidebarCollapsed, setLeftSidebarCollapsed] = useState(false)
  const leftPanelRef = usePanelRef()
  const connection = useAppStore((state) => state.connection)
  const selectedSessionId = useAppStore((state) => state.selectedSessionId)
  const selectSession = useAppStore((state) => state.selectSession)
  const navigate = useNavigate()
  const openSession = (sessionId?: string) => {
    selectSession(sessionId)
    navigate(sessionId ? `/sessions/${encodeURIComponent(sessionId)}` : "/")
  }
  const { data: health, error: healthError } = useServerHealth()
  const { data: sessionResult, isLoading: sessionsLoading } = useSessions()
  const { data: workers = [] } = useWorkers()
  const { data: globalResult } = useGlobalSessions()
  const { data: machineResult } = useMachineSessions()
  const client = usePiServerClient()

  const sessions = useMemo(() => sessionResult?.sessions ?? [], [sessionResult?.sessions])
  // Local sessions already appear in the main tree; repeating them under
  // Global sessions makes it unclear which row is the live bridged TUI.
  const globalSessions = (globalResult?.sessions ?? []).filter((session) => session.workerId !== "local")
  const selectedSession = sessions.find(
    (session) => session.id === selectedSessionId
  )

  // Auto-deselect a session that no longer exists (e.g., stale localStorage
  // reference from a previous server instance).
  useEffect(() => {
    if (selectedSessionId && !sessionsLoading && sessions.length > 0 && !selectedSession) {
      selectSession(undefined)
      navigate("/")
    }
  }, [selectedSessionId, sessionsLoading, sessions, selectedSession, selectSession, navigate])

  const [filterText, setFilterText] = useState("")

  if (!connection) {
    return (
      <main className="flex h-svh items-center justify-center bg-background p-5 text-foreground">
        <FirstServerOnboarding onOpenServers={() => setServerConnectionsOpen(true)} />
        <ServerConnectionsDialog open={serverConnectionsOpen} onOpenChange={setServerConnectionsOpen} />
      </main>
    )
  }

  return (
    <main className="relative h-svh overflow-hidden bg-background p-3 text-foreground">
      {healthError && (
        <div role="alert" className="absolute top-3 right-3 z-10 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive shadow-sm">
          Not connected to pi-server. Check the selected server and retry.
        </div>
      )}
      <div className="hidden h-full lg:block">
      <ResizablePanelGroup orientation="horizontal">
        <ResizablePanel
          className="pr-2"
          collapsible
          collapsedSize="52px"
          defaultSize="22"
          minSize="16"
          maxSize="32"
          panelRef={leftPanelRef}
          onResize={(size) => setLeftSidebarCollapsed(size.inPixels <= 52)}
        >
          <aside className="flex h-full min-w-0 flex-col rounded-xl border border-border bg-muted/[0.15] shadow-sm select-none">
            {leftSidebarCollapsed ? (
              <CollapsedSidebar
                onExpand={() => {
                  leftPanelRef.current?.expand()
                  setLeftSidebarCollapsed(false)
                }}
              />
            ) : (
              <>
                <ServerTreeHeader
                  onCreate={() => setCreateSessionOpen(true)}
                  onManageServers={() => setServerConnectionsOpen(true)}
                  onCollapse={() => {
                    leftPanelRef.current?.collapse()
                    setLeftSidebarCollapsed(true)
                  }}
                />
                <Separator />
                <div className="px-2 pt-2">
                  <div className="relative">
                    <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
                    <input
                      type="text"
                      placeholder="Filter sessions..."
                      value={filterText}
                      onChange={(event) => setFilterText(event.target.value)}
                      className="h-7 w-full rounded-md border border-border bg-muted/40 pl-7 pr-2 text-xs text-foreground placeholder:text-muted-foreground focus:border-primary/40 focus:outline-none"
                    />
                  </div>
                </div>
                <ScrollArea className="min-h-0 flex-1">
                  <nav aria-label="Pi server sessions" className="p-2">
                    <TreeNode
                      expanded
                      icon={<Server className="size-3.5" />}
                      label="Pi Server"
                      status={health?.ok ? "active" : "idle"}
                      badge={health?.capacity ? `${health.capacity.activeSessions}/${health.capacity.maxSessions}` : undefined}
                    >
                      <CapacityControl capacity={health?.capacity} client={client} />
                      {sessionsLoading ? (
                        <div className="flex items-center gap-2 px-3 py-2 text-xs text-muted-foreground">
                          <LoaderCircle className="size-3 animate-spin" />
                          Loading sessions
                        </div>
                      ) : (
                        <SessionTree
                          sessions={sessions}
                          workerIds={workers.map((worker) => worker.id)}
                          selectedSessionId={selectedSessionId}
                          onSelectSession={openSession}
                          filterText={filterText}
                        />
                      )}
                      {globalSessions.length > 0 && (
                        <GlobalSessionList
                          sessions={globalSessions}
                          onOpen={(globalId) => void client.attachGlobalSession(globalId).then((result) => openSession(result.id)).catch(() => { /* handled by query refetch */ })}
                        />
                      )}
                      {(machineResult?.sessions.length ?? 0) > 0 && (
                        <MachineSessionList sessions={machineResult!.sessions} onOpen={async (id) => { const result = await client.openMachineSession(id); openSession(result.id) }} />
                      )}
                    </TreeNode>
                  </nav>
                </ScrollArea>
              </>
            )}
          </aside>
        </ResizablePanel>

        <ResizableHandle className="w-2 bg-transparent after:w-2" />

        <ResizablePanel className="px-1" minSize="35">
          <section className="flex h-full min-w-0 flex-col rounded-xl border border-border bg-background shadow-sm select-none">
            <WorkspaceHeader
              inspectorOpen={inspectorOpen}
              onToggleInspector={() => setInspectorOpen((open) => !open)}
              title={
                selectedSession?.title ||
                selectedSession?.project ||
                "New workspace"
              }
            />
            <Separator />
            {selectedSession ? (
              <SessionWorkspace
                key={selectedSession.id}
                sessionId={selectedSession.id}
              />
            ) : (
              <EmptyWorkspace sessionSelected={false} />
            )}
          </section>
        </ResizablePanel>

        {inspectorOpen && (
          <>
            <ResizableHandle className="w-2 bg-transparent after:w-2" />
            <ResizablePanel
              className="pl-2"
              defaultSize="23"
              minSize="18"
              maxSize="30"
            >
              <SessionInspector session={selectedSession} />
            </ResizablePanel>
          </>
        )}
      </ResizablePanelGroup>
      </div>
      <div className="h-full lg:hidden">
        <MobileWorkspace
          selectedSession={selectedSession}
          sessions={sessions}
          workers={workers}
          globalSessions={globalSessions}
          machineSessions={machineResult?.sessions ?? []}
          onOpenSession={openSession}
          onOpenGlobal={(globalId) => void client.attachGlobalSession(globalId).then((result) => openSession(result.id)).catch(() => {})}
          onOpenMachine={async (id) => { const result = await client.openMachineSession(id); openSession(result.id) }}
          onCreate={() => setCreateSessionOpen(true)}
          onManageServers={() => setServerConnectionsOpen(true)}
        />
      </div>
      <CreateSessionDialog
        open={createSessionOpen}
        onOpenChange={setCreateSessionOpen}
      />
      <ServerConnectionsDialog
        open={serverConnectionsOpen}
        onOpenChange={setServerConnectionsOpen}
      />
    </main>
  )
}

function FirstServerOnboarding({ onOpenServers }: { onOpenServers: () => void }) {
  return (
    <section className="w-full max-w-md rounded-2xl border border-border bg-card p-6 shadow-sm">
      <div className="mb-4 flex size-10 items-center justify-center rounded-xl bg-muted text-muted-foreground"><Server className="size-5" /></div>
      <h1 className="text-lg font-semibold">Connect your first Pi server</h1>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">Webby does not assume a local server. Add the trusted pi-server URL you want to use, then create or open a session.</p>
      <div className="mt-5 rounded-lg bg-muted/50 p-3 font-mono text-xs text-muted-foreground">http://your-laptop-ip:3141</div>
      <Button className="mt-5 w-full" onClick={onOpenServers}>Add Pi server</Button>
    </section>
  )
}

function MobileWorkspace({
  selectedSession,
  sessions,
  workers,
  globalSessions,
  machineSessions,
  onOpenSession,
  onOpenGlobal,
  onOpenMachine,
  onCreate,
  onManageServers,
}: {
  selectedSession?: ApiSession
  sessions: ApiSession[]
  workers: ApiWorker[]
  globalSessions: GlobalSession[]
  machineSessions: MachineSession[]
  onOpenSession: (id?: string) => void
  onOpenGlobal: (id: string) => void
  onOpenMachine: (id: string) => Promise<void>
  onCreate: () => void
  onManageServers: () => void
}) {
  const { theme, setTheme } = useTheme()
  const isDark = theme === "dark"
  const [sessionsOpen, setSessionsOpen] = useState(false)
  const [inspectorOpen, setInspectorOpen] = useState(false)
  const [mobileFilterText, setMobileFilterText] = useState("")
  return (
    <section className="flex h-full min-w-0 flex-col rounded-xl border border-border bg-background shadow-sm">
      <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-3">
        <Button size="icon-sm" variant="ghost" aria-label="Open sessions" onClick={() => setSessionsOpen(true)}><Menu /></Button>
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-sm font-medium">{selectedSession?.title || selectedSession?.project || "Pi"}</h1>
          <p className="text-xs text-muted-foreground">{selectedSession ? "Agent workspace" : "Select a session"}</p>
        </div>
        <Button size="icon-sm" variant="ghost" aria-label="Create session" onClick={onCreate}><Plus /></Button>
        <Button size="icon-sm" variant="ghost" aria-label="Open inspector" disabled={!selectedSession} onClick={() => setInspectorOpen(true)}><PanelRight /></Button>
      </header>
      {selectedSession ? <SessionWorkspace key={selectedSession.id} sessionId={selectedSession.id} /> : <EmptyWorkspace sessionSelected={false} />}
      <Sheet open={sessionsOpen} onOpenChange={setSessionsOpen}>
        <SheetContent side="left" className="w-[88vw] max-w-sm p-0" showCloseButton>
          <SheetHeader>
            <div className="flex items-center justify-between gap-2 pr-8">
              <SheetTitle>Sessions</SheetTitle>
              <div className="flex items-center gap-1">
                <Button size="icon-xs" variant="ghost" aria-label="Manage Pi servers" onClick={onManageServers}><Server /></Button>
                <Button size="icon-xs" variant="ghost" aria-label={isDark ? "Use light theme" : "Use dark theme"} onClick={() => setTheme(isDark ? "light" : "dark")}>
                  {isDark ? <Sun /> : <Moon />}
                </Button>
              </div>
            </div>
          </SheetHeader>
          <Separator />
          <ScrollArea className="min-h-0 flex-1">
            <nav className="p-2">
              <div className="relative mb-2">
                <Search className="pointer-events-none absolute left-2 top-1/2 size-3 -translate-y-1/2 text-muted-foreground" />
                <input
                  type="text"
                  placeholder="Filter sessions..."
                  value={mobileFilterText}
                  onChange={(event) => setMobileFilterText(event.target.value)}
                  className="h-7 w-full rounded-md border border-border bg-muted/40 pl-7 pr-2 text-xs text-foreground placeholder:text-muted-foreground focus:border-primary/40 focus:outline-none"
                />
              </div>
              <SessionTree sessions={sessions} workerIds={workers.map((worker) => worker.id)} selectedSessionId={selectedSession?.id} onSelectSession={(id) => { setSessionsOpen(false); onOpenSession(id) }} filterText={mobileFilterText} />
              {globalSessions.length > 0 && <GlobalSessionList sessions={globalSessions} onOpen={(id) => { setSessionsOpen(false); onOpenGlobal(id) }} />}
              {machineSessions.length > 0 && <MachineSessionList sessions={machineSessions} onOpen={(id) => { setSessionsOpen(false); onOpenMachine(id) }} />}
            </nav>
          </ScrollArea>
        </SheetContent>
      </Sheet>
      <Sheet open={inspectorOpen} onOpenChange={setInspectorOpen}>
        <SheetContent side="right" className="w-[92vw] max-w-md p-0" showCloseButton>
          <SessionInspector session={selectedSession} />
        </SheetContent>
      </Sheet>
    </section>
  )
}

function ServerTreeHeader({
  onCollapse,
  onCreate,
  onManageServers,
}: {
  onCollapse: () => void
  onCreate: () => void
  onManageServers: () => void
}) {
  const { theme, setTheme } = useTheme()
  const isDark = theme === "dark"

  return (
    <div className="flex h-14 items-center justify-between px-3">
      <span className="text-sm font-medium tracking-tight">Pi</span>
      <div className="flex items-center gap-0.5">
        <ThemeToggle
          isDark={isDark}
          onToggle={() => setTheme(isDark ? "light" : "dark")}
        />
        <Button
          size="icon-xs"
          variant="ghost"
          aria-label="Manage Pi servers"
          title="Manage Pi servers"
          onClick={onManageServers}
        >
          <Server />
        </Button>
        <Button
          size="icon-xs"
          variant="ghost"
          aria-label="Create session"
          title="Create session"
          onClick={onCreate}
        >
          <Plus />
        </Button>
        <Button
          size="icon-xs"
          variant="ghost"
          aria-label="Collapse sidebar"
          title="Collapse sidebar"
          onClick={onCollapse}
        >
          <PanelLeftClose />
        </Button>
      </div>
    </div>
  )
}

function CollapsedSidebar({ onExpand }: { onExpand: () => void }) {
  const { theme, setTheme } = useTheme()
  const isDark = theme === "dark"

  return (
    <div className="flex h-full flex-col items-center py-3">
      <span className="text-sm font-medium tracking-tight">Pi</span>
      <Separator className="my-3" />
      <Button
        size="icon-xs"
        variant="ghost"
        aria-label="Expand sidebar"
        title="Expand sidebar"
        onClick={onExpand}
      >
        <PanelLeftOpen />
      </Button>
      <div className="flex-1" />
      <ThemeToggle
        isDark={isDark}
        onToggle={() => setTheme(isDark ? "light" : "dark")}
      />
    </div>
  )
}

function ThemeToggle({
  isDark,
  onToggle,
}: {
  isDark: boolean
  onToggle: () => void
}) {
  return (
    <Button
      size="icon-xs"
      variant="ghost"
      aria-label={isDark ? "Use light theme" : "Use dark theme"}
      title={isDark ? "Use light theme" : "Use dark theme"}
      onClick={onToggle}
    >
      {isDark ? <Sun /> : <Moon />}
    </Button>
  )
}

const STALE_THRESHOLD_MS = 7 * 24 * 60 * 60 * 1000 // 7 days

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

function SessionTree({
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

  return Array.from(workers.entries()).map(([workerId, workerSessions]) => (
    <WorkerBranch
      key={workerId}
      workerId={workerId}
      sessions={workerSessions}
      selectedSessionId={selectedSessionId}
      onSelectSession={onSelectSession}
      pinnedSessionIds={pinnedSessionIds}
    />
  ))
}

function CapacityControl({ capacity, client }: { capacity?: { activeSessions: number; maxSessions: number }; client: PiServerClient }) {
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState("")
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const queryClient = useQueryClient()

  if (!capacity) return null

  const startEdit = () => {
    setValue(String(capacity.maxSessions))
    setEditing(true)
    setError(null)
  }

  const save = async () => {
    const num = parseInt(value, 10)
    if (isNaN(num) || num < 0) {
      setError("Enter 0 or a positive number")
      return
    }
    setSaving(true)
    setError(null)
    try {
      await client.updateCapacity(num)
      setEditing(false)
      void queryClient.invalidateQueries({ queryKey: piQueryKeys.health(client.baseUrl) })
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update")
    } finally {
      setSaving(false)
    }
  }

  const activeColor = capacity.activeSessions >= capacity.maxSessions && capacity.maxSessions > 0
    ? "text-destructive"
    : capacity.activeSessions >= capacity.maxSessions * 0.8 && capacity.maxSessions > 0
      ? "text-amber-500"
      : "text-muted-foreground"

  return (
    <div className="flex h-7 items-center gap-1.5 rounded-md px-3 text-xs">
      <span className="text-muted-foreground">Capacity:</span>
      {editing ? (
        <form onSubmit={(e) => { e.preventDefault(); void save() }} className="flex items-center gap-1">
          <input
            type="number"
            min="0"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            className="h-5 w-12 rounded border border-border bg-muted/40 px-1 text-xs text-center focus:border-primary/40 focus:outline-none"
            autoFocus
            disabled={saving}
          />
          <button type="submit" disabled={saving} className="rounded px-1 text-[10px] text-primary hover:bg-muted disabled:opacity-50">{saving ? "..." : "Save"}</button>
          <button type="button" onClick={() => setEditing(false)} className="rounded px-1 text-[10px] text-muted-foreground hover:bg-muted">Cancel</button>
        </form>
      ) : (
        <>
          <span className={`tabular-nums font-medium ${activeColor}`}>
            {capacity.activeSessions}/{capacity.maxSessions === 0 ? "∞" : capacity.maxSessions}
          </span>
          <button
            type="button"
            onClick={startEdit}
            className="rounded p-0.5 text-muted-foreground hover:bg-muted hover:text-foreground"
            aria-label="Change session limit"
            title="Change session limit"
          >
            <span className="text-[10px]">✎</span>
          </button>
        </>
      )}
      {error && <span className="text-[10px] text-destructive">{error}</span>}
    </div>
  )
}

function MachineSessionList({ sessions, onOpen }: { sessions: MachineSession[]; onOpen: (id: string) => Promise<void> }) {
  const [openError, setOpenError] = useState<string | null>(null)

  const handleOpen = async (id: string) => {
    setOpenError(null)
    try {
      await onOpen(id)
    } catch (error) {
      const message = error instanceof Error ? error.message : "Failed to open session"
      setOpenError(message)
      // Auto-clear after 5 seconds
      setTimeout(() => setOpenError(null), 5000)
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

  // Filter out sessions with empty or missing CWD (defensive — server also filters)
  const validSessions = sessions.filter((s) => s.cwd && s.cwd.trim().length > 0)
  const invalidCount = sessions.length - validSessions.length

  // Avoid showing machine sessions that are already visible as live sessions
  const visibleSessions = validSessions.filter((s) => !liveSessionCwds.has(s.cwd))
  const alreadyOpenSessions = validSessions.filter((s) => liveSessionCwds.has(s.cwd))

  if (validSessions.length === 0 && invalidCount === 0) return null

  const [expanded, setExpanded] = useState(false)

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

function GlobalSessionList({
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

function WorkerBranch({
  workerId,
  sessions,
  selectedSessionId,
  onSelectSession,
  pinnedSessionIds,
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
}) {
  const nodeId = `worker:${workerId}`
  const expanded = useAppStore((state) =>
    Boolean(state.expandedTreeNodes[nodeId])
  )
  const toggle = useAppStore((state) => state.toggleTreeNode)
  const projects = groupByProject(sessions)

  // Auto-expand when filtering
  const hasFilter = sessions.length > 0

  return (
    <TreeNode
      expanded={expanded || hasFilter}
      icon={<Wifi className="size-3.5" />}
      label={workerId === "local" ? "Local" : workerId === "external" ? "Live TUI bridge" : workerId}
      onClick={() => toggle(nodeId)}
    >
      {Array.from(projects.entries()).map(([project, projectSessions]) => (
        <ProjectBranch
          key={project}
          project={project}
          sessions={projectSessions}
          selectedSessionId={selectedSessionId}
          onSelectSession={onSelectSession}
          pinnedSessionIds={pinnedSessionIds}
          source={workerId}
        />
      ))}
      {sessions.length === 0 && <EmptyTreeState compact />}
    </TreeNode>
  )
}

function ProjectBranch({
  project,
  sessions,
  selectedSessionId,
  onSelectSession,
  pinnedSessionIds,
  source,
}: {
  project: string
  sessions: Array<{ id: string; status?: string; title?: string; updatedAt?: string; state?: Record<string, unknown> }>
  selectedSessionId?: string
  onSelectSession: (sessionId?: string) => void
  pinnedSessionIds: Record<string, true>
  source?: string
}) {
  const nodeId = `project:${project}`
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
      return bTime - aTime
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
      expanded={expanded}
      icon={<FolderGit2 className="size-3.5" />}
      label={project}
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
            className="flex h-6 w-full items-center gap-1.5 rounded-md px-3 text-[10px] text-muted-foreground hover:bg-sidebar-accent hover:text-sidebar-foreground"
            style={{ paddingLeft: `${8 + 2 * 14}px` }}
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

function SessionLeaf({
  session,
  source,
  selected,
  pinned,
  onSelect,
}: {
  session: { id: string; status?: string; title?: string; updatedAt?: string; state?: Record<string, unknown> }
  source?: string
  selected: boolean
  pinned: boolean
  onSelect: (sessionId?: string) => void
}) {
  const togglePin = useAppStore((state) => state.togglePinSession)
  const isRelay = source === "external" || session.state?.external === true
  const label = session.title || shortSessionID(session.id)

  return (
    <div className="group/leaf relative">
      <button
        type="button"
        onClick={() => onSelect(session.id)}
        className={cn(
          "flex h-7 w-full items-center gap-1.5 rounded-md pr-2 pl-10 text-left text-xs text-sidebar-foreground transition-colors hover:bg-sidebar-accent",
          selected && "bg-sidebar-accent text-sidebar-accent-foreground"
        )}
      >
        <span className="w-3 shrink-0" />
        <MessageSquare className="size-3.5 shrink-0 text-muted-foreground" />
        <span className="min-w-0 flex-1 truncate">
          {isRelay && <span className="mr-0.5 text-muted-foreground opacity-50">↗</span>}
          {pinned && <Star className="mr-0.5 inline size-3 shrink-0 fill-amber-400 text-amber-400" />}
          {label}
        </span>
        <StatusDot status={sessionStatus(session.status, (session.state?.runtimeStatus as { state?: string } | undefined)?.state)} />
      </button>
      {/* Status detail label */}
      {(() => {
        const rt = session.state?.runtimeStatus as { state?: string; detail?: string } | undefined
        if (!rt || rt.state === "idle" || rt.state === "created") return null
        const detail = rt.detail || rt.state
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
        className="absolute right-1 top-1/2 -translate-y-1/2 rounded p-0.5 opacity-0 transition-opacity hover:bg-muted group-hover/leaf:opacity-100"
        aria-label={pinned ? "Unpin session" : "Pin session"}
        title={pinned ? "Unpin" : "Pin"}
      >
        <Star className={cn("size-3", pinned ? "fill-amber-400 text-amber-400" : "text-muted-foreground")} />
      </button>
    </div>
  )
}

function TreeNode({
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
        className={cn(
          "group flex h-7 w-full items-center gap-1.5 rounded-md pr-2 text-left text-xs text-sidebar-foreground transition-colors hover:bg-sidebar-accent",
          selected && "bg-sidebar-accent text-sidebar-accent-foreground",
          !onClick && "cursor-default hover:bg-transparent"
        )}
        style={{ paddingLeft: `${8 + depth * 14}px` }}
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
        {badge && <span className="shrink-0 rounded bg-muted px-1 py-0.5 text-[10px] tabular-nums text-muted-foreground">{badge}</span>}
        {status && <StatusDot status={status} />}
      </button>
      {hasChildren && expanded && <div>{children}</div>}
    </div>
  )
}

function StatusDot({ status }: { status: SessionStatus }) {
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

function EmptyTreeState({ compact = false }: { compact?: boolean }) {
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

function WorkspaceHeader({
  inspectorOpen,
  onToggleInspector,
  title,
}: {
  inspectorOpen: boolean
  onToggleInspector: () => void
  title: string
}) {
  return (
    <header className="flex h-14 shrink-0 items-center justify-between px-5">
      <div className="min-w-0">
        <h1 className="truncate text-sm font-medium tracking-tight">{title}</h1>
        <p className="mt-0.5 text-xs text-muted-foreground">Agent workspace</p>
      </div>
      <Button
        size="icon-xs"
        variant="ghost"
        aria-label={inspectorOpen ? "Hide inspector" : "Show inspector"}
        title={inspectorOpen ? "Hide inspector" : "Show inspector"}
        onClick={onToggleInspector}
      >
        {inspectorOpen ? <PanelRightClose /> : <PanelRightOpen />}
      </Button>
    </header>
  )
}

function EmptyWorkspace({ sessionSelected }: { sessionSelected: boolean }) {
  return (
    <div className="flex min-h-0 flex-1 items-center justify-center px-6">
      <div className="max-w-sm text-center">
        <div className="mx-auto mb-4 flex size-10 items-center justify-center rounded-xl border border-border bg-muted/40 text-muted-foreground">
          <MessageSquare className="size-4" />
        </div>
        <h2 className="text-sm font-medium">
          {sessionSelected ? "Session ready" : "Select a session"}
        </h2>
        <p className="mt-2 text-sm leading-6 text-muted-foreground">
          {sessionSelected
            ? "The agent conversation and live tool activity will appear here."
            : "Choose a session from the server tree to open its agent workspace."}
        </p>
      </div>
    </div>
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

// sidebarSessionLabel removed — labels now inline in SessionLeaf

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
    case "waiting_for_input": return "waiting"
    case "starting":
    case "reconnecting": return "reconnecting"
    case "error":
    case "failed":
    case "stopped": return "error"
    case "running": return "active"
    case "idle":
    case "created": return "idle"
    default:
      // Unknown states get a console warning so we can add mapping for new
      // server states instead of silently showing idle.
      if (state) console.warn(`[sessionStatus] unknown state: ${state}`)
      return "idle"
  }
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
