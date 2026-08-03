import { useState } from "react"
import { ChevronDown, FilePlus2, FileText, FileX2 } from "lucide-react"
import type { GitFileChange } from "@pi-stack/webby-shared/api/client"

const statusMeta: Record<string, { label: string; className: string; icon: typeof FileText }> = {
  A: { label: "Added", className: "border-emerald-500/30 bg-emerald-500/10 text-emerald-400", icon: FilePlus2 },
  D: { label: "Deleted", className: "border-red-500/30 bg-red-500/10 text-red-400", icon: FileX2 },
}

export function ChangedFilesList({ changes }: { changes: GitFileChange[] }) {
  const [expanded, setExpanded] = useState<string | undefined>()
  if (changes.length === 0) return null
  return (
    <section aria-label="Changed files" className="mx-auto w-full max-w-3xl overflow-hidden rounded-lg border border-border/60 bg-background/70 text-xs shadow-sm">
      <div className="flex items-center justify-between border-b border-border/50 px-3 py-2">
        <span className="font-medium text-muted-foreground">Changed files</span>
        <span className="text-muted-foreground">{changes.length} {changes.length === 1 ? "file" : "files"}</span>
      </div>
      <div role="list">
        {changes.map((change) => {
          const meta = statusMeta[change.status] ?? { label: "Modified", className: "border-amber-500/30 bg-amber-500/10 text-amber-300", icon: FileText }
          const Icon = meta.icon
          const isExpanded = expanded === change.path
          return (
            <div key={change.path} role="listitem" className="border-b border-border/40 last:border-b-0">
              <button type="button" onClick={() => setExpanded(isExpanded ? undefined : change.path)} aria-expanded={isExpanded} className="flex w-full items-center gap-2 px-3 py-2 text-left hover:bg-muted/40">
                <span aria-label={meta.label} className={`flex size-5 shrink-0 items-center justify-center rounded border font-mono text-[10px] font-semibold ${meta.className}`}><Icon className="size-3" /></span>
                <span className="min-w-0 flex-1 truncate font-mono text-foreground" title={change.path}>{change.path}</span>
                {change.additions > 0 && <span className="font-mono text-emerald-400">+{change.additions}</span>}
                {change.deletions > 0 && <span className="font-mono text-red-400">-{change.deletions}</span>}
                <ChevronDown className={`size-3.5 text-muted-foreground transition-transform ${isExpanded ? "rotate-180" : ""}`} />
              </button>
              {isExpanded && <div className="border-t border-border/40 bg-muted/20 px-10 py-2 text-[11px] text-muted-foreground">{meta.label} · {change.additions} additions · {change.deletions} deletions</div>}
            </div>
          )
        })}
      </div>
    </section>
  )
}
