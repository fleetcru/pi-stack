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
		images := relayImages(command["images"])
		if message == "" && len(images) == 0 {
			return fmt.Errorf("message or images is required")
		}
		commandType, _ := command["type"].(string)
		if !t.external.enqueue(t.id, ExternalCommand{ID: NewSessionID(), Type: "prompt", Message: message, Images: images, Delivery: externalPromptDelivery(commandType)}) {
			return fmt.Errorf("relay is unavailable: session may be stale or stopped")
		}
		return nil
	case "set_model":
		provider, _ := command["provider"].(string)
		modelID, _ := command["modelId"].(string)
		if provider == "" || modelID == "" {
			return fmt.Errorf("provider and modelId are required")
		}
		if !t.external.enqueue(t.id, ExternalCommand{ID: NewSessionID(), Type: "set_model", Provider: provider, ModelID: modelID}) {
			return fmt.Errorf("relay is unavailable: session may be stale or stopped")
		}
		return nil
	case "set_thinking_level":
		level, _ := command["level"].(string)
		if level == "" {
			return fmt.Errorf("level is required")
		}
		if !t.external.enqueue(t.id, ExternalCommand{ID: NewSessionID(), Type: "set_thinking_level", Level: level}) {
			return fmt.Errorf("relay is unavailable: session may be stale or stopped")
		}
		return nil
	case "extension_ui_response":
		if err := validateExtensionUIResponseCommand(command); err != nil {
			return err
		}
		requestID, _ := command["id"].(string)
		if requestID == "" {
			return fmt.Errorf("id is required")
		}
		queued := ExternalCommand{
			ID:           NewSessionID(),
			Type:         "extension_ui_response",
			RequestID:    requestID,
			Value:        stringField(command, "value"),
			Selections:   relayStringSlice(command["selections"]),
			Comment:      stringField(command, "comment"),
			ResponseKind: stringField(command, "responseKind"),
		}
		if value, ok := command["cancelled"].(bool); ok {
			queued.Cancelled = &value
		}
		if value, ok := command["confirmed"].(bool); ok {
			queued.Confirmed = &value
		}
		accepted, conflict := t.external.enqueueUIResponse(t.id, requestID, queued)
		if !accepted {
			if conflict {
				return fmt.Errorf("%w: request %q is no longer pending", errExtensionUIRequestMismatch, requestID)
			}
			return fmt.Errorf("relay is unavailable: session may be stale or stopped")
		}
		t.external.publish(t.id, RPCEvent{"type": "extension_ui_closed", "id": requestID})
		return nil
	default:
		return fmt.Errorf("relay transport does not support %q; supported commands: prompt, steer, follow-up, abort, set_model, set_thinking_level, extension_ui_response", command["type"])
	}
}
func relayImages(value any) []any {
	images, ok := value.([]any)
	if !ok {
		return nil
	}
	return append([]any(nil), images...)
}

func relayStringSlice(value any) []string {
	switch values := value.(type) {
	case []string:
		return append([]string(nil), values...)
	case []any:
		out := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok && text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
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
		models := s.AvailableModels
		if len(models) == 0 && s.Model != nil {
			models = []any{s.Model}
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
