import { useState } from "react"
import { useSessionGitFileDiff } from "@/api/hooks"
import { ChevronDown, FilePlus2, FileText, FileX2 } from "lucide-react"
import type { GitFileChange } from "@pi-stack/webby-shared/api/client"

const statusMeta: Record<string, { label: string; className: string; icon: typeof FileText }> = {
  A: { label: "Added", className: "border-emerald-500/30 bg-emerald-500/10 text-emerald-400", icon: FilePlus2 },
  D: { label: "Deleted", className: "border-red-500/30 bg-red-500/10 text-red-400", icon: FileX2 },
}

export function ChangedFilesList({ changes, sessionId }: { changes: GitFileChange[]; sessionId: string }) {
  // Whole-card collapse, default collapsed. The header is a button that toggles
  // the file list open/closed; each file keeps its own diff-expand chevron.
  const [expanded, setExpanded] = useState<string | undefined>()
  const [open, setOpen] = useState(false)
  if (changes.length === 0) return null
  return (
    <section aria-label="Changed files" className="mx-auto w-full max-w-3xl overflow-hidden rounded-lg border border-border/60 bg-background/70 text-xs shadow-sm">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        className="flex w-full items-center justify-between gap-2 border-b border-border/50 px-3 py-2 text-left hover:bg-muted/40"
      >
        <span className="flex items-center gap-1.5 font-medium text-muted-foreground">
          <ChevronDown className={`size-3.5 text-muted-foreground transition-transform ${open ? "rotate-0" : "-rotate-90"}`} />
          Changed files
        </span>
        <span className="text-muted-foreground">{changes.length} {changes.length === 1 ? "file" : "files"}</span>
      </button>
      {open && (<div role="list">
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
              {isExpanded && <FileDiffRow sessionId={sessionId} path={change.path} additions={change.additions} deletions={change.deletions} />}
            </div>
          )
        })}
      </div>)}
    </section>
  )
}

function FileDiffRow({ sessionId, path, additions, deletions }: { sessionId: string; path: string; additions: number; deletions: number }) {
  const diffQuery = useSessionGitFileDiff(sessionId, path)
  if (diffQuery.isLoading) return <div className="border-t border-border/40 bg-muted/20 px-10 py-3 text-[11px] text-muted-foreground">Loading diff…</div>
  if (diffQuery.isError) return <div className="border-t border-border/40 bg-muted/20 px-10 py-3 text-[11px] text-destructive">Could not load diff.</div>
  const lines = parseDiff(diffQuery.data?.diff ?? "")
  return (
    <div className="border-t border-border/40 bg-[#0d1117] px-2 py-2">
      <div className="mb-2 flex items-center justify-between px-2 text-[10px] text-muted-foreground">
        <span>{additions} additions · {deletions} deletions</span>
        <span className="font-mono">{path}</span>
      </div>
      <div className="max-h-72 overflow-auto rounded border border-white/10 font-mono text-[11px] leading-5">
        {lines.length === 0 ? <div className="px-3 py-2 text-muted-foreground">No textual diff available.</div> : lines.map((line, index) => (
          <div key={`${index}-${line.text}`} className={`whitespace-pre px-3 ${line.kind === "add" ? "bg-emerald-500/15 text-emerald-200" : line.kind === "remove" ? "bg-red-500/15 text-red-200" : line.kind === "meta" ? "text-sky-300" : "text-slate-300"}`}>
            {line.text || " "}
          </div>
        ))}
      </div>
    </div>
  )
}

function parseDiff(diff: string): Array<{ text: string; kind: "add" | "remove" | "meta" | "context" }> {
  return diff.split("\n").filter((line) => line.length > 0).map((text) => ({
    text,
    kind: text.startsWith("+++") || text.startsWith("---") || text.startsWith("@@") || text.startsWith("diff ") || text.startsWith("new file") ? "meta" : text.startsWith("+") ? "add" : text.startsWith("-") ? "remove" : "context",
  }))
}
