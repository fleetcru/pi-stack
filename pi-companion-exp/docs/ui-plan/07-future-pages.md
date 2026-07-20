# 07 Future Implementation Plan

## Home

Later real data:
- daemon health
- active sessions count
- connected workers
- recent errors
- quick session resume

## Sessions

Later real data:
- `GET /v1/sessions`
- create session
- stop session
- resume session
- open session detail

## Session Detail

Later real data:
- WebSocket: `/v1/sessions/{id}/ws?since=lastEventId`
- send prompt: `POST /v1/sessions/{id}/send`
- steer via convenience/RPC endpoint
- abort/cancel when endpoint exists

Display:
- assistant messages
- user messages
- tool calls
- file changes
- daemon events

## Workers

Later real data:
- list workers
- worker health
- capacity
- create remote session
- show Tailscale/private IP host

## Settings

Later real data:
- DataStore persistence
- server URL
- bearer token
- last event IDs by session
- default cwd root
- WebSocket reconnect preferences
