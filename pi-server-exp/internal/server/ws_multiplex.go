package server

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// sessionSub holds the subscription state for one session on a multiplexed
// connection.
type sessionSub struct {
	events <-chan RPCEvent
	unsub  func()
}

// sessionMultiplexWebSocket handles a single WebSocket connection that can
// subscribe to multiple sessions simultaneously. Each message is tagged with
// a session ID so the server knows which session it belongs to.
//
// Protocol:
//
//	C→S: {"session":"abc","type":"subscribe"}
//	C→S: {"session":"abc","type":"unsubscribe"}
//	C→S: {"session":"abc","type":"prompt","message":"hello"}
//	S→C: {"session":"abc","event":{...}}
//	S→C: {"session":"abc","type":"daemon_error","error":"..."}
func (s *Server) sessionMultiplexWebSocket(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := requestContext(r.Context(), s.cfg.RequestTimeout)
	defer cancel()

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	codec := ParseCodec(r.URL.Query().Get("codec"))

	subs := make(map[string]*sessionSub)
	var subsMu sync.Mutex

	out := make(chan any, 128)
	done := make(chan struct{})
	var once sync.Once
	closeDone := func() { once.Do(func() { close(done) }) }

	// Writer goroutine
	go func() {
		defer closeDone()
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case msg := <-out:
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

	// Reader goroutine
	go func() {
		defer closeDone()
		for {
			var raw map[string]any
			if err := codec.ReadWebSocket(conn, &raw); err != nil {
				return
			}

			sessionID, _ := raw["session"].(string)
			msgType, _ := raw["type"].(string)

			switch msgType {
			case "subscribe":
				if sessionID == "" {
					continue
				}
				subsMu.Lock()
				_, alreadySubbed := subs[sessionID]
				if !alreadySubbed {
					subs[sessionID] = &sessionSub{}
				}
				subsMu.Unlock()
				if alreadySubbed {
					continue
				}
				go s.handleMultiplexSubscribe(ctx, sessionID, subs, &subsMu, out, done)

			case "unsubscribe":
				subsMu.Lock()
				if sub, ok := subs[sessionID]; ok {
					if sub.unsub != nil {
						sub.unsub()
					}
					delete(subs, sessionID)
				}
				subsMu.Unlock()

			case "prompt", "steer", "follow_up", "abort":
				if sessionID == "" {
					continue
				}
				cmd := RPCCommand{"type": msgType}
				if msgType != "abort" {
					message, _ := raw["message"].(string)
					if message == "" && len(relayImages(raw["images"])) == 0 {
						sendMultiplexError(sessionID, "message or images is required", out, done)
						continue
					}
					cmd["message"] = message
				}
				if images, ok := raw["images"]; ok {
					cmd["images"] = images
				}
				s.routeMultiplexCommand(ctx, sessionID, cmd, out, done)
			}
		}
	}()

	<-done
	subsMu.Lock()
	for _, sub := range subs {
		if sub.unsub != nil {
			sub.unsub()
		}
	}
	subsMu.Unlock()
}

// routeMultiplexCommand sends a command to the target session.
func (s *Server) routeMultiplexCommand(ctx context.Context, sessionID string, cmd RPCCommand, out chan any, done chan struct{}) {
	// Try local sessions
	if p, ok := s.sessions.Get(sessionID); ok {
		if cmd["type"] == "prompt" && !s.admitLocalRun(ctx, p) {
			sendMultiplexError(sessionID, "run capacity is busy or this session already has an active run", out, done)
			return
		}
		if _, err := p.Request(ctx, cmd); err != nil {
			if cmd["type"] == "prompt" {
				p.releaseAdmission()
			}
			select {
			case out <- map[string]any{"session": sessionID, "type": "daemon_error", "error": err.Error()}:
			case <-done:
			}
		}
		return
	}
	// Try external sessions
	if _, ok := s.external.get(sessionID); ok {
		var queued ExternalCommand
		switch cmd["type"] {
		case "abort":
			queued = ExternalCommand{ID: NewSessionID(), Type: "abort"}
		case "prompt", "steer", "follow_up":
			commandType, _ := cmd["type"].(string)
			message, _ := cmd["message"].(string)
			images := relayImages(cmd["images"])
			if message == "" && len(images) == 0 {
				sendMultiplexError(sessionID, "message is required", out, done)
				return
			}
			if commandType == "prompt" && !s.acquireDistributedRun(ctx, sessionID, "relay:"+sessionID) {
				sendMultiplexError(sessionID, "hub run capacity is busy or this relay already has an active run", out, done)
				return
			}
			queued = ExternalCommand{ID: NewSessionID(), Type: "prompt", Message: message, Images: images, Delivery: externalPromptDelivery(commandType)}
		}
		if !s.external.enqueue(sessionID, queued) {
			if cmd["type"] == "prompt" {
				s.releaseDistributedRun(sessionID)
			}
			select {
			case out <- map[string]any{"session": sessionID, "type": "daemon_error", "error": "relay command rejected"}:
			case <-done:
			}
		}
	}
}

func sendMultiplexError(sessionID, message string, out chan any, done chan struct{}) {
	select {
	case out <- map[string]any{"session": sessionID, "type": "daemon_error", "error": message}:
	case <-done:
	}
}

// handleMultiplexSubscribe subscribes to a session and forwards events.
func (s *Server) handleMultiplexSubscribe(
	ctx context.Context,
	sessionID string,
	subs map[string]*sessionSub,
	subsMu *sync.Mutex,
	out chan any,
	done chan struct{},
) {
	// Try local sessions
	if p, ok := s.sessions.Get(sessionID); ok {
		events, replay, unsub := p.SubscribeSince(0)
		sub := &sessionSub{events: events, unsub: unsub}
		if !activateMultiplexSubscription(sessionID, sub, subs, subsMu, done) {
			unsub()
			return
		}
		for _, record := range replay {
			message := any(record.Event)
			if record.ID != 0 {
				message = eventWithID(record.Event, record.ID)
			}
			select {
			case out <- map[string]any{"session": sessionID, "event": message}:
			case <-done:
				return
			}
		}
		go forwardMultiplex(sessionID, events, out, done)
		slog.Info("multiplex: subscribed to local session", "session", sessionID)
		return
	}
	// Try external sessions
	if _, ok := s.external.get(sessionID); ok {
		events, replay, unsub, ok := s.external.subscribe(sessionID, 0)
		if !ok {
			return
		}
		sub := &sessionSub{events: events, unsub: unsub}
		if !activateMultiplexSubscription(sessionID, sub, subs, subsMu, done) {
			unsub()
			return
		}
		for _, ev := range replay {
			select {
			case out <- map[string]any{"session": sessionID, "event": ev}:
			case <-done:
				return
			}
		}
		go forwardMultiplex(sessionID, events, out, done)
		slog.Info("multiplex: subscribed to external session", "session", sessionID)
		return
	}
	// Not found. Remove the reservation so a later subscribe can retry.
	subsMu.Lock()
	delete(subs, sessionID)
	subsMu.Unlock()
	sendMultiplexError(sessionID, "session not found", out, done)
}

func activateMultiplexSubscription(sessionID string, sub *sessionSub, subs map[string]*sessionSub, subsMu *sync.Mutex, done chan struct{}) bool {
	subsMu.Lock()
	defer subsMu.Unlock()
	select {
	case <-done:
		delete(subs, sessionID)
		return false
	default:
	}
	reserved, ok := subs[sessionID]
	if !ok || reserved.unsub != nil {
		return false
	}
	subs[sessionID] = sub
	return true
}

// forwardMultiplex forwards events from a channel to the multiplex output.
func forwardMultiplex(sessionID string, events <-chan RPCEvent, out chan any, done chan struct{}) {
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			select {
			case out <- map[string]any{"session": sessionID, "event": ev}:
			case <-done:
				return
			}
		case <-done:
			return
		}
	}
}
