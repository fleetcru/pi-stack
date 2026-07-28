# Remote Pi Bridge Plan

## Purpose

Make it easy to run a Pi TUI on the VPS, view it from the laptop terminal, and expose the same session to Webby/Desktop through `pi-server` over Tailscale.

## Current working setup

```text
Laptop terminal
    │ Tailscale SSH
    ▼
Pi TUI on VPS
    │ external-session-bridge
    ▼
pi-server on 127.0.0.1:3141
    │ Tailscale Serve HTTPS
    ▼
Webby / Desktop / Companion
```

The VPS currently:

- Installs and authenticates Tailscale during cloud-init.
- Binds `pi-server` to `127.0.0.1:3141`.
- Publishes the service through Tailscale Serve.
- Keeps the public UFW rule for port `3141` disabled.
- Stores the Pi-server bearer token in `/root/pi-server-auth-token`.

## Actions needed now

### 1. Install the bridge on the VPS

The VPS needs a copy of:

```text
pi-server-exp/extensions/external-session-bridge.ts
```

The current manual installation is:

```bash
mkdir -p /root/.pi/agent/extensions
curl -fsSL https://raw.githubusercontent.com/fleetcru/pi-stack/main/pi-server-exp/extensions/external-session-bridge.ts \
  -o /root/.pi/agent/extensions/external-session-bridge.ts
```

Prefer a pinned release, commit, or checksum instead of an unpinned `main` URL.

### 2. Start a remote Pi TUI

```bash
cd /srv/projects
PI_EXTERNAL_RELAY_URL="http://127.0.0.1:3141" \
PI_EXTERNAL_RELAY_TOKEN="$(cat /root/pi-server-auth-token)" \
pi --extension /root/.pi/agent/extensions/external-session-bridge.ts
```

Verify from inside Pi:

```text
/bridge-status
```

Expected result:

```text
Bridge: connected
```

### 3. Access the VPS terminal from the laptop

```powershell
ssh -t root@pi-vps.taild0277.ts.net
```

The remote Pi TUI will render in the laptop terminal while running on the VPS.

## Recommended improvements

### A. Add a `pi-remote` wrapper

Create `/usr/local/bin/pi-remote` on the VPS so users do not need to remember environment variables or extension paths.

Responsibilities:

- Set `PI_EXTERNAL_RELAY_URL=http://127.0.0.1:3141`.
- Read the token from a protected file.
- Load the bridge extension.
- Choose a default project directory.
- Support `--session` for resuming an existing Pi session.
- Fail clearly if `pi-server`, Tailscale, or the token is unavailable.

Example usage:

```bash
pi-remote
pi-remote --session /path/to/session.jsonl
```

### B. Add a laptop-side SSH launcher

Add a PowerShell command/script in this repository that runs:

```powershell
ssh -t root@pi-vps.taild0277.ts.net pi-remote
```

This should become the normal laptop workflow.

### C. Provision the bridge from cloud-init

The cloud-init should install the bridge automatically instead of requiring a second manual step. Prefer:

1. A tagged, short-lived or narrowly scoped deployment artifact.
2. A pinned Git commit or release URL.
3. SHA-256 verification before installation.
4. File permissions of `0600` for any token-bearing configuration.

Do not store bearer tokens in repository files or public URLs.

### D. Improve bridge configuration

The bridge currently relies primarily on process environment variables. Improve this by:

- Supporting a root/user-readable config file with strict permissions.
- Never writing real tokens into diagnostic output.
- Clearly distinguishing missing configuration from connection failure.
- Showing the HTTP status and failure category in `/bridge-status`.
- Adding a `/bridge-reconnect` notification with the failed URL and status class, but never the token.

### E. Improve diagnostics

Add checks to the wrapper and documentation for:

```bash
systemctl is-active --quiet pi-server
curl --fail http://127.0.0.1:3141/healthz
systemctl is-active --quiet tailscaled
tailscale serve status
```

Bridge failures should distinguish:

- DNS/Tailscale connectivity failure.
- TLS or Tailscale Serve failure.
- HTTP `401` token failure.
- HTTP `403` origin failure.
- Missing or unloaded extension.
- Server-side registration failure.

### F. Make sessions survive disconnects

The TUI process currently owns the session. If SSH disconnects, the process may stop unless it is launched inside `tmux` or another supervisor.

Recommended workflow:

```bash
tmux new -As pi-remote
pi-remote
```

Document how to reconnect:

```bash
tmux attach -t pi-remote
```

### G. Clarify session ownership

Document this important limitation:

- A Pi TUI started on the VPS can be bridged to `pi-server`.
- A `pi-server`-managed child process cannot automatically be attached to by a separate local Pi TUI.
- To interact with a VPS session in a laptop terminal, SSH to the VPS and run the TUI there.
- Webby/Desktop/Companion can control a bridged VPS TUI through the bridge.

If local Pi must directly control a remote managed session, that requires a new remote-TUI/client protocol rather than a small bridge change.

## Security requirements

- Keep `pi-server` bound to localhost.
- Keep public UFW access to port `3141` disabled.
- Use Tailscale ACLs to limit access to the server.
- Keep `PI_SERVER_AUTH_TOKEN` enabled.
- Do not place Pi-server or Tailscale tokens in this repository.
- Rotate any token exposed in chat, shell history, logs, or cloud-init copies.
- Verify downloaded bridge code and server binaries where practical.

## Acceptance criteria

The bridge work is complete when:

- `ssh -t root@pi-vps.taild0277.ts.net pi-remote` starts a usable VPS Pi TUI.
- `/bridge-status` reports `Bridge: connected`.
- The session appears in Webby/Desktop.
- A prompt sent from Webby reaches the VPS TUI.
- A prompt entered in the VPS TUI appears in Webby/Desktop.
- The VPS remains inaccessible on public TCP port `3141`.
- Reconnecting SSH or restarting `pi-server` does not permanently orphan the session.
- No credentials appear in logs or repository files.
