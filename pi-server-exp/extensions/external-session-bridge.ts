import type { ExtensionAPI } from "@earendil-works/pi-coding-agent";
import { existsSync, readFileSync } from "node:fs";
import { join } from "node:path";

type BridgeConfig = { relayUrl?: string; relayToken?: string };
function loadConfig(): BridgeConfig {
  const path = join(process.env.USERPROFILE ?? process.env.HOME ?? "", ".pi", "agent", "bridge-config.json");
  try { return existsSync(path) ? JSON.parse(readFileSync(path, "utf8")) as BridgeConfig : {}; } catch { return {}; }
}
const bridgeConfig = loadConfig();
const baseUrl = (process.env.PI_EXTERNAL_RELAY_URL ?? bridgeConfig.relayUrl)?.replace(/\/$/, "");
const token = process.env.PI_EXTERNAL_RELAY_TOKEN ?? bridgeConfig.relayToken;
const eventQueueLimit = 200;
const headers = () => ({ "content-type": "application/json", ...(token ? { authorization: `Bearer ${token}` } : {}) });

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
  let registered = false;
  let pollRunning = false;
  let flushRunning = false;
  let ui: { setStatus: (key: string, text?: string) => void; notify: (message: string, level?: "info" | "warning" | "error") => void } | undefined;
  let sessionCtx: { model?: unknown; abort: () => void } | undefined;
  let relaySocket: WebSocket | undefined;
  let relayReconnectTimer: ReturnType<typeof setTimeout> | undefined;
  let abortCurrent: (() => void) | undefined;
  const pendingEvents: unknown[] = [];
  const handledCommands = new Set<string>();

  const request = async (path: string, method = "GET", body?: unknown) => {
    if (!baseUrl) throw new Error("PI_EXTERNAL_RELAY_URL is not configured");
    const response = await fetch(`${baseUrl}${path}`, {
      method,
      headers: method === "GET" ? (token ? { authorization: `Bearer ${token}` } : {}) : headers(),
      body: body === undefined ? undefined : JSON.stringify(body),
    });
    if (!response.ok) throw new Error(`relay HTTP ${response.status}`);
    return response;
  };

  const register = async () => {
    if (!id) return false;
    try {
      const response = await request("/v1/external-sessions/register", "POST", { id, cwd, title, sessionPath, bridgeId });
      const data = await response.json() as { lease?: string };
      if (!data.lease) throw new Error("relay did not issue a command lease");
      lease = data.lease;
      registered = true;
      ui?.setStatus("external-session-bridge", "Bridge: connected");
      return true;
    } catch {
      registered = false;
      ui?.setStatus("external-session-bridge", "Bridge: reconnecting");
      return false;
    }
  };

  const emit = (event: unknown) => {
    // Single ordered sender: only use the socket when no backlog exists and no
    // HTTP flush is in flight, otherwise newer events could overtake older ones.
    if (relaySocket?.readyState === WebSocket.OPEN && pendingEvents.length === 0 && !flushRunning) {
      relaySocket.send(JSON.stringify({ type: "event", event }));
      return;
    }
    pendingEvents.push(event);
    if (pendingEvents.length > eventQueueLimit) pendingEvents.shift();
    void flushEvents();
  };

  const flushEvents = async () => {
    if (flushRunning || !id || stopped || relaySocket?.readyState === WebSocket.OPEN) return;
    flushRunning = true;
    try {
      if (!registered && !(await register())) return;
      while (pendingEvents.length && !stopped) {
        const event = pendingEvents[0];
        try {
          await request(`/v1/external-sessions/${id}/events`, "POST", event);
          pendingEvents.shift();
        } catch {
          registered = false;
          break;
        }
      }
    } finally {
      flushRunning = false;
    }
  };

  const acknowledge = async (commandId: string) => {
    if (relaySocket?.readyState === WebSocket.OPEN) {
      relaySocket.send(JSON.stringify({ type: "ack", ids: [commandId] }));
      return;
    }
    try { await request(`/v1/external-sessions/${id}/ack`, "POST", { ids: [commandId], lease }); } catch { registered = false; /* server retries delivery after lease renewal */ }
  };

  const deliverCommand = async (command: { id: string; type: string; message?: string; delivery?: "steer" | "followUp" | "prompt" }) => {
    if (handledCommands.has(command.id)) { await acknowledge(command.id); return; }
    if (command.type === "abort") {
      abortCurrent?.();
      emit({ type: "bridge_receipt", commandId: command.id, status: "delivered" });
      ui?.notify("Remote stop requested", "warning");
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
    if (handledCommands.size > 500) handledCommands.delete(handledCommands.values().next().value!);
    await acknowledge(command.id);
  };

  const connectRelay = () => {
    if (stopped || !id || !baseUrl || relaySocket?.readyState === WebSocket.OPEN || relaySocket?.readyState === WebSocket.CONNECTING) return;
    // Authenticate via a WebSocket subprotocol instead of a URL query token so
    // the secret stays out of URLs, proxy logs, and process listings. Tokens
    // outside the subprotocol grammar fall back to the deprecated query param.
    const subprotocolSafe = token ? /^[A-Za-z0-9._~-]+$/.test(token) : false;
    const relayQuery = new URLSearchParams({ lease });
    if (token && !subprotocolSafe) relayQuery.set("relayToken", token);
    const wsUrl = baseUrl.replace(/^http/, "ws") + `/v1/external-sessions/relay/${encodeURIComponent(id)}?${relayQuery}`;
    try {
      const socket = subprotocolSafe ? new WebSocket(wsUrl, [`pi-relay.${token}`]) : new WebSocket(wsUrl);
      relaySocket = socket;
      socket.onopen = async () => {
        ui?.setStatus("external-session-bridge", "Bridge: connected");
        // Re-emit current model/thinking state so the server refreshes after
        // any events that were dropped during the disconnect window.
        if (sessionCtx?.model) emit({ type: "model_select", model: sessionCtx.model });
        emit({ type: "thinking_level_select", level: pi.getThinkingLevel() });
        // Wait for any in-flight HTTP flush before draining the backlog over the
        // socket so both paths never send the same queue concurrently.
        while (flushRunning) await new Promise((resolve) => setTimeout(resolve, 50));
        while (pendingEvents.length && socket.readyState === WebSocket.OPEN) socket.send(JSON.stringify({ type: "event", event: pendingEvents.shift() }));
      };
      socket.onmessage = async (message) => {
        try {
          const envelope = JSON.parse(String(message.data)) as { type?: string; command?: { id: string; type: string; message?: string; delivery?: "steer" | "followUp" | "prompt" } };
          if (envelope.type === "command" && envelope.command) await deliverCommand(envelope.command);
        } catch (error) { ui?.notify(`Bridge command failed: ${error instanceof Error ? error.message : "unknown error"}`, "error"); }
      };
      socket.onclose = () => {
        if (relaySocket === socket) relaySocket = undefined;
        if (!stopped) { ui?.setStatus("external-session-bridge", "Bridge: reconnecting"); relayReconnectTimer = setTimeout(connectRelay, 1_000); }
      };
      socket.onerror = () => socket.close();
    } catch { relayReconnectTimer = setTimeout(connectRelay, 2_000); }
  };

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
        const response = await request(`/v1/external-sessions/${id}/commands?lease=${encodeURIComponent(lease)}`);
        const data = await response.json() as { commands?: Array<{ id: string; type: string; message?: string; delivery?: "steer" | "followUp" | "prompt" }> };
        for (const command of data.commands ?? []) await deliverCommand(command);
      } catch {
        registered = false;
      }
      await flushEvents();
      await new Promise((resolve) => setTimeout(resolve, registered ? 750 : 2_000));
    }
    pollRunning = false;
  }

  const reconnectBridge = async () => {
    stopped = false;
    registered = false;
    relaySocket?.close();
    relaySocket = undefined;
    if (relayReconnectTimer) clearTimeout(relayReconnectTimer);
    const ok = await register();
    if (ok) { await flushEvents(); connectRelay(); }
  };

  pi.registerCommand("bridge-status", {
    description: "Show external session bridge status",
    handler: async (_args, ctx) => {
      ctx.ui.notify(baseUrl
        ? `Bridge: ${registered ? "connected" : "reconnecting"} · ${id || "no session"} · queued events: ${pendingEvents.length}`
        : "Bridge: disabled — set PI_EXTERNAL_RELAY_URL, then restart Pi.", "info");
    },
  });

  pi.registerCommand("bridge-reconnect", {
    description: "Reconnect this Pi session to pi-server",
    handler: async (_args, ctx) => {
      ctx.ui.setStatus("external-session-bridge", "Bridge: reconnecting");
      await reconnectBridge();
      ctx.ui.notify(`Bridge: ${registered ? "connected" : "unable to connect"}`, registered ? "info" : "error");
    },
  });

  pi.registerCommand("bridge-disconnect", {
    description: "Disconnect this Pi session from pi-server until the next reconnect",
    handler: async (_args, ctx) => {
      stopped = true;
      relaySocket?.close();
      ui?.setStatus("external-session-bridge", "Bridge: disconnected");
      ctx.ui.notify("Bridge disconnected. Use /bridge-reconnect to restore it.", "info");
    },
  });

  pi.on("session_start", async (_event, ctx) => {
    ui = ctx.ui;
    sessionCtx = ctx;
    abortCurrent = () => ctx.abort();
    if (!baseUrl) {
      ui.setStatus("external-session-bridge", "Bridge: disabled (set PI_EXTERNAL_RELAY_URL)");
      return;
    }
    stopped = false;
    registered = false;
    sessionPath = ctx.sessionManager.getSessionFile() ?? "";
    id = sessionId(sessionPath);
    cwd = process.cwd();
    title = pi.getSessionName() ?? "";
    if (ctx.model) emit({ type: "model_select", model: ctx.model });
    emit({ type: "thinking_level_select", level: pi.getThinkingLevel() });
    ui.setStatus("external-session-bridge", "Bridge: connecting");
    await register();
    void flushEvents();
    connectRelay();
    // HTTP polling remains a temporary fallback if a network blocks WebSockets.
    void pollCommands();
  });
  pi.on("session_shutdown", async () => {
    stopped = true;
    relaySocket?.close();
    abortCurrent = undefined;
    if (relayReconnectTimer) clearTimeout(relayReconnectTimer);
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
