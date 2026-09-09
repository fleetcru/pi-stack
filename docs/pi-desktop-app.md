# pi-desktop-app — file guide

Tauri v2 desktop client. The React side is a near-mirror of pi-webby (same shared library, same components copied/synced by `sync-components.ps1`), plus desktop-only Tauri integration. `src-tauri/target` and `dist` are build output.

## Rust shell (`src-tauri/`)

- `src-tauri/src/lib.rs` — Tauri builder: notification and shell plugins, debug logging, system tray (Show/Quit menu), hidden-window cold-start reveal after the React `pi-app-ready` event, and window-close-to-tray behavior via `WindowEvent`.
- `src-tauri/src/main.rs` — calls `run()`.
- `tauri.conf.json` — window config, bundle targets, capability allowlist (notifications, shell, clipboard).
- `capabilities/` — Tauri v2 permission files granting the frontend access to plugins.

## React side (`src/`)

### App plumbing
- `main.tsx`, `App.tsx`, `init-shared.ts` — same structure as pi-webby: router entry, session-route synchronization into the store, shared init. `main.tsx` emits `pi-app-ready` after the first animation frame so the native shell can reveal the initially hidden main window without showing the WebView2 startup frame.
- `state/app-store.ts` — shared store factory with the `"pi-desktop-ui"` persistence key.
- `api/*` — re-exports of the shared client/hooks/socket/types; `provider.tsx` adds the React Query provider with shared cache defaults.
- `lib/utils.ts` — `cn()` helper.

### Desktop-specific hooks
- `hooks/use-notifications.ts` — OS notifications via the Tauri notification plugin; requests permission on first use.
- `hooks/use-session-notifications.ts` — watches a session's `runtime_state` and fires a native notification on `working → idle`, i.e. "Pi finished your task" while you're in another window.

### Components
- `components/workspace-shell.tsx` — the workspace reveals after the first React frame; worker, global-session, and machine-session inventory queries start on the next frame so health and primary sessions can render first.
- `quick-session-composer.tsx`, `session-workspace.tsx`, `session-inspector.tsx`, `sidebar-tree.tsx`, `create-session-dialog.tsx`, `server-connections-dialog.tsx`, `worker-management-dialog.tsx`, `machine-session-list.tsx`, `changed-files-list.tsx`, `capacity-control.tsx`, `theme-provider.tsx` — functionally aligned with their pi-webby-exp counterparts; see the webby doc for details. Home navigation, the roomier session tree, worker management, worker-aware project roots, and the searchable model picker are carried across to desktop. Desktop keeps the same first-server onboarding as webby with a one-click local option, but its local address is `http://127.0.0.1:3142` rather than the browser hostname, and connection hints use port `3142`.
- `components/ui/*` — shadcn/ui primitives, same set as webby.

## Sync discipline

Because webby and desktop share component sources, edits should be made in webby first and then propagated to desktop. The root sync scripts cover shared `components/ui` primitives only; app-level components such as `workspace-shell.tsx`, `sidebar-tree.tsx`, and `quick-session-composer.tsx` must be copied or merged separately while preserving desktop-only behavior.
