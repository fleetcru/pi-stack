import { useState } from "react"
import { useQueryClient } from "@tanstack/react-query"

import { piQueryKeys } from "@/api/hooks"
import type { PiServerClient } from "@/api/client"

export function CapacityControl({ capacity, client }: { capacity?: { activeSessions: number; maxSessions: number }; client: PiServerClient }) {
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
