# Feature proposals and history

> Status checked against the current codebase. Global session discovery is partially implemented for local sessions and registered workers. Peer-server aggregation and live session sharing are not implemented. The current API is documented in the server OpenAPI document.

## 1. Global Session Discovery

> **Current status: Partially implemented.** `GET /v1/global-sessions` lists local sessions and registered-worker sessions. Webby and Companion can display the results. Peer-server aggregation, the proposed `servers` response array, and the proposed peer environment variables are not implemented.
>
> Worker sessions can be attached through `POST /v1/global-sessions/{id}/attach`, which creates or reuses the existing remote-session mapping. This is different from the share-token attach flow described in section 2.

### Problem

Today each `pi-server` instance only knows about its own local sessions and explicitly registered remote workers. If you run pi-server on your laptop, your phone's pi-companion can see those sessions — but there's no single view of *all* Pi sessions across every machine you own. You have to manually add each server in settings and switch between them.

### Goal

A single endpoint (and client UI) that answers: **"What Pi sessions exist everywhere I have access?"**

### Design

#### Server side

The current implementation exposes:

```
GET /v1/global-sessions
POST /v1/global-sessions/{id}/attach
```

The list endpoint aggregates the local server and registered workers. It does not query peer servers.

The implemented endpoint aggregates:

1. **Local sessions** — the ones this pi-server instance owns directly.
2. **Registered workers** — each worker's `/v1/sessions?scope=all` is proxied and merged.

**Peer servers** remain a proposed third source and are not queried by the current endpoint.

The response shape below remains the target proposal. The current response contains `sessions` and `partialFailures`; each session has `id`, `originId`, `workerId`, `session`, and `reachable` fields.

Target response shape:

```jsonc
{
  "sessions": [
    {
      "id": "abc123",
      "workerId": "laptop",           // "local" or worker id
      "serverUrl": "http://10.0.0.5:3141",  // originating server
      "cwd": "/home/user/project",
      "status": "running",
      "project": "pi-stack",
      "title": "Refactor auth layer",
      "owner": "basin",
      "labels": ["backend"],
      "createdAt": "2025-07-17T10:00:00Z",
      "updatedAt": "2025-07-17T12:30:00Z",
      "state": { "model": { "name": "claude-sonnet-4-20250514" }, "isStreaming": false }
    }
  ],
  "servers": [
    { "url": "http://10.0.0.5:3141", "name": "Laptop", "reachable": true, "sessionCount": 3 },
    { "url": "http://10.0.0.9:3141", "name": "Desktop", "reachable": true, "sessionCount": 1 },
    { "url": "http://10.0.0.12:3141", "name": "Work", "reachable": false, "sessionCount": 0 }
  ]
}
```

#### Configuration

Peer-server configuration is **not implemented**. The following remains a proposal:

Add to pi-server config (env vars or config file):

```
PI_SERVER_PEER_SERVERS=http://10.0.0.5:3141,http://10.0.0.9:3141
PI_SERVER_PEER_AUTH_TOKENS=token-laptop,token-desktop   // optional per-peer auth
```

If peer support is implemented later, peers should be queried with a short timeout (2-5s). Unreachable peers should be marked `reachable: false` without blocking the response.

#### Client behavior

- **pi-webby**: Implemented. The Global session view lists available local and worker sessions and opens worker sessions through the coordinator attach flow.
- **pi-companion**: Implemented in the Home session inventory with global-session models, loading, and attach support. The UI covers local and registered-worker results, not peer servers.

#### Security

- The implemented worker flow uses explicitly registered workers; peer-server configuration and peer auth tokens are not present.
- Worker auth tokens remain server-side and are not exposed to clients.
- The current global-session response reports local and registered-worker sessions. Peer-server filtering is not applicable because peer-server aggregation is not implemented.

---

## 2. Live Session Sharing (Remote View & Interact)

> **Current status: Not implemented.** There are no share-token endpoints, share permissions, share/revoke UI, shared-session attach route, or deep-link handlers in the current codebase. Existing global-session attachment is a separate worker-proxy feature.

### Problem

You start a Pi session on your laptop. Later you're on your phone or a different machine and want to **see what Pi is doing right now and send it messages** — without creating a new session. Today you'd have to SSH into the laptop or remember to register it as a worker.

### Goal

**Any client can attach to any existing session in real-time** — viewing the live conversation and sending prompts — as if they were sitting at the original machine.

### Design

#### Concept: Session Sharing

A session owner **shares** a session, generating a short-lived share token. Any authorized client can use that token to attach to the session as a viewer/interactor.

#### Server side

The following endpoints are still proposed and are not present:

```
POST /v1/sessions/{id}/share
  → { "shareToken": "sh_...", "expiresAt": "2025-07-18T10:00:00Z", "permissions": "view|interact" }

DELETE /v1/sessions/{id}/share
  → revoke all active shares

GET /v1/sessions/{id}/shares
  → { "shares": [{ "token": "sh_...", "createdAt": "...", "expiresAt": "...", "permissions": "..." }] }
```

Attach to a shared session:

```
POST /v1/shared/attach
  { "shareToken": "sh_abc123" }
  → {
      "sessionId": "original-session-id",
      "serverUrl": "http://10.0.0.5:3141",
      "proxySessionId": "remote-xyz",   // use existing remote proxy infrastructure
      "ws": "/v1/sessions/remote-xyz/ws",
      "permissions": "view|interact"
    }
```

Once attached, the client uses the existing remote session proxy (`/v1/sessions/{id}/...`) and WebSocket streaming. No new transport — it piggybacks on the worker proxy mechanism.

#### Permissions

| Permission | Can view timeline | Can send prompts | Can abort/compact | Can edit metadata |
|---|---|---|---|---|
| `view` | ✅ | ❌ | ❌ | ❌ |
| `interact` | ✅ | ✅ | ✅ | ❌ |
| `admin` | ✅ | ✅ | ✅ | ✅ |

Default share permission is `view`. Owner can upgrade to `interact`.

#### Flow: Phone viewing laptop session

```
1. Laptop: Pi session running, user sends "share this session" or clicks Share in UI
2. Pi-server creates share token, shows QR code / deep link
3. Phone: Scans QR or opens link → pi-companion opens
4. pi-companion calls POST /v1/shared/attach with the token
5. pi-server resolves the token → finds original session → creates remote proxy mapping
6. Phone now has a proxySessionId, connects WebSocket
7. Phone sees live streaming conversation, tool calls, file changes in real-time
8. If permissions = "interact", phone can send prompts that Pi executes on the laptop
```

#### Flow: Web app viewing from another browser

```
1. User opens pi-webby on work computer
2. Navigates to "Shared" section, enters share token (or clicks link)
3. Same attach flow as above
4. Web app shows the session in read-only or interactive mode
```

#### Client UX

**pi-webby:**
- "Share" button on session header → generates token, shows modal with link + QR + copy button
- "Shared with me" section in sidebar → enter token or auto-detect from URL params
- Shared sessions shown with a 👁 or 🤝 badge
- View-only mode grays out the prompt bar

**pi-companion:**
- "Share" option in session detail menu → shows share sheet with QR code
- Deep link: `picompanion://share/sh_abc123` → auto-attaches
- "Shared sessions" section on Home screen
- View-only mode hides the input bar, interact mode shows it with a banner: "Interacting on Laptop's session"

#### Share token security

- Tokens are random 32-byte strings, URL-safe base64 encoded.
- Tokens expire (default 24h, configurable).
- Tokens are bound to one session — can't be reused for other sessions.
- Optional: IP allowlist per share (`PI_SERVER_SHARE_ALLOWED_IPS`).
- Share tokens are stored in `PI_SERVER_DATA_DIR/shares.json`, encrypted at rest is a future consideration.
- The owner can revoke individual shares or all shares for a session.

#### Deep link handling

Register URL schemes:

- **Android**: `picompanion://share/{token}` — pi-companion handles via intent filter
- **Web**: `https://your-pi-server/v1/shared/attach?token={token}` — pi-webby page that auto-attaches and redirects to the session

---

## Implementation Order

| Step | What | Current status | Depends on |
|---|---|---|---|
| 1 | Global session discovery endpoint | **Partial:** local sessions and registered workers only | Nothing — standalone |
| 2 | Global sessions UI in Webby + Companion | **Partial:** current local/worker inventory is wired in both clients | Step 1 |
| 3 | Session sharing server endpoints | **Not implemented** | Nothing — standalone |
| 4 | Share/revoke UI in both clients | **Not implemented** | Step 3 |
| 5 | Shared session attach + proxy | **Not implemented:** global worker attach is separate | Steps 3 + existing remote proxy |
| 6 | Deep link handling (Android intent, web redirect) | **Not implemented** | Step 3 |
| 7 | Permissions enforcement on proxy layer | **Not implemented** | Step 5 |

The local and registered-worker portion of step 1 and the sharing work in step 3 are independent. The remaining peer-discovery and sharing steps follow their respective predecessors.
