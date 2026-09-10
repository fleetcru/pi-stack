import { useMemo, useState } from "react"
import { useNavigate } from "react-router"
import { useQuery, useQueryClient } from "@tanstack/react-query"
import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  Check,
  Clipboard,
  Cpu,
  Gauge,
  LoaderCircle,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  Save,
  Server,
  ShieldCheck,
  Trash2,
  Users,
  X,
} from "lucide-react"

import {
  PiServerApiError,
  PiServerClient,
  type AdminSettings,
  type AdminState,
  type ApiWorker,
  type CreatedTrustedDevice,
  type TrustedDevice,
} from "@/api/client"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card"
import { Checkbox } from "@/components/ui/checkbox"
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog"
import { Field, FieldDescription, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Skeleton } from "@/components/ui/skeleton"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { useAppStore } from "@/state/app-store"
import type { ServerConnectionSettings } from "@pi-stack/webby-shared/state/app-store"

type FieldKind = "text" | "number" | "list" | "boolean"
type SettingField = { key: keyof AdminSettings; label: string; kind: FieldKind; hint?: string }
type SettingGroup = { title: string; description: string; fields: SettingField[] }

const settingGroups: SettingGroup[] = [
  {
    title: "Capacity",
    description: "These limits apply immediately. Use 0 for unlimited where supported.",
    fields: [
      { key: "maxSessions", label: "Maximum sessions", kind: "number" },
      { key: "maxActiveRuns", label: "Active runs", kind: "number" },
      { key: "maxRunsPerSession", label: "Runs per session", kind: "number" },
      { key: "maxRunsPerWorker", label: "Runs per worker", kind: "number" },
      { key: "maxQueuedRuns", label: "Queued runs", kind: "number" },
    ],
  },
  {
    title: "Server process",
    description: "Process and storage changes take effect after a manual server restart.",
    fields: [
      { key: "addr", label: "Listen address", kind: "text", hint: "host:port" },
      { key: "piBinary", label: "Pi binary", kind: "text" },
      { key: "cwd", label: "Default working directory", kind: "text" },
      { key: "dataDir", label: "Data directory", kind: "text" },
      { key: "extensions", label: "Extensions", kind: "list", hint: "Comma separated" },
    ],
  },
  {
    title: "Network policy",
    description: "Restrict browser origins, working directories, and remote worker hosts.",
    fields: [
      { key: "allowedOrigins", label: "Allowed origins", kind: "list", hint: "Comma separated" },
      { key: "allowedRoots", label: "Allowed roots", kind: "list", hint: "Comma separated" },
      { key: "allowedWorkerHosts", label: "Allowed worker hosts", kind: "list", hint: "Comma separated" },
    ],
  },
  {
    title: "Timeouts",
    description: "Use Go duration values such as 30s, 5m, or 2h.",
    fields: [
      { key: "shutdownTimeout", label: "Shutdown", kind: "text" },
      { key: "requestTimeout", label: "Request", kind: "text" },
      { key: "readTimeout", label: "Read", kind: "text" },
      { key: "writeTimeout", label: "Write", kind: "text" },
      { key: "idleTimeout", label: "Idle", kind: "text" },
      { key: "distributedRunTimeout", label: "Distributed run", kind: "text" },
    ],
  },
  {
    title: "Reliability and retention",
    description: "Tune process recovery, retained events, file watchers, and debug logging.",
    fields: [
      { key: "restartMax", label: "Restart attempts", kind: "number" },
      { key: "restartBackoff", label: "Restart backoff", kind: "text" },
      { key: "eventHistoryMax", label: "Event history count", kind: "number" },
      { key: "eventHistoryBytes", label: "Event history bytes", kind: "number" },
      { key: "maxWatches", label: "Maximum file watches", kind: "number" },
      { key: "debug", label: "Debug logging", kind: "boolean" },
    ],
  },
]

function createClient(server: ServerConnectionSettings) {
  return new PiServerClient({ baseUrl: server.baseUrl, token: server.token })
}

function isLocalServer(server: ServerConnectionSettings) {
  try {
    const host = new URL(server.baseUrl).hostname
    return host === "localhost" || host === "127.0.0.1" || host === "::1"
  } catch {
    return false
  }
}

function errorMessage(error: unknown) {
  if (error instanceof PiServerApiError) {
    if (error.status === 401 || error.status === 403) return "This server needs its bootstrap admin token. Edit the saved server connection and try again."
    if (error.status === 404) return "Upgrade this pi-server to a build that supports Desktop administration."
    return error.message
  }
  return error instanceof Error ? error.message : "The request failed."
}

function formatDuration(seconds: number) {
  if (seconds < 60) return `${seconds}s`
  if (seconds < 3600) return `${Math.floor(seconds / 60)}m`
  if (seconds < 86400) return `${Math.floor(seconds / 3600)}h ${Math.floor((seconds % 3600) / 60)}m`
  return `${Math.floor(seconds / 86400)}d ${Math.floor((seconds % 86400) / 3600)}h`
}

function formatDate(value?: string) {
  if (!value || value.startsWith("0001-")) return "Never"
  const date = new Date(value)
  return Number.isNaN(date.getTime()) ? "Unknown" : date.toLocaleString()
}

export function ServerAdminPage() {
  const navigate = useNavigate()
  const servers = useAppStore((state) => state.servers)
  const activeServer = useAppStore((state) => state.connection)
  const setConnection = useAppStore((state) => state.setConnection)
  const [requestedUrl, setRequestedUrl] = useState(activeServer?.baseUrl ?? servers[0]?.baseUrl ?? "")
  const selectedUrl = servers.some((server) => server.baseUrl === requestedUrl)
    ? requestedUrl
    : activeServer?.baseUrl ?? servers[0]?.baseUrl ?? ""
  const selectedServer = servers.find((server) => server.baseUrl === selectedUrl)
  const client = useMemo(() => selectedServer ? createClient(selectedServer) : undefined, [selectedServer])
  const adminQuery = useQuery({
    queryKey: ["desktop-admin", client?.cacheScope, "state"],
    queryFn: () => client!.adminState(),
    enabled: Boolean(client),
    refetchInterval: 30_000,
  })

  return (
    <main className="flex h-svh flex-col overflow-hidden bg-background text-foreground">
      <header className="flex h-14 shrink-0 items-center gap-3 border-b border-border px-4">
        <Button size="icon-sm" variant="ghost" aria-label="Back to workspace" onClick={() => navigate("/")}><ArrowLeft /></Button>
        <div className="min-w-0 flex-1">
          <h1 className="truncate text-sm font-semibold">Server administration</h1>
          <p className="truncate text-xs text-muted-foreground">Manage every Pi server from one place</p>
        </div>
        {selectedServer && activeServer?.baseUrl !== selectedServer.baseUrl && (
          <Button size="sm" variant="outline" onClick={() => setConnection(selectedServer)}>Use in workspace</Button>
        )}
        <Button size="icon-sm" variant="ghost" aria-label="Refresh server" title="Refresh" disabled={!client || adminQuery.isFetching} onClick={() => void adminQuery.refetch()}>
          <RefreshCw className={adminQuery.isFetching ? "animate-spin" : undefined} />
        </Button>
      </header>
      <div className="grid min-h-0 flex-1 grid-cols-[220px_minmax(0,1fr)] max-md:grid-cols-1">
        <aside className="border-r border-border bg-card/30 p-3 max-md:border-r-0 max-md:border-b">
          <div className="mb-2 flex items-center justify-between px-2">
            <p className="text-xs font-medium text-muted-foreground">Configured servers</p>
            <Badge variant="secondary">{servers.length}</Badge>
          </div>
          <div className="flex flex-col gap-1 max-md:flex-row max-md:overflow-x-auto">
            {servers.map((server) => {
              const selected = server.baseUrl === selectedUrl
              const active = server.baseUrl === activeServer?.baseUrl
              return (
                <button
                  key={server.baseUrl}
                  type="button"
                  className={`flex min-w-0 items-center gap-2 rounded-lg border px-2.5 py-2 text-left transition-colors ${selected ? "border-primary/40 bg-primary/8" : "border-transparent hover:bg-muted/60"}`}
                  onClick={() => setRequestedUrl(server.baseUrl)}
                >
                  <span className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted text-muted-foreground"><Server className="size-4" /></span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{server.name || server.baseUrl}</span>
                    <span className="block truncate text-xs text-muted-foreground">{isLocalServer(server) ? "Local" : "Remote"}</span>
                  </span>
                  {active && <span className="size-1.5 shrink-0 rounded-full bg-primary" title="Active workspace server" />}
                </button>
              )
            })}
          </div>
        </aside>
        <ScrollArea className="min-h-0">
          <div className="mx-auto flex w-full max-w-6xl flex-col gap-4 p-5 max-sm:p-3">
            {!selectedServer ? (
              <Card><CardHeader><CardTitle>No configured servers</CardTitle><CardDescription>Add a server from the workspace, then return here to administer it.</CardDescription></CardHeader></Card>
            ) : adminQuery.isLoading ? (
              <AdminSkeleton />
            ) : adminQuery.error ? (
              <Alert variant="destructive"><AlertTriangle /><AlertTitle>Could not open administration</AlertTitle><AlertDescription>{errorMessage(adminQuery.error)}</AlertDescription></Alert>
            ) : adminQuery.data && client ? (
              <AdminServerContent key={client.cacheScope} server={selectedServer} client={client} state={adminQuery.data} onRefresh={() => void adminQuery.refetch()} />
            ) : null}
          </div>
        </ScrollArea>
      </div>
    </main>
  )
}

function AdminSkeleton() {
  return <div className="flex flex-col gap-4"><div className="grid grid-cols-4 gap-3 max-lg:grid-cols-2">{Array.from({ length: 4 }, (_, index) => <Skeleton key={index} className="h-28 rounded-xl" />)}</div><Skeleton className="h-96 rounded-xl" /></div>
}

function AdminServerContent({ server, client, state, onRefresh }: { server: ServerConnectionSettings; client: PiServerClient; state: AdminState; onRefresh: () => void }) {
  return (
    <>
      <div className="flex min-w-0 items-center gap-3">
        <div className="flex size-10 shrink-0 items-center justify-center rounded-xl border border-border bg-card"><ShieldCheck className="size-5" /></div>
        <div className="min-w-0 flex-1"><h2 className="truncate text-lg font-semibold">{server.name || server.baseUrl}</h2><p className="truncate text-xs text-muted-foreground">{server.baseUrl}</p></div>
        <Badge variant={state.restartRequired ? "outline" : "secondary"}>{state.restartRequired ? "Restart pending" : "Configuration active"}</Badge>
      </div>
      <Overview state={state} />
      {state.overview.warnings.length > 0 ? state.overview.warnings.map((warning) => <Alert key={warning}><AlertTriangle /><AlertTitle>Server warning</AlertTitle><AlertDescription>{warning}</AlertDescription></Alert>) : <Alert><Check /><AlertTitle>Server healthy</AlertTitle><AlertDescription>No current server warnings.</AlertDescription></Alert>}
      <Tabs defaultValue="settings" className="min-w-0">
        <TabsList className="w-full justify-start overflow-hidden">
          <TabsTrigger value="settings">Settings</TabsTrigger>
          <TabsTrigger value="workers">Workers</TabsTrigger>
          <TabsTrigger value="devices">Trusted devices</TabsTrigger>
        </TabsList>
        <TabsContent value="settings"><SettingsPanel client={client} state={state} onRefresh={onRefresh} /></TabsContent>
        <TabsContent value="workers"><WorkersPanel client={client} /></TabsContent>
        <TabsContent value="devices"><DevicesPanel client={client} state={state} /></TabsContent>
      </Tabs>
    </>
  )
}

function Overview({ state }: { state: AdminState }) {
  const overview = state.overview
  const cards = [
    { label: "Uptime", value: formatDuration(overview.uptimeSeconds), detail: `API ${overview.apiVersion}`, icon: Activity },
    { label: "Sessions", value: `${overview.sessions.active} / ${overview.sessions.max || "∞"}`, detail: `${overview.sessions.registered} registered`, icon: Users },
    { label: "Scheduler", value: `${overview.scheduler.active} / ${overview.scheduler.globalLimit || "∞"}`, detail: `${overview.scheduler.queued} queued`, icon: Gauge },
    { label: "Workers", value: String(overview.workers.total), detail: overview.workers.unhealthy ? `${overview.workers.unhealthy} need attention` : "All reporting online", icon: Cpu },
  ]
  return <div className="grid grid-cols-4 gap-3 max-lg:grid-cols-2 max-sm:grid-cols-1">{cards.map(({ label, value, detail, icon: Icon }) => <Card key={label}><CardHeader className="pb-2"><CardDescription className="flex items-center gap-2"><Icon className="size-3.5" />{label}</CardDescription><CardTitle className="text-2xl">{value}</CardTitle></CardHeader><CardContent><p className="text-xs text-muted-foreground">{detail}</p></CardContent></Card>)}</div>
}

function SettingsPanel({ client, state, onRefresh }: { client: PiServerClient; state: AdminState; onRefresh: () => void }) {
  const [draft, setDraft] = useState<AdminSettings>(state.settings)
  const [saving, setSaving] = useState(false)
  const [notice, setNotice] = useState<string>()
  const [noticeError, setNoticeError] = useState(false)

  const dirty = JSON.stringify(draft) !== JSON.stringify(state.settings)

  async function save() {
    setSaving(true); setNotice(undefined); setNoticeError(false)
    try {
      const result = await client.updateAdminSettings(draft)
      setNotice(result.restartRequired ? "Saved. Restart this server to apply pending startup settings." : "Saved and applied to the running server.")
      onRefresh()
    } catch (error) {
      setNoticeError(true)
      setNotice(errorMessage(error))
    } finally {
      setSaving(false)
    }
  }

  return <div className="flex flex-col gap-4 pt-4">
    <div className="flex flex-wrap items-center justify-between gap-3"><div><h3 className="text-sm font-semibold">Configuration</h3><p className="text-xs text-muted-foreground">Persisted at {state.overview.configPath}</p></div><div className="flex gap-2"><Button size="sm" variant="outline" disabled={!dirty || saving} onClick={() => { setDraft(state.settings); setNotice(undefined); setNoticeError(false) }}><RotateCcw data-icon="inline-start" />Reset</Button><Button size="sm" disabled={!dirty || saving} onClick={() => void save()}>{saving ? <LoaderCircle className="animate-spin" data-icon="inline-start" /> : <Save data-icon="inline-start" />}Save settings</Button></div></div>
    {notice && <Alert variant={noticeError ? "destructive" : "default"}><AlertTitle>Configuration</AlertTitle><AlertDescription>{notice}</AlertDescription></Alert>}
    <div className="grid grid-cols-2 gap-4 max-lg:grid-cols-1">{settingGroups.map((group) => <Card key={group.title}><CardHeader><CardTitle className="text-sm">{group.title}</CardTitle><CardDescription>{group.description}</CardDescription></CardHeader><CardContent><FieldGroup className="grid grid-cols-2 gap-3 max-sm:grid-cols-1">{group.fields.map((field) => <SettingControl key={field.key} field={field} value={draft[field.key]} savedValue={state.settings[field.key]} effectiveValue={state.effectiveSettings[field.key]} source={state.sources[field.key]} live={state.runtimeFields.includes(field.key)} onChange={(value) => setDraft((current) => ({ ...current, [field.key]: value }))} />)}</FieldGroup></CardContent></Card>)}</div>
  </div>
}

function SettingControl({ field, value, savedValue, effectiveValue, source, live, onChange }: { field: SettingField; value: AdminSettings[keyof AdminSettings]; savedValue: AdminSettings[keyof AdminSettings]; effectiveValue: AdminSettings[keyof AdminSettings]; source?: string; live: boolean; onChange: (value: AdminSettings[keyof AdminSettings]) => void }) {
  const pending = !live && JSON.stringify(savedValue) !== JSON.stringify(effectiveValue)
  if (field.kind === "boolean") {
    return <Field orientation="horizontal" className="col-span-full rounded-lg border border-border p-3"><Checkbox id={field.key} checked={Boolean(value)} onCheckedChange={(checked) => onChange(checked === true)} /><div className="min-w-0"><FieldLabel htmlFor={field.key}>{field.label}</FieldLabel><FieldDescription>{live ? "Applies live" : pending ? "Restart pending" : "Requires restart"} · {source || "default"}</FieldDescription></div></Field>
  }
  const stringValue = Array.isArray(value) ? value.join(", ") : String(value ?? "")
  return <Field className={field.kind === "list" ? "col-span-full" : undefined}><div className="flex items-center justify-between gap-2"><FieldLabel htmlFor={field.key}>{field.label}</FieldLabel><Badge variant={pending ? "outline" : "secondary"}>{live ? "Live" : pending ? "Pending" : "Restart"}</Badge></div><Input id={field.key} type={field.kind === "number" ? "number" : "text"} min={field.kind === "number" ? 0 : undefined} value={stringValue} onChange={(event) => { const next = field.kind === "number" ? Number(event.target.value) : field.kind === "list" ? event.target.value.split(",").map((item) => item.trim()).filter(Boolean) : event.target.value; onChange(next) }} /><FieldDescription>{field.hint ? `${field.hint} · ` : ""}Source: {source || "default"}</FieldDescription></Field>
}

function WorkersPanel({ client }: { client: PiServerClient }) {
  const queryClient = useQueryClient()
  const queryKey = ["desktop-admin", client.cacheScope, "workers"]
  const workersQuery = useQuery({ queryKey, queryFn: () => client.listWorkers().then((result) => result.workers), refetchInterval: 30_000 })
  const [editing, setEditing] = useState<string>()
  const [id, setId] = useState("")
  const [url, setUrl] = useState("")
  const [token, setToken] = useState("")
  const [tags, setTags] = useState("")
  const [busy, setBusy] = useState<string>()
  const [notice, setNotice] = useState<string>()
  const [deleteWorker, setDeleteWorker] = useState<ApiWorker>()

  function reset() { setEditing(undefined); setId(""); setUrl(""); setToken(""); setTags("") }
  async function refresh() { await queryClient.invalidateQueries({ queryKey }) }
  async function save() {
    setBusy("save"); setNotice(undefined)
    const worker = { id: id.trim(), url: url.trim(), token: token.trim() || undefined, tags: tags.split(",").map((item) => item.trim()).filter(Boolean) }
    try { if (editing) await client.updateWorker(editing, worker); else await client.addWorker(worker); reset(); await refresh() } catch (error) { setNotice(errorMessage(error)) } finally { setBusy(undefined) }
  }
  async function probe(worker: ApiWorker) { setBusy(`probe:${worker.id}`); try { await client.probeWorkerHealth(worker.id); await refresh() } catch (error) { setNotice(errorMessage(error)) } finally { setBusy(undefined) } }
  async function remove() { if (!deleteWorker) return; setBusy(`delete:${deleteWorker.id}`); try { await client.deleteWorker(deleteWorker.id, true); setDeleteWorker(undefined); await refresh() } catch (error) { setNotice(errorMessage(error)) } finally { setBusy(undefined) } }

  return <div className="grid grid-cols-[minmax(0,1.4fr)_minmax(280px,.6fr)] gap-4 pt-4 max-lg:grid-cols-1">
    <Card><CardHeader><CardTitle className="text-sm">Registered workers</CardTitle><CardDescription>Probe remote workers or update where this coordinator reaches them.</CardDescription></CardHeader><CardContent className="flex flex-col gap-2">{workersQuery.isLoading ? <Skeleton className="h-24" /> : workersQuery.data?.map((worker) => <div key={worker.id} className="flex items-center gap-3 rounded-lg border border-border p-3"><span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted"><Cpu className="size-4" /></span><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><p className="truncate text-sm font-medium">{worker.id}</p><Badge variant="secondary">{worker.id === "local" ? "Local" : worker.status || "Unknown"}</Badge></div><p className="truncate text-xs text-muted-foreground">{worker.url}{worker.tags?.length ? ` · ${worker.tags.join(", ")}` : ""}</p></div>{worker.id !== "local" && <><Button size="icon-sm" variant="ghost" aria-label={`Probe ${worker.id}`} disabled={busy === `probe:${worker.id}`} onClick={() => void probe(worker)}>{busy === `probe:${worker.id}` ? <LoaderCircle className="animate-spin" /> : <Activity />}</Button><Button size="icon-sm" variant="ghost" aria-label={`Edit ${worker.id}`} onClick={() => { setEditing(worker.id); setId(worker.id); setUrl(worker.url); setToken(""); setTags(worker.tags?.join(", ") || "") }}><Pencil /></Button><Button size="icon-sm" variant="ghost" aria-label={`Delete ${worker.id}`} onClick={() => setDeleteWorker(worker)}><Trash2 /></Button></>}</div>)}</CardContent></Card>
    <Card><CardHeader><CardTitle className="text-sm">{editing ? `Edit ${editing}` : "Add remote worker"}</CardTitle><CardDescription>Use HTTPS when the worker is outside this device.</CardDescription></CardHeader><CardContent><FieldGroup><Field><FieldLabel htmlFor="worker-id">Worker ID</FieldLabel><Input id="worker-id" value={id} disabled={Boolean(editing)} onChange={(event) => setId(event.target.value)} /></Field><Field><FieldLabel htmlFor="worker-url">Worker URL</FieldLabel><Input id="worker-url" value={url} placeholder="https://worker.example.com:3142" onChange={(event) => setUrl(event.target.value)} /></Field><Field><FieldLabel htmlFor="worker-token">Worker token</FieldLabel><Input id="worker-token" type="password" value={token} placeholder={editing ? "Leave blank to keep current" : "Optional"} onChange={(event) => setToken(event.target.value)} /></Field><Field><FieldLabel htmlFor="worker-tags">Tags</FieldLabel><Input id="worker-tags" value={tags} placeholder="gpu, office" onChange={(event) => setTags(event.target.value)} /></Field>{notice && <FieldDescription className="text-destructive">{notice}</FieldDescription>}<div className="flex justify-end gap-2">{editing && <Button variant="ghost" onClick={reset}><X data-icon="inline-start" />Cancel</Button>}<Button disabled={!id.trim() || !url.trim() || busy === "save"} onClick={() => void save()}>{busy === "save" ? <LoaderCircle className="animate-spin" data-icon="inline-start" /> : editing ? <Check data-icon="inline-start" /> : <Plus data-icon="inline-start" />}{editing ? "Save worker" : "Add worker"}</Button></div></FieldGroup></CardContent></Card>
    <AlertDialog open={Boolean(deleteWorker)} onOpenChange={(open) => !open && setDeleteWorker(undefined)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>Delete worker?</AlertDialogTitle><AlertDialogDescription>This removes {deleteWorker?.id} and any remote-session mappings owned by it. Running work may be interrupted.</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction disabled={busy?.startsWith("delete:")} onClick={() => void remove()}>Delete worker</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>
}

function DevicesPanel({ client, state }: { client: PiServerClient; state: AdminState }) {
  const queryClient = useQueryClient()
  const queryKey = ["desktop-admin", client.cacheScope, "devices"]
  const devicesQuery = useQuery({ queryKey, queryFn: () => client.listDevices().then((result) => result.devices) })
  const [name, setName] = useState("")
  const [pairOpen, setPairOpen] = useState(false)
  const [created, setCreated] = useState<CreatedTrustedDevice>()
  const [busy, setBusy] = useState<string>()
  const [notice, setNotice] = useState<string>()
  const [removeDevice, setRemoveDevice] = useState<{ device: TrustedDevice; purge: boolean }>()
  async function refresh() { await queryClient.invalidateQueries({ queryKey }) }
  async function create() { setBusy("create"); setNotice(undefined); try { const device = await client.createDevice(name.trim()); setCreated(device); setName(""); await refresh() } catch (error) { setNotice(errorMessage(error)) } finally { setBusy(undefined) } }
  async function remove() { if (!removeDevice) return; setBusy("remove"); try { if (removeDevice.purge) await client.purgeDevice(removeDevice.device.id); else await client.revokeDevice(removeDevice.device.id); setRemoveDevice(undefined); await refresh() } catch (error) { setNotice(errorMessage(error)) } finally { setBusy(undefined) } }
  const pairingPayload = created ? JSON.stringify({ baseUrl: client.baseUrl, token: created.token }) : ""
  return <div className="flex flex-col gap-4 pt-4"><div className="flex items-center justify-between gap-3"><div><h3 className="text-sm font-semibold">Trusted devices</h3><p className="text-xs text-muted-foreground">Pair Companion devices and revoke credentials that should no longer connect.</p></div><Button size="sm" onClick={() => { setCreated(undefined); setPairOpen(true) }}><Plus data-icon="inline-start" />Pair device</Button></div>{notice && <Alert><AlertTitle>Device management</AlertTitle><AlertDescription>{notice}</AlertDescription></Alert>}<Card><CardContent className="flex flex-col gap-2 pt-6">{devicesQuery.isLoading ? <Skeleton className="h-24" /> : devicesQuery.error ? <p className="text-sm text-destructive">{errorMessage(devicesQuery.error)}</p> : devicesQuery.data?.length ? devicesQuery.data.map((device) => <div key={device.id} className="flex items-center gap-3 rounded-lg border border-border p-3"><span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-muted"><ShieldCheck className="size-4" /></span><div className="min-w-0 flex-1"><div className="flex items-center gap-2"><p className="truncate text-sm font-medium">{device.name || "Trusted device"}</p><Badge variant={device.revokedAt ? "outline" : "secondary"}>{device.revokedAt ? "Revoked" : "Active"}</Badge></div><p className="truncate text-xs text-muted-foreground">Created {formatDate(device.createdAt)} · Last seen {formatDate(device.lastSeen)}</p></div>{device.revokedAt ? <Button size="sm" variant="ghost" onClick={() => setRemoveDevice({ device, purge: true })}><Trash2 data-icon="inline-start" />Delete</Button> : <Button size="sm" variant="outline" onClick={() => setRemoveDevice({ device, purge: false })}>Revoke</Button>}</div>) : <p className="rounded-lg border border-dashed border-border p-6 text-center text-sm text-muted-foreground">No trusted devices are paired with this server.</p>}</CardContent></Card>
    <Dialog open={pairOpen} onOpenChange={(open) => { setPairOpen(open); if (!open) setCreated(undefined) }}><DialogContent><DialogHeader><DialogTitle>Pair trusted device</DialogTitle><DialogDescription>{created ? "Copy the connection payload now. The token cannot be shown again after this dialog closes." : "Create a one-time credential for a Companion device."}</DialogDescription></DialogHeader>{created ? <FieldGroup><Alert><ShieldCheck /><AlertTitle>Credential created</AlertTitle><AlertDescription>Store it now. Pi Server saves only a fingerprint of this token.</AlertDescription></Alert><Field><FieldLabel htmlFor="pairing-payload">Connection payload</FieldLabel><Input id="pairing-payload" readOnly value={pairingPayload} /><FieldDescription>Uses the same server URL that this Desktop app can reach. Other advertised addresses: {state.pairingEndpoints.length || "none"}.</FieldDescription></Field></FieldGroup> : <FieldGroup><Field><FieldLabel htmlFor="device-name">Device name</FieldLabel><Input id="device-name" value={name} placeholder="My phone" onChange={(event) => setName(event.target.value)} /></Field>{notice && <FieldDescription className="text-destructive">{notice}</FieldDescription>}</FieldGroup>}<DialogFooter>{created ? <Button onClick={() => void navigator.clipboard.writeText(pairingPayload)}><Clipboard data-icon="inline-start" />Copy payload</Button> : <Button disabled={!name.trim() || busy === "create"} onClick={() => void create()}>{busy === "create" ? <LoaderCircle className="animate-spin" data-icon="inline-start" /> : <Plus data-icon="inline-start" />}Create credential</Button>}</DialogFooter></DialogContent></Dialog>
    <AlertDialog open={Boolean(removeDevice)} onOpenChange={(open) => !open && setRemoveDevice(undefined)}><AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{removeDevice?.purge ? "Delete device record?" : "Revoke device?"}</AlertDialogTitle><AlertDialogDescription>{removeDevice?.purge ? `Permanently delete ${removeDevice.device.name || "this device"}. This cannot be undone.` : `${removeDevice?.device.name || "This device"} will immediately lose access to the server.`}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>Cancel</AlertDialogCancel><AlertDialogAction disabled={busy === "remove"} onClick={() => void remove()}>{removeDevice?.purge ? "Delete" : "Revoke"}</AlertDialogAction></AlertDialogFooter></AlertDialogContent></AlertDialog>
  </div>
}
