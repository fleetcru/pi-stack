import { useState } from "react"
import { Check, Pencil, Plus, Trash2, X } from "lucide-react"

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
import { Checkbox } from "@/components/ui/checkbox"
import { useAppStore } from "@/state/app-store"

/** A small connection picker; it intentionally uses the existing dialog style. */
export function ServerConnectionsDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
}) {
  const servers = useAppStore((state) => state.servers)
  const active = useAppStore((state) => state.connection)
  const setConnection = useAppStore((state) => state.setConnection)
  const addServer = useAppStore((state) => state.addServer)
  const updateServer = useAppStore((state) => state.updateServer)
  const removeServer = useAppStore((state) => state.removeServer)
  const [name, setName] = useState("")
  const [baseUrl, setBaseUrl] = useState("")
  const [token, setToken] = useState("")
  const [rememberToken, setRememberToken] = useState(false)
  const [editingUrl, setEditingUrl] = useState<string>()
  const [removingUrl, setRemovingUrl] = useState<string>()

  function resetForm() {
    setName(""); setBaseUrl(""); setToken(""); setRememberToken(false); setEditingUrl(undefined)
  }

  function add() {
    const normalized = baseUrl.trim().replace(/\/+$/, "")
    if (!normalized) return
    try {
      new URL(normalized)
    } catch {
      return // invalid URL
    }
    const values = {
      baseUrl: normalized,
      name: name.trim() || normalized,
      token: token.trim() || undefined,
      rememberToken,
    }
    if (editingUrl) updateServer(editingUrl, values)
    else addServer(values)
    resetForm()
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Pi servers</DialogTitle>
          <DialogDescription>
            Switch between trusted pi-server instances. Tokens stay in memory unless you explicitly choose to remember one on this browser.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          {servers.length === 0 && <p className="rounded-md border border-dashed p-4 text-center text-sm text-muted-foreground">No servers yet. Add a trusted pi-server below.</p>}
          {servers.map((server) => (
            <div key={server.baseUrl} className={`flex items-center gap-2 rounded-md border p-2 ${server.baseUrl === active?.baseUrl ? "border-primary/50 bg-primary/5" : "border-border"}`}>
              <button
                type="button"
                className="min-w-0 flex-1 text-left text-sm"
                onClick={() => {
                  setConnection({ ...server, token: server.token ?? (server.baseUrl === active?.baseUrl ? active.token : undefined) })
                  onOpenChange(false)
                }}
              >
                <span className="block truncate font-medium">{server.name || server.baseUrl}</span>
                <span className="block truncate text-xs text-muted-foreground">{server.baseUrl}</span>
              </button>
              {removingUrl === server.baseUrl ? <><span className="text-xs text-destructive">Remove?</span><Button size="icon-xs" variant="ghost" aria-label={`Confirm remove ${server.name || server.baseUrl}`} onClick={() => { removeServer(server.baseUrl); setRemovingUrl(undefined) }}><Check /></Button><Button size="icon-xs" variant="ghost" aria-label="Cancel remove" onClick={() => setRemovingUrl(undefined)}><X /></Button></> : <><Button size="icon-xs" variant="ghost" aria-label={`Edit ${server.name || server.baseUrl}`} onClick={() => { setEditingUrl(server.baseUrl); setName(server.name || ""); setBaseUrl(server.baseUrl); setToken(server.token || ""); setRememberToken(server.rememberToken === true); setRemovingUrl(undefined) }}><Pencil /></Button><Button size="icon-xs" variant="ghost" aria-label={`Remove ${server.name || server.baseUrl}`} onClick={() => setRemovingUrl(server.baseUrl)}><Trash2 /></Button></>}
            </div>
          ))}
        </div>
        <div className="grid gap-3 border-t border-border pt-4">
          <div className="flex items-center justify-between"><div><p className="text-sm font-medium">{editingUrl ? "Edit server" : "Add a server"}</p><p className="text-xs text-muted-foreground">Use a name you will recognize later.</p></div>{editingUrl && <Button type="button" size="sm" variant="ghost" onClick={resetForm}><X /> Cancel</Button>}</div>
          <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Server name (optional)" />
          <Input value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} placeholder="https://pi-server.example:3141" />
          {(() => {
            try {
              const url = new URL(baseUrl.trim())
              return url.protocol === "http:" && url.hostname !== "localhost" && url.hostname !== "127.0.0.1"
            } catch { /* ignore invalid URL while typing */ }
            return false
          })() && <p className="text-xs text-amber-600 dark:text-amber-400">HTTP sends your token in plaintext. Use HTTPS for remote servers.</p>}
          <Input value={token} onChange={(event) => setToken(event.target.value)} placeholder="Auth token (raw value; do not include Bearer)" type="password" />
          <label className="flex items-center gap-2 text-xs text-muted-foreground">
            <Checkbox checked={rememberToken} onCheckedChange={(checked) => setRememberToken(checked === true)} />
            Remember token on this browser
          </label>
          {rememberToken && <p className="text-xs text-amber-600 dark:text-amber-400">This stores the bearer token in browser storage. Any script or extension with page access may read it.</p>}
        </div>
        <DialogFooter>
          <Button type="button" onClick={add} disabled={!baseUrl.trim()}><>{editingUrl ? <Check /> : <Plus />}</>{editingUrl ? "Save changes" : "Add server"}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
