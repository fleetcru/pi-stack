import { ChevronDown, Folder, FolderOpen, House, LoaderCircle } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Input } from "@/components/ui/input"
import { cn } from "@/lib/utils"

export type DirectoryEntry = { name: string; path: string }

export function QuickSessionMoreOptions({
  cwd,
  effectiveCwd,
  title,
  isolated,
  sessionCount,
  args,
  labels,
  advancedOpen,
  browsing,
  browserLoading,
  browserPath,
  browserParent,
  directories,
  browserError,
  onCwdChange,
  onTitleChange,
  onIsolatedChange,
  onSessionCountChange,
  onArgsChange,
  onLabelsChange,
  onAdvancedOpenChange,
  onToggleBrowsing,
  onLoadDirectories,
  onUseBrowserPath,
}: {
  cwd: string
  effectiveCwd: string
  title: string
  isolated: boolean
  sessionCount: number
  args: string
  labels: string
  advancedOpen: boolean
  browsing: boolean
  browserLoading: boolean
  browserPath?: string
  browserParent?: string
  directories: DirectoryEntry[]
  browserError?: string
  onCwdChange: (value: string) => void
  onTitleChange: (value: string) => void
  onIsolatedChange: (value: boolean) => void
  onSessionCountChange: (value: number) => void
  onArgsChange: (value: string) => void
  onLabelsChange: (value: string) => void
  onAdvancedOpenChange: (open: boolean) => void
  onToggleBrowsing: () => void
  onLoadDirectories: (path?: string) => void
  onUseBrowserPath: () => void
}) {
  return (
    <div className="mt-3 grid gap-3 rounded-xl border border-border/70 bg-card/40 p-3 text-sm">
      <label className="grid gap-2">
        <span className="flex items-center justify-between font-medium">
          Project folder
          <Button type="button" size="xs" variant="ghost" onClick={onToggleBrowsing}>
            Browse
          </Button>
        </span>
        <Input
          value={cwd || effectiveCwd}
          onChange={(event) => onCwdChange(event.target.value)}
          placeholder="/home/user/project"
          aria-label="Project folder path"
        />
      </label>
      {browsing && (
        <div className="overflow-hidden rounded-lg border border-border bg-muted/20 text-sm">
          <div className="flex items-center gap-2 border-b border-border px-2 py-2">
            <Button type="button" size="icon-xs" variant="ghost" title="Allowed roots" onClick={() => onLoadDirectories()}><House /></Button>
            <Button type="button" size="xs" variant="ghost" disabled={!browserParent} onClick={() => onLoadDirectories(browserParent)}>Up</Button>
            <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{browserPath || "Choose an allowed root"}</span>
          </div>
          <div className="max-h-44 overflow-auto p-1.5">
            {browserLoading ? (
              <div className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"><LoaderCircle className="size-3 animate-spin" /> Loading folders</div>
            ) : browserError ? (
              <p className="px-2 py-3 text-xs text-destructive">{browserError}</p>
            ) : directories.map((directory) => (
              <button
                key={directory.path}
                type="button"
                className={cn("flex w-full items-center gap-2 rounded-md px-2 py-2 text-left hover:bg-muted", (cwd || effectiveCwd) === directory.path && "bg-primary/10 text-primary")}
                onClick={() => onLoadDirectories(directory.path)}
              >
                <Folder className="size-3.5 shrink-0 text-muted-foreground" />
                <span className="min-w-0 flex-1 truncate">{directory.name}</span>
              </button>
            ))}
          </div>
          {browserPath && (
            <div className="flex justify-end border-t border-border p-2">
              <Button type="button" size="xs" onClick={onUseBrowserPath}>
                <FolderOpen /> Use this folder
              </Button>
            </div>
          )}
        </div>
      )}
      <label className="grid gap-2">
        <span>Title <span className="text-muted-foreground">(optional)</span></span>
        <Input value={title} onChange={(event) => onTitleChange(event.target.value)} placeholder="Refactor authentication" />
      </label>
      <label className="flex items-start gap-2 rounded-lg border border-border/70 p-3">
        <input
          type="checkbox"
          checked={isolated}
          onChange={(event) => onIsolatedChange(event.target.checked)}
          className="mt-0.5 accent-primary"
        />
        <span className="grid gap-0.5">
          <span className="font-medium">Isolated git worktree</span>
          <span className="text-muted-foreground">
            Spawn a fresh feature branch in <span className="font-mono">.pi-worktrees/&lt;title&gt;</span> so
            agent edits never touch your working tree. Cleaned up on session close.
          </span>
        </span>
      </label>
      <label className="grid gap-2">
        Number of sessions
        <Input
          type="number"
          min={1}
          max={12}
          value={sessionCount}
          step={1}
          onChange={(event) => onSessionCountChange(Math.min(12, Math.max(1, Math.trunc(Number(event.target.value) || 1))))}
          aria-describedby="session-count-help"
        />
        <span id="session-count-help" className="text-xs text-muted-foreground">Start up to 12 identical sessions at once. A number is added to each title.</span>
      </label>
      <Collapsible open={advancedOpen} onOpenChange={onAdvancedOpenChange} className="rounded-lg border border-border/70">
        <CollapsibleTrigger className="flex w-full items-center justify-between px-3 py-2.5 text-sm font-medium hover:bg-muted/50">
          Advanced options
          <ChevronDown className={cn("size-4 text-muted-foreground transition-transform", advancedOpen && "rotate-180")} />
        </CollapsibleTrigger>
        <CollapsibleContent className="grid gap-4 border-t border-border/70 p-3">
          <label className="grid gap-2">
            Arguments <span className="text-muted-foreground">(optional)</span>
            <Input value={args} onChange={(event) => onArgsChange(event.target.value)} placeholder="--model provider/model" />
          </label>
          <label className="grid gap-2">
            Labels <span className="text-muted-foreground">(comma separated)</span>
            <Input value={labels} onChange={(event) => onLabelsChange(event.target.value)} placeholder="backend, urgent" />
          </label>
        </CollapsibleContent>
      </Collapsible>
    </div>
  )
}
