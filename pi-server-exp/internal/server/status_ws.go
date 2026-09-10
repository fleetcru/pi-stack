package server

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// statusTicketScope marks tickets that authorize the server-wide status
// socket only — never a detailed session socket.
const statusTicketScope = "status"

// issueScoped issues a single-use ticket bound to an arbitrary scope
// ("status", or "session:<id>" for detailed session sockets). Existing
// session-ticket methods delegate to this with their session scope.
func (st *wsTicketStore) issueScoped(scope, tokenFP string) (string, time.Time, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	ticket := hex.EncodeToString(raw[:])
	expiresAt := time.Now().UTC().Add(ticketTTL)
	st.mu.Lock()
	st.purgeLocked(time.Now().UTC())
	st.tickets[hashTicket(ticket)] = &wsTicketRecord{
		sessionID: scope,
		tokenFP:   tokenFP,
		expiresAt: expiresAt,
	}
	st.mu.Unlock()
	return ticket, expiresAt, nil
}

// consumeScoped validates and single-uses a scoped ticket.
func (st *wsTicketStore) consumeScoped(ticket, scope, tokenFP string) string {
	if ticket == "" {
		return CodeInvalidTicket
	}
	key := hashTicket(ticket)
	now := time.Now().UTC()
	st.mu.Lock()
	defer st.mu.Unlock()
	rec, ok := st.tickets[key]
	if !ok {
		st.purgeLocked(now)
		return CodeInvalidTicket
	}
	if now.After(rec.expiresAt) {
		delete(st.tickets, key)
		st.purgeLocked(now)
		return CodeTicketExpired
	}
	if rec.sessionID != scope {
		delete(st.tickets, key)
		return CodeTicketSessionMismatch
	}
	if tokenFP != "" && tokenFP != "anonymous" && rec.tokenFP != tokenFP {
		delete(st.tickets, key)
		return CodeInvalidTicket
	}
	delete(st.tickets, key)
	st.purgeLocked(now)
	return ""
}

// createStatusTicket issues a single-use ticket for the server-wide status
// WebSocket. The ticket cannot be redeemed against any session socket.
func (s *Server) createStatusTicket(w http.ResponseWriter, r *http.Request) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	ticket, expiresAt, err := s.wsTickets.issueScoped(statusTicketScope, tokenFingerprint(provided))
	if err != nil {
		writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "failed to issue ticket")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"ticket":    ticket,
		"expiresAt": expiresAt.Format(time.RFC3339),
		"ws":        "/v1/status/ws?ticket=" + ticket,
	})
}

// statusWebSocket streams server-wide status: a snapshot on connect (or a
// bounded replay when ?since=<cursor> is still covered by the ring), then
// live deltas. This socket is read-only and never carries session events.
func (s *Server) statusWebSocket(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AuthToken != "" {
		if r.Header.Get("Authorization") == "Bearer "+s.cfg.AuthToken {
			// Bearer-authenticated: fine.
		} else {
			ticket := r.URL.Query().Get("ticket")
			code := s.wsTickets.consumeScoped(ticket, statusTicketScope, tokenFingerprint(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")))
			if code != "" {
				msg := "invalid status ticket"
				status := http.StatusUnauthorized
				switch code {
				case CodeTicketExpired:
					msg = "status ticket expired"
				case CodeTicketSessionMismatch:
					msg = "status ticket is not valid for the status socket"
					status = http.StatusForbidden
				}
				writeErrorCode(w, r, status, code, msg)
				return
			}
		}
	}
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	conn.SetReadLimit(1 << 20)

	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	cursor := since
	if since > 0 {
		if replay, ok := s.status.replaySince(since); ok {
			seq := s.status.currentCursor()
			writeStatusMessage(conn, map[string]any{"type": "status_replay", "events": replay, "cursor": seq})
			cursor = seq
		} else {
			snap, seq := s.status.snapshot()
			writeStatusMessage(conn, map[string]any{"type": "status_snapshot", "events": snap, "cursor": seq, "gap": true})
			cursor = seq
		}
	} else {
		snap, seq := s.status.snapshot()
		writeStatusMessage(conn, map[string]any{"type": "status_snapshot", "events": snap, "cursor": seq})
		cursor = seq
	}

	events, unsubscribe := s.status.subscribe()
	defer unsubscribe()

	const pongWait = 60 * time.Second
	const pingPeriod = 25 * time.Second
	_ = conn.SetReadDeadline(time.Now().Add(pongWait))
	conn.SetPongHandler(func(string) error { return conn.SetReadDeadline(time.Now().Add(pongWait)) })
	done := make(chan struct{})
	defer close(done)
	go func() {
		defer func() { _ = conn.Close() }()
		ticker := time.NewTicker(pingPeriod)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(10*time.Second)); err != nil {
					return
				}
			case <-done:
				return
			}
		}
	}()
	// Reader: drain and ignore any client frames; the socket is read-only.
	go func() {
		defer func() { _ = conn.Close() }()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	for {
		select {
		case ev := <-events:
			cursor++
			msg := map[string]any{"type": "status", "event": ev, "cursor": cursor}
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
		case <-done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

func writeStatusMessage(conn *websocket.Conn, msg map[string]any) {
	_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	_ = conn.WriteJSON(msg)
}
