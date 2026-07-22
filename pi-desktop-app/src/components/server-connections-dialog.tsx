import { useState } from "react"
import { Trash2 } from "lucide-react"

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
  const removeServer = useAppStore((state) => state.removeServer)
  const [name, setName] = useState("")
  const [baseUrl, setBaseUrl] = useState("")
  const [token, setToken] = useState("")

  function add() {
    const normalized = baseUrl.trim().replace(/\/+$/, "")
    if (!normalized) return
    try {
      new URL(normalized)
    } catch {
      return // invalid URL
    }
    addServer({ baseUrl: normalized, name: name.trim() || normalized, token: token || undefined })
    setName("")
    setBaseUrl("")
    setToken("")
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Pi servers</DialogTitle>
          <DialogDescription>
            Switch between trusted pi-server instances. Tokens stay in memory and are never saved to browser storage.
          </DialogDescription>
        </DialogHeader>
        <div className="grid gap-2">
          {servers.map((server) => (
            <div key={server.baseUrl} className="flex items-center gap-2 rounded-md border border-border p-2">
              <button
                type="button"
                className="min-w-0 flex-1 text-left text-sm"
                onClick={() => {
                  setConnection({ ...server, token: server.baseUrl === active?.baseUrl ? active.token : undefined })
                  onOpenChange(false)
                }}
              >
                <span className="block truncate font-medium">{server.name || server.baseUrl}</span>
                <span className="block truncate text-xs text-muted-foreground">{server.baseUrl}</span>
              </button>
              <Button
                size="icon-xs"
                variant="ghost"
                aria-label={`Remove ${server.name || server.baseUrl}`}
                onClick={() => removeServer(server.baseUrl)}
              >
                <Trash2 />
              </Button>
            </div>
          ))}
        </div>
        <div className="grid gap-3 border-t border-border pt-4">
          <Input value={name} onChange={(event) => setName(event.target.value)} placeholder="Server name (optional)" />
          <Input value={baseUrl} onChange={(event) => setBaseUrl(event.target.value)} placeholder="https://pi-server.example:3141" />
          {(() => {
            try {
              const url = new URL(baseUrl.trim())
              if (url.protocol === "http:" && url.hostname !== "localhost" && url.hostname !== "127.0.0.1") {
                return <p className="text-xs text-amber-600 dark:text-amber-400">HTTP sends your token in plaintext. Use HTTPS for remote servers.</p>
              }
            } catch { /* ignore invalid URL while typing */ }
            return null
          })()}
          <Input value={token} onChange={(event) => setToken(event.target.value)} placeholder="Bearer token (not saved)" type="password" />
        </div>
        <DialogFooter>
          <Button type="button" onClick={add} disabled={!baseUrl.trim()}>Add server</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
