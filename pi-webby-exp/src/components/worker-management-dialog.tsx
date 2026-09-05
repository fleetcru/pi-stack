import { useState } from "react"
import { Check, Pencil, Plus, RefreshCw, Trash2, X } from "lucide-react"
import { useQueryClient } from "@tanstack/react-query"
import { useWorkers, usePiServerClient, piQueryKeys } from "@/api/hooks"
import { Button } from "@/components/ui/button"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"

export function WorkerManagementDialog({ open, onOpenChange }: { open: boolean; onOpenChange: (open: boolean) => void }) {
  const { data: workers = [] } = useWorkers()
  const client = usePiServerClient()
  const queryClient = useQueryClient()
  const [editing, setEditing] = useState<string>()
  const [removing, setRemoving] = useState<string>()
  const [id, setId] = useState("")
  const [url, setUrl] = useState("")
  const [token, setToken] = useState("")
  const [tags, setTags] = useState("")
  const [error, setError] = useState("")
  const reset = () => { setEditing(undefined); setId(""); setUrl(""); setToken(""); setTags(""); setError("") }
  const refresh = () => queryClient.invalidateQueries({ queryKey: piQueryKeys.workers(client.cacheScope) })
  async function save() {
    if (!id.trim() || !url.trim()) return
    try {
      const body = { id: id.trim(), url: url.trim(), ...(token.trim() ? { token: token.trim() } : {}), tags: tags.split(",").map((tag) => tag.trim()).filter(Boolean) }
      if (editing) await client.updateWorker(editing, body)
      else await client.addWorker(body)
      reset(); refresh()
    } catch (e) { setError(e instanceof Error ? e.message : "Unable to save worker") }
  }
  async function remove(workerId: string) { try { await client.deleteWorker(workerId); setRemoving(undefined); refresh() } catch (e) { setError(e instanceof Error ? e.message : "Unable to remove worker") } }
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent className="sm:max-w-lg"><DialogHeader><DialogTitle>Manage workers</DialogTitle><DialogDescription>Add remote Pi workers that should appear when creating sessions. The local worker cannot be changed.</DialogDescription></DialogHeader>
    <div className="grid gap-2">{workers.map((worker) => <div key={worker.id} className="flex items-center gap-2 rounded-md border p-2"><div className="min-w-0 flex-1"><p className="truncate text-sm font-medium">{worker.id}{worker.id === "local" && <span className="ml-2 text-xs text-muted-foreground">Local</span>}</p><p className="truncate text-xs text-muted-foreground">{worker.url} · {worker.status || "unknown"}</p></div>{worker.id !== "local" && (removing === worker.id ? <><span className="text-xs text-destructive">Remove?</span><Button size="icon-xs" variant="ghost" aria-label={`Confirm remove ${worker.id}`} onClick={() => void remove(worker.id)}><Check /></Button><Button size="icon-xs" variant="ghost" aria-label="Cancel remove worker" onClick={() => setRemoving(undefined)}><X /></Button></> : <><Button size="icon-xs" variant="ghost" aria-label={`Edit worker ${worker.id}`} onClick={() => { setEditing(worker.id); setId(worker.id); setUrl(worker.url); setToken(""); setTags(worker.tags?.join(", ") || "") }}><Pencil /></Button><Button size="icon-xs" variant="ghost" aria-label={`Remove worker ${worker.id}`} onClick={() => setRemoving(worker.id)}><Trash2 /></Button></>)}</div>)}</div>
    <div className="grid gap-3 border-t pt-4"><div className="flex items-center justify-between"><div><p className="text-sm font-medium">{editing ? "Edit worker" : "Add a remote worker"}</p><p className="text-xs text-muted-foreground">Use HTTPS for remote workers.</p></div>{editing && <Button size="sm" variant="ghost" onClick={reset}><X /> Cancel</Button>}</div><Input value={id} onChange={(e) => setId(e.target.value)} placeholder="Worker ID" disabled={Boolean(editing)} /><Input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://worker.example.com:3141" /><Input value={token} onChange={(e) => setToken(e.target.value)} placeholder={editing ? "New token (leave blank to keep current)" : "Worker auth token (optional)"} type="password" /><Input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="Tags, comma separated (optional)" />{error && <p role="alert" className="text-xs text-destructive">{error}</p>}</div>
    <DialogFooter><Button variant="outline" onClick={() => { refresh(); setError("") }}><RefreshCw /> Refresh</Button><Button onClick={() => void save()} disabled={!id.trim() || !url.trim()}>{editing ? <Check /> : <Plus />}{editing ? "Save changes" : "Add worker"}</Button></DialogFooter>
  </DialogContent></Dialog>
}
