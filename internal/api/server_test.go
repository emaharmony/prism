package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/emaharmony/prism/internal/delegation"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/session"
	"github.com/emaharmony/prism/internal/task"
)

func newTestAPI(t *testing.T) (*Server, func()) {
	t.Helper()

	// Create temp directory for stores
	dir := t.TempDir()

	// Task store
	store, err := task.NewStore(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatalf("failed to create task store: %v", err)
	}

	// Session manager
	sessions, err := session.NewManager(filepath.Join(dir, "sessions.db"), 50, 30*time.Minute, 4, "truncate")
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	// Delegation engine
	engine := delegation.NewEngine(store, nil)

	// Approval manager
	approval := delegation.NewApprovalManager(store, engine)

	// Tracker
	tracker := delegation.NewTracker(store, engine, delegation.TrackerConfig{
		TaskTimeout:   10 * time.Minute,
		CheckInterval: 1 * time.Minute,
	})

	// Config for orchestrator
	cfg := &orchestrator.Config{
		Prism: orchestrator.PrismConfig{
			DataDir: dir,
		},
		Agents: []orchestrator.AgentConfig{
			{
				ID:           "lumi",
				Role:         "lead",
				Provider:     "ollama",
				Model:        "glm-5.1:cloud",
				Primary:      true,
				Capabilities: []string{"plan", "delegate", "review", "approve"},
			},
			{
				ID:           "mango",
				Role:         "coder",
				Provider:     "ollama",
				Model:        "deepseek-v4-pro:cloud",
				Capabilities: []string{"code", "test", "delegate"},
			},
		},
		Sessions: orchestrator.SessionConfig{
			MaxContextMessages: 50,
			CompactionStrategy: "truncate",
		},
	}

	orch, err := orchestrator.New(cfg)
	if err != nil {
		t.Fatalf("failed to create orchestrator: %v", err)
	}

	server := NewServer(Config{
		Addr:     ":0", // random port for testing
		Orch:     orch,
		Store:    store,
		Sessions: sessions,
		Engine:   engine,
		Approval: approval,
		Tracker:  tracker,
		NATS:     nil, // No NATS in tests
	})

	cleanup := func() {
		store.Close()
		sessions.Close()
	}

	return server, cleanup
}

func TestAPI_Status(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "running" {
		t.Errorf("expected status running, got %v", resp["status"])
	}
}

func TestAPI_Agents(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPI_AgentDetail(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	// Existing agent
	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/lumi", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for existing agent, got %d", w.Code)
	}

	// Non-existent agent
	req = httptest.NewRequest(http.MethodGet, "/api/v1/agents/nonexistent", nil)
	w = httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for nonexistent agent, got %d", w.Code)
	}
}

func TestAPI_Sessions(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/sessions", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPI_Tasks(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var tasks []any
	if err := json.Unmarshal(w.Body.Bytes(), &tasks); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
}

func TestAPI_Tasks_ByStatus(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/tasks?status=completed", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPI_Approvals(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/approvals", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPI_Approval_Grant(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	// Create an approval first
	ctx := t.Context()
	approval, err := s.approval.RequestApproval(ctx, "lumi", delegation.ApprovalPush, "Push to main?", "origin/main")
	if err != nil {
		t.Fatalf("failed to create approval: %v", err)
	}

	// Grant it via API
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+approval.TaskID+"/grant?by=user-test", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "granted" {
		t.Errorf("expected status granted, got %s", resp["status"])
	}
}

func TestAPI_Approval_Deny(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	// Create an approval
	ctx := t.Context()
	approval, err := s.approval.RequestApproval(ctx, "lumi", delegation.ApprovalDeploy, "Deploy to prod?", "production")
	if err != nil {
		t.Fatalf("failed to create approval: %v", err)
	}

	// Deny it via API
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/"+approval.TaskID+"/deny?by=user-test&reason=not+ready", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["status"] != "denied" {
		t.Errorf("expected status denied, got %s", resp["status"])
	}
}

func TestAPI_Workflows(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPI_Costs(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/costs", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPI_MethodNotAllowed(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	// POST to a GET-only endpoint
	req := httptest.NewRequest(http.MethodPost, "/api/v1/status", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestAPI_ApprovalAction_Invalid(t *testing.T) {
	s, cleanup := newTestAPI(t)
	defer cleanup()

	// Invalid action
	req := httptest.NewRequest(http.MethodPost, "/api/v1/approvals/fake-id/invalid", nil)
	w := httptest.NewRecorder()

	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}