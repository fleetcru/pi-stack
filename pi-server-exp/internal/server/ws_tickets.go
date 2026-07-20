package server

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"
)

const ticketTTL = 60 * time.Second

type wsTicketRecord struct {
	sessionID string
	tokenFP   string
	expiresAt time.Time
}

type wsTicketStore struct {
	mu      sync.Mutex
	tickets map[string]*wsTicketRecord
}

func newWSTicketStore() *wsTicketStore {
	return &wsTicketStore{tickets: map[string]*wsTicketRecord{}}
}

func tokenFingerprint(token string) string {
	if token == "" {
		return "anonymous"
	}
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func hashTicket(ticket string) string {
	sum := sha256.Sum256([]byte(ticket))
	return hex.EncodeToString(sum[:])
}

func (st *wsTicketStore) issue(sessionID, tokenFP string) (ticket string, expiresAt time.Time, err error) {
	var raw [32]byte
	if _, err = rand.Read(raw[:]); err != nil {
		return "", time.Time{}, err
	}
	ticket = hex.EncodeToString(raw[:])
	expiresAt = time.Now().UTC().Add(ticketTTL)
	st.mu.Lock()
	st.purgeLocked(time.Now().UTC())
	st.tickets[hashTicket(ticket)] = &wsTicketRecord{
		sessionID: sessionID,
		tokenFP:   tokenFP,
		expiresAt: expiresAt,
	}
	st.mu.Unlock()
	return ticket, expiresAt, nil
}

// consume validates and single-uses a ticket for the given session.
// Returns a stable error code on failure.
func (st *wsTicketStore) consume(ticket, sessionID, tokenFP string) (code string) {
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
	if rec.sessionID != sessionID {
		delete(st.tickets, key)
		return CodeTicketSessionMismatch
	}
	if rec.tokenFP != tokenFP {
		delete(st.tickets, key)
		return CodeInvalidTicket
	}
	delete(st.tickets, key)
	st.purgeLocked(now)
	return ""
}

func (st *wsTicketStore) purgeLocked(now time.Time) {
	for k, rec := range st.tickets {
		if now.After(rec.expiresAt) {
			delete(st.tickets, k)
		}
	}
}

func (s *Server) createWSTicket(w http.ResponseWriter, r *http.Request) {
	var input struct {
		SessionID string `json:"sessionId"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "invalid request body")
		return
	}
	if input.SessionID == "" {
		writeErrorCode(w, r, http.StatusBadRequest, CodeBadRequest, "sessionId required")
		return
	}
	if !s.sessionExists(input.SessionID) {
		writeErrorCode(w, r, http.StatusNotFound, CodeSessionNotFound, "session not found")
		return
	}
	fp := tokenFingerprint(s.cfg.AuthToken)
	ticket, expiresAt, err := s.wsTickets.issue(input.SessionID, fp)
	if err != nil {
		writeErrorCode(w, r, http.StatusInternalServerError, CodeInternal, "failed to issue ticket")
		return
	}
	wsPath := "/v1/sessions/" + input.SessionID + "/ws?ticket=" + ticket
	writeJSON(w, http.StatusCreated, map[string]any{
		"ticket":    ticket,
		"expiresAt": expiresAt.Format(time.RFC3339),
		"ws":        wsPath,
	})
}

func (s *Server) sessionExists(id string) bool {
	if _, ok := s.sessions.GetSpec(id); ok {
		return true
	}
	if _, ok := s.remoteSessions.Get(id); ok {
		return true
	}
	_, ok := s.external.get(id)
	return ok
}

func (s *Server) authorizeSessionWS(w http.ResponseWriter, r *http.Request, sessionID string) bool {
	if s.cfg.AuthToken == "" {
		return true
	}
	if r.Header.Get("Authorization") == "Bearer "+s.cfg.AuthToken {
		return true
	}
	ticket := r.URL.Query().Get("ticket")
	if ticket == "" {
		writeErrorCode(w, r, http.StatusUnauthorized, CodeUnauthorized, "unauthorized")
		return false
	}
	code := s.wsTickets.consume(ticket, sessionID, tokenFingerprint(s.cfg.AuthToken))
	if code != "" {
		msg := "invalid websocket ticket"
		status := http.StatusUnauthorized
		switch code {
		case CodeTicketExpired:
			msg = "websocket ticket expired"
		case CodeTicketSessionMismatch:
			msg = "websocket ticket session mismatch"
			status = http.StatusForbidden
		}
		writeErrorCode(w, r, status, code, msg)
		return false
	}
	return true
}

func isSessionWSRequest(r *http.Request) bool {
	if r.Method != http.MethodGet {
		return false
	}
	path := r.URL.Path
	return strings.HasSuffix(path, "/ws") && strings.HasPrefix(path, "/v1/sessions/")
}
