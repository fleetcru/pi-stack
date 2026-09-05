# pi-desktop-app — file guide

Tauri v2 desktop client. The React side is a near-mirror of pi-webby (same shared library, same components copied/synced by `sync-components.ps1`), plus desktop-only Tauri integration. `src-tauri/target` and `dist` are build output.

## Rust shell (`src-tauri/`)

- `src-tauri/src/lib.rs` — Tauri builder: notification and shell plugins, debug logging, system tray (Show/Quit menu), window-close-to-tray behavior via `WindowEvent`.
- `src-tauri/src/main.rs` — calls `run()`.
- `tauri.conf.json` — window config, bundle targets, capability allowlist (notifications, shell, clipboard).
- `capabilities/` — Tauri v2 permission files granting the frontend access to plugins.

## React side (`src/`)

### App plumbing
- `main.tsx`, `App.tsx`, `init-shared.ts` — same structure as pi-webby: router entry, session-route synchronization into the store, shared init.
- `state/app-store.ts` — shared store factory with the `"pi-desktop-ui"` persistence key.
- `api/*` — re-exports of the shared client/hooks/socket/types; `provider.tsx` adds the React Query provider with shared cache defaults.
- `lib/utils.ts` — `cn()` helper.

### Desktop-specific hooks
- `hooks/use-notifications.ts` — OS notifications via the Tauri notification plugin; requests permission on first use.
- `hooks/use-session-notifications.ts` — watches a session's `runtime_state` and fires a native notification on `working → idle`, i.e. "Pi finished your task" while you're in another window.

### Components
- `components/workspace-shell.tsx`, `session-workspace.tsx`, `session-inspector.tsx`, `sidebar-tree.tsx`, `create-session-dialog.tsx`, `server-connections-dialog.tsx`, `machine-session-list.tsx`, `changed-files-list.tsx`, `capacity-control.tsx`, `theme-provider.tsx` — functionally identical to their pi-webby-exp counterparts (kept in sync by `sync-components.ps1/.sh`); see the webby doc for details.
- `components/ui/*` — shadcn/ui primitives, same set as webby.

## Sync discipline

Because webby and desktop share component sources, edits should be made once and propagated with the sync scripts at the repo root; diverging copies is the main maintenance hazard here.
