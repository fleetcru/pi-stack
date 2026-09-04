package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func (s *Server) externalSessionWebSocket(w http.ResponseWriter, r *http.Request, id string) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(8 << 20) // 8 MiB, matching prompt HTTP body limit
	codec := ParseCodec(r.URL.Query().Get("codec"))
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	events, replay, unsubscribe, ok := s.external.subscribe(id, since)
	if !ok {
		return
	}
	defer unsubscribe()
	done := make(chan struct{})
	defer close(done)
	// Detect dead mobile/NAT clients instead of leaking the read goroutine.
	const pongWait = 60 * time.Second
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	// The read goroutine below may need to nack failed enqueues while the main
	// loop streams events; serialize all writes.
	var writeMu sync.Mutex
	write := func(v any) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return codec.WriteWebSocket(conn, v)
	}
	// Companion sends prompts over its session WebSocket. Translate supported
	// Pi RPC commands into relay commands just like the REST convenience paths.
	go func() {
		for {
			var command RPCCommand
			if err := codec.ReadWebSocket(conn, &command); err != nil {
				return
			}
			var queued ExternalCommand
			startsRun := false
			switch command["type"] {
			case "abort":
				queued = ExternalCommand{ID: NewSessionID(), Type: "abort"}
			case "prompt", "steer", "follow_up":
				commandType, _ := command["type"].(string)
				startsRun = commandType == "prompt"
				message, _ := command["message"].(string)
				images := relayImages(command["images"])
				if message == "" && len(images) == 0 {
					continue
				}
				queued = ExternalCommand{ID: NewSessionID(), Type: "prompt", Message: message, Images: images, Delivery: externalPromptDelivery(commandType)}
			default:
				continue
			}
			if startsRun && !s.acquireDistributedRun(r.Context(), id, "relay:"+id) {
				_ = write(map[string]any{"type": "daemon_error", "error": "hub run capacity is busy or this relay already has an active run", "commandId": queued.ID})
				continue
			}
			if startsRun {
				s.setDistributedRunMetadata(id, "relay", "")
			}
			// A failed enqueue (stale session, full queue) must be signalled back,
			// otherwise the client believes the prompt was delivered.
			if !s.external.enqueue(id, queued) {
				if startsRun {
					s.releaseDistributedRun(id)
				}
				_ = write(map[string]any{"type": "daemon_error", "error": "relay command rejected: session unavailable or queue full", "commandId": queued.ID})
			}
		}
	}()
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for _, ev := range replay {
		if err := write(ev); err != nil {
			return
		}
	}
	for {
		select {
		case ev := <-events:
			if err := write(ev); err != nil {
				return
			}
		case <-ping.C:
			writeMu.Lock()
			err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second))
			writeMu.Unlock()
			if err != nil {
				return
			}
		case <-r.Context().Done():
			return
		case <-done:
			return
		}
	}
}
