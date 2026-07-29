# Durable Remote Session Recovery — Implementation Plan

## 1. Goal

Make bridged VPS Pi sessions survive:

- `pi-server` restarts
- SSH disconnects
- Tailscale interruptions
- TUI bridge reconnects

Queued commands must remain available and be delivered after reconnection.

The result should feel like a durable remote session rather than a temporary WebSocket connection.

---

## 2. Current Foundation

The stack already provides several useful building blocks:

- External session registration
- Stable session IDs derived from Pi session files
- Relay leases and generation counters
- Persistent command storage
- WebSocket reconnect backoff
- HTTP polling fallback
- In-memory event replay ring
- Command acknowledgements

The main missing capability is durable external-session metadata. The server currently treats relay connections as temporary and removes stale relay session specs during startup.

The implementation should extend the existing relay design rather than replace it.

---

## 3. Scope

### In scope

- Durable external-session identity and metadata
- Offline/reconnecting/connected lifecycle states
- Automatic bridge reconnection after server restart
- Durable command queue delivery
- Lease and duplicate-connection protection
- Client connection-state UX
- Restart, reconnect, and delivery tests

### Out of scope for the first version

- Starting or supervising the Pi TUI from `pi-server`
- Attaching a local TUI to a server-managed child process
- Full event-stream persistence
- Multi-user authorization beyond the existing server authentication model
- Replacing JSON persistence with a database

---

## 4. Durable Session Model

Separate the durable Pi session from the live relay connection.

### Durable session

Represents the Pi session itself:

```text
external session ID
Pi session path
working directory
session title
server identity
creation time
last seen time
last event ID
lifecycle status
```

### Live relay connection

Represents the current bridge connection:

```text
bridge ID
lease ID
lease generation
connected time
last heartbeat
latency
WebSocket state
```

A lost relay connection must not delete the durable session.

### Lifecycle states

```text
offline
reconnecting
connected
working
waiting_for_input
stopped
expired
```

A session remains visible to clients while it is offline or reconnecting.

---

## 5. Durable Storage

Add a metadata store:

```text
PI_SERVER_DATA_DIR/external-sessions.json
```

Example record:

```json
{
  "id": "external-project-session-abc",
  "cwd": "/srv/projects",
  "title": "VPS Pi",
  "sessionPath": "/home/pi/.pi/sessions/project/session.jsonl",
  "status": "offline",
  "createdAt": "2026-07-29T10:00:00Z",
  "lastSeenAt": "2026-07-29T10:15:00Z",
  "lastEventId": 428
}
```

Keep the existing command store separate:

```text
external-sessions.json
relay-commands.json
```

### Storage requirements

- Atomic temporary-file plus rename writes
- `0600` file permissions
- Corrupt-store preservation for manual recovery
- Versioned storage format
- Bounded metadata size
- Clear load/save errors in logs

Potential implementation files:

```text
internal/server/external_session_store.go
internal/server/external_session_store_test.go
```

---

## 6. Server Startup Recovery

Update `internal/server/server.go` and `external_sessions.go`.

Current behavior removes stale relay session specs during startup. Replace that behavior with:

1. Load durable external-session records.
2. Mark previously connected sessions as `offline`.
3. Preserve their IDs and metadata.
4. Load pending commands from the existing command store.
5. Make sessions visible to clients.
6. Wait for the bridge to reclaim each session.

Do not mark a session `connected` until a live bridge completes registration successfully.

A stale session should eventually become `expired` only after a configurable retention period, not immediately after a server restart.

---

## 7. Reconnect Handshake

Extend the existing `/v1/external-sessions/register` endpoint rather than replacing it.

The bridge should register with the same stable session ID and provide recovery information:

```json
{
  "id": "external-project-session-abc",
  "cwd": "/srv/projects",
  "title": "VPS Pi",
  "sessionPath": "/home/pi/.pi/sessions/project/session.jsonl",
  "bridgeId": "bridge-instance-123",
  "lastEventId": 428,
  "resume": true
}
```

The server responds with:

```json
{
  "sessionId": "external-project-session-abc",
  "lease": "lease-token",
  "status": "reconnected",
  "pendingCommandCount": 3,
  "eventsAvailableSince": 429
}
```

The response should indicate whether the registration created a new session or resumed an existing one.

### Backwards compatibility

Older bridge versions that do not send `bridgeId`, `lastEventId`, or `resume` should continue to register using the existing behavior.

---

## 8. Stable Bridge Identity

Update `external-session-bridge.ts` so its identity remains stable across bridge reconnects and Pi restarts.

Preferred identity sources:

1. Explicit reservation/session ID
2. Stable Pi session file identity
3. Persisted bridge identity file

Do not use `process.pid` as the long-term session identity because it changes after every process restart.

The bridge should preserve:

```text
session ID
bridge ID
last known event ID
```

The bridge must never persist or display bearer tokens in diagnostic output.

---

## 9. Durable Command Delivery

The existing command persistence is the foundation for this feature. Extend commands with explicit lifecycle fields:

```json
{
  "id": "command-123",
  "type": "prompt",
  "message": "Run the tests",
  "status": "queued",
  "createdAt": "2026-07-29T10:20:00Z",
  "attempts": 0,
  "expiresAt": "2026-07-30T10:20:00Z"
}
```

### Delivery flow

1. Client submits a command.
2. Server assigns a unique command ID.
3. Server persists the command before responding.
4. Server returns `202 Accepted` with the command ID.
5. Bridge receives the command.
6. Bridge sends it to Pi.
7. Bridge sends a delivery receipt.
8. Server marks the command as delivered.
9. Server removes or archives it after acknowledgement.

### Delivery guarantee

Exactly-once delivery cannot be guaranteed if Pi accepts a message and the TUI crashes before acknowledging it.

The first implementation should guarantee:

```text
at-least-once delivery with command IDs and deduplication where possible
```

Do not claim exactly-once behavior unless Pi provides durable idempotency support.

### Queue limits

Add configurable limits:

```text
Maximum commands per session: 100
Maximum command size: 1 MiB
Maximum command age: 24 hours
```

Abort commands should remain privileged to bypass a full queue, as in the existing implementation.

Expired or rejected commands must be visible to the sender rather than silently discarded.

---

## 10. Lease and Duplicate-Connection Protection

Reuse the existing lease-generation mechanism.

When a new bridge connects:

1. Authenticate the bridge.
2. Validate the session ID.
3. Create a new relay generation.
4. Invalidate the old relay.
5. Preserve pending commands.
6. Attach the new relay.
7. Deliver commands only to the current generation.

If two TUIs attempt to claim the same session, return a clear conflict unless the new bridge is explicitly replacing the old one.

Example response:

```text
409 external session is already claimed
```

Old relay callbacks must not be allowed to:

- Acknowledge new commands
- Change current connection state
- Replace the active lease
- Delete the durable session

---

## 11. Event Recovery

The current event ring is in memory and will be lost on a server restart. Do not persist the complete event stream in the first version.

After reconnect:

1. Server detects that the requested event cursor is unavailable.
2. Server sends `events_lost` or `reconcile_required`.
3. Client refetches authoritative state through HTTP.
4. Client refetches messages, stats, and last assistant text as needed.
5. Client resets its event cursor.
6. Live WebSocket delivery resumes.

Possible future persisted event types:

```text
message_start
message_end
tool execution summary
bridge receipt
session status changes
```

Persisting only important events avoids unbounded event-store growth.

---

## 12. Bridge Reconnection Behavior

Update:

```text
pi-server-exp/extensions/external-session-bridge.ts
```

The bridge should:

- Re-register automatically after HTTP failure
- Reconnect WebSocket using exponential backoff and jitter
- Refresh its lease after server restart
- Report `offline`, `reconnecting`, and `connected`
- Flush local pending events after reconnect
- Use HTTP polling when WebSocket authentication or transport fails
- Stop retrying cleanly when Pi exits
- Never log relay tokens

Recommended backoff:

```text
1s, 2s, 4s, 8s, 16s, 30s maximum
```

Reset the backoff after a successful registration and healthy WebSocket connection.

---

## 13. API Changes

Possible additions:

```text
GET    /v1/external-sessions
GET    /v1/external-sessions/{id}
POST   /v1/external-sessions/{id}/commands
GET    /v1/external-sessions/{id}/commands
POST   /v1/external-sessions/{id}/claim
POST   /v1/external-sessions/{id}/release
POST   /v1/external-sessions/{id}/reconcile
DELETE /v1/external-sessions/{id}
```

Existing endpoints remain supported:

```text
POST /v1/external-sessions/register
GET  /v1/external-sessions/{id}/commands
POST /v1/external-sessions/{id}/ack
GET  /v1/external-sessions/relay/{id}
```

Update the server OpenAPI document and regenerate client types. Do not hand-edit generated client types.

---

## 14. Client UX

Update Webby, Desktop, and Android to show durable relay states.

### Session list states

```text
VPS Pi
Offline — waiting for bridge
```

```text
VPS Pi
Reconnecting...
```

```text
VPS Pi
Connected
```

### Queued command behavior

When a user sends a command while offline:

```text
Pi is offline. Your message will be delivered when it reconnects.
```

Display:

- Pending command count
- Last seen timestamp
- Relay latency
- Reconnect button
- Retry failed command
- Cancel queued command
- Expiration state
- Delivery receipt

When the bridge reconnects:

```text
3 queued messages delivered
```

---

## 15. Security and Authorization

All reservation, claim, command, and release endpoints must use the existing server authentication.

For the current single-user architecture, the bearer token is sufficient. If multi-user support is added later, reservations must include an owner/device identity.

Requirements:

- Do not place tokens in URLs.
- Do not write tokens to logs.
- Validate all session IDs and command IDs.
- Limit message and queue sizes.
- Prevent arbitrary clients from claiming another user’s reservation.
- Bind claims to the configured server.
- Audit creation, claims, releases, and command delivery.

---

## 16. Testing Plan

### Server unit tests

Add tests for:

- External-session persistence
- Restart loading
- Offline-to-connected transitions
- Reconnect using the same ID
- Lease rotation
- Duplicate claim rejection
- Queued command replay
- Command acknowledgement
- Expired commands
- Corrupt-store recovery
- Atomic persistence
- Event reconciliation after restart

### Server integration test

Test the complete flow:

```text
register
→ disconnect
→ restart server
→ reconnect bridge
→ send queued command
→ acknowledge command
```

Also test concurrent:

- Command enqueue
- Bridge reconnect
- Lease rotation
- Command acknowledgement

Run:

```bash
cd pi-server-exp
go test ./... -race -count=3
go vet ./...
```

### Bridge tests

Test:

- Stable identity
- Registration retry
- Backoff behavior
- WebSocket reconnect
- HTTP polling fallback
- Duplicate command protection
- Server restart recovery
- Token redaction

### Client tests

Test:

- Offline session rendering
- Reconnecting state
- Queued command display
- Delivered receipts
- Failed and expired commands
- Reconciliation after `events_lost`

---

## 17. Implementation Phases

### Phase 1 — Durable metadata

- Add external-session store.
- Persist session metadata on registration.
- Stop deleting stale external sessions on startup.
- Mark sessions offline after restart.
- Add persistence tests.

### Phase 2 — Reconnect handshake

- Extend registration payload.
- Preserve stable session IDs.
- Restore relay leases.
- Mark sessions connected after successful claim.
- Add reconnect tests.

### Phase 3 — Durable command lifecycle

- Add command status fields.
- Persist commands before returning success.
- Replay pending commands after reconnect.
- Add delivery receipts and expiration.
- Add queue limits.

### Phase 4 — Client UX

- Add connection-state indicators.
- Show pending command count.
- Add retry and cancel controls.
- Add reconciliation handling.

### Phase 5 — Operational hardening

- Add cleanup of expired records.
- Add metrics and diagnostics.
- Add storage migrations.
- Add release notes and operator documentation.

---

## 18. Acceptance Criteria

The feature is complete when:

1. A VPS Pi TUI registers normally.
2. Webby can send commands to it.
3. `pi-server` is restarted.
4. The same session remains visible with the same ID.
5. The bridge reconnects automatically.
6. Commands sent while offline are persisted.
7. Queued commands are delivered after reconnect.
8. Commands are not acknowledged before Pi accepts them.
9. Duplicate bridge connections cannot both control the session.
10. Clients show accurate offline, reconnecting, and connected states.
11. Server restarts do not lose session metadata or queued commands.
12. Existing live bridge behavior remains backwards compatible.
