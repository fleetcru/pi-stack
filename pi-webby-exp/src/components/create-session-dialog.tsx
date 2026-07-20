import { useState } from "react"
import { ChevronDown, Folder, FolderOpen, House, LoaderCircle } from "lucide-react"

import { useCreateSession, usePiServerClient, useWorkers } from "@/api/hooks"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from "@/components/ui/select"
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/components/ui/collapsible"
import { cn } from "@/lib/utils"
import { useAppStore } from "@/state/app-store"

export function CreateSessionDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const [cwd, setCwd] = useState("")
  const [title, setTitle] = useState("")
  const [workerId, setWorkerId] = useState("")
  const [workerHealth, setWorkerHealth] = useState<Record<string, string>>({})
  const [args, setArgs] = useState("")
  const [labels, setLabels] = useState("")
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const [browsing, setBrowsing] = useState(false)
  const [browserLoading, setBrowserLoading] = useState(false)
  const [browserPath, setBrowserPath] = useState<string>()
  const [browserParent, setBrowserParent] = useState<string>()
  const [directories, setDirectories] = useState<Array<{ name: string; path: string }>>([])
  const [browserError, setBrowserError] = useState<string>()
  const createSession = useCreateSession()
  const client = usePiServerClient()
  const { data: workers = [] } = useWorkers()
  const selectSession = useAppStore((state) => state.selectSession)

  async function loadDirectories(path?: string) {
    setBrowserError(undefined)
    setBrowserLoading(true)
    try {
      const result = await client.listDirectories(path)
      setBrowserPath(result.path)
      setBrowserParent(result.parent)
      setDirectories(path ? (result.directories ?? []) : (result.roots?.length ? result.roots : result.directories ?? []))
    } catch (error) {
      setBrowserError(error instanceof Error ? error.message : "Could not load directories")
    } finally {
      setBrowserLoading(false)
    }
  }

  async function refreshWorkerHealth() {
    const results = await Promise.all(workers.filter((worker) => worker.id !== "local").map(async (worker) => {
      try {
        await client.getWorkerHealth(worker.id)
        return [worker.id, "reachable"] as const
      } catch {
        return [worker.id, "unreachable"] as const
      }
    }))
    setWorkerHealth(Object.fromEntries(results))
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (!cwd.trim()) return
    if (!workerId) return
    const input = {
      cwd: cwd.trim(), title: title.trim() || undefined, start: true,
      args: args.split(/\s+/).filter(Boolean),
      labels: labels.split(",").map((label) => label.trim()).filter(Boolean),
    }
    const session = workerId === "local"
      ? await createSession.mutateAsync(input)
      : await client.createWorkerSession(workerId, input)
    selectSession(session.id)
    onOpenChange(false)
    setCwd("")
    setTitle("")
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New session</DialogTitle>
          <DialogDescription>
            Start a Pi agent in an allowed project folder.
          </DialogDescription>
        </DialogHeader>
        <form className="grid gap-4" onSubmit={submit}>
          <div className="flex items-center justify-between">
            <span className="text-sm font-medium">Worker</span>
            <Button type="button" variant="ghost" size="xs" onClick={() => void refreshWorkerHealth()}>
              Check health
            </Button>
          </div>
          <Select value={workerId || null} onValueChange={(value) => setWorkerId(String(value))}>
            <SelectTrigger className="w-full border-input bg-background" aria-label="Worker">
              <SelectValue placeholder="Select a worker" />
            </SelectTrigger>
            <SelectContent side="bottom" align="start" alignItemWithTrigger={false}>
              <SelectGroup>
                <SelectLabel>Session location</SelectLabel>
                <SelectItem value="local">Local</SelectItem>
              </SelectGroup>
              {workers.filter((worker) => worker.id !== "local").length > 0 && (
                <SelectGroup>
                  <SelectLabel>Remote workers</SelectLabel>
                  {workers.filter((worker) => worker.id !== "local").map((worker) => (
                    <SelectItem key={worker.id} value={worker.id} disabled={worker.status === "error"}>
                      {worker.id}{workerHealth[worker.id] ? ` · ${workerHealth[worker.id]}` : worker.status ? ` · ${worker.status}` : ""}
                    </SelectItem>
                  ))}
                </SelectGroup>
              )}
            </SelectContent>
          </Select>
          <label className="grid gap-2 text-sm">
            <span className="flex items-center justify-between">Project folder
              <Button type="button" size="xs" variant="ghost" onClick={() => { setBrowsing(!browsing); if (!browsing) void loadDirectories() }}>Browse</Button>
            </span>
            <Input
              autoFocus
              value={cwd}
              onChange={(event) => setCwd(event.target.value)}
              placeholder="/home/user/project"
            />
          </label>
          {browsing && (
            <div className="overflow-hidden rounded-lg border border-border bg-muted/20 text-sm">
              <div className="flex items-center gap-2 border-b border-border px-2 py-2">
                <Button type="button" size="icon-xs" variant="ghost" title="Allowed roots" onClick={() => void loadDirectories()}><House /></Button>
                <Button type="button" size="xs" variant="ghost" disabled={!browserParent} onClick={() => void loadDirectories(browserParent)}>Up</Button>
                <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">{browserPath || "Choose an allowed root"}</span>
              </div>
              <div className="max-h-44 overflow-auto p-1.5">
                {browserLoading ? <div className="flex items-center gap-2 px-2 py-3 text-xs text-muted-foreground"><LoaderCircle className="size-3 animate-spin" /> Loading folders</div> : browserError ? <p className="px-2 py-3 text-xs text-destructive">{browserError}</p> : directories.map((directory) => (
                  <button key={directory.path} type="button" className={cn("flex w-full items-center gap-2 rounded-md px-2 py-2 text-left hover:bg-muted", cwd === directory.path && "bg-primary/10 text-primary")} onClick={() => void loadDirectories(directory.path)}>
                    <Folder className="size-3.5 shrink-0 text-muted-foreground" />
                    <span className="min-w-0 flex-1 truncate">{directory.name}</span>
                  </button>
                ))}
              </div>
              {browserPath && <div className="flex justify-end border-t border-border p-2"><Button type="button" size="xs" onClick={() => { setCwd(browserPath); setBrowsing(false) }}><FolderOpen /> Use this folder</Button></div>}
            </div>
          )}
          <label className="grid gap-2 text-sm">
            Title <span className="text-muted-foreground">(optional)</span>
            <Input
              value={title}
              onChange={(event) => setTitle(event.target.value)}
              placeholder="Refactor authentication"
            />
          </label>
          <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen} className="rounded-lg border border-border/70">
            <CollapsibleTrigger className="flex w-full items-center justify-between px-3 py-2.5 text-sm font-medium hover:bg-muted/50">
              Advanced options
              <ChevronDown className={cn("size-4 text-muted-foreground transition-transform", advancedOpen && "rotate-180")} />
            </CollapsibleTrigger>
            <CollapsibleContent className="grid gap-4 border-t border-border/70 p-3">
              <label className="grid gap-2 text-sm">
                Arguments <span className="text-muted-foreground">(optional)</span>
                <Input value={args} onChange={(event) => setArgs(event.target.value)} placeholder="--model provider/model" />
              </label>
              <label className="grid gap-2 text-sm">
                Labels <span className="text-muted-foreground">(comma separated)</span>
                <Input value={labels} onChange={(event) => setLabels(event.target.value)} placeholder="backend, urgent" />
              </label>
            </CollapsibleContent>
          </Collapsible>
          {createSession.error instanceof Error && (
            <p className="text-sm text-destructive">
              {createSession.error.message}
            </p>
          )}
          <DialogFooter>
            <Button
              type="submit"
              disabled={createSession.isPending || !cwd.trim() || !workerId}
            >
              {createSession.isPending ? "Starting…" : "Create session"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
