import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { existsSync, readFileSync, writeFileSync, mkdirSync } from "node:fs";
import { join, dirname } from "node:path";

type BridgeConfig = { relayUrl?: string; relayToken?: string };
const configPath = join(process.env.USERPROFILE ?? process.env.HOME ?? "", ".pi", "agent", "bridge-config.json");
function loadConfig(): BridgeConfig {
  try { return existsSync(configPath) ? JSON.parse(readFileSync(configPath, "utf8")) as BridgeConfig : {}; } catch { return {}; }
}
function saveConfig(config: BridgeConfig): void {
  try {
    mkdirSync(dirname(configPath), { recursive: true });
    writeFileSync(configPath, JSON.stringify(config, null, 2), "utf8");
  } catch (err) {
    throw new Error(`Failed to save config: ${err instanceof Error ? err.message : "unknown"}`);
  }
}
const eventQueueLimit = 200;
const requestTimeoutMs = 15_000;

function sessionId(file?: string): string {
  const match = file?.match(/_([0-9a-f-]{16,})\.jsonl$/i);
  if (match) return match[1];
  // Fall back to a stable per-file id: a Date.now() suffix re-registers the
  // same TUI as a new session on every restart, orphaning the old entry.
  const stem = file?.replace(/.*[\\/]/, "").replace(/\.jsonl$/i, "").replace(/[^A-Za-z0-9_-]/g, "-");
  return stem ? `external-${stem}` : `external-${process.pid}`;
}

/**
 * Reliable outbound bridge for an interactive Pi TUI. It tolerates pi-server
 * restarts: registration and queued events retry, while remote commands remain
 * on the server until this extension acknowledges them after injection.
 */
export default function externalSessionBridge(pi: ExtensionAPI) {
  let id = "";
  // Stable for this TUI process: re-registering preserves its lease, while a
  // second TUI attached to the same session file rotates the old lease.
  const bridgeId = crypto.randomUUID();
  let lease = "";
  let cwd = process.cwd();
  let sessionPath = "";
  let title = "";
  let stopped = false;
  let baseUrl = "";
  let token: string | undefined;
  const headers = () => ({ "content-type": "application/json", ...(token ? { authorization: `Bearer ${token}` } : {}) });
  let registered = false;
  let pollRunning = false;
  let flushRunning = false;
  let ui: { setStatus: (key: string, text?: string) => void; notify: (message: string, level?: "info" | "warning" | "error") => void } | undefined;
  let sessionCtx: { model?: unknown; abort: () => void } | undefined;
  let relaySocket: WebSocket | undefined;
  let relayReconnectTimer: ReturnType<typeof setTimeout> | undefined;
  let abortCurrent: (() => void) | undefined;
  /** Generation counter bumped on every new socket or registration so stale
   *  callbacks (onopen/onclose/onmessage) cannot mutate live state or schedule
   *  duplicate reconnects after we have already moved on. */
  let socketGen = 0;
  /** In-flight registration promise so concurrent callers serialize. */
  let registerInFlight: Promise<boolean> | undefined;
  /** Exponential backoff state for reconnect attempts (ms). */
  let backoffMs = 1_000;
  const backoffCeiling = 30_000;
  const pendingEvents: unknown[] = [];
  const handledCommands = new Set<string>();

  /** Reload URL/token configuration so /bridge-reconnect can apply changes
   * without restarting Pi. The config file is canonical for the relay URL;
   * the token still prefers the process environment. */
  const refreshConfig = (): boolean => {
    const config = loadConfig();
    const nextBaseUrl = (config.relayUrl ?? process.env.PI_EXTERNAL_RELAY_URL)?.replace(/\/$/, "") ?? "";
    const nextToken = process.env.PI_EXTERNAL_RELAY_TOKEN ?? config.relayToken;
    const changed = nextBaseUrl !== baseUrl || nextToken !== token;
    baseUrl = nextBaseUrl;
    token = nextToken;
    return changed;
  };

  // ── helpers ────────────────────────────────────────────────────────────

  /** Wrap fetch with AbortController so requests never hang forever. */
  const fetchWithTimeout = async (input: RequestInfo | URL, init?: RequestInit): Promise<Response> => {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), requestTimeoutMs);
    try {
      return await fetch(input, { ...init, signal: controller.signal });
    } finally {
      clearTimeout(timer);
    }
  };

  const request = async (path: string, method = "GET", body?: unknown) => {
    if (!baseUrl) throw new Error("PI_EXTERNAL_RELAY_URL is not configured");
    const response = await fetchWithTimeout(`${baseUrl}${path}`, {
      method,
      headers: method === "GET" ? (token ? { authorization: `Bearer ${token}` } : {}) : headers(),
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!response.ok) {
      let detail = "";
      try { detail = await response.text(); } catch { /* ignore */ }
      throw new Error(`relay HTTP ${response.status}: ${detail.slice(0, 200)}`);
    }
    return response;
  };

  /** Compute the next exponential backoff with jitter, capped. */
  const nextBackoff = (): number => {
    const jitter = backoffMs * (0.5 + Math.random() * 0.5); // 50-100% of current
    backoffMs = Math.min(backoffMs * 2, backoffCeiling);
    return jitter;
  };

  /** Reset backoff after a healthy registration or WS open. */
  const resetBackoff = () => { backoffMs = 1_000; };

  /** Safe one-line status string for diagnostics (no tokens). */
  const bridgeSummary = (): string => {
    const wsState = relaySocket ? ["CONNECTING", "OPEN", "CLOSING", "CLOSED"][relaySocket.readyState] ?? "UNKNOWN" : "none";
    let host = "n/a";
    if (baseUrl) {
      try { host = new URL(baseUrl).host; } catch { host = "invalid-url"; }
    }
    return `host=${host} session=${id || "(none)"} ws=${wsState} queued=${pendingEvents.length}`;
  };

  // ── registration (serialized) ─────────────────────────────────────────

  const registerInner = async (): Promise<boolean> => {
    if (!id) return false;
    try {
      const response = await request("/v1/external-sessions/register", "POST", { id, cwd, title, sessionPath, bridgeId });
      const data = await response.json() as { lease?: string };
      if (!data.lease) throw new Error("relay did not issue a command lease");
      lease = data.lease;
      registered = true;
      resetBackoff();
      ui?.setStatus("external-session-bridge", "Bridge: connected");
      return true;
    } catch (err) {
      registered = false;
      ui?.setStatus("external-session-bridge", `Bridge: reconnecting (${err instanceof Error ? err.message : "unknown"})`);
      return false;
    }
  };

  /**
   * Serialized register: concurrent callers (poll, flush, event handlers)
   * share the same in-flight promise so we never fire two parallel
   * registrations that race over the lease.
   */
  const register = async (): Promise<boolean> => {
    if (registerInFlight) return registerInFlight;
    registerInFlight = registerInner().finally(() => { registerInFlight = undefined; });
    return registerInFlight;
  };

  // ── event transport ───────────────────────────────────────────────────

  const emit = (event: unknown) => {
    if (stopped) return;
    // Single ordered sender: only use the socket when no backlog exists and no
    // HTTP flush is in flight, otherwise newer events could overtake older ones.
    if (relaySocket?.readyState === WebSocket.OPEN && pendingEvents.length === 0 && !flushRunning) {
      try {
        relaySocket.send(JSON.stringify({ type: "event", event }));
      } catch {
        // Socket send failed (half-dead connection) — fall back to queue.
        pendingEvents.push(event);
        if (pendingEvents.length > eventQueueLimit) pendingEvents.shift();
        void flushEvents();
      }
      return;
    }
    pendingEvents.push(event);
    if (pendingEvents.length > eventQueueLimit) pendingEvents.shift();
    void flushEvents();
  };

  /** Send one event over HTTP, returning whether it succeeded. */
  const sendEventHttp = async (event: unknown): Promise<boolean> => {
    try {
      if (!registered && !(await register())) return false;
      await request(`/v1/external-sessions/${id}/events`, "POST", event);
      return true;
    } catch (err) {
      ui?.setStatus("external-session-bridge", `Bridge: event send failed (${err instanceof Error ? err.message : "unknown"})`);
      registered = false;
      return false;
    }
  };

  const flushEvents = async () => {
    if (flushRunning || !id || stopped) return;
    // If a healthy socket is open, prefer it — the onopen handler drains
    // the backlog, so we just wait for that.
    if (relaySocket?.readyState === WebSocket.OPEN) return;
    flushRunning = true;
    try {
      while (pendingEvents.length && !stopped) {
        // If a socket appeared while we were flushing, hand off to it.
        if (relaySocket?.readyState === WebSocket.OPEN) break;
        const ok = await sendEventHttp(pendingEvents[0]);
        if (ok) pendingEvents.shift();
        else break;
      }
    } finally {
      flushRunning = false;
    }
  };

  // ── command ack / delivery ────────────────────────────────────────────

  const acknowledge = async (commandId: string) => {
    if (relaySocket?.readyState === WebSocket.OPEN) {
      relaySocket.send(JSON.stringify({ type: "ack", ids: [commandId] }));
      return;
    }
    try { await request(`/v1/external-sessions/${id}/ack`, "POST", { ids: [commandId], lease }); } catch { registered = false; /* server retries delivery after lease renewal */ }
  };

  const deliverCommand = async (command: { id: string; type: string; message?: string; delivery?: "steer" | "followUp" | "prompt"; provider?: string; modelId?: string; level?: string }) => {
    if (handledCommands.has(command.id)) { await acknowledge(command.id); return; }
    if (command.type === "abort") {
      abortCurrent?.();
      emit({ type: "bridge_receipt", commandId: command.id, status: "delivered" });
      ui?.notify("Remote stop requested", "warning");
      handledCommands.add(command.id);
      await acknowledge(command.id);
      return;
    }
    if (command.type === "set_model") {
      const provider = command.provider;
      const modelId = command.modelId;
      if (provider && modelId) {
        const ok = await pi.setModel({ provider, id: modelId });
        emit({ type: "bridge_receipt", commandId: command.id, status: ok ? "delivered" : "failed" });
        ui?.notify(ok ? `Model changed to ${provider}/${modelId}` : `Failed to set model ${provider}/${modelId}`, ok ? "info" : "error");
      }
      handledCommands.add(command.id);
      await acknowledge(command.id);
      return;
    }
    if (command.type === "set_thinking_level") {
      const level = command.level;
      if (level) {
        pi.setThinkingLevel(level as any);
        emit({ type: "bridge_receipt", commandId: command.id, status: "delivered" });
        ui?.notify(`Thinking level changed to ${level}`, "info");
      }
      handledCommands.add(command.id);
      await acknowledge(command.id);
      return;
    }
    if (command.type !== "prompt" || !command.message) return;
    const delivery = command.delivery === "steer" ? "steer" : "followUp";
    pi.sendUserMessage(command.message, { deliverAs: delivery });
    emit({ type: "bridge_receipt", commandId: command.id, status: "delivered" });
    ui?.notify(`Remote ${delivery === "steer" ? "steer" : "message"} received`, "info");
    handledCommands.add(command.id);
    if (handledCommands.size > 500) {
      const first = handledCommands.values().next().value;
      if (first !== undefined) handledCommands.delete(first);
    }
    await acknowledge(command.id);
  };

  // ── WebSocket ─────────────────────────────────────────────────────────

  const connectRelay = () => {
    if (stopped || !id || !baseUrl) return;
    // Guard: don't open a duplicate socket.
    if (relaySocket?.readyState === WebSocket.OPEN || relaySocket?.readyState === WebSocket.CONNECTING) return;
    // Require registration + lease before attempting WS so the handshake
    // never proceeds without a valid lease.
    if (!registered || !lease) { void register().then((ok) => { if (ok && !stopped) connectRelay(); }); return; }

    const gen = ++socketGen;

    // Authenticate via a WebSocket subprotocol instead of a URL query token so
    // the secret stays out of URLs, proxy logs, and process listings. Tokens
    // outside the subprotocol grammar fall back to the deprecated query param.
    const subprotocolSafe = token ? /^[A-Za-z0-9._~-]+$/.test(token) : false;
    if (token && !subprotocolSafe) {
      // Never put a bearer token in the WebSocket URL. The server deliberately
      // accepts relay credentials only through Sec-WebSocket-Protocol. HTTP
      // polling remains available as the safe fallback for unusual tokens.
      ui?.setStatus("external-session-bridge", "Bridge: connected (HTTP fallback; token is not WebSocket-safe)");
      return;
    }
    const relayQuery = new URLSearchParams({ lease });
    const wsUrl = baseUrl.replace(/^http/, "ws") + `/v1/external-sessions/relay/${encodeURIComponent(id)}?${relayQuery}`;
    try {
      const socket = subprotocolSafe ? new WebSocket(wsUrl, [`pi-relay.${token}`]) : new WebSocket(wsUrl);
      relaySocket = socket;

      socket.onopen = async () => {
        // Stale-guard: a newer socket already took over.
        if (gen !== socketGen) return;
        resetBackoff();
        ui?.setStatus("external-session-bridge", "Bridge: connected");
        // Re-emit current model/thinking state so the server refreshes after
        // any events that were dropped during the disconnect window.
        if (sessionCtx?.model) emit({ type: "model_select", model: sessionCtx.model });
        emit({ type: "thinking_level_select", level: pi.getThinkingLevel() });
        // Re-emit available models on reconnect.
        try {
          const scoped = sessionCtx?.getScopedModels?.();
          if (scoped && scoped.length > 0) {
            const models = scoped.map((sm: any) => sm.model ?? sm);
            emit({ type: "available_models", models });
          }
        } catch { /* getScopedModels may not be available */ }
        // Wait for any in-flight HTTP flush before draining the backlog over the
        // socket so both paths never send the same queue concurrently.
        while (flushRunning) await new Promise((resolve) => setTimeout(resolve, 50));
        while (pendingEvents.length && socket.readyState === WebSocket.OPEN && gen === socketGen) {
          socket.send(JSON.stringify({ type: "event", event: pendingEvents.shift() }));
        }
      };

      socket.onmessage = async (message) => {
        if (gen !== socketGen) return; // stale socket — ignore
        try {
          const envelope = JSON.parse(String(message.data)) as { type?: string; command?: { id: string; type: string; message?: string; delivery?: "steer" | "followUp" | "prompt"; provider?: string; modelId?: string; level?: string } };
          if (envelope.type === "command" && envelope.command) await deliverCommand(envelope.command);
        } catch (error) { ui?.notify(`Bridge command failed: ${error instanceof Error ? error.message : "unknown error"}`, "error"); }
      };

      socket.onclose = () => {
        if (gen !== socketGen) return; // stale socket — don't mutate state or reschedule
        if (relaySocket === socket) relaySocket = undefined;
        if (!stopped) {
          ui?.setStatus("external-session-bridge", `Bridge: reconnecting (ws closed)`);
          scheduleReconnect();
        }
      };

      socket.onerror = () => socket.close();
    } catch {
      scheduleReconnect();
    }
  };

  /** Schedule a reconnect with exponential backoff + jitter. */
  const scheduleReconnect = () => {
    if (relayReconnectTimer) clearTimeout(relayReconnectTimer);
    const delay = nextBackoff();
    relayReconnectTimer = setTimeout(() => { relayReconnectTimer = undefined; connectRelay(); }, delay);
  };

  // ── HTTP polling fallback ─────────────────────────────────────────────

  async function pollCommands() {
    if (pollRunning) return;
    pollRunning = true;
    while (!stopped && id) {
      if (relaySocket?.readyState === WebSocket.OPEN) {
        await new Promise((resolve) => setTimeout(resolve, 5_000));
        continue;
      }
      try {
        if (!registered && !(await register())) throw new Error("not registered");
        if (registered && !relaySocket) connectRelay();
        const response = await request(`/v1/external-sessions/${id}/commands?lease=${encodeURIComponent(lease)}`);
        const data = await response.json() as { commands?: Array<{ id: string; type: string; message?: string; delivery?: "steer" | "followUp" | "prompt"; provider?: string; modelId?: string; level?: string }> };
        for (const command of data.commands ?? []) await deliverCommand(command);
      } catch {
        registered = false;
      }
      await flushEvents();
      await new Promise((resolve) => setTimeout(resolve, registered ? 750 : 2_000));
    }
    pollRunning = false;
  }

  // ── reconnect ─────────────────────────────────────────────────────────

  const reconnectBridge = async () => {
    const configChanged = refreshConfig();
    stopped = false;
    registered = false;
    // Bump generation so any in-flight socket callbacks become no-ops.
    socketGen++;
    relaySocket?.close();
    relaySocket = undefined;
    if (configChanged) lease = "";
    if (relayReconnectTimer) clearTimeout(relayReconnectTimer);
    relayReconnectTimer = undefined;
    resetBackoff();
    const ok = await register();
    if (ok) { await flushEvents(); connectRelay(); }
  };

  // ── commands ──────────────────────────────────────────────────────────

  pi.registerCommand("bridge-status", {
    description: "Show external session bridge status",
    handler: async (_args, ctx) => {
      if (!baseUrl) {
        ctx.ui.notify("Bridge: disabled — set PI_EXTERNAL_RELAY_URL, then restart Pi.", "info");
        return;
      }
      const regState = registered ? "registered" : "unregistered";
      ctx.ui.notify(
        `Bridge: ${regState} · ${bridgeSummary()}`,
        registered ? "info" : "warning",
      );
    },
  });

  pi.registerCommand("bridge-reconnect", {
    description: "Reconnect this Pi session to pi-server",
    handler: async (_args, ctx) => {
      ctx.ui.setStatus("external-session-bridge", "Bridge: reconnecting");
      await reconnectBridge();
      ctx.ui.notify(`Bridge: ${registered ? "connected" : "unable to connect"} (${bridgeSummary()})`, registered ? "info" : "error");
    },
  });

  pi.registerCommand("bridge-disconnect", {
    description: "Disconnect this Pi session from pi-server until the next reconnect",
    handler: async (_args, ctx) => {
      stopped = true;
      socketGen++; // invalidate any outstanding socket callbacks
      relaySocket?.close();
      relaySocket = undefined;
      if (relayReconnectTimer) clearTimeout(relayReconnectTimer);
      relayReconnectTimer = undefined;
      ui?.setStatus("external-session-bridge", "Bridge: disconnected");
      ctx.ui.notify("Bridge disconnected. Use /bridge-reconnect to restore it.", "info");
    },
  });

  pi.registerCommand("bridge-register", {
    description: "Register a new bridge URL (saves to config and reconnects)",
    handler: async (args, ctx) => {
      let url = args?.trim();
      if (!url) {
        url = await ctx.ui.input("Enter bridge URL:", baseUrl || "");
        if (!url) {
          ctx.ui.notify("Bridge registration cancelled.", "info");
          return;
        }
      }
      // Basic URL validation
      try {
        new URL(url);
      } catch {
        ctx.ui.notify(`Invalid URL: ${url}`, "error");
        return;
      }
      // Save to config
      const config = loadConfig();
      config.relayUrl = url;
      saveConfig(config);
      ctx.ui.notify(`Bridge URL saved: ${url}`, "info");
      // Reconnect with new URL
      ctx.ui.setStatus("external-session-bridge", "Bridge: reconnecting");
      await reconnectBridge();
      ctx.ui.notify(`Bridge: ${registered ? "connected" : "unable to connect"} (${bridgeSummary()})`, registered ? "info" : "error");
    },
  });

  // ── Pi events ─────────────────────────────────────────────────────────

  pi.on("session_start", async (_event, ctx) => {
    ui = ctx.ui;
    sessionCtx = ctx;
    abortCurrent = () => ctx.abort();
    refreshConfig();
    if (!baseUrl) {
      ui.setStatus("external-session-bridge", "Bridge: disabled (set PI_EXTERNAL_RELAY_URL)");
      return;
    }
    stopped = false;
    registered = false;
    socketGen++;
    sessionPath = ctx.sessionManager.getSessionFile() ?? "";
    id = sessionId(sessionPath);
    cwd = process.cwd();
    title = pi.getSessionName() ?? "";
    if (ctx.model) emit({ type: "model_select", model: ctx.model });
    emit({ type: "thinking_level_select", level: pi.getThinkingLevel() });
    // Report available models so the server can serve them to companion apps.
    try {
      const scoped = ctx.getScopedModels?.();
      if (scoped && scoped.length > 0) {
        const models = scoped.map((sm: any) => sm.model ?? sm);
        emit({ type: "available_models", models });
      }
    } catch { /* getScopedModels may not be available in all contexts */ }
    ui.setStatus("external-session-bridge", "Bridge: connecting");
    await register();
    void flushEvents();
    // connectRelay now gates on registered + lease being present.
    connectRelay();
    // HTTP polling remains a temporary fallback if a network blocks WebSockets.
    void pollCommands();
  });
  pi.on("session_shutdown", async () => {
    stopped = true;
    socketGen++;
    relaySocket?.close();
    relaySocket = undefined;
    abortCurrent = undefined;
    if (relayReconnectTimer) clearTimeout(relayReconnectTimer);
    relayReconnectTimer = undefined;
    ui?.setStatus("external-session-bridge", undefined);
    emit({ type: "message_end", message: { role: "assistant" } });
  });
  pi.on("session_info_changed", async (event) => {
    title = event.name ?? "";
    registered = false; // next poll/flush refreshes title and heartbeat
    await register();
  });
  pi.on("model_select", async (event) => emit({ type: "model_select", model: event.model }));
  pi.on("thinking_level_select", async (event) => emit({ type: "thinking_level_select", level: event.level }));
  pi.on("message_start", async (event) => emit({ type: "message_start", message: event.message }));
  pi.on("message_update", async (event) => emit({ type: "message_update", assistantMessageEvent: event.assistantMessageEvent }));
  pi.on("message_end", async (event) => emit({ type: "message_end", message: event.message }));
  pi.on("tool_execution_start", async (event) => emit({ type: "tool_execution_start", toolName: event.toolName, toolCallId: event.toolCallId, args: event.args }));
  pi.on("tool_execution_update", async (event) => emit({ type: "tool_execution_update", toolName: event.toolName, toolCallId: event.toolCallId, partialResult: event.partialResult }));
  pi.on("tool_execution_end", async (event) => emit({ type: "tool_execution_end", toolName: event.toolName, toolCallId: event.toolCallId, result: event.result }));
}
