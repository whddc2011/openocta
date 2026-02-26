// Package http provides the Gateway HTTP server skeleton.
// Routes: /api/*, /v1/chat/completions, /v1/responses, health, WebSocket upgrade.
package http

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cexll/agentsdk-go/pkg/middleware"
	"github.com/cexll/agentsdk-go/pkg/tool"
	"github.com/openclaw/openclaw/pkg/acp/mcp"
	"github.com/openclaw/openclaw/pkg/channels"
	"github.com/openclaw/openclaw/pkg/channels/builtin"
	"github.com/openclaw/openclaw/pkg/channels/dingtalk"
	"github.com/openclaw/openclaw/pkg/channels/feishu"
	"github.com/openclaw/openclaw/pkg/config"
	"github.com/openclaw/openclaw/pkg/cron"
	"github.com/openclaw/openclaw/pkg/gateway/handlers"
	"github.com/openclaw/openclaw/pkg/gateway/protocol"
	"github.com/openclaw/openclaw/pkg/gateway/ws"
	"github.com/openclaw/openclaw/pkg/outbound"
	"github.com/openclaw/openclaw/pkg/paths"
)

// Server is the Gateway HTTP server.
type Server struct {
	addr       string
	version    string
	server     *http.Server
	mux        *http.ServeMux
	hub        *ws.Hub
	ctx        *handlers.Context
	mcpManager *mcp.Manager
	distOnce   sync.Once
	distDir    string
	distErr    error
}

// isTruthyEnv returns true if the env var is set to a truthy value (1, true, yes).
func isTruthyEnv(env func(string) string, key string) bool {
	v := env(key)
	return v == "1" || v == "true" || v == "yes"
}

// NewServer creates a new HTTP server with WebSocket hub and handlers.
func NewServer(addr string, version string) *Server {
	mux := http.NewServeMux()
	env := func(k string) string { return os.Getenv(k) }
	stateDir := paths.ResolveStateDir(env)
	skipCron := isTruthyEnv(env, "OPENCLAW_SKIP_CRON")
	skipChannels := isTruthyEnv(env, "OPENCLAW_SKIP_CHANNELS") || isTruthyEnv(env, "OPENCLAW_SKIP_PROVIDERS")

	var cronSvc *cron.Service
	if !skipCron {
		svc, _ := cron.NewService(filepath.Join(stateDir, "cron", "jobs.json"))
		cronSvc = svc
	}
	var cronSvcIf interface {
		List(bool) ([]cron.CronJob, error)
		Add(cron.JobCreate) (cron.CronJob, error)
		Remove(string) error
	}
	if cronSvc != nil {
		cronSvcIf = cronSvc
	}
	chReg := channels.NewRegistry()
	outReg := outbound.NewAdapterRegistry()
	if !skipChannels {
		builtin.Register(chReg)
		loadConfig := func() (*config.OpenClawConfig, error) {
			snap, err := handlers.LoadConfigSnapshot(env)
			if err != nil || snap == nil {
				return nil, err
			}
			return snap.Config, nil
		}
		// Register stub outbound adapters for each channel; override with real adapters when configured
		for _, p := range chReg.List() {
			var adapter outbound.OutboundAdapter = &outbound.StubAdapter{ChannelID: p.ID()}
			switch p.ID() {
			case "dingtalk":
				adapter = dingtalk.NewAdapter(loadConfig)
			case "feishu":
				adapter = feishu.NewAdapter(loadConfig)
			}
			outReg.Register(p.ID(), adapter)
		}
	}
	hub := ws.NewHub(version, nil, nil) // Create hub first to get broadcast functions

	// Load configuration at startup
	cfg, err := config.Load(env)
	if err != nil {
		// Log error but continue with default config
		cfg = &config.OpenClawConfig{}
	}

	// Apply environment variables from config.env.vars
	if cfg != nil && cfg.Env != nil && cfg.Env.Vars != nil {
		for k, v := range cfg.Env.Vars {
			if os.Getenv(k) == "" {
				os.Setenv(k, v)
			}
		}
	}

	// MCP: connect to configured MCP servers and expose tools to the agent
	var mcpManager *mcp.Manager
	if cfg != nil && cfg.Mcp != nil && len(cfg.Mcp.Servers) > 0 {
		mgr, err := mcp.NewManager(context.Background(), cfg)
		if err != nil {
			slog.Warn("mcp: failed to start MCP manager, agent will run without MCP tools", "error", err)
		} else {
			mcpManager = mgr
		}
	}
	ctx := &handlers.Context{
		Version:             version,
		GetStatusSummary:    func() (interface{}, error) { return handlers.DefaultStatusSummary(), nil },
		LoadConfigSnapshot:  func() (*handlers.ConfigSnapshot, error) { return handlers.LoadConfigSnapshot(env) },
		GetSessionStorePath: func() string { return filepath.Join(stateDir, "sessions") },
		LoadModelCatalog:    func() []handlers.ModelEntry { return []handlers.ModelEntry{} },
		ChannelRegistry:     chReg,
		OutboundRegistry:    outReg,
		CronService:         cronSvcIf,
		GetCronStorePath:    func() string { return filepath.Join(stateDir, "cron", "jobs.json") },
		AgentRunSeq:         make(map[string]int64), // Initialize sequence counter
		Config:              cfg,                    // Store loaded config
		Broadcast: func(event string, payload interface{}, opts *handlers.BroadcastOptions) {
			if opts == nil {
				opts = &handlers.BroadcastOptions{
					DropIfSlow: false,
				}
			}
			hub.Broadcast(event, payload, &ws.BroadcastOptions{
				DropIfSlow:   opts != nil && opts.DropIfSlow,
				StateVersion: opts.StateVersion,
			})
		},
		BroadcastToConnIds: func(event string, payload interface{}, connIds map[string]bool, opts *handlers.BroadcastOptions) {
			hub.BroadcastToConnIds(event, payload, connIds, &ws.BroadcastOptions{
				DropIfSlow:   opts != nil && opts.DropIfSlow,
				StateVersion: opts.StateVersion,
			})
		},
		NodeSendToSession: func(sessionKey string, event string, payload interface{}) {
			// TODO: Implement node subscription and forwarding
			// For now, just broadcast to all clients
			hub.Broadcast(event, payload, nil)
		},
		MCPTools: func(ctx context.Context) ([]tool.Tool, error) {
			if mcpManager == nil {
				return nil, nil
			}
			return mcpManager.Tools(ctx)
		},
	}

	// Update hub with context and create registry
	hub.SetContext(ctx)
	reg := handlers.NewRegistry(ctx)
	// Allow agent tools to synchronously invoke gateway methods
	ctx.InvokeMethod = func(method string, params map[string]interface{}) (ok bool, payload interface{}, err *protocol.ErrorShape) {
		var resultOk bool
		var resultPayload interface{}
		var resultErr *protocol.ErrorShape
		done := make(chan struct{})
		opts := handlers.HandlerOpts{
			Req:     protocol.RequestFrame{Method: method, Params: params},
			Params:  params,
			Context: ctx,
			Respond: func(o bool, p interface{}, e *protocol.ErrorShape, meta map[string]interface{}) {
				resultOk, resultPayload, resultErr = o, p, e
				close(done)
			},
		}
		reg.Dispatch(opts)
		<-done
		return resultOk, resultPayload, resultErr
	}
	hub.SetHandlers(&reg)
	if cronSvc != nil {
		cronSvc.SetDeps(&cron.Deps{
			EnqueueSystemEvent:  func(text string) {}, // no-op for now
			RequestHeartbeatNow: func(reason string) {},
			RunIsolatedAgentJob: func(job cron.CronJob, message string) {
				agentID := job.AgentID
				if agentID == "" {
					agentID = "main"
				}
				handlers.RunIsolatedAgentTurn(ctx, agentID, "cron:"+job.ID, message)
			},
		})
		cronSvc.Start()
	}
	go hub.Run()

	s := &Server{
		addr:       addr,
		version:    version,
		mux:        mux,
		hub:        hub,
		ctx:        ctx,
		mcpManager: mcpManager,
	}
	s.registerRoutes()
	return s
}

// Handler returns the HTTP handler for testing (e.g. httptest).
// Wraps mux to handle WebSocket upgrade on root path (ws://host:port) for TS compatibility.
func (s *Server) Handler() http.Handler {
	handlerFunc := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" && r.Method == "GET" && strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			s.handleWSUpgrade(w, r)
			return
		}
		s.mux.ServeHTTP(w, r)
	})

	// todo： 生产环境建议去掉，仅用于开发环境
	httpTraceDir := filepath.Join(".", ".claude-trace")
	writer, err := middleware.NewFileHTTPTraceWriter(httpTraceDir)
	var handler http.Handler
	if err != nil {
		log.Printf("HTTP trace disabled: %v", err)
	} else {
		httpTrace := middleware.NewHTTPTraceMiddleware(
			writer,
			middleware.WithHTTPTraceMaxBodyBytes(2<<20),
		)
		handler = httpTrace.Wrap(handlerFunc) // mux 为业务 handler
	}
	return handler
}

func (s *Server) registerRoutes() {
	// Serve frontend (./dist) at root for local dev / single-binary use.
	// Most-specific patterns (e.g. /api/, /ws) still win over this catch-all.
	s.mux.Handle("GET /", http.HandlerFunc(s.handleDist))
	s.mux.HandleFunc("GET /health", s.handleHealth)
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
	//s.mux.HandleFunc("/api/", s.handleAPICatchAll)
	//s.mux.HandleFunc("POST /v1/chat/completions", s.handleNotImplemented)
	//s.mux.HandleFunc("POST /v1/responses", s.handleNotImplemented)
	s.mux.HandleFunc("GET /ws", s.handleWSUpgrade)
	s.mux.HandleFunc("POST /hooks/", s.handleHooks)
	s.mux.HandleFunc("POST /hooks", s.handleHooks)
}

// resolveDistDir resolves the frontend dist directory from multiple candidates.
// Order: 1) OPENOCTA_FRONTEND_DIR env; 2) cwd/dist/control-ui; 3) parent(cwd)/dist/control-ui.
// Returns the first path that exists and contains index.html; otherwise an error listing all tried paths.
func resolveDistDir() (string, error) {
	var candidates []string
	if env := strings.TrimSpace(os.Getenv("OPENOCTA_FRONTEND_DIR")); env != "" {
		p := filepath.Clean(env)
		if !strings.HasSuffix(p, "control-ui") {
			p = filepath.Join(p, "control-ui")
		}
		candidates = append(candidates, p)
	}
	cwd, _ := os.Getwd()
	candidates = append(candidates, filepath.Join(cwd, "dist", "control-ui"))
	candidates = append(candidates, filepath.Join(filepath.Dir(cwd), "dist", "control-ui"))

	for _, dir := range candidates {
		indexPath := filepath.Join(dir, "index.html")
		if _, err := os.Stat(indexPath); err == nil {
			return filepath.Clean(dir), nil
		}
	}
	return "", fmt.Errorf("前端文件不存在，已尝试路径: %s", strings.Join(candidates, " / "))
}

// handleDist serves the static frontend from the resolved dist directory.
// - GET / returns dist/index.html (200, no redirect)
// - GET /assets/... serves static files
// - If a path does not exist and the client accepts HTML, fall back to index.html (SPA routing)
func (s *Server) handleDist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	s.distOnce.Do(func() {
		s.distDir, s.distErr = resolveDistDir()
	})
	if s.distErr != nil {
		http.Error(w, s.distErr.Error(), http.StatusInternalServerError)
		return
	}
	distDir := s.distDir
	indexPath := filepath.Join(distDir, "index.html")

	// Serve index.html for root or SPA fallback (no FileServer to avoid 301 redirects)
	serveIndex := func() {
		f, err := os.Open(indexPath)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		info, err := f.Stat()
		if err != nil || info.IsDir() {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		http.ServeContent(w, r, "index.html", info.ModTime(), f)
	}

	cleanPath := path.Clean("/" + strings.TrimSpace(r.URL.Path))
	if cleanPath == "/" || cleanPath == "" {
		serveIndex()
		return
	}

	// Static file: resolve under distDir (no .. escape)
	name := strings.TrimPrefix(cleanPath, "/")
	name = filepath.Clean(name)
	if name == "" || strings.HasPrefix(name, "..") {
		http.NotFound(w, r)
		return
	}
	fullPath := filepath.Join(distDir, name)
	f, err := os.Open(fullPath)
	if err != nil {
		accept := strings.ToLower(r.Header.Get("Accept"))
		if strings.Contains(accept, "text/html") || strings.Contains(accept, "*/*") {
			serveIndex()
			return
		}
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if info.IsDir() {
		http.NotFound(w, r)
		return
	}
	http.ServeContent(w, r, info.Name(), info.ModTime(), f)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"ok":true,"version":"` + s.version + `"}`))
}

func (s *Server) handleAPICatchAll(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleNotImplemented(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

func (s *Server) handleWSUpgrade(w http.ResponseWriter, r *http.Request) {
	s.hub.ServeWS(w, r)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe() error {
	s.server = &http.Server{
		Addr:         s.addr,
		Handler:      s.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return s.server.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.mcpManager != nil {
		_ = s.mcpManager.Close()
		s.mcpManager = nil
	}
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}
