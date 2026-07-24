package server

import (
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) sessionPost(w http.ResponseWriter, r *http.Request) {
	id, action := splitSessionPath(r.URL.Path)
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
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Message == "" {
				writeErrorText(w, http.StatusBadRequest, "message is required")
				return
			}
			// External TUI sessions should feel interactive. Pi still injects only
			// at a safe boundary, but steer arrives before another LLM/tool turn;
			// followUp can otherwise wait through an entire tool chain.
			delivery := "steer"
			if action == "follow-up" {
				delivery = "followUp"
			}
			command := ExternalCommand{ID: NewSessionID(), Type: "prompt", Message: body.Message, Delivery: delivery}
			if !s.external.enqueue(id, command) {
				writeErrorText(w, http.StatusBadGateway, "relay is unavailable")
				return
			}
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
			// The relay exposes the active TUI model. Model selection itself stays
			// in Pi TUI until relay control support is added.
			models := []any{}
			if external.Model != nil {
				models = append(models, external.Model)
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
	s.requestAndWrite(w, r, p, cmd)
}

func (s *Server) handleRawSend(w http.ResponseWriter, r *http.Request, p *PiProcess) {
	var cmd RPCCommand
	if err := json.NewDecoder(r.Body).Decode(&cmd); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := p.Send(cmd); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}

func (s *Server) requestAndWrite(w http.ResponseWriter, r *http.Request, p *PiProcess, cmd RPCCommand) {
	ctx, cancel := requestContext(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	resp, err := p.Request(ctx, cmd)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}
