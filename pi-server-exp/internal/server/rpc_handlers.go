package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) sessionPost(w http.ResponseWriter, r *http.Request) {
	id, action := splitSessionPath(r.URL.Path)
	if action == "ui-response" {
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	} else if action == "send" || action == "command" {
		r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	}
	if s.proxyRemoteSession(w, r, id, action) {
		return
	}
	if strings.HasPrefix(action, "git/") {
		s.gitHandler(w, r)
		return
	}
	if _, external := s.external.get(id); external {
		if action == "abort" {
			command := ExternalCommand{ID: NewSessionID(), Type: "abort"}
			if !s.external.enqueue(id, command) {
				writeErrorText(w, http.StatusBadGateway, "relay is unavailable")
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "commandId": command.ID, "delivery": "queued"})
			return
		}
		if action == "prompt" || action == "steer" || action == "follow-up" {
			var body struct {
				Message string `json:"message"`
				Images  []any  `json:"images"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if body.Message == "" && len(body.Images) == 0 {
				writeErrorText(w, http.StatusBadRequest, "message or images is required")
				return
			}
			idemKey := ""
			if headerKey := r.Header.Get("X-Idempotency-Key"); headerKey != "" {
				idemKey = id + ":" + headerKey
				if receipt, ok := s.receipts.get(idemKey); ok {
					writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "idempotent": true, "commandId": receipt.CommandID})
					return
				}
				if s.idempotencySeen(idemKey) {
					writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "idempotent": true})
					return
				}
			}
			admitted := action == "prompt"
			if admitted && !s.acquireDistributedRun(r.Context(), id, "relay:"+id) {
				writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "hub run capacity is busy or this relay already has an active run", "scheduler": s.admission.Snapshot()})
				return
			}
			if admitted {
				s.setDistributedRunMetadata(id, "relay", "")
			}
			command := ExternalCommand{ID: NewSessionID(), Type: "prompt", Message: body.Message, Images: body.Images, Delivery: externalPromptDelivery(action)}
			if !s.external.enqueue(id, command) {
				if admitted {
					s.releaseDistributedRun(id)
				}
				writeErrorText(w, http.StatusBadGateway, "relay is unavailable")
				return
			}
			if idemKey != "" {
				s.recordIdempotency(idemKey)
				s.receipts.put(idemKey, command.ID, idempotencyTTL)
			}
			writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "commandId": command.ID, "delivery": "queued"})
			return
		}
		if action == "model" || action == "thinking-level" {
			command, _, err := commandFromBody(action, r)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			transport := relayTransport{id: id, external: s.external}
			if err := transport.Send(command); err != nil {
				writeError(w, http.StatusBadGateway, err)
				return
			}
			writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "delivery": "queued"})
			return
		}
		if action == "ui-response" {
			var body struct {
				ID           string   `json:"id"`
				Cancelled    *bool    `json:"cancelled"`
				Value        string   `json:"value"`
				Confirmed    *bool    `json:"confirmed"`
				Selections   []string `json:"selections"`
				Comment      string   `json:"comment"`
				ResponseKind string   `json:"responseKind"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			if err := validateExtensionUIResponseFields(body.ID, body.Value, body.Comment, body.Selections, body.ResponseKind); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
			command := ExternalCommand{
				ID:           NewSessionID(),
				Type:         "extension_ui_response",
				RequestID:    body.ID,
				Cancelled:    body.Cancelled,
				Value:        body.Value,
				Confirmed:    body.Confirmed,
				Selections:   body.Selections,
				Comment:      body.Comment,
				ResponseKind: body.ResponseKind,
			}
			accepted, conflict := s.external.enqueueUIResponse(id, body.ID, command)
			if !accepted {
				if conflict {
					writeErrorText(w, http.StatusConflict, "extension UI request is no longer pending")
				} else {
					writeErrorText(w, http.StatusBadGateway, "relay is unavailable")
				}
				return
			}
			// Close all client dialogs as soon as the durable response command is
			// accepted. The bridge emits the same close after tool completion.
			s.external.publish(id, RPCEvent{"type": "extension_ui_closed", "id": body.ID})
			writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "commandId": command.ID, "delivery": "queued"})
			return
		}
		writeErrorText(w, http.StatusBadRequest, "external session control not supported")
		return
	}
	if action == "metadata" {
		s.updateSessionMetadata(w, r, id)
		return
	}
	p, ok := s.getSession(id)
	if !ok {
		writeErrorText(w, http.StatusNotFound, "session not found")
		return
	}
	// Idempotency: if the client sends X-Idempotency-Key, check whether we
	// already processed this request. Reject duplicates within a 60s window.
	if idemKey := r.Header.Get("X-Idempotency-Key"); idemKey != "" {
		if s.checkIdempotency(id + ":" + idemKey) {
			writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true, "idempotent": true})
			return
		}
	}
	if err := s.ensureSessionCapacity(p); err != nil {
		writeError(w, http.StatusTooManyRequests, err)
		return
	}
	switch action {
	case "command":
		s.handleRawCommand(w, r, p)
	case "send":
		s.handleRawSend(w, r, p)
	case "prompt", "steer", "follow-up", "abort", "compact", "bash", "ui-response",
		"model", "cycle-model", "thinking-level", "cycle-thinking-level",
		"steering-mode", "follow-up-mode", "auto-compaction", "auto-retry", "abort-retry",
		"switch", "fork", "clone", "new", "name", "export-html", "abort-bash":
		s.handleConvenienceCommand(w, r, p, action)
	default:
		http.NotFound(w, r)
	}
}

func externalPromptDelivery(action string) string {
	switch action {
	case "steer":
		return "steer"
	case "follow-up", "follow_up":
		return "followUp"
	default:
		// A normal prompt should start a turn immediately when the TUI is idle.
		// The bridge selects steer only when Pi reports an active turn.
		return "prompt"
	}
}

func (s *Server) sessionGet(w http.ResponseWriter, r *http.Request) {
	id, action := splitSessionPath(r.URL.Path)
	if action == "summary" {
		s.sessionSummary(w, r, id)
		return
	}
	if action == "files/content" {
		s.sessionFileContent(w, r, id)
		return
	}
	if strings.HasPrefix(action, "git/") {
		s.gitHandler(w, r)
		return
	}
	if action == "ws" {
		if !s.authorizeSessionWS(w, r, id) {
			return
		}
	}
	if s.proxyRemoteSession(w, r, id, action) {
		return
	}
	if external, ok := s.external.get(id); ok {
		switch action {
		case "ws":
			s.externalSessionWebSocket(w, r, id)
		case "summary":
			running := external.Status != "stale" && external.Status != "stopped"
			writeJSON(w, http.StatusOK, SessionSummary{ID: external.ID, WorkerID: "external", CWD: external.CWD, Status: external.Status, Title: external.Title, UpdatedAt: external.UpdatedAt, State: map[string]any{"external": true, "running": running, "relayConnected": external.RelayConnected}})
		case "state":
			if snap := s.external.stateSnapshot(id); snap != nil {
				writeJSON(w, http.StatusOK, map[string]any{"command": "get_state", "success": true, "data": snap})
			} else {
				writeErrorText(w, http.StatusNotFound, "external session not found")
			}
		case "messages":
			s.relaySessionMessages(w, r, external)
		case "models":
			// Return the full available models list reported by the bridge.
			// Falls back to the active model if the bridge hasn't reported yet.
			models := external.AvailableModels
			if len(models) == 0 && external.Model != nil {
				models = []any{external.Model}
			}
			writeJSON(w, http.StatusOK, map[string]any{"command": "get_available_models", "success": true, "data": map[string]any{"models": models}})
		case "stats":
			writeJSON(w, http.StatusOK, map[string]any{"command": "get_session_stats", "success": true, "data": relaySessionStats(external)})
		case "commands", "entries", "tree", "last-assistant-text", "fork-messages":
			writeJSON(w, http.StatusOK, map[string]any{"command": action, "success": true, "data": map[string]any{}})
		default:
			writeErrorText(w, http.StatusBadRequest, "external session resource is unavailable")
		}
		return
	}
	p, ok := s.getSession(id)
	if !ok {
		writeErrorCode(w, r, http.StatusNotFound, CodeSessionNotFound, "session not found")
		return
	}
	if err := s.ensureSessionCapacity(p); err != nil {
		writeErrorCode(w, r, http.StatusTooManyRequests, CodeCapacityExceeded, err.Error())
		return
	}
	if action == "ws" {
		s.sessionWebSocket(w, r, p)
		return
	}
	if action == "daemon-status" {
		s.daemonStatus(w, r)
		return
	}
	if action == "events" {
		s.eventHistory(w, r)
		return
	}
	if action == "messages" {
		s.sessionMessages(w, r, p)
		return
	}
	cmd, ok := getCommandForAction(action, r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.requestAndWrite(w, r, p, cmd)
}

func (s *Server) handleRawCommand(w http.ResponseWriter, r *http.Request, p *PiProcess) {
	var cmd RPCCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cmd["type"] == "extension_ui_response" {
		if err := validateExtensionUIResponseCommand(cmd); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		if err := p.Send(cmd); err != nil {
			writeRPCSendError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
		return
	}
	if cmd["type"] == "prompt" {
		ctx, cancel := requestContext(r.Context(), s.cfg.RequestTimeout)
		defer cancel()
		if !s.admitLocalRun(ctx, p) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "run capacity is busy or this session already has an active run", "scheduler": s.admission.Snapshot()})
			return
		}
		resp, err := p.Request(ctx, cmd)
		if err != nil {
			p.releaseAdmission()
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	s.requestAndWrite(w, r, p, cmd)
}

func (s *Server) handleRawSend(w http.ResponseWriter, r *http.Request, p *PiProcess) {
	var cmd RPCCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if cmd["type"] == "extension_ui_response" {
		if err := validateExtensionUIResponseCommand(cmd); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}
	if cmd["type"] == "prompt" && !s.admitLocalRun(r.Context(), p) {
		writeJSON(w, http.StatusTooManyRequests, map[string]any{"error": "run capacity is busy or this session already has an active run", "scheduler": s.admission.Snapshot()})
		return
	}
	if err := p.Send(cmd); err != nil {
		if cmd["type"] == "prompt" {
			p.releaseAdmission()
		}
		writeRPCSendError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

func writeRPCSendError(w http.ResponseWriter, err error) {
	if errors.Is(err, errExtensionUIRequestMismatch) {
		writeError(w, http.StatusConflict, err)
		return
	}
	writeError(w, http.StatusBadGateway, err)
}

func (s *Server) requestAndWrite(w http.ResponseWriter, r *http.Request, p *PiProcess, cmd RPCCommand) {
	ctx, cancel := requestContext(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	resp, err := p.Request(ctx, cmd)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	if cmd["type"] == "get_state" {
		if data, ok := resp["data"].(map[string]any); ok {
			data["pendingExtensionUiRequest"] = p.Status()["pendingExtensionUiRequest"]
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
