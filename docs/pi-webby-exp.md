# pi-webby-exp — file guide

React 19 + Vite 8 + Tailwind v4 + shadcn/ui browser client for pi-server. Most data logic lives in `pi-webby-shared`; this package is the UI shell and app-specific wiring.

## App plumbing

- `src/main.tsx` — Vite entry: mounts React, router, query client, theme provider.
- `src/App.tsx` — routes. `/session/:id` synchronizes the URL into the app store inside `useLayoutEffect` so a refresh never flashes the previously persisted session before paint.
- `src/init-shared.ts` — shared-library init (fetch baseline, error reporting hooks) run before components render.
- `src/state/app-store.ts` — instantiates the shared store factory with the `"pi-webby-ui"` persistence key and re-exports `useAppStore`.
- `src/lib/utils.ts` — `cn()` class-name helper (shadcn convention).

## API layer

- `src/api/*` — thin re-exports of `@pi-stack/webby-shared/api` (client, hooks, socket, types) plus `provider.tsx`, which supplies the configured `PiServerClient` to the React tree for the currently selected server connection.

## Major components (`src/components/`)

- `workspace-shell.tsx` — the main three-panel layout: sidebar (session tree), center workspace, right inspector. Manages panel resizing, collapse states, the command palette, theme toggle, and navigation between sessions. The sidebar brand and workspace header provide direct home actions, so returning to the quick-session composer never depends on browser history. The worker-management button opens its dialog on desktop and mobile. Heavy workspace components are lazy-loaded.
  Before connecting, its empty state offers one-click local server setup. After connecting with no session selected, it centers the quick-session composer.
- `session-workspace.tsx` — the chat view. Subscribes to the active session socket, renders streaming assistant text, tool-call cards (bash, read, edit…), message markdown (react-markdown + sanitize), image attachments, prompt bar with model/thinking selectors, and stop/retry actions.
- `session-inspector.tsx` — right panel: session details, file tree browser with file preview, git status/branches/worktrees, and the guided commit/push flow (`resolveGitQuickAction` computing Commit → Commit & push → Push → blocked, with the GitHub compare link when `githubRepo` is known).
- `sidebar-tree.tsx` — grouped session list (local, remote, relay, machine-discovered) with search and per-session context menus. Worker, project, and session rows use roomier targets, project session counts, clearer selection, and accessible hover/focus pin controls.
- `quick-session-composer.tsx` — compact empty-workspace composer. It uses a wide, low-profile prompt card with a searchable, provider-grouped model picker, a thinking/effort selector, and send button. Inline controls below the card select an allowed project root and local or remote worker; roots, models, and effort apply to the selected worker. "More options" opens the full dialog for worktrees and batch-session settings.
- `create-session-dialog.tsx` — full new-session form: cwd picker, args, worktree toggle, extension options.
  The form also supports an optional initial message, sent immediately after creation so sessions can run in the background.
- `server-connections-dialog.tsx` — manage multiple pi-server connections (name, URL, token, remember-token) backed by the persisted store. Connection hints use the shared `3142` server port.
  The dialog presents active-server highlighting, an empty state, inline editing, and a two-step remove confirmation.
- `worker-management-dialog.tsx` — manage remote workers through the server worker CRUD API, including add, edit, health/status refresh, and confirmed removal; the local worker is immutable.
- `server-admin-page.tsx` — full server administration route (\`/admin\`), mirrored with Desktop. It switches among configured servers and manages overview/warnings, all persisted settings (live vs restart-required), remote workers, and trusted-device credentials through the bearer-authenticated \`/v1/admin/*\`, \`/v1/workers/*\`, and \`/v1/devices/*\` APIs.
- `machine-session-list.tsx` — browse Pi sessions discovered on the machine but not managed by the server; can attach/adopt them.
- `changed-files-list.tsx` — compact list of git working-tree changes used in the inspector and commit dialog.
- `capacity-control.tsx` — admin UI for adjusting server capacity limits at runtime (talks to the admin settings endpoint).
- `theme-provider.tsx` — light/dark/system theme with persistence.

## Hooks

- `src/hooks/use-image-attachments.ts` — pasting/selecting images for prompts: encodes to base64, tracks pending attachments, cleanup on send.
- `src/hooks/use-mobile.ts` — media-query helper for responsive panel behavior.

## UI kit

- `src/components/ui/*` — shadcn/ui primitives (button, dialog, dropdown, tabs, sidebar, message/message-scroller/bubble for chat, etc.). Generated and customized via the shadcn CLI; they are presentational and rarely edited by hand.

## Verification

```bash
pnpm typecheck && pnpm lint && pnpm test && pnpm build
```
