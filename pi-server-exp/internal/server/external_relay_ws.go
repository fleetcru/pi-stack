package server

import (
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// External relay is an outbound WebSocket opened by the Pi TUI extension.
// Unlike the browser session socket, it carries both relayed Pi events and
// server-pushed remote controls.
func (s *Server) externalRelayWebSocket(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/v1/external-sessions/relay/")
	if !validSessionID(id) {
		writeErrorText(w, http.StatusBadRequest, "invalid external session id")
		return
	}
	commands, pending, generation, detach, exists, authorized := s.external.attachRelay(id, r.URL.Query().Get("lease"))
	if !exists {
		writeErrorText(w, http.StatusNotFound, "external session not found")
		return
	}
	if !authorized {
		writeErrorText(w, http.StatusConflict, "external relay lease is no longer current")
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		detach()
		return
	}
	defer detach()
	defer conn.Close()
	// Detect dead bridge connections instead of keeping a stale relay.
	const pongWait = 60 * time.Second
	const pingPeriod = 25 * time.Second
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	// The pong handler runs on the read goroutine while pings are sent from the
	// write loop below; keep the shared timestamp atomic.
	var lastPingNanos atomic.Int64
	lastPingNanos.Store(time.Now().UnixNano())
	conn.SetPongHandler(func(string) error {
		s.external.setRelayLatency(id, generation, time.Since(time.Unix(0, lastPingNanos.Load())))
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var envelope struct {
				Type  string   `json:"type"`
				Event RPCEvent `json:"event"`
				IDs   []string `json:"ids"`
			}
			if err := conn.ReadJSON(&envelope); err != nil {
				return
			}
			if !s.external.isCurrentRelay(id, generation) {
				return
			}
			switch envelope.Type {
			case "event":
				s.external.publish(id, envelope.Event)
			case "ack":
				s.external.acknowledge(id, envelope.IDs)
			case "heartbeat":
				s.external.heartbeatRelay(id, generation)
			}
		}
	}()
	writeCommand := func(command ExternalCommand) bool {
		return conn.WriteJSON(map[string]any{"type": "command", "command": command}) == nil
	}
	for _, command := range pending {
		if !writeCommand(command) {
			return
		}
	}
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case command := <-commands:
			if !writeCommand(command) {
				return
			}
		case <-ping.C:
			lastPingNanos.Store(time.Now().UnixNano())
			if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
				return
			}
		case <-done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func isExternalRelayWSRequest(r *http.Request) bool {
	return r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/v1/external-sessions/relay/")
}

// relayWSAuthenticated accepts the `pi-relay.<token>` subprotocol credential.
// The query parameter fallback was removed — tokens must be sent via the
// Sec-WebSocket-Protocol header to avoid leaking credentials in logs.
func relayWSAuthenticated(r *http.Request, token string) bool {
	for _, protocol := range strings.Split(r.Header.Get("Sec-WebSocket-Protocol"), ",") {
		if strings.TrimSpace(protocol) == "pi-relay."+token {
			return true
		}
	}
	return false
}
