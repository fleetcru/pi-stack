package server

import (
	"context"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

type Server struct {
	cfg               Config
	maxSessionsAtomic int64 // atomic; mirrors cfg.MaxSessions for lock-free reads
	logger            *slog.Logger
	httpSrv           *http.Server
	sessions          *SessionRegistry
	admission         *TaskAdmission
	workers           *WorkerRegistry
	remoteSessions    *RemoteSessionRegistry
	external          *ExternalRegistry
	wsTickets         *wsTicketStore
	devices           *deviceRegistry
	receipts          *commandReceiptStore
	httpClient        *http.Client
	upgrader          websocket.Upgrader
	watchMu           sync.Mutex
	watchers          map[string]func()
	historyMu         sync.Mutex
	historyCache      map[string]historyCacheEntry
	historyIndexes    map[string]historyIndex
	historyIndexPaths map[string]string
	historyOwnerMu    sync.Mutex
	historyOwners     map[string]string
	stateCacheMu      sync.Mutex
	stateCache        map[string]cachedSessionState
	pendingTitleMu    sync.Mutex
	pendingTitle      map[string]bool
	// Idempotency cache: maps "sessionId:key" to expiry time.
	// Prevents duplicate prompt processing within a TTL window.
	idempotencyMu             sync.Mutex
	idempotency               map[string]time.Time
	resolvedRoots             []string // pre-resolved allowed roots (symlinks evaluated)
	stopHeartbeat             chan struct{}
	shutdownOnce              sync.Once
	sessionBridge             *SessionBridge
	distributedMu             sync.Mutex
	distributedPersistMu      sync.Mutex
	distributedRuns           map[string]distributedReservation // hub session ID -> reservation
	distributedPath           string
	distributedReconstructing bool
	startedAt                 time.Time
	admin                     *adminState
}

func New(cfg Config, logger *slog.Logger) *Server {
	if cfg.ConfigSources == nil {
		cfg.ConfigSources = defaultConfigSources()
	}
	if cfg.AdminConfigPath == "" {
		cfg.AdminConfigPath = filepath.Join(cfg.DataDir, adminConfigFilename)
	}
	s := &Server{
		cfg:               cfg,
		maxSessionsAtomic: int64(cfg.MaxSessions),
		logger:            logger,
		sessions:          NewSessionRegistry(filepath.Join(cfg.DataDir, "sessions.json"), cfg.MaxSessions),
		admission:         NewTaskAdmissionWithQueue(cfg.MaxActiveRuns, cfg.MaxRunsPerSession, cfg.MaxRunsPerWorker, cfg.MaxQueuedRuns),
		workers:           NewWorkerRegistry(filepath.Join(cfg.DataDir, "workers.json")),
		remoteSessions:    NewRemoteSessionRegistry(filepath.Join(cfg.DataDir, "remote-sessions.json")),
		external:          newExternalRegistry(filepath.Join(cfg.DataDir, "relay-commands.json")),
		wsTickets:         newWSTicketStore(),
		devices:           newDeviceRegistry(filepath.Join(cfg.DataDir, "devices.json")),
		receipts:          newCommandReceiptStore(filepath.Join(cfg.DataDir, "command-receipts.json")),
		httpClient:        &http.Client{Timeout: cfg.RequestTimeout, Transport: &http.Transport{MaxIdleConns: 64, MaxIdleConnsPerHost: 16, MaxConnsPerHost: 32, IdleConnTimeout: 90 * time.Second}},
		watchers:          map[string]func(){},
		historyCache:      map[string]historyCacheEntry{},
		historyIndexes:    map[string]historyIndex{},
		historyIndexPaths: map[string]string{},
		historyOwners:     map[string]string{},
		stateCache:        map[string]cachedSessionState{},
		pendingTitle:      map[string]bool{},
		resolvedRoots:     resolveAllowedRoots(cfg.AllowedRoots),
		stopHeartbeat:     make(chan struct{}),
		distributedRuns:   map[string]distributedReservation{},
		distributedPath:   filepath.Join(cfg.DataDir, "distributed-runs.json"),
		startedAt:         time.Now(),
		admin:             newAdminState(cfg),
	}
	s.httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return http.ErrUseLastResponse
		}
		if _, err := s.validateWorkerURL(req.URL.String()); err != nil {
			return err
		}
		return nil
	}
	s.external.onLifecycle = func(sessionID, eventType string) {
		if eventType == "agent_start" {
			s.observeDistributedRun(sessionID, "relay:"+sessionID, "relay")
		} else if eventType == "agent_settled" {
			// agent_end may be followed by automatic retry, compaction, or a queued
			// continuation. Only settled means the relay run slot is reusable.
			s.releaseDistributedRun(sessionID)
		}
	}
	if err := s.sessions.Load(); err != nil {
		logger.Warn("failed to load session registry", "error", err)
	}
	activeJournalIDs := make(map[string]struct{})
	for _, spec := range s.sessions.ListSpecs() {
		activeJournalIDs[safeEventJournalName(spec.ID)] = struct{}{}
	}
	if err := cleanupEventJournals(cfg.DataDir, activeJournalIDs); err != nil {
		logger.Warn("failed to clean up event journals", "error", err)
	}
	// Relay specs from a previous run have no live bridge after restart.
	// Remove them so clients don't see sessions they can't interact with.
	for _, spec := range s.sessions.ListSpecs() {
		if spec.Transport == "relay" {
			logger.Info("removing stale relay session spec", "id", spec.ID)
			_ = s.sessions.Delete(spec.ID)
		}
	}
	for _, spec := range s.sessions.ListSpecs() {
		if spec.Transport == "rpc" {
			if err := s.reserveHistoryOwner(spec); err != nil {
				logger.Warn("session history already owned; session disabled", "session", spec.ID, "error", err)
			}
		}
	}
	if err := s.devices.load(); err != nil {
		logger.Warn("failed to load device registry", "error", err)
	}
	if err := s.workers.Load(); err != nil {
		logger.Warn("failed to load worker registry", "error", err)
	}
	if err := s.remoteSessions.Load(); err != nil {
		logger.Warn("failed to load remote session registry", "error", err)
	}
	s.restoreDistributedRuns()
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
		// Re-link managed sessions to Pi's native session store after restart.
		// This ensures pi -r can discover sessions created via the API.
		go func() {
			for _, spec := range s.sessions.ListSpecs() {
				if spec.ManagedSessionDir != "" && spec.Transport == "rpc" {
					if err := bridge.LinkManagedSession(spec.ID, spec.ManagedSessionDir, spec.CWD); err != nil {
						logger.Debug("failed to re-link managed session", "id", spec.ID, "error", err)
					}
				}
			}
		}()
	}
	s.httpSrv = &http.Server{
		Addr:              cfg.Addr,
		Handler:           requestIDMiddleware(loggingMiddleware(logger, corsMiddleware(cfg.AllowedOrigins, authMiddlewareWithDevices(cfg.AuthToken, s.devices, recoverMiddleware(s.routes()))))),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
		IdleTimeout:       cfg.IdleTimeout,
	}
	return s
}

func (s *Server) ListenAndServe() error { return s.httpSrv.ListenAndServe() }

func (s *Server) maxSessionsAtomicValue() int64 {
	return atomic.LoadInt64(&s.maxSessionsAtomic)
}

func (s *Server) Shutdown(ctx context.Context) error {
	s.shutdownOnce.Do(func() {
		close(s.stopHeartbeat)
		s.stopWatchers()
		s.sessions.CloseAll(ctx)
	})
	return s.httpSrv.Shutdown(ctx)
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /admin", s.adminPage)
	mux.HandleFunc("/admin/", s.adminRoot)
	mux.HandleFunc("GET /healthz", s.health)
	mux.HandleFunc("GET /v1/capabilities", s.capabilities)
	mux.HandleFunc("GET /v1/diagnostics", s.diagnostics)
	mux.HandleFunc("PATCH /v1/capacity", s.updateCapacity)
	mux.HandleFunc("GET /v1/scheduler", s.schedulerStatus)
	mux.HandleFunc("GET /v1/rpc/commands", s.rpcCommandCatalog)
	mux.HandleFunc("GET /openapi.json", s.openapi)
	mux.HandleFunc("GET /v1/directories", s.listDirectories)
	mux.HandleFunc("GET /v1/files", s.listFiles)
	mux.HandleFunc("GET /v1/files/tree", s.fileTree)
	mux.HandleFunc("GET /v1/files/content", s.fileContent)
	mux.HandleFunc("GET /v1/devices", s.listDevices)
	mux.HandleFunc("POST /v1/devices", s.createDevice)
	mux.HandleFunc("DELETE /v1/devices/", s.revokeDevice)
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
	mux.HandleFunc("GET /v1/ws", s.sessionMultiplexWebSocket)
	return mux
}
