package server

import "net/http"

func (s *Server) openapi(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"openapi":  "3.1.0",
		"info":     map[string]any{"title": "pi-server", "version": APIVersion, "description": "HTTP/WebSocket daemon for Pi RPC"},
		"security": []map[string][]string{{"bearerAuth": []string{}}},
		"components": map[string]any{
			"securitySchemes": map[string]any{"bearerAuth": map[string]any{"type": "http", "scheme": "bearer"}},
			"schemas":         schemas(),
		},
		"paths": paths(),
	})
}

func schemas() map[string]any {
	obj := func(props map[string]any, required ...string) map[string]any {
		m := map[string]any{"type": "object", "properties": props}
		if len(required) > 0 {
			m["required"] = required
		}
		return m
	}
	return map[string]any{
		"Error": obj(map[string]any{
			"error":     str(),
			"code":      str(),
			"requestId": str(),
		}, "error"),
		"Capabilities": obj(map[string]any{
			"apiVersion":   str(),
			"capabilities": map[string]any{"type": "object", "additionalProperties": schemaBool()},
		}, "apiVersion", "capabilities"),
		"WSTicketRequest":  obj(map[string]any{"sessionId": str()}, "sessionId"),
		"WSTicketResponse": obj(map[string]any{"ticket": str(), "expiresAt": str(), "ws": str()}, "ticket", "expiresAt", "ws"),
		"RPCCommand":       map[string]any{"type": "object", "additionalProperties": true, "required": []string{"type"}, "properties": map[string]any{"type": map[string]string{"type": "string"}}},
		"RPCResponse":      map[string]any{"type": "object", "additionalProperties": true},
		"SessionSpec": obj(map[string]any{
			"id": str(), "cwd": str(), "args": arr(str()),
			"env":         map[string]any{"type": "object", "additionalProperties": str()},
			"sessionPath": str(), "managed": schemaBool(), "transport": str(), "restart": schemaBool(), "status": str(),
			"project": str(), "title": str(), "taskType": str(), "owner": str(),
			"labels": arr(str()), "metadata": map[string]any{"type": "object", "additionalProperties": str()},
			"createdAt": str(), "updatedAt": str(),
		}, "id", "cwd"),
		"SessionSummary": obj(map[string]any{
			"id": str(), "workerId": str(), "workerSessionId": str(), "cwd": str(), "status": str(), "managed": schemaBool(), "transport": str(),
			"project": str(), "title": str(), "taskType": str(), "labels": arr(str()),
			"createdAt": str(), "updatedAt": str(), "state": map[string]any{"type": "object", "additionalProperties": true},
		}, "id", "workerId"),
		"CreateSessionRequest": obj(map[string]any{
			"id": str(), "cwd": str(), "args": arr(str()),
			"env":         map[string]any{"type": "object", "additionalProperties": str()},
			"sessionPath": str(), "start": schemaBool(), "restart": schemaBool(),
			"project": str(), "title": str(), "taskType": str(), "owner": str(),
			"labels": arr(str()), "metadata": map[string]any{"type": "object", "additionalProperties": str()},
		}),
		"Worker":                obj(map[string]any{"id": str(), "url": str(), "tags": arr(str()), "status": str(), "lastHeartbeat": str(), "activeSessions": map[string]string{"type": "integer"}, "maxSessions": map[string]string{"type": "integer"}}, "id", "url"),
		"WorkerWrite":           obj(map[string]any{"id": str(), "url": str(), "token": str(), "tags": arr(str())}, "id", "url"),
		"FileInfo":              obj(map[string]any{"name": str(), "path": str(), "isDir": schemaBool(), "size": map[string]string{"type": "integer"}}, "name", "path", "isDir"),
		"FileContent":           obj(map[string]any{"path": str(), "mimeType": str(), "encoding": str(), "truncated": schemaBool(), "content": str(), "size": map[string]string{"type": "integer"}, "binary": schemaBool()}, "path"),
		"ImageContent":          obj(map[string]any{"type": map[string]any{"type": "string", "const": "image"}, "data": str(), "mimeType": str()}, "type", "data", "mimeType"),
		"PromptRequest":         obj(map[string]any{"message": str(), "streamingBehavior": map[string]any{"type": "string", "enum": []string{"steer", "followUp"}}, "images": arr(ref("ImageContent"))}, "message"),
		"ExtensionUIResponse":   obj(map[string]any{"id": str(), "value": map[string]any{}, "confirmed": schemaBool(), "cancelled": schemaBool()}, "id"),
		"SessionMetadataUpdate": obj(map[string]any{"project": str(), "title": str(), "taskType": str(), "owner": str(), "labels": arr(str()), "metadata": map[string]any{"type": "object", "additionalProperties": str()}}),
		"BashRequest":           obj(map[string]any{"command": str()}, "command"),
		"ModelRequest":          obj(map[string]any{"provider": str(), "modelId": str()}, "provider", "modelId"),
		"ThinkingRequest":       obj(map[string]any{"level": str()}, "level"),
		"ModeRequest":           obj(map[string]any{"mode": str()}, "mode"),
		"EnabledRequest":        obj(map[string]any{"enabled": schemaBool()}, "enabled"),
	}
}

func paths() map[string]any {
	p := map[string]any{}
	get := func(path, summary string) { p[path] = map[string]any{"get": op(summary, "RPCResponse", "")} }
	post := func(path, summary, body string) { p[path] = map[string]any{"post": op(summary, "RPCResponse", body)} }
	p["/healthz"] = map[string]any{"get": op("Health check", "RPCResponse", "")}
	p["/openapi.json"] = map[string]any{"get": op("OpenAPI document", "RPCResponse", "")}
	p["/v1/capabilities"] = map[string]any{"get": op("API capabilities", "Capabilities", "")}
	p["/v1/ws-tickets"] = map[string]any{"post": op("Issue WebSocket ticket", "WSTicketResponse", "WSTicketRequest")}
	p["/v1/rpc/commands"] = map[string]any{"get": op("List wrapper endpoints", "RPCResponse", "")}
	p["/v1/sessions"] = map[string]any{"get": op("List sessions (scope=local|all)", "RPCResponse", ""), "post": op("Create session", "RPCResponse", "CreateSessionRequest")}
	p["/v1/remote-sessions"] = map[string]any{"get": op("List remote session mappings", "RPCResponse", "")}
	p["/v1/global-sessions"] = map[string]any{"get": op("List all local and registered-worker sessions", "RPCResponse", "")}
	p["/v1/machine-sessions"] = map[string]any{"get": op("Discover persisted Pi sessions in the default user store", "RPCResponse", "")}
	post("/v1/machine-sessions/{id}/open", "Open a discovered machine session", "")
	post("/v1/global-sessions/{id}/attach", "Attach global session through coordinator", "")
	p["/v1/workers"] = map[string]any{"get": op("List workers", "RPCResponse", ""), "post": op("Register worker", "Worker", "WorkerWrite")}
	p["/v1/workers/{id}"] = map[string]any{
		"get":    op("Get worker", "Worker", ""),
		"put":    op("Update worker", "Worker", "WorkerWrite"),
		"delete": op("Delete worker", "RPCResponse", ""),
	}
	get("/v1/workers/{id}/health", "Worker health probe")
	p["/v1/workers/{id}/health"] = map[string]any{
		"get":  op("Worker health probe", "RPCResponse", ""),
		"post": op("Immediate worker health probe", "RPCResponse", ""),
	}
	post("/v1/workers/{id}/sessions", "Create session on worker", "CreateSessionRequest")
	get("/v1/workers/{id}/sessions/{sessionId}/ws", "Proxy remote worker WebSocket")
	get("/v1/directories", "Browse allowed session directories")
	get("/v1/files", "List directory files")
	get("/v1/files/content", "Read bounded file content")
	get("/v1/sessions/{id}/files/content", "Session-scoped bounded file content")
	get("/v1/sessions/{id}/summary", "Session inventory summary")
	for _, x := range []string{"status", "diff", "log", "head"} {
		get("/v1/sessions/{id}/git/"+x, "Read-only Git "+x)
	}
	get("/v1/files/tree", "List directory tree")
	for _, x := range []string{"state", "messages", "stats", "models", "commands", "entries", "tree", "last-assistant-text", "fork-messages", "daemon-status", "events"} {
		get("/v1/sessions/{id}/"+x, "Pi RPC "+x)
	}
	post("/v1/sessions/{id}/command", "Raw Pi RPC command", "RPCCommand")
	post("/v1/sessions/{id}/send", "Raw Pi RPC fire-and-forget", "RPCCommand")
	for _, x := range []struct{ path, body string }{{"prompt", "PromptRequest"}, {"steer", "PromptRequest"}, {"follow-up", "PromptRequest"}, {"bash", "BashRequest"}, {"model", "ModelRequest"}, {"thinking-level", "ThinkingRequest"}, {"steering-mode", "ModeRequest"}, {"follow-up-mode", "ModeRequest"}, {"auto-compaction", "EnabledRequest"}, {"auto-retry", "EnabledRequest"}} {
		post("/v1/sessions/{id}/"+x.path, "Pi RPC "+x.path, x.body)
	}
	post("/v1/sessions/{id}/metadata", "Update persisted app metadata", "SessionMetadataUpdate")
	post("/v1/sessions/{id}/ui-response", "Respond to Pi extension UI", "ExtensionUIResponse")
	for _, x := range []string{"abort", "compact", "cycle-model", "cycle-thinking-level", "abort-retry", "switch", "fork", "clone", "new", "name", "export-html", "abort-bash"} {
		post("/v1/sessions/{id}/"+x, "Pi RPC "+x, "RPCCommand")
	}
	get("/v1/sessions/{id}/ws", "Session WebSocket")
	return p
}

func op(summary, responseSchema, bodySchema string) map[string]any {
	o := map[string]any{"summary": summary, "responses": map[string]any{"200": resp(responseSchema), "400": resp("Error"), "401": resp("Error"), "502": resp("Error")}}
	if bodySchema != "" {
		o["requestBody"] = map[string]any{"content": map[string]any{"application/json": map[string]any{"schema": ref(bodySchema)}}}
	}
	return o
}
func resp(schema string) map[string]any {
	return map[string]any{"description": "response", "content": map[string]any{"application/json": map[string]any{"schema": ref(schema)}}}
}
func ref(name string) map[string]string {
	return map[string]string{"$ref": "#/components/schemas/" + name}
}
func str() map[string]string        { return map[string]string{"type": "string"} }
func schemaBool() map[string]string { return map[string]string{"type": "boolean"} }
func arr(items any) map[string]any  { return map[string]any{"type": "array", "items": items} }
