# External Pi TUI Bridge

This bridge makes a normal interactive Pi TUI session visible in pi-server, Webby, and Companion without starting a second Pi process for the same JSONL session.

When pi-server starts with `PI_SERVER_AUTH_TOKEN`, it updates `~/.pi/agent/bridge-config.json` with the detected server URL and token. The TUI bridge reads that file automatically. Use `PI_SERVER_BRIDGE_URL` to override the detected URL.

## Run a bridged TUI session

Start pi-server-exp first, then install the bridge configuration once:

```powershell
cd C:\Users\basin\Desktop\pi-stack
.\install-exp-external-bridge.ps1 -ServerPort 3142 -RelayUrl "http://127.0.0.1:3142" -AuthToken "your-server-token"
```

This copies the extension to Pi's user extensions directory and writes
`~/.pi/agent/bridge-config.json` with the relay URL and token. New Pi TUI
sessions can then start normally. The config file contains the bearer token,
so keep the `.pi` directory private. The server executable also refreshes this
same file on every authenticated startup.

For a one-command server startup plus bridge setup:

```powershell
.\start-exp-server.ps1 -Port 3142 -AuthToken "your-server-token" -InstallExternalBridge
```

You can also run `/bridge-register` in Pi. It asks for the relay URL and then the optional bearer token. Leave the token prompt blank only when the server has no `PI_SERVER_AUTH_TOKEN`.

The extension registers the current Pi JSONL session with:

```text
POST /v1/external-sessions/register
```

and forwards message/tool events. Webby and Companion discover it through their normal session list within the next refresh.

## Remote controls currently bridged

Webby/Companion can send:

- a normal prompt;
- a steer message;
- a follow-up message.

The extension receives commands over WebSocket (with HTTP polling fallback) and injects them through Pi's `sendUserMessage` API. Commands remain queued until the extension confirms that an idle prompt actually appeared in Pi or safely queued it into an active turn. It forwards user, assistant, tool, and agent lifecycle events back to clients.

## Important limitations

- The terminal Pi TUI remains the process owner. pi-server does not restart it.
- The bridge retries registration/events after pi-server restarts and marks a relay stale after 20 seconds without a heartbeat.
- One Pi process must own a JSONL session. Machine Session Discovery now returns the live relay instead of starting a second RPC process when a bridged TUI already owns that history file.
- If an old server already started an RPC process for the same JSONL file, stop that managed session and use the relay entry before continuing; concurrent Pi processes do not synchronize live state and can corrupt or overwrite history.
- Prompt, steer, follow-up, abort, model/thinking changes, and structured extension UI responses are bridged. Compact is not currently bridged.
- The bridge is authenticated by the normal pi-server bearer middleware. Keep the relay URL on a trusted LAN/Tailscale network.

## For an already-running TUI

Pi must load the extension before it can relay. If the TUI supports `/reload` and the extension is installed in Pi's user extensions directory, reload it; otherwise restart that TUI with the command above. A process cannot be externally bridged without the extension running inside it.
