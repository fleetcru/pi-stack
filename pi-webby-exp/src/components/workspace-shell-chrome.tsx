import {
  House,
  Moon,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
  Plus,
  Server,
  ShieldCheck,
  Cpu,
  Sun,
} from "lucide-react"

import { Button } from "@/components/ui/button"
import { useTheme } from "@/components/theme-provider"
import { Separator } from "@/components/ui/separator"

export function FirstServerOnboarding({ onOpenServers, onUseLocal }: { onOpenServers: () => void; onUseLocal: () => void }) {
  return (
    <section className="w-full max-w-md rounded-2xl border border-border bg-card p-6 shadow-sm">
      <div className="mb-4 flex size-10 items-center justify-center rounded-xl bg-muted text-muted-foreground"><Server className="size-5" /></div>
      <h1 className="text-lg font-semibold">Connect your first Pi server</h1>
      <p className="mt-2 text-sm leading-6 text-muted-foreground">Use the local Pi server instantly, or connect to a different trusted server.</p>
      <div className="mt-5 grid gap-2 sm:grid-cols-2"><Button onClick={onUseLocal}>Use local server</Button><Button variant="outline" onClick={onOpenServers}>Add remote server</Button></div>
    </section>
  )
}

export function ServerTreeHeader({
  onCollapse,
  onHome,
  onCreate,
  onManageServers,
  onManageWorkers,
  onAdmin,
}: {
  onCollapse: () => void
  onHome: () => void
  onCreate: () => void
  onManageServers: () => void
  onManageWorkers: () => void
  onAdmin: () => void
}) {
  const { theme, setTheme } = useTheme()
  const isDark = theme === "dark"

  return (
    <div className="flex h-14 items-center justify-between px-2.5">
      <Button size="sm" variant="ghost" className="px-2 font-semibold" onClick={onHome} aria-label="Go to home">
        <House data-icon="inline-start" />
        Pi
      </Button>
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
        <Button size="icon-xs" variant="ghost" aria-label="Manage workers" title="Manage workers" onClick={onManageWorkers}><Cpu /></Button>
        <Button size="icon-xs" variant="ghost" aria-label="Server administration" title="Server administration" onClick={onAdmin}><ShieldCheck /></Button>
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

export function CollapsedSidebar({ onExpand, onHome, onAdmin }: { onExpand: () => void; onHome: () => void; onAdmin: () => void }) {
  const { theme, setTheme } = useTheme()
  const isDark = theme === "dark"

  return (
    <div className="flex h-full flex-col items-center py-3">
      <Button size="icon-xs" variant="ghost" aria-label="Go to home" title="Home" onClick={onHome}><House /></Button>
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
      <Button size="icon-xs" variant="ghost" aria-label="Server administration" title="Server administration" onClick={onAdmin}><ShieldCheck /></Button>
      <ThemeToggle
        isDark={isDark}
        onToggle={() => setTheme(isDark ? "light" : "dark")}
      />
    </div>
  )
}

export function ThemeToggle({
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

export function WorkspaceHeader({
  inspectorOpen,
  onHome,
  onToggleInspector,
  showHome,
  title,
}: {
  inspectorOpen: boolean
  onHome: () => void
  onToggleInspector: () => void
  showHome: boolean
  title: string
}) {
  return (
    <header className="flex h-14 shrink-0 items-center justify-between px-3">
      <div className="flex min-w-0 items-center gap-2">
        {showHome && (
          <Button size="icon-sm" variant="ghost" aria-label="Go to home" title="Home" onClick={onHome}>
            <House />
          </Button>
        )}
        <div className="min-w-0">
          <h1 className="truncate text-sm font-medium tracking-tight">{title}</h1>
          <p className="mt-0.5 text-xs text-muted-foreground">{showHome ? "Agent workspace" : "Start or open a session"}</p>
        </div>
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
