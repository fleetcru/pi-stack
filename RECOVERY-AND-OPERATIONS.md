# Recovery and trusted-device operations

The daemon keeps recovery state in `PI_SERVER_DATA_DIR`:

- `events/`: bounded per-session event journals
- `idempotency.json`: existing short-lived request-key suppression
- `command-receipts.json`: accepted relay command IDs for retry responses
- `devices.json`: hashed per-device credentials
- `sessions.json`, `workers.json`, and remote-session state

The bootstrap value in `PI_SERVER_AUTH_TOKEN` remains valid for existing
clients and is required for device administration. Create a device with:

```http
POST /v1/devices
Authorization: Bearer <bootstrap-token>
Content-Type: application/json

{"name":"phone"}
```

The response contains the device token once. Store it securely and use it as
the Bearer token from that device. List devices with `GET /v1/devices` and
revoke one with `DELETE /v1/devices/{id}`. Revocation is persisted and takes
effect on the next request. Device tokens are never written in plaintext.

`GET /v1/diagnostics` reports uptime, session/worker/device counts, and event
journal disk usage. It intentionally does not expose tokens, command payloads,
or session contents.

Worker generations are advanced when a worker is added or updated. A remote
lifecycle subscription remembers the generation it started with and stops when
the worker configuration changes, preventing an old connection from releasing
capacity for a newer worker configuration.
