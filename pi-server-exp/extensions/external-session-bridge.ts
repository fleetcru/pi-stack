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
  let sessionCtx: { model?: unknown; abort: () => void; isIdle?: () => boolean; session?: { prompt: (text: string, options?: { streamingBehavior?: "steer" | "followUp"; expandPromptTemplates?: boolean; source?: string }) => Promise<void> } } | undefined;
  let relaySocket: WebSocket | undefined;
  let relayReconnectTimer: ReturnType<typeof setTimeout> | undefined;
  let relayConnectInFlight = false;
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
  const promptCommandsInFlight = new Set<string>();
  const promptDeliveryAttempts = new Map<string, number>();
  const userMessageWaiters = new Set<{
    text: string;
    hasImage: boolean;
    resolve: (matched: boolean) => void;
    timer: ReturnType<typeof setTimeout>;
  }>();
  let pendingAskRequest: ({ id: string } & Record<string, unknown>) | undefined;
  let pendingAskLease = "";

  /** Reload URL/token configuration so /bridge-reconnect can apply changes
   * without restarting Pi. The local config file is canonical, with the
   * environment used as a fallback for older setups. */
  const refreshConfig = (): boolean => {
    const config = loadConfig();
    const nextBaseUrl = (config.relayUrl ?? process.env.PI_EXTERNAL_RELAY_URL)?.replace(/\/$/, "") ?? "";
    const nextToken = config.relayToken ?? process.env.PI_EXTERNAL_RELAY_TOKEN;
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
    // Re-read bridge-config.json on every registration attempt so reconnects
    // pick up a server restart that changed relayUrl/relayToken without
    // requiring /bridge-reconnect or a Pi reload.
    if (refreshConfig()) {
      lease = "";
      registered = false;
    }
    try {
      const response = await request("/v1/external-sessions/register", "POST", { id, cwd, title, sessionPath, bridgeId });
      const data = await response.json() as { lease?: string };
      if (!data.lease) throw new Error("relay did not issue a command lease");
      lease = data.lease;
      registered = true;
      resetBackoff();
      ui?.setStatus("external-session-bridge", "Bridge: connected");
      // A daemon restart loses its in-memory event ring and pending dialog
      // state. Re-announce a still-blocking ask once for each new lease.
      if (pendingAskRequest && pendingAskLease !== lease) {
        pendingAskLease = lease;
        emit({ ...pendingAskRequest, type: "extension_ui_request", method: "ask_user" });
      }
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

  const userMessageText = (message: any): string => {
    if (!message || message.role !== "user") return "";
    if (typeof message.content === "string") return message.content;
    if (!Array.isArray(message.content)) return "";
    return message.content
      .filter((block: any) => block?.type === "text" && typeof block.text === "string")
      .map((block: any) => block.text)
      .join("");
  };

  const observeUserMessage = (message: any) => {
    const text = userMessageText(message);
    const hasImage = Array.isArray(message?.content) && message.content.some((block: any) => block?.type === "image");
    for (const waiter of userMessageWaiters) {
      if (waiter.text !== text || waiter.hasImage !== hasImage) continue;
      clearTimeout(waiter.timer);
      userMessageWaiters.delete(waiter);
      waiter.resolve(true);
      break;
    }
  };

  const waitForUserMessage = (text: string, hasImage = false, timeoutMs = 1_000): Promise<boolean> =>
    new Promise((resolve) => {
      const waiter = {
        text,
        hasImage,
        resolve,
        timer: setTimeout(() => {
          userMessageWaiters.delete(waiter);
          resolve(false);
        }, timeoutMs),
      };
      userMessageWaiters.add(waiter);
    });

  const acknowledge = async (commandId: string) => {
    if (relaySocket?.readyState === WebSocket.OPEN) {
      relaySocket.send(JSON.stringify({ type: "ack", ids: [commandId] }));
      return;
    }
    try { await request(`/v1/external-sessions/${id}/ack`, "POST", { ids: [commandId], lease }); } catch { registered = false; /* server retries delivery after lease renewal */ }
  };

  type RelayImage = { type: "image"; data: string; mimeType: string };
  type PiUserContent =
    | { type: "text"; text: string }
    | { type: "image"; data: string; mimeType: string };

  const isBase64 = (value: unknown): value is string =>
    typeof value === "string" && value.length > 0 && value.length % 4 === 0 && /^[A-Za-z0-9+/]+={0,2}$/.test(value);

  // Pi's TUI history writer drops the base64 payload from user image blocks
  // when persisting a session. On the next turn the replayed message reaches
  // the provider with `data: undefined`, which Codex rejects as an invalid
  // base64 value. Cache relay image payloads by content hash and restore them
  // into the outgoing provider request via before_provider_request.
  const relayImageCache = new Map<string, string>(); // key -> base64 data
  const MAX_CACHE_BYTES = 30 * 1024 * 1024;
  let cacheTotalBytes = 0;

  const hashImageKey = async (mimeType: string, data: string): Promise<string> => {
    try {
      const { createHash } = await import("crypto");
      return createHash("sha256").update(`${mimeType}:${data.slice(0, 256)}:${data.length}:${data.slice(-64)}`).digest("hex").slice(0, 32);
    } catch {
      return `${mimeType}:${data.length}:${data.slice(0, 32)}`;
    }
  };

  const cacheRelayImages = async (images?: RelayImage[]) => {
    for (const image of images ?? []) {
      if (!isBase64(image.data)) continue;
      const key = await hashImageKey(image.mimeType, image.data);
      const old = relayImageCache.get(key);
      if (old) cacheTotalBytes -= old.length;
      relayImageCache.set(key, image.data);
      cacheTotalBytes += image.data.length;
    }
    while (cacheTotalBytes > MAX_CACHE_BYTES && relayImageCache.size > 1) {
      const oldest = relayImageCache.keys().next().value;
      if (oldest === undefined) break;
      cacheTotalBytes -= relayImageCache.get(oldest)?.length ?? 0;
      relayImageCache.delete(oldest);
    }
  };

  const restoreImageBlock = async (block: any): Promise<any> => {
    if (!block || block.type !== "image" || isBase64(block.data)) return block;
    const sourceData = block.source?.type === "base64" ? block.source.data : undefined;
    if (isBase64(sourceData)) return { type: "image", data: sourceData, mimeType: block.source.mediaType ?? block.mimeType ?? "image/png" };
    const mimeType = String(block.mimeType ?? "image/png");
    const declaredLength = Number(block.contentLength ?? block.originalSize ?? 0);
    let candidate: { key: string; data: string } | undefined;
    let ambiguous = false;
    for (const [key, data] of relayImageCache) {
      const keyMime = key.includes(":") && !/^[0-9a-f]{32}$/.test(key) ? key.split(":")[0] : "";
      if (keyMime && keyMime !== mimeType) continue;
      if (declaredLength > 0 && Math.abs(data.length - declaredLength) > 8) continue;
      if (candidate && candidate.data !== data) ambiguous = true;
      candidate = { key, data };
    }
    if (candidate && !ambiguous) {
      return { type: "image", data: candidate.data, mimeType };
    }
    return null; // unrecoverable — drop the broken block instead of poisoning the request
  };

  const restoreMessageImages = async (message: any): Promise<any> => {
    const content = message?.message?.content ?? message?.content;
    if (!Array.isArray(content) || !content.some((b: any) => b?.type === "image" && !isBase64(b.data))) return message;
    const restored: any[] = [];
    for (const block of content) {
      const fixed = await restoreImageBlock(block);
      if (fixed) restored.push(fixed);
    }
    if (restored.length === 0 && content.every((b: any) => b?.type === "image")) return null;
    const target = message.message ? message.message : message;
    return { ...message, ...(message.message ? { message: { ...target, content: restored } } : { content: restored }) };
  };

  // Rewrite a provider request payload (Responses `input` or Chat Completions
  // `messages`) so hollow replayed image blocks carry real cached base64 again.
  const repairProviderPayload = async (payload: unknown): Promise<unknown> => {
    if (!payload || typeof payload !== "object") return payload;
    const root = payload as Record<string, any>;
    const list = Array.isArray(root.input) ? "input" : Array.isArray(root.messages) ? "messages" : undefined;
    if (!list) return payload;
    let dirty = false;
    const nextList: any[] = [];
    for (const item of root[list]) {
      const content = item?.content;
      if (!Array.isArray(content)) { nextList.push(item); continue; }
      const nextContent: any[] = [];
      for (const block of content) {
        if (block?.type === "input_image" && typeof block.image_url === "string" && !/;base64,[A-Za-z0-9+/=]{16,}/.test(block.image_url)) {
          const mime = block.image_url.match(/^data:([^;,]+)/)?.[1] ?? "image/jpeg";
          const fixed = await restoreImageBlock({ type: "image", mimeType: mime });
          if (fixed) { nextContent.push({ ...block, image_url: `data:${fixed.mimeType};base64,${fixed.data}` }); dirty = true; }
          else { dirty = true; } // drop the hollow image entirely
          continue;
        }
        if (block?.type === "image_url" && typeof block.image_url?.url === "string" && !/;base64,[A-Za-z0-9+/=]{16,}/.test(block.image_url.url)) {
          const mime = block.image_url.url.match(/^data:([^;,]+)/)?.[1] ?? "image/jpeg";
          const fixed = await restoreImageBlock({ type: "image", mimeType: mime });
          if (fixed) { nextContent.push({ ...block, image_url: { ...block.image_url, url: `data:${fixed.mimeType};base64,${fixed.data}` } }); dirty = true; }
          else { dirty = true; }
          continue;
        }
        if (block?.type === "image" && !isBase64(block.data)) {
          const fixed = await restoreImageBlock(block);
          if (fixed) { nextContent.push(fixed); dirty = true; }
          else { dirty = true; }
          continue;
        }
        nextContent.push(block);
      }
      nextList.push(dirty ? { ...item, content: nextContent.filter(Boolean) } : item);
    }
    if (!dirty) return payload;
    return { ...root, [list]: nextList };
  };

  const userMessageContent = (message: string, images?: RelayImage[]): string | PiUserContent[] => {
    if (!images?.length) return message;
    return [
      { type: "text", text: message },
      ...images
        // Defensively accept raw base64 or a full data URL; the provider
        // adapter adds its own prefix.
        .map((image) => {
          const raw = typeof image.data === "string" ? image.data : String(image?.data ?? "");
          const data = raw.startsWith("data:") && raw.includes("base64,") ? raw.slice(raw.indexOf("base64,") + 7) : raw;
          return { type: "image" as const, data, mimeType: image.mimeType };
        })
        .filter((image) => isBase64(image.data)),
    ];
  };

  const deliverCommand = async (command: { id: string; type: string; message?: string; images?: RelayImage[]; delivery?: "steer" | "followUp" | "prompt"; provider?: string; modelId?: string; level?: string; requestId?: string; value?: string; cancelled?: boolean; confirmed?: boolean; selections?: string[]; comment?: string; responseKind?: string }) => {
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
    if (command.type === "extension_ui_response") {
      // Route the mobile answer back into Pi on the shared extension bus so
      // the ask_user extension can resume the pending dialog.
      if (command.requestId) {
        pi.events.emit("ask:remote-response", {
          id: command.requestId,
          cancelled: command.cancelled,
          value: command.value,
          confirmed: command.confirmed,
          selections: command.selections,
          comment: command.comment,
          responseKind: command.responseKind,
        });
        emit({ type: "bridge_receipt", commandId: command.id, status: "delivered" });
      } else {
        emit({ type: "bridge_receipt", commandId: command.id, status: "failed" });
      }
      handledCommands.add(command.id);
      if (handledCommands.size > 500) {
        const first = handledCommands.values().next().value;
        if (first !== undefined) handledCommands.delete(first);
      }
      await acknowledge(command.id);
      return;
    }
    if (command.type !== "prompt" || (!command.message && !command.images?.length)) return;
    const imageLengths = (command.images ?? []).map((image) => typeof image?.data === "string" ? image.data.length : 0);
    console.error(`[external-session-bridge] prompt ${command.id}: images=${imageLengths.length} dataLengths=${imageLengths.join(",")}`);
    if (promptCommandsInFlight.has(command.id)) return;
    promptCommandsInFlight.add(command.id);
    try {
      const requestedDelivery = command.delivery ?? "prompt";
      await cacheRelayImages(command.images);
      const content = userMessageContent(command.message, command.images);
      const isSlashCommand = typeof command.message === "string" && /^\/\S+/.test(command.message.trim());
      if (isSlashCommand && sessionCtx?.session) {
        // sendUserMessage deliberately disables command handling. Use the
        // session prompt API so slash commands execute locally and never
        // become an LLM prompt.
        await sessionCtx.session.prompt(command.message!.trim(), {
          expandPromptTemplates: true,
          source: "extension",
          ...((sessionCtx.isIdle?.() ?? true) ? {} : { streamingBehavior: requestedDelivery === "followUp" ? "followUp" : "steer" }),
        });
        promptDeliveryAttempts.delete(command.id);
        emit({ type: "bridge_receipt", commandId: command.id, status: "delivered" });
        handledCommands.add(command.id);
        await acknowledge(command.id);
        return;
      }
      const attempt = (promptDeliveryAttempts.get(command.id) ?? 0) + 1;
      promptDeliveryAttempts.set(command.id, attempt);
      const idle = sessionCtx?.isIdle?.() ?? false;
      if (requestedDelivery === "prompt" && idle) {
        // sendUserMessage() is intentionally fire-and-forget: Pi reports an
        // idle→working race through its extension error channel rather than by
        // throwing here. Confirm that the user message actually entered the TUI
        // before acknowledging the durable server command.
        const appeared = waitForUserMessage(command.message ?? "", Boolean(command.images?.length));
        if (attempt === 1) {
          // Never inject the same idle prompt twice solely because its echo was
          // late. Later attempts only re-check state and either steer an active
          // turn or fail permanently with a visible receipt.
          pi.sendUserMessage(content);
        }
        if (!(await appeared)) {
          if (!(sessionCtx?.isIdle?.() ?? false)) {
            // The first attempt made Pi active, but its user-message echo was
            // late. Treat the active turn as confirmation instead of steering
            // the same text and risking duplicate execution/tool side effects.
            promptDeliveryAttempts.delete(command.id);
            emit({ type: "bridge_receipt", commandId: command.id, status: "delivered" });
            handledCommands.add(command.id);
            await acknowledge(command.id);
            return;
          } else {
            // Keep the command durable and force a fresh relay attachment so it
            // is retried. Never emit a false delivered receipt or ack it away.
            emit({ type: "bridge_receipt", commandId: command.id, status: "failed" });
            if (attempt < 3) {
              ui?.notify(`Remote message was not accepted by Pi; retrying (${attempt}/3)`, "warning");
              relaySocket?.close();
              return;
            }
            // Avoid an infinite reconnect loop and notification storm. Keep the
            // failed command visible through its receipt, but acknowledge it so
            // a message that Pi accepted without a timely echo cannot be
            // injected repeatedly into the same TUI history.
            ui?.notify("Remote message could not be confirmed after 3 attempts", "error");
            promptDeliveryAttempts.delete(command.id);
            handledCommands.add(command.id);
            await acknowledge(command.id);
            return;
          }
        }
      } else {
        const delivery = requestedDelivery === "followUp" ? "followUp" : "steer";
        pi.sendUserMessage(content, { deliverAs: delivery });
      }
      promptDeliveryAttempts.delete(command.id);
      emit({ type: "bridge_receipt", commandId: command.id, status: "delivered" });
      ui?.notify(`Remote ${requestedDelivery === "steer" ? "steer" : "message"} received`, "info");
      handledCommands.add(command.id);
      if (handledCommands.size > 500) {
        const first = handledCommands.values().next().value;
        if (first !== undefined) handledCommands.delete(first);
      }
      await acknowledge(command.id);
    } finally {
      promptCommandsInFlight.delete(command.id);
    }
  };

  // ── WebSocket ─────────────────────────────────────────────────────────

  const connectRelay = async () => {
    if (stopped || !id || !baseUrl) return;
    // Guard both the socket and the async registration path. Without the
    // second guard, polling and reconnect timers can each start registration
    // and then race to open sockets for different leases.
    if (relaySocket?.readyState === WebSocket.OPEN || relaySocket?.readyState === WebSocket.CONNECTING || relayConnectInFlight) return;
    relayConnectInFlight = true;
    // Require registration + lease before attempting WS. Awaiting this
      // directly avoids the old recursive register().then(connectRelay)
      // pattern and gives failed registration one explicit retry path.
      if (!registered || !lease) {
        const ok = await register();
        if (!ok || stopped) {
          relayConnectInFlight = false;
          if (!stopped) scheduleReconnect();
          return;
        }
      }

      const gen = ++socketGen;

    // Authenticate via a WebSocket subprotocol instead of a URL query token so
    // the secret stays out of URLs, proxy logs, and process listings. Tokens
    // outside the subprotocol grammar fall back to the deprecated query param.
    // Tokens travel in Sec-WebSocket-Protocol, never in the URL. Subprotocol
    // values must be valid HTTP header tokens, so non URL-safe tokens are
    // base64url-encoded under the pi-relay-b64. prefix (the server decodes
    // both forms). This removes the old HTTP-polling fallback entirely.
    const subprotocol = (() => {
      if (!token) return undefined;
      return /^[A-Za-z0-9._~-]+$/.test(token)
        ? `pi-relay.${token}`
        : `pi-relay-b64.${Buffer.from(token, "utf8").toString("base64url")}`;
    })();
    const relayQuery = new URLSearchParams({ lease });
    const wsUrl = baseUrl.replace(/^http/, "ws") + `/v1/external-sessions/relay/${encodeURIComponent(id)}?${relayQuery}`;
    try {
      const socket = subprotocol ? new WebSocket(wsUrl, [subprotocol]) : new WebSocket(wsUrl);
      relaySocket = socket;

      socket.onopen = async () => {
        relayConnectInFlight = false;
        // Stale-guard: a newer socket already took over.
        if (gen !== socketGen) return;
        resetBackoff();
        ui?.setStatus("external-session-bridge", "Bridge: connected");
        // Re-emit current model/thinking state so the server refreshes after
        // any events that were dropped during the disconnect window.
        if (sessionCtx?.model) emit({ type: "model_select", model: sessionCtx.model });
        emit({ type: "thinking_level_select", level: pi.getThinkingLevel() });
        // Publish the live command catalog so remote clients can offer the
        // same extension, prompt-template, and skill commands as the TUI.
        try {
          const session: any = sessionCtx?.session;
          const commands = [
            ...(session?.extensionRunner?.getRegisteredCommands?.() ?? []).map((c: any) => ({ name: c.invocationName, description: c.description, source: "extension" })),
            ...(session?.promptTemplates ?? []).map((c: any) => ({ name: c.name, description: c.description, source: "prompt" })),
            ...(session?.resourceLoader?.getSkills?.()?.skills ?? []).map((s: any) => ({ name: `skill:${s.name}`, description: s.description, source: "skill" })),
          ].filter((c: any) => typeof c.name === "string" && c.name.length > 0);
          emit({ type: "available_commands", commands });
        } catch { /* command discovery is best-effort during reconnect */ }
        // Re-emit available models on reconnect.
        try {
          let models: any[] = [];
          const scoped = sessionCtx?.getScopedModels?.();
          if (scoped && scoped.length > 0) {
            models = scoped.map((sm: any) => sm.model ?? sm);
          }
          if (models.length === 0) {
            const all = (sessionCtx as any)?.getModels?.();
            if (all && all.length > 0) models = all;
          }
          if (models.length === 0) {
            const avail = (sessionCtx as any)?.getAvailableModels?.();
            if (avail && avail.length > 0) models = avail;
          }
          if (models.length === 0 && sessionCtx?.model) {
            const m = sessionCtx.model as any;
            if (m.provider && m.id) {
              models = [{ provider: m.provider, id: m.id, name: m.name ?? m.id }];
            }
          }
          // Only emit if we found a real model list (not just the active model).
          // The emitModels() function may have already sent the full list from
          // models-store.json — don't overwrite it with a single-item fallback.
          if (models.length > 1) emit({ type: "available_models", models });
        } catch { /* model APIs may not be available */ }
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
          const envelope = JSON.parse(String(message.data)) as { type?: string; command?: { id: string; type: string; message?: string; delivery?: "steer" | "followUp" | "prompt"; provider?: string; modelId?: string; level?: string; requestId?: string; value?: string; cancelled?: boolean; confirmed?: boolean; selections?: string[]; comment?: string; responseKind?: string } };
          if (envelope.type === "command" && envelope.command) await deliverCommand(envelope.command);
        } catch (error) { ui?.notify(`Bridge command failed: ${error instanceof Error ? error.message : "unknown error"}`, "error"); }
      };

      socket.onclose = () => {
        relayConnectInFlight = false;
        if (gen !== socketGen) return; // stale socket — don't mutate state or reschedule
        if (relaySocket === socket) relaySocket = undefined;
        if (!stopped) {
          ui?.setStatus("external-session-bridge", `Bridge: reconnecting (ws closed)`);
          scheduleReconnect();
        }
      };

      socket.onerror = () => socket.close();
    } catch (error) {
      relayConnectInFlight = false;
      registered = false;
      ui?.setStatus("external-session-bridge", `Bridge: reconnecting (${error instanceof Error ? error.message : "WebSocket setup failed"})`);
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
      // The poller is both the fallback transport and the recovery supervisor:
      // a closed/failed socket must not leave us registered but disconnected.
      // connectRelay() is guarded against concurrent registration/socket setup.
      if (relaySocket?.readyState === WebSocket.OPEN) {
        await new Promise((resolve) => setTimeout(resolve, 5_000));
        continue;
      }
      try {
        if (!registered && !(await register())) throw new Error("not registered");
        if (registered && !relaySocket) void connectRelay();
        const response = await request(`/v1/external-sessions/${id}/commands?lease=${encodeURIComponent(lease)}`);
        const data = await response.json() as { commands?: Array<{ id: string; type: string; message?: string; delivery?: "steer" | "followUp" | "prompt"; provider?: string; modelId?: string; level?: string; requestId?: string; value?: string; cancelled?: boolean; confirmed?: boolean; selections?: string[]; comment?: string; responseKind?: string }> };
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
      relayConnectInFlight = false;
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
    description: "Register a bridge URL and optional token, then reconnect",
    handler: async (args, ctx) => {
      let url = args?.trim();
      if (!url) {
        url = await ctx.ui.input("Enter bridge URL:", baseUrl || "");
        if (!url) {
          ctx.ui.notify("Bridge registration cancelled.", "info");
          return;
        }
      }
      try {
        new URL(url);
      } catch {
        ctx.ui.notify(`Invalid URL: ${url}`, "error");
        return;
      }
      const config = loadConfig();
      const savedToken = process.env.PI_EXTERNAL_RELAY_TOKEN ?? config.relayToken ?? "";
      const enteredToken = await ctx.ui.input("Enter bridge token, or leave blank for no token:", savedToken);
      if (enteredToken === undefined) {
        ctx.ui.notify("Bridge registration cancelled.", "info");
        return;
      }
      config.relayUrl = url;
      if (enteredToken.trim()) config.relayToken = enteredToken.trim();
      else delete config.relayToken;
      saveConfig(config);
      ctx.ui.notify(`Bridge settings saved: ${url}`, "info");
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
    // This bridge represents an existing interactive TUI. Server-started RPC
    // sessions already have a native transport and extension UI protocol.
    if (ctx.mode !== "tui") {
      stopped = true;
      return;
    }
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
    // Try multiple API paths since getScopedModels may not exist in all Pi versions.
    try {
      const scoped = ctx.getScopedModels?.();
      if (scoped && scoped.length > 0) {
        const models = scoped.map((sm: any) => sm.model ?? sm);
        emit({ type: "available_models", models });
      } else {
        // Fallback: try getModels or list available from the provider
        const allModels = (ctx as any).getModels?.() ?? [];
        if (allModels.length > 0) {
          emit({ type: "available_models", models: allModels });
        }
      }
    } catch { /* model APIs may not be available in all contexts */ }
    ui.setStatus("external-session-bridge", "Bridge: connecting");
    await register();
    void flushEvents();
    // connectRelay now gates on registered + lease being present.
    connectRelay();
    // HTTP polling remains a temporary fallback if a network blocks WebSockets.
    void pollCommands();
    // Emit available models so the server can serve them to companion/webby.
    // Model discovery is intentionally ordered from most authoritative to
    // least authoritative: current Pi scope, current Pi model list, the local
    // models store, then the active model. Reconnect uses the same ordering but
    // only emits a list with more than one model, so a temporary API gap cannot
    // overwrite a previously published catalog with a one-item fallback.
    // Try multiple sources: Pi API → models-store.json → active model fallback.
    let modelsEmitted = false;
    const emitModels = async () => {
      if (stopped || modelsEmitted) return;
      try {
        let models: any[] = [];
        // 1. Try Pi API: getScopedModels
        const scoped = (ctx as any).getScopedModels?.();
        if (scoped && scoped.length > 0) {
          models = scoped.map((sm: any) => sm.model ?? sm);
        }
        // 2. Try Pi API: getModels
        if (models.length === 0) {
          const all = (ctx as any).getModels?.();
          if (all && all.length > 0) models = all;
        }
        // 3. Read models-store.json from ~/.pi/agent/
        if (models.length === 0) {
          try {
            const fs = await import("fs");
            const path = await import("path");
            const home = process.env.HOME || process.env.USERPROFILE || "";
            const storePath = path.join(home, ".pi", "agent", "models-store.json");
            const raw = fs.readFileSync(storePath, "utf-8");
            const store = JSON.parse(raw);
            for (const [provider, providerData] of Object.entries(store)) {
              const providerModels = (providerData as any)?.models;
              if (Array.isArray(providerModels)) {
                for (const m of providerModels) {
                  models.push({ provider, id: m.id, name: m.name ?? m.id });
                }
              }
            }
          } catch { /* models-store.json not available */ }
        }
        // 4. Fallback: construct from active model
        if (models.length === 0 && ctx.model) {
          const m = ctx.model as any;
          if (m.provider && m.id) {
            models = [{ provider: m.provider, id: m.id, name: m.name ?? m.id }];
          }
        }
        if (models.length > 0) {
          emit({ type: "available_models", models });
          modelsEmitted = true;
        }
      } catch { /* ignore */ }
    };
    // Try immediately, then retry every 5s until we get models
    void emitModels();
    const modelPoller = setInterval(() => {
      if (modelsEmitted || stopped) { clearInterval(modelPoller); return; }
      void emitModels();
    }, 5_000);
  });
  pi.on("session_shutdown", async () => {
    stopped = true;
    relayConnectInFlight = false;
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
  // Forward real run lifecycle events. The server uses them to release relay
  // admission slots, while Companion uses them to settle send/spinner state.
  pi.on("agent_start", async () => emit({ type: "agent_start" }));
  pi.on("agent_end", async () => emit({ type: "agent_end" }));
  pi.on("agent_settled", async () => emit({ type: "agent_settled" }));
  pi.on("message_start", async (event) => {
    observeUserMessage(event.message);
    emit({ type: "message_start", message: event.message });
  });
  pi.on("message_update", async (event) => emit({ type: "message_update", assistantMessageEvent: event.assistantMessageEvent }));
  pi.on("message_end", async (event) => {
    // Restore image data stripped by Pi's history writer before re-emitting,
    // so remote clients keep the real attachment instead of a hollow reference.
    const restored = await restoreMessageImages(event.message);
    if (restored === null) return;
    emit({ type: "message_end", message: restored.message ?? restored });
  });
  // Re-inject cached base64 into the live provider request, because replayed
  // history carries image blocks without data (`base64,undefined`).
  pi.on("before_provider_request", async (event) => {
    try {
      const repaired = await repairProviderPayload(event.payload);
      if (repaired !== event.payload) return repaired;
    } catch { /* best effort — never block a request over image repair */ }
  });
  pi.on("tool_execution_start", async (event) => emit({ type: "tool_execution_start", toolName: event.toolName, toolCallId: event.toolCallId, args: event.args }));
  pi.on("tool_execution_update", async (event) => emit({ type: "tool_execution_update", toolName: event.toolName, toolCallId: event.toolCallId, partialResult: event.partialResult }));
  pi.on("tool_execution_end", async (event) => emit({ type: "tool_execution_end", toolName: event.toolName, toolCallId: event.toolCallId, result: event.result }));

  // Mobile ask_user relay: the ask-user extension publishes dialogs on the
  // shared extension bus; forward them as extension UI requests/close events so
  // companion apps render and answer them, and the daemon tracks the session
  // as waiting_for_input until ask:closed arrives.
  pi.events.on("ask:requested", (event) => {
    if (!event || typeof event !== "object") return;
    const request = event as { id?: string } & Record<string, unknown>;
    if (!request.id) return;
    pendingAskRequest = request as { id: string } & Record<string, unknown>;
    pendingAskLease = registered ? lease : "";
    emit({ ...request, type: "extension_ui_request", method: "ask_user" });
  });
  pi.events.on("ask:closed", (event) => {
    const requestId = (event as { id?: string } | undefined)?.id;
    if (!requestId) return;
    if (pendingAskRequest?.id === requestId) {
      pendingAskRequest = undefined;
      pendingAskLease = "";
    }
    emit({ type: "extension_ui_closed", id: requestId });
  });
}
