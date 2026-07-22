import type { components } from "@/api/types"

export type ApiCapabilities = components["schemas"]["Capabilities"]
export type ApiWorker = components["schemas"]["Worker"]
export type ApiSession = components["schemas"]["SessionSummary"]
export type ApiSessionSpec = components["schemas"]["SessionSpec"]
export type RpcCommand = components["schemas"]["RPCCommand"]
export type RpcResponse = components["schemas"]["RPCResponse"]
export type PromptRequest = components["schemas"]["PromptRequest"]
export type SessionMetadataUpdate =
  components["schemas"]["SessionMetadataUpdate"]
export type WSTicket = components["schemas"]["WSTicketResponse"]

export interface PiServerClientOptions {
  /** HTTP(S) base URL. Defaults to VITE_PI_SERVER_URL, then localhost:3141. */
  baseUrl?: string
  /** Optional PI_SERVER_AUTH_TOKEN. Keep this out of browser storage. */
  token?: string
  fetch?: typeof globalThis.fetch
}

/** The full server request accepts metadata even though older OpenAPI documents omit it. */
export interface CreateSessionRequest {
  id?: string
  cwd: string
  args?: string[]
  env?: Record<string, string>
  sessionPath?: string
  start?: boolean
  restart?: boolean
  project?: string
  title?: string
  taskType?: string
  owner?: string
  labels?: string[]
  metadata?: Record<string, string>
}

export interface SessionListResponse {
  sessions: ApiSession[]
  partialFailures?: ApiPartialFailure[]
}

export interface ApiPartialFailure {
  workerId?: string
  sessionId?: string
  error: string
  code?: string
}

export interface MachineSession {
  id: string
  path: string
  cwd: string
  createdAt: string
  updatedAt: string
  size: number
}

export interface GlobalSession {
  id: string
  originId: string
  workerId: string
  session: ApiSession
  reachable: boolean
}

export interface EventRecord {
  id: number
  timestamp: string
  event: Record<string, unknown>
}

export interface GitResponse {
  cwd: string
  command: string
  output: string
}

export interface FileInfo {
  name: string
  path: string
  isDir: boolean
  size: number
}

export interface DirectoryEntry {
  name: string
  path: string
}

export interface DirectoryListResponse {
  path?: string
  parent?: string
  directories?: DirectoryEntry[]
  roots?: DirectoryEntry[]
}

export interface FileContent {
  path: string
  mimeType?: string
  encoding?: string
  truncated?: boolean
  binary?: boolean
  content?: string
  size?: number
}

export interface HealthResponse {
  ok: boolean
  apiVersion: string
  capabilities: Record<string, boolean>
  sessions: string[]
  capacity: { activeSessions: number; maxSessions: number }
}

export class PiServerApiError extends Error {
  readonly status: number
  readonly code?: string
  readonly requestId?: string

  constructor(
    status: number,
    message: string,
    code?: string,
    requestId?: string
  ) {
    super(message)
    this.name = "PiServerApiError"
    this.status = status
    this.code = code
    this.requestId = requestId
  }
}

/**
 * Browser-safe typed client for pi-server. WebSocket connections use the
 * server's short-lived tickets because browsers cannot set Authorization on a
 * WebSocket upgrade request.
 */
export class PiServerClient {
  readonly baseUrl: string
  private readonly token?: string
  private readonly fetchFn: typeof globalThis.fetch

  constructor(options: PiServerClientOptions = {}) {
    this.baseUrl = normalizeBaseUrl(
      options.baseUrl ??
        import.meta.env.VITE_PI_SERVER_URL ??
        "http://127.0.0.1:3141"
    )
    this.token = options.token ?? import.meta.env.VITE_PI_SERVER_AUTH_TOKEN
    this.fetchFn = options.fetch ?? globalThis.fetch.bind(globalThis)
  }

  health(): Promise<HealthResponse> {
    return this.request("/healthz")
  }

  capabilities(): Promise<ApiCapabilities> {
    return this.request("/v1/capabilities")
  }

  updateCapacity(maxSessions: number): Promise<{ activeSessions: number; maxSessions: number }> {
    return this.request("/v1/capacity", { method: "PATCH", body: JSON.stringify({ maxSessions }) })
  }

  listSessions(): Promise<SessionListResponse> {
    return this.request("/v1/sessions?scope=all&include=state")
  }

  listGlobalSessions(): Promise<{ sessions: GlobalSession[]; partialFailures?: ApiPartialFailure[] }> {
    return this.request("/v1/global-sessions")
  }

  listMachineSessions(): Promise<{ root: string; sessions: MachineSession[] }> {
    return this.request("/v1/machine-sessions")
  }

  openMachineSession(machineSessionId: string): Promise<{ id: string; cwd: string; ws: string }> {
    return this.request(`/v1/machine-sessions/${encodeURIComponent(machineSessionId)}/open`, { method: "POST" })
  }

  attachGlobalSession(globalId: string): Promise<{ id: string; attached: boolean }> {
    return this.request(`/v1/global-sessions/${encodeURIComponent(globalId)}/attach`, { method: "POST" })
  }

  getSession(id: string): Promise<ApiSession> {
    return this.request(`/v1/sessions/${encodeURIComponent(id)}/summary`)
  }

  createSession(
    input: CreateSessionRequest
  ): Promise<{ id: string; cwd: string; args: string[]; ws: string }> {
    return this.request("/v1/sessions", { method: "POST", body: input })
  }

  createWorkerSession(
    workerId: string,
    input: CreateSessionRequest
  ): Promise<{ id: string; cwd: string; args: string[]; ws: string }> {
    return this.request(`/v1/workers/${encodeURIComponent(workerId)}/sessions`, {
      method: "POST",
      body: input,
    })
  }

  getWorkerHealth(id: string): Promise<HealthResponse> {
    return this.request(`/v1/workers/${encodeURIComponent(id)}/health`)
  }

  deleteSession(id: string): Promise<{ deleted: string }> {
    return this.request(`/v1/sessions/${encodeURIComponent(id)}`, {
      method: "DELETE",
    })
  }

  updateSessionMetadata(
    id: string,
    update: SessionMetadataUpdate
  ): Promise<ApiSessionSpec> {
    return this.request(`/v1/sessions/${encodeURIComponent(id)}/metadata`, {
      method: "POST",
      body: update,
    })
  }

  getSessionMessages(id: string, offset = 0, limit = 75): Promise<RpcResponse> {
    const query = new URLSearchParams({ offset: String(offset), limit: String(limit) })
    return this.request(`/v1/sessions/${encodeURIComponent(id)}/messages?${query}`)
  }

  getSessionData(
    id: string,
    resource:
      | "state"
      | "messages"
      | "stats"
      | "models"
      | "commands"
      | "entries"
      | "tree"
      | "last-assistant-text"
      | "fork-messages"
      | "daemon-status"
  ): Promise<RpcResponse> {
    return this.request(`/v1/sessions/${encodeURIComponent(id)}/${resource}`)
  }

  getSessionEvents(
    id: string,
    since?: number,
    limit = 200
  ): Promise<{ events: EventRecord[]; since: number }> {
    const query = new URLSearchParams({ limit: String(limit) })
    if (since !== undefined) query.set("since", String(since))
    return this.request(
      `/v1/sessions/${encodeURIComponent(id)}/events?${query}`
    )
  }

  getSessionGit(
    id: string,
    resource: "status" | "diff" | "log" | "head"
  ): Promise<GitResponse> {
    return this.request(
      `/v1/sessions/${encodeURIComponent(id)}/git/${resource}`
    )
  }

  listDirectories(path?: string): Promise<DirectoryListResponse> {
    const query = path ? `?${new URLSearchParams({ path })}` : ""
    return this.request(`/v1/directories${query}`)
  }

  getFileTree(
    cwd: string
  ): Promise<{ cwd: string; limit: number; files: FileInfo[] }> {
    const query = new URLSearchParams({ cwd })
    return this.request(`/v1/files/tree?${query}`)
  }

  getSessionFileContent(sessionId: string, path: string): Promise<FileContent> {
    const query = new URLSearchParams({ path })
    return this.request(`/v1/sessions/${encodeURIComponent(sessionId)}/files/content?${query}`)
  }

  prompt(id: string, request: PromptRequest): Promise<RpcResponse> {
    return this.sessionPost(id, "prompt", request)
  }

  steer(id: string, request: PromptRequest): Promise<RpcResponse> {
    return this.sessionPost(id, "steer", request)
  }

  followUp(id: string, request: PromptRequest): Promise<RpcResponse> {
    return this.sessionPost(id, "follow-up", request)
  }

  abort(id: string): Promise<RpcResponse> {
    return this.sessionPost(id, "abort", {})
  }

  compact(id: string): Promise<RpcResponse> {
    return this.sessionPost(id, "compact", {})
  }

  /** Covers every current and future Pi RPC command without waiting for a UI wrapper. */
  command(id: string, command: RpcCommand): Promise<RpcResponse> {
    return this.sessionPost(id, "command", command)
  }

  send(id: string, command: RpcCommand): Promise<{ accepted: boolean }> {
    return this.sessionPost(id, "send", command)
  }

  sessionPost<T = RpcResponse>(
    id: string,
    action: string,
    body: unknown
  ): Promise<T> {
    return this.request(`/v1/sessions/${encodeURIComponent(id)}/${action}`, {
      method: "POST",
      body,
    })
  }

  listWorkers(): Promise<{ workers: ApiWorker[] }> {
    return this.request("/v1/workers")
  }

  addWorker(worker: {
    id: string
    url: string
    token?: string
    tags?: string[]
  }): Promise<ApiWorker> {
    return this.request("/v1/workers", { method: "POST", body: worker })
  }

  updateWorker(
    id: string,
    worker: { id: string; url: string; token?: string; tags?: string[] }
  ): Promise<ApiWorker> {
    return this.request(`/v1/workers/${encodeURIComponent(id)}`, {
      method: "PUT",
      body: worker,
    })
  }

  deleteWorker(id: string, force = false): Promise<RpcResponse> {
    const suffix = force ? "?force=true" : ""
    return this.request(`/v1/workers/${encodeURIComponent(id)}${suffix}`, {
      method: "DELETE",
    })
  }

  issueWebSocketTicket(sessionId: string): Promise<WSTicket> {
    return this.request("/v1/ws-tickets", {
      method: "POST",
      body: { sessionId },
    })
  }

  webSocketUrl(path: string): string {
    const url = new URL(path, `${this.baseUrl}/`)
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:"
    return url.toString()
  }

  private async request<T>(
    path: string,
    init: { method?: string; body?: unknown } = {}
  ): Promise<T> {
    const headers = new Headers({ Accept: "application/json" })
    if (init.body !== undefined) headers.set("Content-Type", "application/json")
    if (this.token) headers.set("Authorization", `Bearer ${this.token}`)

    let response: Response
    try {
      response = await this.fetchFn(new URL(path, `${this.baseUrl}/`), {
        method: init.method ?? "GET",
        headers,
        body: init.body === undefined ? undefined : JSON.stringify(init.body),
      })
    } catch (error) {
      throw new PiServerApiError(
        0,
        error instanceof Error ? error.message : "Unable to reach pi-server"
      )
    }

    const payload: unknown = await response.json().catch(() => undefined)
    if (!response.ok) {
      const error = isApiError(payload) ? payload : undefined
      throw new PiServerApiError(
        response.status,
        error?.error ?? response.statusText,
        error?.code,
        error?.requestId
      )
    }
    // Guard against proxy returning HTML error pages or server returning
    // unexpected shapes after an upgrade.
    if (payload === null || typeof payload !== "object") {
      throw new PiServerApiError(
        response.status,
        "Unexpected response format from pi-server"
      )
    }
    return payload as T
  }
}

function normalizeBaseUrl(url: string): string {
  return url.replace(/\/+$/, "")
}

function isApiError(
  value: unknown
): value is { error: string; code?: string; requestId?: string } {
  return (
    typeof value === "object" &&
    value !== null &&
    "error" in value &&
    typeof value.error === "string"
  )
}
