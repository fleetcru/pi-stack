# Pi Companion server wiring plan

> Historical implementation plan. Networking is now implemented. Use the server OpenAPI document and current client code for endpoint and payload details.

Goal: document the phased replacement of mock UI data with real `pi-server` REST and WebSocket data.

Non-goals for first wiring pass:
- no public internet hardening
- no account system
- no Git write/checkpoint operations
- no push notifications

Assumptions:
- server runs on a private/local/Tailscale network
- Android app talks to `pi-server` over HTTP and WebSocket
- settings store server URL and optional bearer token
- mock UI remains useful as preview/fallback data

Recommended package layout:

```text
app/src/main/java/com/example/picompanion/
├── data/
│   ├── api/
│   ├── model/
│   ├── repository/
│   ├── settings/
│   └── websocket/
└── ui/
```

Implementation order:

1. Settings persistence
2. API client foundation
3. Session list from server
4. Session detail WebSocket stream
5. Sending prompts
6. Workers page
7. Robust state/error handling
8. Tests and manual test checklist
