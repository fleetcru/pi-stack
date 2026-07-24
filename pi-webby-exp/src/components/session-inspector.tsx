import { useEffect, useState } from "react"
import { GitBranch } from "lucide-react"

import type { ApiSession, RpcResponse } from "@/api/client"
import {
  useFileTree,
  usePiServerClient,
  useSessionData,
  useSessionEvents,
  useSessionFileContent,
  useSessionGit,
  useSessionGitBranches,
  useSessionGitStatus,
  useSessionGitWorktrees,
} from "@/api/hooks"
import { Button } from "@/components/ui/button"
import { Input } from "@/components/ui/input"
import { Progress } from "@/components/ui/progress"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"

export function SessionInspector({ session }: { session?: ApiSession }) {
  return (
    <aside className="flex h-full min-w-0 flex-col rounded-xl border border-border bg-muted/[0.15] shadow-sm select-none">
      <div className="flex h-12 shrink-0 items-center px-4">
        <h2 className="truncate text-sm font-medium">
          {session?.title || session?.project || "Inspector"}
        </h2>
      </div>
      <Separator />
      {session ? (
        <Tabs
          defaultValue="overview"
          className="flex min-h-0 flex-1 flex-col gap-0"
        >
          <TabsList
            variant="line"
            className="grid h-10 w-full shrink-0 grid-cols-4 px-2"
          >
            <TabsTrigger value="overview" className="min-w-0 px-1 text-[11px]">
              Overview
            </TabsTrigger>
            <TabsTrigger value="activity" className="min-w-0 px-1 text-[11px]">
              Activity
            </TabsTrigger>
            <TabsTrigger value="workspace" className="min-w-0 px-1 text-[11px]">
              Files
            </TabsTrigger>
            <TabsTrigger value="settings" className="min-w-0 px-1 text-[11px]">
              Settings
            </TabsTrigger>
          </TabsList>
          <Separator />
          <TabsContent
            value="overview"
            className="min-h-0 flex-1 overflow-hidden"
          >
            <Overview session={session} />
          </TabsContent>
          <TabsContent
            value="activity"
            className="min-h-0 flex-1 overflow-hidden"
          >
            <ActivityFeed sessionId={session.id} />
          </TabsContent>
          <TabsContent
            value="workspace"
            className="min-h-0 flex-1 overflow-hidden"
          >
            <Workspace session={session} />
          </TabsContent>
          <TabsContent
            value="settings"
            className="min-h-0 flex-1 overflow-hidden"
          >
            <Settings session={session} />
          </TabsContent>
        </Tabs>
      ) : (
        <div className="p-4 text-sm text-muted-foreground">
          Select a session to inspect it.
        </div>
      )}
    </aside>
  )
}

function Overview({ session }: { session: ApiSession }) {
  const client = usePiServerClient()
  const stateQuery = useSessionData(session.id, "state", {
    refetchInterval: 2_000,
  })
  const statsQuery = useSessionData(session.id, "stats", {
    refetchInterval: 5_000,
  })
  const state = responseData<StateData>(stateQuery.data)
  const stats = responseData<StatsData>(statsQuery.data)
  const contextPercent = stats?.contextUsage?.percent ?? 0
  const relayState = state as (StateData & { relayConnected?: boolean; relayLatencyMs?: number }) | undefined

  return (
    <ScrollArea className="h-full">
      <div className="space-y-5 p-4">
        <Section title="Runtime">
          <Row
            label="Status"
            value={state?.isStreaming ? "Running" : session.status || "Idle"}
            dot={state?.isStreaming}
          />
          <Row
            label="Model"
            value={state?.model?.name || state?.model?.id || "Not selected"}
          />
          <Row label="Thinking" value={state?.thinkingLevel || "off"} />
          {relayState?.relayConnected !== undefined ? (
            <Row label="Relay" value={relayState.relayConnected ? `Connected · ${relayState.relayLatencyMs ?? "?"} ms` : "Offline"} />
          ) : null}
          <Row label="Queue" value={String(state?.pendingMessageCount ?? 0)} />
        </Section>
        <Section title="Session">
          <Row label="Transport" value={(session.state?.transport as string | undefined) || ((session.state?.external as boolean | undefined) ? "Live TUI relay" : "Local RPC")} />
          <Row label="Server" value={session.workerId || "local"} />
          <Row label="Project" value={session.project || "—"} />
          <Row label="Working directory" value={session.cwd || "—"} />
          <Row label="Task" value={session.taskType || "—"} />
          <Row label="Labels" value={session.labels?.join(", ") || "—"} />
          <Row label="Created" value={formatDate(session.createdAt)} />
          <Row label="Updated" value={formatDate(session.updatedAt)} />
        </Section>
        <Section title="Context">
          <div className="space-y-2">
            <div className="flex justify-between text-xs">
              <span className="text-muted-foreground">Usage</span>
              <span>
                {formatNumber(stats?.contextUsage?.tokens)} /{" "}
                {formatNumber(stats?.contextUsage?.contextWindow)}
              </span>
            </div>
            <Progress value={contextPercent} />
            <Row
              label="Cost"
              value={stats?.cost == null ? "—" : `$${stats.cost.toFixed(4)}`}
            />
            <Row
              label="Messages"
              value={String(stats?.totalMessages ?? state?.messageCount ?? 0)}
            />
          </div>
        </Section>
        <div className="grid grid-cols-2 gap-2">
          <Button
            variant="destructive"
            size="sm"
            onClick={() => void client.abort(session.id)}
          >
            Abort
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => void client.compact(session.id)}
          >
            Compact
          </Button>
        </div>
      </div>
    </ScrollArea>
  )
}

function ActivityFeed({ sessionId }: { sessionId: string }) {
  const { data } = useSessionEvents(sessionId)
  const events = [...(data?.events ?? [])].reverse()
  return (
    <ScrollArea className="h-full">
      <div className="space-y-1 p-3">
        {events.length ? (
          events.map((record) => (
            <div
              key={record.id}
              className="rounded-lg px-2 py-2 hover:bg-muted/50"
            >
              <div className="flex items-center gap-2 text-xs">
                <span className="size-1.5 rounded-full bg-muted-foreground/50" />
                <span className="truncate font-medium">
                  {eventLabel(record.event)}
                </span>
                <time className="ml-auto shrink-0 text-muted-foreground">
                  {new Date(record.timestamp).toLocaleTimeString([], {
                    hour: "2-digit",
                    minute: "2-digit",
                  })}
                </time>
              </div>
              {record.event.toolName != null && (
                <p className="mt-1 pl-3.5 text-xs text-muted-foreground">
                  {String(record.event.toolName)}
                </p>
              )}
            </div>
          ))
        ) : (
          <EmptyLine text="No activity yet" />
        )}
      </div>
    </ScrollArea>
  )
}

function Workspace({ session }: { session: ApiSession }) {
  const client = usePiServerClient()
  const git = useSessionGit(session.id, "status")
  const gitStatus = useSessionGitStatus(session.id)
  const branches = useSessionGitBranches(session.id)
  const worktrees = useSessionGitWorktrees(session.id)
  const diff = useSessionGit(session.id, "diff")
  const log = useSessionGit(session.id, "log")
  const files = useFileTree(session.cwd)
  const [selectedPath, setSelectedPath] = useState<string>()
  const [gitView, setGitView] = useState<"status" | "diff" | "log" | "branches" | "worktrees">("status")
  const [branch, setBranch] = useState("")
  const [worktreePath, setWorktreePath] = useState("")
  const [startPoint, setStartPoint] = useState("")
  const [existingBranch, setExistingBranch] = useState(false)
  const [commitMessage, setCommitMessage] = useState("")
  const [mergeBranch, setMergeBranch] = useState("")
  const [remote, setRemote] = useState("origin")
  const content = useSessionFileContent(session.id, selectedPath)
  const status = gitStatus.data?.status
  return (
    <ScrollArea className="h-full">
      <div className="space-y-5 p-4">
        <Section title="Git">
          <div className="flex items-center gap-2 text-xs">
            <GitBranch className="size-3.5" />
            <span className="font-medium">{status?.branch || git.data?.output.split("\\n")[0] || "No Git repository"}</span>
            {status && <span className="ml-auto text-muted-foreground">{status.staged.length + status.modified.length + status.untracked.length} changed</span>}
          </div>
          <div className="grid grid-cols-5 gap-1">
            {(["status", "diff", "log", "branches", "worktrees"] as const).map((view) => (
              <Button key={view} size="xs" variant={gitView === view ? "default" : "outline"} onClick={() => setGitView(view)}>
                {view[0].toUpperCase() + view.slice(1)}
              </Button>
            ))}
          </div>
          {gitView === "status" && (
            <div className="space-y-1 text-xs text-muted-foreground">
              <Row label="Branch" value={status?.branch || "—"} />
              <Row label="Ahead / behind" value={`${status?.ahead ?? 0} / ${status?.behind ?? 0}`} />
              <Row label="Staged" value={String(status?.staged.length ?? 0)} />
              <Row label="Modified" value={String(status?.modified.length ?? 0)} />
              <Row label="Untracked" value={String(status?.untracked.length ?? 0)} />
              {git.isError && <p className="text-destructive">Git status unavailable.</p>}
              <div className="space-y-1 border-t border-border/60 pt-2">
                <Input placeholder="commit message" value={commitMessage} onChange={(event) => setCommitMessage(event.target.value)} />
                <Button size="sm" disabled={!commitMessage.trim()} onClick={() => { if (window.confirm("Create a commit with all current changes?")) void client.commitSessionGit(session.id, commitMessage.trim()).then(() => { setCommitMessage(""); return gitStatus.refetch() }) }}>Commit all changes</Button>
                <div className="flex gap-1">
                  <Input placeholder="branch to merge" value={mergeBranch} onChange={(event) => setMergeBranch(event.target.value)} />
                  <Button size="sm" variant="outline" disabled={!mergeBranch.trim()} onClick={() => { if (window.confirm(`Merge ${mergeBranch} into the current branch?`)) void client.mergeSessionGit(session.id, mergeBranch.trim()).then(() => { setMergeBranch(""); return gitStatus.refetch() }) }}>Merge</Button>
                </div>
                <div className="flex gap-1">
                  <Input placeholder="remote" value={remote} onChange={(event) => setRemote(event.target.value)} />
                  <Button size="sm" variant="outline" onClick={() => { if (window.confirm(`Pull from ${remote}?`)) void client.pullSessionGit(session.id, remote).then(() => gitStatus.refetch()) }}>Pull</Button>
                  <Button size="sm" variant="outline" onClick={() => { if (window.confirm(`Push to ${remote}?`)) void client.pushSessionGit(session.id, remote).then(() => gitStatus.refetch()) }}>Push</Button>
                </div>
                <Button size="sm" variant="ghost" onClick={() => void client.abortGitMerge(session.id).then(() => gitStatus.refetch())}>Abort merge</Button>
              </div>
            </div>
          )}
          {gitView === "diff" && <pre className="max-h-64 overflow-auto rounded-lg bg-muted/40 p-2 font-mono text-xs whitespace-pre-wrap">{diff.data?.output || "No changes"}</pre>}
          {gitView === "log" && <pre className="max-h-64 overflow-auto rounded-lg bg-muted/40 p-2 font-mono text-xs whitespace-pre-wrap">{log.data?.output || "No commits"}</pre>}
          {gitView === "branches" && <div className="space-y-1 text-xs">{branches.data?.branches.map((item) => <div key={item.name} className="flex justify-between rounded px-2 py-1 hover:bg-muted/40"><span>{item.current ? "* " : "  "}{item.name}</span><span className="text-muted-foreground">{item.remote || "local"}</span></div>)}</div>}
          {gitView === "worktrees" && (
            <div className="space-y-2 text-xs">
              {worktrees.data?.worktrees.map((item) => (
                <div key={item.path} className="rounded border border-border/60 p-2">
                  <div className="font-medium">{item.branch || "detached"}</div>
                  <div className="truncate text-muted-foreground">{item.path}</div>
                  <div className="mt-1 flex gap-1">
                    <Button size="xs" variant="outline" onClick={() => void client.createSession({ cwd: item.path, start: true }).then(() => undefined)}>Open session</Button>
                    <Button size="xs" variant="ghost" onClick={() => void client.removeSessionWorktree(session.id, item.path).then(() => worktrees.refetch())}>Remove</Button>
                  </div>
                </div>
              ))}
              <div className="space-y-1 border-t border-border/60 pt-2">
                <Input list={existingBranch ? "git-branches" : undefined} placeholder={existingBranch ? "existing branch" : "new branch name"} value={branch} onChange={(event) => setBranch(event.target.value)} />
                <datalist id="git-branches">{branches.data?.branches.map((item) => <option key={item.name} value={item.name} />)}</datalist>
                {!existingBranch && <Input placeholder="base branch (optional)" value={startPoint} onChange={(event) => setStartPoint(event.target.value)} />}
                <Input placeholder="path (relative to repo)" value={worktreePath} onChange={(event) => setWorktreePath(event.target.value)} />
                <label className="flex items-center gap-2 text-xs text-muted-foreground">
                  <input type="checkbox" checked={existingBranch} onChange={(event) => setExistingBranch(event.target.checked)} />
                  Use existing branch
                </label>
                <Button size="sm" className="w-full" disabled={!branch || !worktreePath} onClick={() => void client.createSessionWorktree(session.id, { path: worktreePath, branch, startPoint: existingBranch ? undefined : startPoint || undefined, existingBranch }).then(() => { setBranch(""); setStartPoint(""); setWorktreePath(""); return worktrees.refetch() })}>{existingBranch ? "Create from existing branch" : "Create new branch"}</Button>
              </div>
            </div>
          )}
        </Section>
        <Section title={`Files (${files.data?.files.length ?? 0})`}>
          <div className="space-y-1">
            {files.data?.files.slice(0, 40).map((file) => (
              <div
                className="flex min-w-0 items-center gap-2 py-1 text-xs"
                key={file.path}
              >
                <span className="text-muted-foreground">
                  {file.isDir ? "▸" : "·"}
                </span>
                <button
                  type="button"
                  className="min-w-0 truncate text-left hover:text-primary disabled:cursor-default disabled:hover:text-inherit"
                  disabled={file.isDir}
                  title={file.path}
                  onClick={() => setSelectedPath(file.path)}
                >
                  {relativePath(session.cwd, file.path)}
                </button>
              </div>
            ))}
          </div>
        </Section>
        {selectedPath && (
          <Section title={relativePath(session.cwd, selectedPath)}>
            {content.isLoading ? (
              <p className="text-xs text-muted-foreground">Loading file…</p>
            ) : content.error ? (
              <p className="text-xs text-destructive">Could not load file content.</p>
            ) : content.data?.binary ? (
              <p className="text-xs text-muted-foreground">Binary files cannot be previewed.</p>
            ) : (
              <pre className="max-h-64 overflow-auto rounded-lg bg-muted/40 p-2 font-mono text-xs whitespace-pre-wrap">
                {content.data?.content || "Empty file"}
                {content.data?.truncated && "\n\n[Preview truncated at 1 MiB]"}
              </pre>
            )}
          </Section>
        )}
      </div>
    </ScrollArea>
  )
}

function Settings({ session }: { session: ApiSession }) {
  const client = usePiServerClient()
  const stateQuery = useSessionData(session.id, "state", {
    refetchInterval: 5_000,
  })
  const forksQuery = useSessionData(session.id, "fork-messages")
  const state = responseData<StateData>(stateQuery.data)
  const forks =
    responseData<{ messages?: Array<{ entryId: string; text: string }> }>(
      forksQuery.data
    )?.messages ?? []
  const [title, setTitle] = useState(session.title ?? "")
  const [project, setProject] = useState(session.project ?? "")
  const [autoRetry, setAutoRetry] = useState(true)
  // Sync local state when the session prop changes (e.g., after server-side rename).
  useEffect(() => { setTitle(session.title ?? "") }, [session.title])
  useEffect(() => { setProject(session.project ?? "") }, [session.project])

  async function refresh() {
    await stateQuery.refetch()
  }
  return (
    <ScrollArea className="h-full">
      <div className="space-y-5 p-4">
        <Section title="Metadata">
          <div className="space-y-3">
            <label className="flex flex-col gap-1.5 text-xs text-muted-foreground">
              <span>Title</span>
              <Input
                value={title}
                onChange={(event) => setTitle(event.target.value)}
              />
            </label>
            <label className="flex flex-col gap-1.5 text-xs text-muted-foreground">
              <span>Project</span>
              <Input
                value={project}
                onChange={(event) => setProject(event.target.value)}
              />
            </label>
            <div className="pt-1">
              <Button
                size="sm"
                className="w-full"
                onClick={() =>
                  void client.updateSessionMetadata(session.id, {
                    title,
                    project,
                  })
                }
              >
                Save metadata
              </Button>
            </div>
          </div>
        </Section>
        <Section title="Agent">
          <ActionRow
            label="Model"
            value={state?.model?.name || state?.model?.id || "—"}
            action="Cycle"
            onClick={() =>
              void client
                .sessionPost(session.id, "cycle-model", {})
                .then(refresh)
            }
          />
          <ActionRow
            label="Thinking"
            value={state?.thinkingLevel || "off"}
            action="Cycle"
            onClick={() =>
              void client
                .sessionPost(session.id, "cycle-thinking-level", {})
                .then(refresh)
            }
          />
          <ToggleRow
            label="Auto retry"
            checked={autoRetry}
            onChange={(checked) => {
              setAutoRetry(checked)
              void client.sessionPost(session.id, "auto-retry", {
                enabled: checked,
              })
            }}
          />
          <ToggleRow
            label="Auto compact"
            checked={state?.autoCompactionEnabled ?? false}
            onChange={(checked) =>
              void client
                .sessionPost(session.id, "auto-compaction", {
                  enabled: checked,
                })
                .then(refresh)
            }
          />
        </Section>
        <Section title="Session">
          <div className="grid grid-cols-2 gap-2">
            <Button
              size="sm"
              variant="outline"
              disabled={!forks.length}
              onClick={() =>
                void client.sessionPost(session.id, "fork", {
                  entryId: forks.at(-1)?.entryId,
                })
              }
            >
              Fork
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => void client.sessionPost(session.id, "clone", {})}
            >
              Clone
            </Button>
            <Button
              size="sm"
              variant="outline"
              className="col-span-2"
              onClick={() =>
                void client.sessionPost(session.id, "export-html", {})
              }
            >
              Export HTML
            </Button>
          </div>
        </Section>
      </div>
    </ScrollArea>
  )
}

function Section({
  title,
  children,
}: {
  title: string
  children: React.ReactNode
}) {
  return (
    <section className="space-y-3 border-b border-border/70 pb-5 last:border-0 last:pb-0">
      <h3 className="text-[10px] font-medium tracking-[0.14em] text-muted-foreground uppercase">
        {title}
      </h3>
      <div className="space-y-1.5">{children}</div>
    </section>
  )
}
function Row({
  label,
  value,
  dot,
}: {
  label: string
  value: string
  dot?: boolean
}) {
  return (
    <div className="flex min-h-7 items-center justify-between gap-4 text-xs">
      <span className="shrink-0 text-muted-foreground">{label}</span>
      <span className="flex min-w-0 items-center justify-end gap-1.5 truncate text-right font-medium tabular-nums">
        {dot && (
          <span className="size-1.5 shrink-0 rounded-full bg-emerald-500" />
        )}
        {value}
      </span>
    </div>
  )
}
function ActionRow({
  label,
  value,
  action,
  onClick,
}: {
  label: string
  value: string
  action: string
  onClick: () => void
}) {
  return (
    <div className="flex min-h-10 items-center gap-3 text-xs">
      <div className="min-w-0 flex-1">
        <p className="text-muted-foreground">{label}</p>
        <p className="mt-0.5 truncate font-medium">{value}</p>
      </div>
      <Button size="xs" variant="outline" onClick={onClick}>
        {action}
      </Button>
    </div>
  )
}
function ToggleRow({
  label,
  checked,
  onChange,
}: {
  label: string
  checked: boolean
  onChange: (checked: boolean) => void
}) {
  return (
    <div className="flex min-h-8 items-center justify-between gap-4 text-xs">
      <span>{label}</span>
      <Switch size="sm" checked={checked} onCheckedChange={onChange} />
    </div>
  )
}
function EmptyLine({ text }: { text: string }) {
  return <p className="p-3 text-xs text-muted-foreground">{text}</p>
}
function responseData<T>(response?: RpcResponse): T | undefined {
  return response?.data as T | undefined
}
function formatDate(value?: string) {
  return value ? new Date(value).toLocaleString() : "—"
}

function formatNumber(value?: number | null) {
  return value == null
    ? "—"
    : new Intl.NumberFormat(undefined, { notation: "compact" }).format(value)
}
function relativePath(cwd: string | undefined, path: string) {
  if (!cwd) return path
  return path.startsWith(cwd) ? path.slice(cwd.length).replace(/^[\\/]/, "") : path
}
function eventLabel(event: Record<string, unknown>) {
  const type = String(event.type ?? "event")
  return type
    .replaceAll("_", " ")
    .replace(/^./, (letter) => letter.toUpperCase())
}

type StateData = {
  model?: { id?: string; name?: string }
  thinkingLevel?: string
  isStreaming?: boolean
  pendingMessageCount?: number
  messageCount?: number
  autoCompactionEnabled?: boolean
}
type StatsData = {
  totalMessages?: number
  cost?: number
  contextUsage?: {
    tokens?: number | null
    contextWindow?: number
    percent?: number | null
  }
}
