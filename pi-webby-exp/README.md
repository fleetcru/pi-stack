# React + TypeScript + Vite + shadcn/ui

This is a template for a new Vite project with React, TypeScript, and shadcn/ui.

## pi-server integration

The UI-independent API layer lives in `src/api/`:

- `PiServerClient` provides typed REST access to local and remote sessions, workers, metadata, raw Pi RPC commands, and all convenience endpoints.
- `SessionSocket` obtains a short-lived WebSocket ticket for each connection and reconnect, then resumes the event stream using `_daemonEventId`.
- `src/api/types.ts` is generated from the daemon OpenAPI schema; do not hand-edit it.

By default the client connects to `http://127.0.0.1:3141`. Override this at build time with:

```text
VITE_PI_SERVER_URL=http://127.0.0.1:3141
```

For a browser UI served from Vite, start pi-server with the Vite development origin in `PI_SERVER_ALLOWED_ORIGINS`, for example `http://127.0.0.1:5173`. If bearer auth is enabled, pass the token to `PiServerClient` at runtime. Do not put a production token in a `VITE_` variable because it is bundled into browser JavaScript.

## Adding components

To add components to your app, run the following command:

```bash
npx shadcn@latest add button
```

This will place the ui components in the `src/components` directory.

## Using components

To use the components in your app, import them as follows:

```tsx
import { Button } from "@/components/ui/button"
```
