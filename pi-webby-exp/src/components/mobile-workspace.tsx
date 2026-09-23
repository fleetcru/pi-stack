import { lazy, Suspense, useState } from "react"
import { House, Menu, Moon, PanelRight, Plus, Search, Server, ShieldCheck, Cpu, Sun } from "lucide-react"

import type { ApiSession, ApiWorker, GlobalSession, MachineSession } from "@/api/client"
import { GlobalSessionList, MachineSessionList } from "@/components/machine-session-list"
import { EmptyWorkspace } from "@/components/workspace-create-bridge"
import { SessionTree } from "@/components/sidebar-tree"
import { Button } from "@/components/ui/button"
import { useTheme } from "@/components/theme-provider"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Sheet, SheetContent, SheetHeader, SheetTitle } from "@/components/ui/sheet"

const SessionInspector = lazy(() => import("@/components/session-inspector").then((module) => ({ default: module.SessionInspector })))
const SessionWorkspace = lazy(() => import("@/components/session-workspace"))

export function MobileWorkspace({
  selectedSession,
  sessions,
  workers,
  globalSessions,
  machineSessions,
  onOpenSession,
  onOpenGlobal,
  onOpenMachine,
  onHome,
  onCreate,
  focusToken = 0,
  onManageServers,
  onManageWorkers,
  onAdmin,
}: {
  selectedSession?: ApiSession
  sessions: ApiSession[]
  workers: ApiWorker[]
  globalSessions: GlobalSession[]
  machineSessions: MachineSession[]
  onOpenSession: (id?: string) => void
  onOpenGlobal: (id: string) => void
  onOpenMachine: (id: string) => Promise<void>
  onHome: () => void
  onCreate: () => void
  focusToken?: number
  onManageServers: () => void
  onManageWorkers: () => void
  onAdmin: () => void
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
        {selectedSession && <Button size="icon-sm" variant="ghost" aria-label="Go to home" title="Home" onClick={onHome}><House /></Button>}
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-sm font-medium">{selectedSession?.title || selectedSession?.project || "Pi"}</h1>
          <p className="text-xs text-muted-foreground">{selectedSession ? "Agent workspace" : "Select a session"}</p>
        </div>
        <Button size="icon-sm" variant="ghost" aria-label="Create session" onClick={onCreate}><Plus /></Button>
        <Button size="icon-sm" variant="ghost" aria-label="Open inspector" disabled={!selectedSession} onClick={() => setInspectorOpen(true)}><PanelRight /></Button>
      </header>
      {selectedSession ? <Suspense fallback={<div className="h-full animate-pulse rounded-xl bg-muted/30" />}><SessionWorkspace key={selectedSession.id} sessionId={selectedSession.id} /></Suspense> : <EmptyWorkspace onCreated={onOpenSession} focusToken={focusToken} />}
      <Sheet open={sessionsOpen} onOpenChange={setSessionsOpen}>
        <SheetContent side="left" className="w-[88vw] max-w-sm p-0" showCloseButton>
          <SheetHeader>
            <div className="flex items-center justify-between gap-2 pr-8">
              <SheetTitle>Sessions</SheetTitle>
              <div className="flex items-center gap-1">
                <Button size="icon-xs" variant="ghost" aria-label="Go to home" onClick={() => { setSessionsOpen(false); onHome() }}><House /></Button>
                <Button size="icon-xs" variant="ghost" aria-label="Manage Pi servers" onClick={onManageServers}><Server /></Button>
                <Button size="icon-xs" variant="ghost" aria-label="Manage workers" onClick={onManageWorkers}><Cpu /></Button>
                <Button size="icon-xs" variant="ghost" aria-label="Server administration" onClick={() => { setSessionsOpen(false); onAdmin() }}><ShieldCheck /></Button>
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
          <SessionInspector session={selectedSession} />
        </SheetContent>
      </Sheet>
    </section>
  )
}
