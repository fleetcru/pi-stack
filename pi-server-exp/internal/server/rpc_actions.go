package server

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
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
			writeRPCSendError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
		return
	}
	if action == "prompt" {
		ctx, cancel := requestContext(r.Context(), s.cfg.RequestTimeout)
		defer cancel()
		if !s.admitLocalRun(ctx, p) {
			writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error":     "run capacity is busy or this session already has an active run",
				"scheduler": s.admission.Snapshot(),
			})
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

func (s *Server) admitLocalRun(ctx context.Context, p *PiProcess) bool {
	if !s.admission.Acquire(ctx, p.id, "local") {
		return false
	}
	if p.holdAdmission(func() { s.admission.Release(p.id, "local") }) {
		return true
	}
	s.admission.Release(p.id, "local")
	return false
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
		images, err := normalizePromptImages(body["images"])
		if err != nil {
			return nil, false, err
		}
		message := stringField(body, "message")
		if message == "" && len(images) == 0 {
			return nil, false, fmt.Errorf("message or images is required")
		}
		if v, ok := body["streamingBehavior"].(string); ok && v != "steer" && v != "followUp" {
			return nil, false, fmt.Errorf("streamingBehavior must be steer or followUp")
		}
		return RPCCommand{"type": "prompt", "message": message, "streamingBehavior": body["streamingBehavior"], "images": images}, false, nil
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
		if err := validateExtensionUIResponseCommand(body); err != nil {
			return nil, false, err
		}
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

func validateExtensionUIResponseCommand(command map[string]any) error {
	return validateExtensionUIResponseFields(
		stringField(command, "id"),
		stringField(command, "value"),
		stringField(command, "comment"),
		relayStringSlice(command["selections"]),
		stringField(command, "responseKind"),
	)
}

func validateExtensionUIResponseFields(id, value, comment string, selections []string, responseKind string) error {
	if id == "" {
		return fmt.Errorf("id is required")
	}
	if len(id) > 512 {
		return fmt.Errorf("id is too long")
	}
	if len(value) > 32<<10 {
		return fmt.Errorf("value exceeds 32KB")
	}
	if len(comment) > 8<<10 {
		return fmt.Errorf("comment exceeds 8KB")
	}
	if len(selections) > 50 {
		return fmt.Errorf("too many selections")
	}
	for _, selection := range selections {
		if len(selection) > 1024 {
			return fmt.Errorf("selection exceeds 1KB")
		}
	}
	if responseKind != "" && responseKind != "selection" && responseKind != "freeform" {
		return fmt.Errorf("responseKind must be selection or freeform")
	}
	return nil
}

func normalizePromptImages(value any) ([]map[string]any, error) {
	if value == nil {
		return nil, nil
	}
	images, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("images must be an array")
	}
	normalized := make([]map[string]any, 0, len(images))
	for i, raw := range images {
		image, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("images[%d] must be an object", i)
		}
		mimeType := stringField(image, "mimeType")
		if mimeType == "" || !strings.HasPrefix(mimeType, "image/") {
			return nil, fmt.Errorf("images[%d].mimeType must be an image MIME type", i)
		}
		data := stringField(image, "base64")
		if data == "" {
			data = stringField(image, "data")
		}
		if data == "" {
			return nil, fmt.Errorf("images[%d].base64 is required", i)
		}
		if imageType := stringField(image, "type"); imageType != "" && imageType != "image" {
			return nil, fmt.Errorf("images[%d].type must be image", i)
		}
		if _, err := base64.StdEncoding.DecodeString(data); err != nil {
			if _, rawErr := base64.RawStdEncoding.DecodeString(data); rawErr != nil {
				return nil, fmt.Errorf("images[%d].base64 is invalid", i)
			}
		}
		normalized = append(normalized, map[string]any{
			"type":     "image",
			"data":     data,
			"mimeType": mimeType,
		})
	}
	return normalized, nil
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
