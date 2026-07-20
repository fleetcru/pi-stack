package server

import "net/http"

type rpcEndpoint struct {
	Method      string `json:"method"`
	Path        string `json:"path"`
	PiCommand   string `json:"piCommand"`
	Description string `json:"description"`
}

func (s *Server) rpcCommandCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"endpoints": []rpcEndpoint{
		{"GET", "/v1/sessions/{id}/state", "get_state", "Current model/session/queue/streaming state"},
		{"GET", "/v1/sessions/{id}/messages", "get_messages", "Active conversation messages"},
		{"GET", "/v1/sessions/{id}/stats", "get_session_stats", "Token, cost, context and message stats"},
		{"GET", "/v1/sessions/{id}/models", "get_available_models", "Configured/available models"},
		{"GET", "/v1/sessions/{id}/commands", "get_commands", "Extension, skill and prompt commands"},
		{"GET", "/v1/sessions/{id}/entries?since=...", "get_entries", "Append-only session entries with cursor"},
		{"GET", "/v1/sessions/{id}/tree", "get_tree", "Full session tree"},
		{"GET", "/v1/sessions/{id}/last-assistant-text", "get_last_assistant_text", "Last assistant text only"},
		{"GET", "/v1/sessions/{id}/fork-messages", "get_fork_messages", "User messages available for fork"},
		{"POST", "/v1/sessions/{id}/prompt", "prompt", "Send a prompt with optional images/streamingBehavior"},
		{"POST", "/v1/sessions/{id}/steer", "steer", "Queue steering message"},
		{"POST", "/v1/sessions/{id}/follow-up", "follow_up", "Queue follow-up message"},
		{"POST", "/v1/sessions/{id}/abort", "abort", "Abort current run"},
		{"POST", "/v1/sessions/{id}/compact", "compact", "Manual compaction"},
		{"POST", "/v1/sessions/{id}/bash", "bash", "Run bash and add result to context"},
		{"POST", "/v1/sessions/{id}/ui-response", "extension_ui_response", "Respond to extension UI request"},
		{"POST", "/v1/sessions/{id}/command", "any", "Raw request/response Pi RPC command"},
		{"POST", "/v1/sessions/{id}/send", "any", "Raw fire-and-forget Pi RPC command"},
		{"GET", "/v1/sessions/{id}/ws", "any/events", "Bidirectional WebSocket event stream"},
	}})
}
