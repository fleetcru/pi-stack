import { ChevronDown, Folder, FolderOpen, GitBranch, House, LoaderCircle, Tags } from "lucide-react"

import { Button } from "@/components/ui/button"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import { Switch } from "@/components/ui/switch"
import { cn } from "@/lib/utils"

export type DirectoryEntry = { name: string; path: string }

const fieldLabelClass = "flex h-5 items-center text-xs font-medium uppercase tracking-wide text-muted-foreground"
const fieldInputClass = "h-10 w-full rounded-xl border-border/70 bg-background/60"

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
    <div className="mt-3 overflow-hidden rounded-[20px] border border-border/80 bg-muted/50 p-4 shadow-[0_12px_40px_-32px_rgba(0,0,0,0.65)] ring-1 ring-foreground/[0.025]">
      <div className="grid gap-3">
        <div className="grid gap-1.5">
          <div className="flex h-5 items-center justify-between gap-3">
            <Label htmlFor="quick-session-cwd" className={fieldLabelClass}>
              Project folder
            </Label>
            <Button type="button" size="xs" variant="ghost" className="h-5 px-2 text-xs" onClick={onToggleBrowsing}>
              {browsing ? "Hide browser" : "Browse"}
            </Button>
          </div>
          <Input
            id="quick-session-cwd"
            value={cwd || effectiveCwd}
            onChange={(event) => onCwdChange(event.target.value)}
            placeholder="/home/user/project"
            aria-label="Project folder path"
            className={cn(fieldInputClass, "font-mono text-xs")}
          />
        </div>

        {browsing && (
          <div className="overflow-hidden rounded-xl border border-border/70 bg-background/40">
            <div className="flex h-10 items-center gap-2 border-b border-border/70 px-2.5">
              <Button type="button" size="icon-xs" variant="ghost" title="Allowed roots" onClick={() => onLoadDirectories()}>
                <House />
              </Button>
              <Button type="button" size="xs" variant="ghost" disabled={!browserParent} onClick={() => onLoadDirectories(browserParent)}>
                Up
              </Button>
              <span className="min-w-0 flex-1 truncate font-mono text-[11px] text-muted-foreground">
                {browserPath || "Choose an allowed root"}
              </span>
            </div>
            <div className="max-h-44 overflow-auto p-1.5">
              {browserLoading ? (
                <div className="flex items-center gap-2 px-3 py-3 text-xs text-muted-foreground">
                  <LoaderCircle className="size-3.5 animate-spin" /> Loading folders
                </div>
              ) : browserError ? (
                <p className="px-3 py-3 text-xs text-destructive">{browserError}</p>
              ) : directories.length === 0 ? (
                <p className="px-3 py-3 text-xs text-muted-foreground">No folders here</p>
              ) : (
                directories.map((directory) => (
                  <button
                    key={directory.path}
                    type="button"
                    className={cn(
                      "flex h-9 w-full items-center gap-2 rounded-lg px-2.5 text-left text-sm transition-colors hover:bg-muted/80",
                      (cwd || effectiveCwd) === directory.path && "bg-primary/10 text-primary",
                    )}
                    onClick={() => onLoadDirectories(directory.path)}
                  >
                    <Folder className="size-3.5 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate">{directory.name}</span>
                  </button>
                ))
              )}
            </div>
            {browserPath && (
              <div className="flex h-10 items-center justify-end border-t border-border/70 px-2.5">
                <Button type="button" size="xs" className="rounded-lg" onClick={onUseBrowserPath}>
                  <FolderOpen /> Use this folder
                </Button>
              </div>
            )}
          </div>
        )}

        <div className="grid gap-1.5">
          <div className="grid grid-cols-[minmax(0,1fr)_5.5rem] gap-3">
            <Label htmlFor="quick-session-title" className={fieldLabelClass}>
              Title <span className="normal-case tracking-normal text-muted-foreground/80">(optional)</span>
            </Label>
            <Label htmlFor="quick-session-count" className={fieldLabelClass}>
              Sessions
            </Label>
          </div>
          <div className="grid grid-cols-[minmax(0,1fr)_5.5rem] items-center gap-3">
            <Input
              id="quick-session-title"
              value={title}
              onChange={(event) => onTitleChange(event.target.value)}
              placeholder="Refactor authentication"
              className={fieldInputClass}
            />
            <Input
              id="quick-session-count"
              type="number"
              min={1}
              max={12}
              value={sessionCount}
              step={1}
              onChange={(event) => onSessionCountChange(Math.min(12, Math.max(1, Math.trunc(Number(event.target.value) || 1))))}
              aria-describedby="session-count-help"
              className={cn(fieldInputClass, "text-center tabular-nums")}
            />
          </div>
          <p id="session-count-help" className="text-[11px] leading-snug text-muted-foreground">
            Up to 12 at once. Titles get numbered when count is over 1.
          </p>
        </div>

        <div className="flex min-h-10 items-center gap-3 rounded-xl border border-border/70 bg-background/40 px-3 py-2.5">
          <div className="flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted/80 text-muted-foreground">
            <GitBranch className="size-3.5" />
          </div>
          <div className="min-w-0 flex-1">
            <Label htmlFor="quick-session-worktree" className="text-sm font-medium leading-none">
              Isolated git worktree
            </Label>
            <p className="mt-1 text-[11px] leading-snug text-muted-foreground">
              Spawn a branch under <span className="font-mono text-[10px]">.pi-worktrees/&lt;title&gt;</span> so edits stay off your working tree.
            </p>
          </div>
          <Switch
            id="quick-session-worktree"
            checked={isolated}
            onCheckedChange={onIsolatedChange}
            aria-label="Isolated git worktree"
            className="shrink-0"
          />
        </div>

        <Collapsible open={advancedOpen} onOpenChange={onAdvancedOpenChange}>
          <CollapsibleTrigger className="flex h-10 w-full items-center justify-between rounded-xl border border-border/70 bg-background/40 px-3 text-sm font-medium transition-colors hover:bg-muted/40">
            <span className="flex items-center gap-2">
              <Tags className="size-3.5 text-muted-foreground" />
              Arguments &amp; labels
            </span>
            <ChevronDown className={cn("size-4 text-muted-foreground transition-transform", advancedOpen && "rotate-180")} />
          </CollapsibleTrigger>
          <CollapsibleContent className="grid gap-3 pt-3">
            <div className="grid gap-1.5">
              <Label htmlFor="quick-session-args" className={fieldLabelClass}>
                Arguments <span className="normal-case tracking-normal text-muted-foreground/80">(optional)</span>
              </Label>
              <Input
                id="quick-session-args"
                value={args}
                onChange={(event) => onArgsChange(event.target.value)}
                placeholder="--model provider/model"
                className={cn(fieldInputClass, "font-mono text-xs")}
              />
            </div>
            <div className="grid gap-1.5">
              <Label htmlFor="quick-session-labels" className={fieldLabelClass}>
                Labels <span className="normal-case tracking-normal text-muted-foreground/80">(comma separated)</span>
              </Label>
              <Input
                id="quick-session-labels"
                value={labels}
                onChange={(event) => onLabelsChange(event.target.value)}
                placeholder="backend, urgent"
                className={fieldInputClass}
              />
            </div>
          </CollapsibleContent>
        </Collapsible>
      </div>
    </div>
  )
}
