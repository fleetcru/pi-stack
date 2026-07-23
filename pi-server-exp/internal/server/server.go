package server

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type Server struct {
	cfg            Config
	logger         *slog.Logger
	httpSrv        *http.Server
	sessions       *SessionRegistry
	workers        *WorkerRegistry
	remoteSessions *RemoteSessionRegistry
	external       *ExternalRegistry
	wsTickets      *wsTicketStore
	httpClient     *http.Client
	upgrader       websocket.Upgrader
	watchMu        sync.Mutex
	watchers       map[string]func()
	historyMu      sync.Mutex
	historyCache   map[string]historyCacheEntry
	stateCacheMu   sync.Mutex
	stateCache     map[string]cachedSessionState
	pendingTitleMu sync.Mutex
	pendingTitle   map[string]bool
	stopHeartbeat  chan struct{}
	sessionBridge  *SessionBridge
}

func New(cfg Config, logger *slog.Logger) *Server {
	s := &Server{
		cfg:            cfg,
		logger:         logger,
		sessions:       NewSessionRegistry(filepath.Join(cfg.DataDir, "sessions.json"), cfg.MaxSessions),
		workers:        NewWorkerRegistry(filepath.Join(cfg.DataDir, "workers.json")),
		remoteSessions: NewRemoteSessionRegistry(filepath.Join(cfg.DataDir, "remote-sessions.json")),
		external:       newExternalRegistry(filepath.Join(cfg.DataDir, "relay-commands.json")),
		wsTickets:      newWSTicketStore(),
		httpClient:     &http.Client{Timeout: cfg.RequestTimeout, Transport: &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 16, MaxConnsPerHost: 32, IdleConnTimeout: 90 * time.Second}},
		watchers:       map[string]func(){},
		historyCache:   map[string]historyCacheEntry{},
		stateCache:     map[string]cachedSessionState{},
		pendingTitle:   map[string]bool{},
		stopHeartbeat:  make(chan struct{}),
	}
	if err := s.sessions.Load(); err != nil {
		logger.Warn("failed to load session registry", "error", err)
	}
	// Relay specs from a previous run have no live bridge after restart.
	// Remove them so clients don't see sessions they can't interact with.
	for _, spec := range s.sessions.ListSpecs() {
		if spec.Transport == "relay" {
			logger.Info("removing stale relay session spec", "id", spec.ID)
			_ = s.sessions.Delete(spec.ID)
		}
	}
	// Re-link managed sessions to Pi's native session store after restart.
	// This ensures pi -r can discover sessions created via the API.
	if s.sessionBridge != nil {
		go func() {
			for _, spec := range s.sessions.ListSpecs() {
				if spec.ManagedSessionDir != "" && spec.Transport == "rpc" {
					if err := s.sessionBridge.LinkManagedSession(spec.ID, spec.ManagedSessionDir); err != nil {
						logger.Debug("failed to re-link managed session", "id", spec.ID, "error", err)
					}
				}
			}
		}()
	}
	if err := s.workers.Load(); err != nil {
		logger.Warn("failed to load worker registry", "error", err)
	}
	if err := s.remoteSessions.Load(); err != nil {
		logger.Warn("failed to load remote session registry", "error", err)
	}
	s.startWorkerHeartbeats()
	s.upgrader = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			if origin == "" {
				return true // non-browser clients (curl, SDKs)
			}
			if len(cfg.AllowedOrigins) == 0 {
				// Mirror corsMiddleware: reject browser cross-origin requests
				// when no allowlist is configured.
				return false
			}
			// Match HTTP CORS: exact origin or equivalent loopback host.
			return originAllowed(origin, cfg.AllowedOrigins)
		},
	}
	// Initialize session bridge for pi -r compatibility
	if bridge, err := NewSessionBridge(logger); err != nil {
		logger.Warn("session bridge disabled", "error", err)
	} else {
		s.sessionBridge = bridge
	}
	s.httpSrv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           requestIDMiddleware(loggingMiddleware(logger, corsMiddleware(cfg.AllowedOrigins, authMiddleware(cfg.AuthToken, recoverMiddleware(s.routes()))))),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
	return s
}

func (s *Server) ListenAndServe() error { return s.httpSrv.ListenAndServe() }

func (s *Server) Shutdown(ctx context.Context) error {
	close(s.stopHeartbeat)
	s.stopWatchers()
	s.sessions.CloseAll(ctx)
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/capabilities", s.capabilities)
	mux.HandleFunc("PATCH /v1/capacity", s.updateCapacity)
	mux.HandleFunc("GET /v1/rpc/commands", s.rpcCommandCatalog)
	mux.HandleFunc("GET /openapi.json", s.openapi)
	mux.HandleFunc("GET /v1/directories", s.listDirectories)
	mux.HandleFunc("GET /v1/files", s.listFiles)
	mux.HandleFunc("GET /v1/files/tree", s.fileTree)
	mux.HandleFunc("GET /v1/files/content", s.fileContent)
	mux.HandleFunc("GET /v1/workers", s.listWorkers)
	mux.HandleFunc("GET /v1/remote-sessions", s.listRemoteSessions)
	mux.HandleFunc("GET /v1/global-sessions", s.listGlobalSessions)
	mux.HandleFunc("GET /v1/machine-sessions", s.listMachineSessions)
	mux.HandleFunc("POST /v1/machine-sessions/", s.machineSessionPost)
	mux.HandleFunc("POST /v1/global-sessions/", func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/v1/global-sessions/")
		if !strings.HasSuffix(id, "/attach") {
			http.NotFound(w, r)
			return
		}
		s.attachGlobalSession(w, r, strings.TrimSuffix(id, "/attach"))
	})
	mux.HandleFunc("POST /v1/workers", s.addWorker)
	mux.HandleFunc("GET /v1/workers/", s.workerGet)
	mux.HandleFunc("POST /v1/workers/", s.workerPost)
	mux.HandleFunc("PUT /v1/workers/", s.workerPut)
	mux.HandleFunc("DELETE /v1/workers/", s.workerDelete)
	mux.HandleFunc("POST /v1/ws-tickets", s.createWSTicket)
	mux.HandleFunc("POST /v1/external-sessions/register", s.externalRegister)
	mux.HandleFunc("POST /v1/external-sessions/", s.externalPost)
	mux.HandleFunc("GET /v1/external-sessions/", s.externalGet)
	mux.HandleFunc("GET /v1/external-sessions/relay/", s.externalRelayWebSocket)
	mux.HandleFunc("POST /v1/sessions", s.createSession)
	mux.HandleFunc("GET /v1/sessions", s.listSessions)
	mux.HandleFunc("DELETE /v1/sessions/", s.deleteSession)
	mux.HandleFunc("POST /v1/sessions/", s.sessionPost)
	mux.HandleFunc("GET /v1/sessions/", s.sessionGet)
	return mux
}
