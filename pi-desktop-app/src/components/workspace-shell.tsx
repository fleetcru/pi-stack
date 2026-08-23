import { lazy, Suspense, useEffect, useMemo, useState } from "react"
import { useNavigate } from "react-router"
import { usePanelRef } from "react-resizable-panels"
import {
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
  Sun,
} from "lucide-react"

import { useGlobalSessions, useMachineSessions, usePiServerClient, useServerHealth, useSessions, useWorkers } from "@/api/hooks"
import { PiServerApiError, type ApiSession, type ApiWorker, type GlobalSession, type MachineSession } from "@/api/client"
import { CapacityControl } from "@/components/capacity-control"
import { CreateSessionDialog } from "@/components/create-session-dialog"
import { GlobalSessionList, MachineSessionList } from "@/components/machine-session-list"
import { ServerConnectionsDialog } from "@/components/server-connections-dialog"
const SessionInspector = lazy(() => import("@/components/session-inspector").then((module) => ({ default: module.SessionInspector })) )
const SessionWorkspace = lazy(() => import("@/components/session-workspace"))
import { SessionTree, TreeNode } from "@/components/sidebar-tree"
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
import { useAppStore } from "@/state/app-store"

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
  const { data: health, error: healthError, refetch: refetchHealth } = useServerHealth()
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
        <div role="alert" className="absolute top-3 right-3 z-10 flex items-center gap-2 rounded-md border border-destructive/30 bg-destructive/10 px-3 py-2 text-xs text-destructive shadow-sm">
          <span>
            {healthError instanceof PiServerApiError && healthError.status === 401
              ? "Authentication required. Re-enter the server token."
              : "Not connected to pi-server. Check the selected server and retry."}
          </span>
          <Button
            size="xs"
            variant="outline"
            onClick={() => healthError instanceof PiServerApiError && healthError.status === 401
              ? setServerConnectionsOpen(true)
              : void refetchHealth()}
          >
            {healthError instanceof PiServerApiError && healthError.status === 401 ? "Reconnect" : "Retry"}
          </Button>
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
                      aria-label="Filter sessions"
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
              <Suspense fallback={<div className="h-full animate-pulse rounded-xl bg-muted/30" />}>
                <SessionWorkspace key={selectedSession.id} sessionId={selectedSession.id} />
              </Suspense>
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
              <Suspense fallback={<div className="h-full animate-pulse rounded-xl bg-muted/30" />}><SessionInspector session={selectedSession} /></Suspense>
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
      {selectedSession ? <Suspense fallback={<div className="h-full animate-pulse rounded-xl bg-muted/30" />}><SessionWorkspace key={selectedSession.id} sessionId={selectedSession.id} /></Suspense> : <EmptyWorkspace sessionSelected={false} />}
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
                      aria-label="Filter sessions"
                  value={mobileFilterText}
                  onChange={(event) => setMobileFilterText(event.target.value)}
                  className="h-7 w-full rounded-md border border-border bg-muted/40 pl-7 pr-2 text-xs text-foreground placeholder:text-muted-foreground focus:border-primary/40 focus:outline-none"
                />
              </div>
              <SessionTree sessions={sessions} workerIds={workers.map((worker) => worker.id)} selectedSessionId={selectedSession?.id} onSelectSession={(id) => { setSessionsOpen(false); onOpenSession(id) }} filterText={mobileFilterText} />
              {globalSessions.length > 0 && <GlobalSessionList sessions={globalSessions} onOpen={(id) => { setSessionsOpen(false); onOpenGlobal(id) }} />}
              {machineSessions.length > 0 && <MachineSessionList sessions={machineSessions} onOpen={async (id) => { setSessionsOpen(false); await onOpenMachine(id) }} />}
            </nav>
          </ScrollArea>
        </SheetContent>
      </Sheet>
      <Sheet open={inspectorOpen} onOpenChange={setInspectorOpen}>
        <SheetContent side="right" className="w-[92vw] max-w-md p-0" showCloseButton>
          <Suspense fallback={<div className="h-full animate-pulse rounded-xl bg-muted/30" />}><SessionInspector session={selectedSession} /></Suspense>
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
