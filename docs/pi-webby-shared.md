# pi-webby-shared — file guide

The shared session-workspace package parses the authoritative slash-command list returned by each session's `get_commands` resource for the Webby and Desktop prompt bars. Commands not returned by the session are not fabricated; arbitrary custom command text remains pass-through prompt text. Expanded skill payloads remain hidden from displayed user bubbles, and assistant provider errors render as system timeline items.

TypeScript library (`@pi-stack/webby-shared`) containing everything the browser and desktop clients share: the API client, the WebSocket wrapper, React Query hooks, generated types, the Zustand store factory, and session-workspace timeline logic. `dist/` is build output; `src/` is the source of truth.

## `src/api/`

- `types.ts` — **generated** from pi-server's `openapi.json`. Do not hand-edit; regenerate after server API changes. Exports the `components["schemas"]` namespace the rest of the library builds on.
- `client.ts` — `PiServerClient`, a thin typed fetch wrapper over pi-server's REST API: capabilities, sessions CRUD, RPC/prompt, git status/commit/push, file tree/content, workers, daemon status, and WS-ticket requests. Exports re-typed aliases (`ApiSession`, `RpcCommand`, …) and `PiServerApiError` with status/code detail. Base URL defaults to `VITE_PI_SERVER_URL` then `localhost:3141`; token stays out of storage unless the caller opts in.
- `session-socket.ts` — `SessionSocket`, the ticket-authenticated WebSocket for one session. Handles: fetching a fresh single-use ticket per (re)connect, sending `since=<lastEventId>` so the server replays missed events, exponential reconnect backoff, status change callbacks (`idle/connecting/open/reconnecting/closed`), an `onGap` callback when the server cursor jumps (client must resync history), and clean teardown.
- `hooks.ts` — React Query hooks built on the client: `usePiServerClient` (from provider context), `useSessionData`, `useSessionHistory` (re-fetches on socket gaps), `useSessionEvents`, file/git hooks (`useFileTree`, `useSessionFileContent`, `useSessionGit*`), `useSchedulerStatus`, and `useActiveSessionSocket` which owns the long-lived `SessionSocket` for the selected session and fans events into the app store.
- `index.ts` — barrel export.

## `src/state/`

- `app-store.ts` — `createAppStore(storageKey)` factory returning a Zustand store with `persist` middleware. Holds multiple named server connections (URL, optional remembered token), the selected connection and session, per-session live state (socket status, latest event ID), and UI prefs (theme, panel sizes). Webby instantiates it as `"pi-webby-ui"`, desktop as its own key, so the two apps persist independently despite sharing the code.

## `src/session-workspace/`

- `types.ts` / `constants.ts` — shared item shapes and tuning constants for the chat timeline (streaming buffer sizes, dedup window).
- `timeline.ts` — pure functions that turn raw Pi events (`message_start/update/end`, `tool_execution_*`, `runtime_state`) into a flat timeline item list: assistant text accumulation, tool-call pairing with results, dedup by event ID, unique-key generation for list rendering, and extraction of model-provided effort capabilities. Map keys are preserved because null provider translations can still be valid Pi levels; options use stable semantic ordering. File-change events are intentionally omitted here because the web workspace presents them in its Changed files dropdown.
- `index.ts` — barrel export.
