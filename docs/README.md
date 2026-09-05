# Pi Stack — Codebase Guide

These docs explain what each major file in the repository does, project by project. They complement the root `AGENTS.md` (which focuses on architecture decisions, event formats, and pitfalls) by walking through individual files.

## The stack in one paragraph

`pi-server-exp` is the hub: a Go daemon that spawns and manages Pi CLI processes, exposes them over HTTP/WebSocket/SSE, and relays traffic to remote "worker" machines. Three clients talk to it: `pi-webby-exp` (browser), `pi-desktop-app` (Tauri desktop), and `pi-companion-exp` (Android). `pi-webby-shared` holds the TypeScript API client and state code that webby and desktop both reuse. `pi-server-tray` is a small Windows system-tray companion for managing the server.

## Docs

| Doc | Project |
|---|---|
| [pi-server-exp.md](pi-server-exp.md) | Go hub daemon — RPC, sessions, WebSocket, workers, relays |
| [pi-webby-shared.md](pi-webby-shared.md) | Shared TypeScript client library |
| [pi-webby-exp.md](pi-webby-exp.md) | React browser client |
| [pi-desktop-app.md](pi-desktop-app.md) | Tauri desktop client |
| [pi-companion-exp.md](pi-companion-exp.md) | Android (Kotlin/Compose) client |
| [scripts-and-extensions.md](scripts-and-extensions.md) | Pi extensions, tray utility, root scripts |

## Where to start reading

- Want to understand the event flow? Read `pi-server-exp.md` sections on `rpc.go`, `ws_handler.go`, and `session_inventory.go`, then the AGENTS.md "Pi Event Formats" section.
- Want to work on a client? Start with the shared library doc, then the client doc for your platform.
- All clients follow the same pattern: HTTP for commands, WebSocket or SSE for live events, dedup by server-issued monotonic event IDs.
