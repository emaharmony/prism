// Package api provides the Prism HTTP API server (REST + SSE).
//
// Endpoints:
//   GET /api/v1/status          — System status
//   GET /api/v1/agents           — List agents
//   GET /api/v1/agents/{id}      — Agent detail
//   GET /api/v1/sessions          — List sessions
//   GET /api/v1/sessions/{id}    — Session detail
//   GET /api/v1/tasks             — List tasks
//   GET /api/v1/tasks/{id}       — Task detail
//   GET /api/v1/approvals         — List pending approvals
//   POST /api/v1/approvals/{id}/grant — Grant approval
//   POST /api/v1/approvals/{id}/deny   — Deny approval
//   GET /api/v1/events/stream    — SSE event stream
//   GET /api/v1/workflows        — List workflows
//   GET /api/v1/costs             — Cost summary
package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/delegation"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/session"
	"github.com/emaharmony/prism/internal/task"
	"github.com/emaharmony/prism/internal/workflow"
	"github.com/nats-io/nats.go"
)

// Server provides the Prism HTTP API.
type Server struct {
	addr      string
	orch      *orchestrator.Orchestrator
	store     *task.Store
	sessions  *session.Manager
	engine    *delegation.Engine
	approval  *delegation.ApprovalManager
	tracker   *delegation.Tracker
	nc        *nats.Conn
	mux       *http.ServeMux
}

// Config holds API server configuration.
type Config struct {
	Addr     string // listen address (e.g., ":8081")
	Orch     *orchestrator.Orchestrator
	Store    *task.Store
	Sessions *session.Manager
	Engine   *delegation.Engine
	Approval *delegation.ApprovalManager
	Tracker  *delegation.Tracker
	NATS     *nats.Conn
}

// NewServer creates a new API server.
func NewServer(cfg Config) *Server {
	s := &Server{
		addr:      cfg.Addr,
		orch:      cfg.Orch,
		store:     cfg.Store,
		sessions:  cfg.Sessions,
		engine:    cfg.Engine,
		approval:  cfg.Approval,
		tracker:   cfg.Tracker,
		nc:        cfg.NATS,
		mux:       http.NewServeMux(),
	}
	s.routes()
	return s
}

// routes registers all API endpoints.
func (s *Server) routes() {
	s.mux.HandleFunc("/api/v1/status", s.handleStatus)
	s.mux.HandleFunc("/api/v1/agents", s.handleAgents)
	s.mux.HandleFunc("/api/v1/agents/", s.handleAgentDetail)
	s.mux.HandleFunc("/api/v1/sessions", s.handleSessions)
	s.mux.HandleFunc("/api/v1/sessions/", s.handleSessionDetail)
	s.mux.HandleFunc("/api/v1/tasks", s.handleTasks)
	s.mux.HandleFunc("/api/v1/tasks/", s.handleTaskDetail)
	s.mux.HandleFunc("/api/v1/approvals", s.handleApprovals)
	s.mux.HandleFunc("/api/v1/approvals/", s.handleApprovalAction)
	s.mux.HandleFunc("/api/v1/events/stream", s.handleEventStream)
	s.mux.HandleFunc("/api/v1/workflows", s.handleWorkflows)
	s.mux.HandleFunc("/api/v1/workflows/", s.handleWorkflowSVG)
	s.mux.HandleFunc("/api/v1/costs", s.handleCosts)
}

// Start starts the API server with timeouts and panic recovery.
func (s *Server) Start() error {
	log.Printf("[API] starting on %s", s.addr)

	handler := panicRecovery(s.mux)

	srv := &http.Server{
		Addr:         s.addr,
		Handler:      handler,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	return srv.ListenAndServe()
}

// panicRecovery wraps a handler with panic recovery.
func panicRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("[API] panic recovered: %v", err)
				http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// StartWithHandler returns the http.Handler without starting a server.
// Useful for embedding in another server or testing.
func (s *Server) Handler() http.Handler {
	return s.mux
}

// --- Status ---

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := map[string]any{
		"status":    "running",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "0.23.0",
	}

	if s.orch != nil {
		status["agents"] = len(s.orch.Agents.List())
	}

	if s.store != nil && s.tracker != nil {
		if tracker, err := s.tracker.TaskStatus(); err == nil {
			status["tasks"] = tracker.ByStatus
			status["tasks_total"] = tracker.Total
		}
	}

	writeJSON(w, status)
}

// --- Agents ---

func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.orch == nil {
		writeJSON(w, []any{})
		return
	}

	writeJSON(w, s.orch.Status())
}

func (s *Server) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/agents/")
	if id == "" {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}

	if s.orch == nil {
		writeJSONError(w, "orchestrator not available", http.StatusServiceUnavailable)
		return
	}

	agent, err := s.orch.GetAgent(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, agent)
}

// --- Sessions ---

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.sessions == nil {
		writeJSON(w, []any{})
		return
	}

	sessions, err := s.sessions.ListActive()
	if err != nil {
		writeJSONError(w, "failed to list sessions", http.StatusInternalServerError)
		return
	}

	writeJSON(w, sessions)
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/sessions/")
	if id == "" {
		http.Error(w, "session id required", http.StatusBadRequest)
		return
	}

	if s.sessions == nil {
		writeJSONError(w, "session manager not available", http.StatusServiceUnavailable)
		return
	}

	sess, err := s.sessions.Get(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, sess)
}

// --- Tasks ---

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.store == nil {
		writeJSON(w, []any{})
		return
	}

	// Optional status filter
	statusFilter := r.URL.Query().Get("status")

	var tasks []*task.Task
	var err error

	if statusFilter != "" {
		tasks, err = s.store.ListByStatus(task.Status(statusFilter))
	} else {
		// List all tasks by querying each status
		for _, st := range []task.Status{
			task.StatusCreated, task.StatusAssigned, task.StatusInProgress,
			task.StatusCompleted, task.StatusFailed, task.StatusCancelled,
		} {
			stTasks, serr := s.store.ListByStatus(st)
			if serr != nil {
				continue
			}
			tasks = append(tasks, stTasks...)
		}
		err = nil
	}

	if err != nil {
		writeJSONError(w, "failed to list tasks", http.StatusInternalServerError)
		return
	}

	writeJSON(w, tasks)
}

func (s *Server) handleTaskDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/v1/tasks/")
	if id == "" {
		http.Error(w, "task id required", http.StatusBadRequest)
		return
	}

	if s.store == nil {
		writeJSONError(w, "task store not available", http.StatusServiceUnavailable)
		return
	}

	tsk, err := s.store.Get(id)
	if err != nil {
		writeJSONError(w, err.Error(), http.StatusNotFound)
		return
	}

	writeJSON(w, tsk)
}

// --- Approvals ---

func (s *Server) handleApprovals(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.approval == nil {
		writeJSON(w, []any{})
		return
	}

	approvals, err := s.approval.ListPendingApprovals()
	if err != nil {
		writeJSONError(w, "failed to list approvals", http.StatusInternalServerError)
		return
	}

	writeJSON(w, approvals)
}

func (s *Server) handleApprovalAction(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/approvals/")

	// Expected: {id}/grant or {id}/deny
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 {
		http.Error(w, "expected /api/v1/approvals/{id}/grant or /api/v1/approvals/{id}/deny", http.StatusBadRequest)
		return
	}

	taskID := parts[0]
	action := parts[1]

	if s.approval == nil {
		writeJSONError(w, "approval manager not available", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()

	switch action {
	case "grant":
		// Read resolved_by from query param or JSON body
		resolvedBy := r.URL.Query().Get("by")
		if resolvedBy == "" {
			resolvedBy = "api:user"
		}

		if err := s.approval.GrantApproval(ctx, taskID, resolvedBy); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "granted", "task_id": taskID})

	case "deny":
		resolvedBy := r.URL.Query().Get("by")
		if resolvedBy == "" {
			resolvedBy = "api:user"
		}
		reason := r.URL.Query().Get("reason")
		if reason == "" {
			reason = "denied via API"
		}

		if err := s.approval.DenyApproval(ctx, taskID, resolvedBy, reason); err != nil {
			writeJSONError(w, err.Error(), http.StatusBadRequest)
			return
		}
		writeJSON(w, map[string]string{"status": "denied", "task_id": taskID})

	default:
		http.Error(w, "unknown action: use 'grant' or 'deny'", http.StatusBadRequest)
	}
}

// --- Event Stream (SSE) ---

func (s *Server) handleEventStream(w http.ResponseWriter, r *http.Request) {
	if s.nc == nil {
		writeJSONError(w, "NATS not connected", http.StatusServiceUnavailable)
		return
	}

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Optional subject filter — restrict to safe prefixes for security
	subject := r.URL.Query().Get("subject")
	if subject == "" {
		subject = ">"
	}

	// Block overly broad subscriptions for unauthenticated access
	// Allow specific prefixes like "lumi.*", "mango.*", "prism.*"
	// but deny unrestricted ">" without explicit opt-in
	if subject == ">" {
		// TODO: Add authentication check here. For now, allow unrestricted
		// but log a warning.
		log.Printf("[API] SSE: unrestricted subject subscription from %s", r.RemoteAddr)
	}

	// Subscribe to NATS
	sub, err := s.nc.SubscribeSync(subject)
	if err != nil {
		writeJSONError(w, fmt.Sprintf("subscribe failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer sub.Unsubscribe()

	// Send connected event
	fmt.Fprintf(w, "event: connected\ndata: {\"subject\":\"%s\"}\n\n", subject)
	flusher.Flush()

	// Heartbeat ticker
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	// Event loop
	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, "event: heartbeat\ndata: {\"ts\":\"%s\"}\n\n", time.Now().UTC().Format(time.RFC3339))
			flusher.Flush()
		default:
			// Non-blocking check for NATS messages
			msg, err := sub.NextMsg(100 * time.Millisecond)
			if err != nil {
				if err == nats.ErrTimeout {
					continue
				}
				return
			}

			// Parse event data
			var data map[string]any
			if err := json.Unmarshal(msg.Data, &data); err != nil {
				data = map[string]any{"raw": string(msg.Data)}
			}

			eventJSON, _ := json.Marshal(data)
			fmt.Fprintf(w, "event: %s\ndata: %s\nid: %s\n\n",
				msg.Subject,
				string(eventJSON),
				strconv.FormatInt(time.Now().UnixNano(), 10),
			)
			flusher.Flush()
		}
	}
}

// --- Workflows ---

func (s *Server) handleWorkflows(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Workflows are loaded from the orchestrator config
	if s.orch == nil {
		writeJSON(w, []any{})
		return
	}

	writeJSON(w, s.orch.Workflows())
}

// --- Costs ---

func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Cost data comes from the cost tracker in the orchestrator
	// For now, return a placeholder
	writeJSON(w, map[string]any{
		"note":      "cost tracking endpoint — integrate with internal/cost",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}

// --- Workflow SVG ---

func (s *Server) handleWorkflowSVG(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Path: /api/v1/workflows/{type}
	diagramType := strings.TrimPrefix(r.URL.Path, "/api/v1/workflows/")
	if diagramType == "" || diagramType == "list" {
		// List available diagram types
		writeJSON(w, map[string]any{
			"types": []string{"topology", "agents", "feedback", "delegation", "approval", "events"},
		})
		return
	}

	// Get agents from orchestrator
	var agents []orchestrator.AgentConfig
	if s.orch != nil {
		agents = s.orch.Config.Agents
	}

	cfg := workflow.DefaultConfig()

	// Query params for customization
	if r.URL.Query().Get("theme") == "light" {
		cfg.DarkTheme = false
	}
	if r.URL.Query().Get("capabilities") == "true" {
		cfg.ShowCapabilities = true
	}

	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Cache-Control", "no-cache")

	workflow.GenerateWorkflow(w, diagramType, agents, cfg)
}

// --- Helpers ---

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[API] json encode error: %v", err)
	}
}

func writeJSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}