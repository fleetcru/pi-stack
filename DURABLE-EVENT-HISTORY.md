# Durable daemon event history

Pi Stack keeps its existing live event ring and transport behavior, while also
persisting daemon-emitted session events under:

```
<PI_SERVER_DATA_DIR>/events/<safe-session-id>.jsonl
```

Each record contains the daemon event ID, timestamp, and original Pi event. On
session reconstruction, the server restores the last event ID and the bounded
replay window before accepting new events. This means a client reconnecting
after a daemon restart can continue using the existing `since` cursor instead
of silently restarting at event ID 1.

The journal is intentionally additive:

- WebSocket and SSE payloads are unchanged.
- Existing Pi JSONL transcript history remains the source for full message
  history.
- The journal is bounded using the existing `PI_SERVER_EVENT_HISTORY_MAX` and
  `PI_SERVER_EVENT_HISTORY_BYTES` settings.
- Writes are flushed with `fsync` before an event is published to subscribers.
- Old records are compacted atomically after the journal grows beyond twice the
  configured replay budget.
- A torn final record is ignored during startup so a crash does not make the
  session unavailable.

This is a durable replay journal, not yet a full event-sourced database. The
next safe extension would be durable command receipts keyed by client command
ID. That can provide idempotent command retry without changing existing
endpoints; it should be added before introducing cross-device conflict
resolution or multi-user authorization.


## Durable command idempotency

Existing clients may send `X-Idempotency-Key` on session commands. The daemon
now persists those keys in `<PI_SERVER_DATA_DIR>/idempotency.json` for the
existing 60-second window. A retry from a second paired device therefore
preserves the current duplicate-suppression behavior across a daemon restart.

The response shape and HTTP status are unchanged. This is deliberately a
deduplication receipt, not a replayable response store; future work can retain
the original accepted response if clients need that stronger guarantee.
