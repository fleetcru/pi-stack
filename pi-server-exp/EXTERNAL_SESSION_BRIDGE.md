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

The extension polls the server every 750 ms and injects the command with Pi's `sendUserMessage` API. Commands remain queued until the extension acknowledges injection, preventing loss when pi-server or the bridge restarts. It emits the resulting user message and assistant/tool stream back to the clients.

## Important limitations

- The terminal Pi TUI remains the process owner. pi-server does not restart it.
- The bridge retries registration/events after pi-server restarts and marks a relay stale after 20 seconds without a heartbeat.
- Do not also resume the same session through Machine Session Discovery while the bridged TUI is running.
- Abort, compact, model changes, and extension UI responses are intentionally not bridged yet; they require explicit matching Pi extension APIs and will be added after prompt flow is proven.
- The bridge is authenticated by the normal pi-server bearer middleware. Keep the relay URL on a trusted LAN/Tailscale network.

## For an already-running TUI

Pi must load the extension before it can relay. If the TUI supports `/reload` and the extension is installed in Pi's user extensions directory, reload it; otherwise restart that TUI with the command above. A process cannot be externally bridged without the extension running inside it.
