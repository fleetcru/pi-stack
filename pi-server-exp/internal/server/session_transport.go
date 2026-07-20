package server

import (
	"context"
	"fmt"
)

// SessionTransport is the server-facing contract for a live Pi session. The
// HTTP/WebSocket handlers must depend on this rather than whether Pi is a
// child RPC process or an interactive TUI connected through the relay.
type SessionTransport interface {
	Kind() string
	Start(context.Context) error
	Request(context.Context, RPCCommand) (RPCEvent, error)
	Send(RPCCommand) error
	Status() map[string]any
	SubscribeSince(uint64) (<-chan RPCEvent, []EventRecord, func())
}

type relayTransport struct {
	id       string
	external *ExternalRegistry
}

func (t relayTransport) Kind() string                { return "relay" }
func (t relayTransport) Start(context.Context) error { return nil }
func (t relayTransport) Status() map[string]any {
	if snap := t.external.stateSnapshot(t.id); snap != nil {
		return snap
	}
	return map[string]any{"running": false, "status": "stopped"}
}
func (t relayTransport) Send(command RPCCommand) error {
	switch command["type"] {
	case "abort":
		if !t.external.enqueue(t.id, ExternalCommand{ID: NewSessionID(), Type: "abort"}) {
			return fmt.Errorf("relay is unavailable: session may be stale or stopped")
		}
		return nil
	case "prompt", "steer", "follow_up":
		message, _ := command["message"].(string)
		if message == "" {
			return fmt.Errorf("message is required")
		}
		delivery := "steer"
		if command["type"] == "follow_up" {
			delivery = "followUp"
		}
		if !t.external.enqueue(t.id, ExternalCommand{ID: NewSessionID(), Type: "prompt", Message: message, Delivery: delivery}) {
			return fmt.Errorf("relay is unavailable: session may be stale or stopped")
		}
		return nil
	default:
		return fmt.Errorf("relay transport does not support %q; supported commands: prompt, steer, follow-up, abort", command["type"])
	}
}
func (t relayTransport) Request(_ context.Context, command RPCCommand) (RPCEvent, error) {
	switch command["type"] {
	case "get_state":
		snap := t.external.stateSnapshot(t.id)
		if snap == nil {
			return nil, fmt.Errorf("relay session not found")
		}
		return RPCEvent{"type": "response", "success": true, "command": "get_state", "data": map[string]any{"isStreaming": snap["running"], "model": snap["model"], "thinkingLevel": snap["thinkingLevel"], "external": true}}, nil
	case "get_available_models":
		s, ok := t.external.get(t.id)
		if !ok {
			return nil, fmt.Errorf("relay session not found")
		}
		models := []any{}
		if s.Model != nil {
			models = append(models, s.Model)
		}
		return RPCEvent{"type": "response", "success": true, "command": "get_available_models", "data": map[string]any{"models": models}}, nil
	default:
		return nil, fmt.Errorf("relay transport does not support %q", command["type"])
	}
}
func (t relayTransport) SubscribeSince(since uint64) (<-chan RPCEvent, []EventRecord, func()) {
	ch, replay, unsubscribe, ok := t.external.subscribe(t.id, since)
	if !ok {
		return make(chan RPCEvent), nil, func() {}
	}
	records := make([]EventRecord, 0, len(replay))
	for _, event := range replay {
		// Replay events from the external registry already embed _daemonEventId;
		// preserve it on the record so consumers never re-stamp it with 0.
		id, _ := event["_daemonEventId"].(uint64)
		records = append(records, EventRecord{ID: id, Event: event})
	}
	return ch, records, unsubscribe
}

func (s *Server) resolveSessionTransport(id string) (SessionTransport, bool) {
	if _, ok := s.external.get(id); ok {
		return relayTransport{id: id, external: s.external}, true
	}
	p, ok := s.getSession(id)
	if !ok {
		return nil, false
	}
	return p, true
}

func (p *PiProcess) Kind() string { return "rpc" }
