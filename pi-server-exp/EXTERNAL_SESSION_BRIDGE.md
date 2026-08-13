# External Pi TUI Bridge

This bridge makes a normal interactive Pi TUI session visible in pi-server, Webby, and Companion without starting a second Pi process for the same JSONL session.

## Run a bridged TUI session

Start pi-server-exp first, then in the terminal where you start interactive Pi:

```powershell
$env:PI_EXTERNAL_RELAY_URL = "http://127.0.0.1:3142"
# Set this too when PI_SERVER_AUTH_TOKEN is configured:
# $env:PI_EXTERNAL_RELAY_TOKEN = "your-server-token"

pi --extension "C:\Users\basin\Desktop\pi-stack\pi-server-exp\extensions\external-session-bridge.ts"
```

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
