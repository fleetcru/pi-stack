package server

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

func (s *Server) sessionWebSocket(w http.ResponseWriter, r *http.Request, p *PiProcess) {
	ctx, cancel := requestContext(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	if err := p.Start(ctx); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	// Cap incoming WebSocket frames to prevent OOM from a malicious or
	// buggy client sending multi-gigabyte messages.
	conn.SetReadLimit(1 << 20) // 1 MiB, matching HTTP body limit
	// Negotiate event encoding format. "msgpack" sends binary MessagePack
	// frames (~30% smaller, faster parse). "json" (default) is backward-
	// compatible with existing clients.
	codec := ParseCodec(r.URL.Query().Get("codec"))
	// Filesystem watching is opt-in because recursively watching a workspace can
	// consume many OS handles. Clients request it with ?watch=files.
	if r.URL.Query().Get("watch") == "files" {
		go s.ensureFileWatcher(p)
	}
	// Detect dead mobile/NAT connections instead of keeping a stale subscriber.
	const pongWait = 60 * time.Second
	const pingPeriod = 25 * time.Second
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	events, replay, unsubscribe := p.SubscribeSince(since)
	defer func() {
		unsubscribe()
		// Stop the file watcher when the last WS subscriber disconnects.
		// Without this, watchers accumulate until session deletion.
		if r.URL.Query().Get("watch") == "files" && p.SubscriberCount() == 0 {
			s.stopWatcher(p.id)
		}
	}()
	// Keep a stalled client from retaining a large backlog of Pi events. Start
	// the sole writer before queuing replay: a replay may legitimately exceed
	// this buffer (the configured history defaults to 100 events).
	// Leave room for a burst of small text deltas while the socket writer is
	// briefly busy with the network. The process subscriber has a larger buffer
	// as well, so ordinary UI stalls do not lose response fragments.
	out := make(chan any, 256)
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }
	go func() {
		defer closeDone()
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case msg := <-out:
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := codec.WriteWebSocket(conn, msg); err != nil {
					return
				}
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()
	for _, record := range replay {
		message := any(record.Event)
		if record.ID != 0 {
			message = eventWithID(record.Event, record.ID)
		}
		select {
		case out <- message:
		case <-done:
			return
		case <-r.Context().Done():
			return
		}
	}
	go func() {
		defer closeDone()
		for {
			var cmd RPCCommand
			if err := codec.ReadWebSocket(conn, &cmd); err != nil {
				return
			}
			if cmd["type"] == "prompt" && !s.admitLocalRun(r.Context(), p) {
				select {
				case out <- map[string]any{"type": "daemon_error", "error": "run capacity is busy or this session already has an active run", "scheduler": s.admission.Snapshot()}:
				case <-done:
				}
				continue
			}
			if err := p.Send(cmd); err != nil {
				if cmd["type"] == "prompt" {
					p.releaseAdmission()
				}
				select {
				case out <- map[string]any{"type": "daemon_error", "error": err.Error()}:
				case <-done:
				}
			}
		}
	}()
	for {
		select {
		case ev := <-events:
			select {
			case out <- ev:
			case <-done:
				return
			}
		case <-done:
			return
		case <-r.Context().Done():
			return
		}
	}
}
