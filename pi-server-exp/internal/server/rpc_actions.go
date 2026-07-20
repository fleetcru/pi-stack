package server

import (
	"encoding/json"
	"fmt"
	"net/http"
)

func getCommandForAction(action string, r *http.Request) (RPCCommand, bool) {
	switch action {
	case "state":
		return RPCCommand{"type": "get_state"}, true
	case "messages":
		return RPCCommand{"type": "get_messages"}, true
	case "stats":
		return RPCCommand{"type": "get_session_stats"}, true
	case "models":
		return RPCCommand{"type": "get_available_models"}, true
	case "commands":
		return RPCCommand{"type": "get_commands"}, true
	case "entries":
		cmd := RPCCommand{"type": "get_entries"}
		if since := r.URL.Query().Get("since"); since != "" {
			cmd["since"] = since
		}
		return cmd, true
	case "tree":
		return RPCCommand{"type": "get_tree"}, true
	case "last-assistant-text":
		return RPCCommand{"type": "get_last_assistant_text"}, true
	case "fork-messages":
		return RPCCommand{"type": "get_fork_messages"}, true
	}
	return nil, false
}

func (s *Server) handleConvenienceCommand(w http.ResponseWriter, r *http.Request, p *PiProcess, action string) {
	cmd, fireAndForget, err := commandFromBody(action, r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if fireAndForget {
		if err := p.Send(cmd); err != nil {
			writeError(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
		return
	}
	s.requestAndWrite(w, r, p, cmd)
}

func commandFromBody(action string, r *http.Request) (RPCCommand, bool, error) {
	var body map[string]any
	if r.Body != http.NoBody {
		_ = json.NewDecoder(r.Body).Decode(&body)
	}
	if body == nil {
		body = map[string]any{}
	}
	switch action {
	case "prompt":
		if err := requireString(body, "message"); err != nil {
			return nil, false, err
		}
		if v, ok := body["streamingBehavior"].(string); ok && v != "steer" && v != "followUp" {
			return nil, false, fmt.Errorf("streamingBehavior must be steer or followUp")
		}
		return RPCCommand{"type": "prompt", "message": stringField(body, "message"), "streamingBehavior": body["streamingBehavior"], "images": body["images"]}, false, nil
	case "steer":
		if err := requireString(body, "message"); err != nil {
			return nil, false, err
		}
		return RPCCommand{"type": "steer", "message": stringField(body, "message"), "images": body["images"]}, false, nil
	case "follow-up":
		if err := requireString(body, "message"); err != nil {
			return nil, false, err
		}
		return RPCCommand{"type": "follow_up", "message": stringField(body, "message"), "images": body["images"]}, false, nil
	case "abort":
		return RPCCommand{"type": "abort"}, true, nil
	case "compact":
		cmd := RPCCommand{"type": "compact"}
		if v := stringField(body, "customInstructions"); v != "" {
			cmd["customInstructions"] = v
		}
		return cmd, false, nil
	case "bash":
		if err := requireString(body, "command"); err != nil {
			return nil, false, err
		}
		return RPCCommand{"type": "bash", "command": stringField(body, "command")}, false, nil
	case "ui-response":
		body["type"] = "extension_ui_response"
		return RPCCommand(body), true, nil
	case "model":
		if err := requireString(body, "provider"); err != nil {
			return nil, false, err
		}
		if err := requireString(body, "modelId"); err != nil {
			return nil, false, err
		}
		return RPCCommand{"type": "set_model", "provider": stringField(body, "provider"), "modelId": stringField(body, "modelId")}, false, nil
	case "cycle-model":
		return RPCCommand{"type": "cycle_model"}, false, nil
	case "thinking-level":
		return RPCCommand{"type": "set_thinking_level", "level": stringField(body, "level")}, false, nil
	case "cycle-thinking-level":
		return RPCCommand{"type": "cycle_thinking_level"}, false, nil
	case "steering-mode":
		return RPCCommand{"type": "set_steering_mode", "mode": stringField(body, "mode")}, false, nil
	case "follow-up-mode":
		return RPCCommand{"type": "set_follow_up_mode", "mode": stringField(body, "mode")}, false, nil
	case "auto-compaction":
		return RPCCommand{"type": "set_auto_compaction", "enabled": boolField(body, "enabled")}, false, nil
	case "auto-retry":
		return RPCCommand{"type": "set_auto_retry", "enabled": boolField(body, "enabled")}, false, nil
	case "abort-retry":
		return RPCCommand{"type": "abort_retry"}, true, nil
	case "switch":
		return RPCCommand{"type": "switch_session", "sessionPath": stringField(body, "sessionPath")}, false, nil
	case "fork":
		return RPCCommand{"type": "fork", "entryId": stringField(body, "entryId")}, false, nil
	case "clone":
		return RPCCommand{"type": "clone"}, false, nil
	case "new":
		cmd := RPCCommand{"type": "new_session"}
		if v := stringField(body, "parentSession"); v != "" {
			cmd["parentSession"] = v
		}
		return cmd, false, nil
	case "name":
		return RPCCommand{"type": "set_session_name", "name": stringField(body, "name")}, false, nil
	case "export-html":
		cmd := RPCCommand{"type": "export_html"}
		if v := stringField(body, "outputPath"); v != "" {
			cmd["outputPath"] = v
		}
		return cmd, false, nil
	case "abort-bash":
		return RPCCommand{"type": "abort_bash"}, true, nil
	}
	return nil, false, nil
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func requireString(m map[string]any, key string) error {
	if stringField(m, key) == "" {
		return fmt.Errorf("%s is required", key)
	}
	return nil
}

func boolField(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}
