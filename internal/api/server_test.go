package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emaharmony/prism/internal/delegation"
	"github.com/emaharmony/prism/internal/editor"
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

func TestAPI_Status_NilTracker(t *testing.T) {
	// Test with nil tracker to verify no panic
	dir := t.TempDir()
	store, err := task.NewStore(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	sessions, err := session.NewManager(filepath.Join(dir, "sessions.db"), 50, 30*time.Minute, 4, "truncate")
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	defer sessions.Close()

	// Create server with nil tracker
	server := NewServer(Config{
		Addr:     ":0",
		Orch:     nil,
		Store:    store,
		Sessions: sessions,
		Engine:   nil,
		Approval: nil,
		Tracker:  nil, // nil tracker
		NATS:     nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestAPI_Status_NilOrchestrator(t *testing.T) {
	// Test with nil orchestrator to verify graceful handling
	server := NewServer(Config{
		Addr:     ":0",
		Orch:     nil,
		Store:    nil,
		Sessions: nil,
		Engine:   nil,
		Approval: nil,
		Tracker:  nil,
		NATS:     nil,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/status", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Editor API Tests ---

func TestAPI_EditorState_Get(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/editor", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var state editor.EditorState
	if err := json.Unmarshal(w.Body.Bytes(), &state); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(state.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(state.Nodes))
	}
	if len(state.Edges) == 0 {
		t.Error("expected at least 1 edge")
	}
}

func TestAPI_EditorState_Put_Validate(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	body := `{
		"nodes": [
			{"id": "lumi", "type": "agent", "role": "lead", "model": "glm-5.1:cloud", "primary": true},
			{"id": "mango", "type": "agent", "role": "coder", "model": "deepseek-v4-pro:cloud"}
		],
		"edges": [
			{"id": "e1", "from": "lumi", "to": "mango", "type": "delegation"}
		]
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/v1/editor", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if result["valid"] != true {
		t.Errorf("expected valid=true, got %v", result["valid"])
	}
}

func TestAPI_EditorState_Put_Invalid(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	// Duplicate node IDs — should fail validation
	body := `{
		"nodes": [
			{"id": "dup", "type": "agent", "primary": true},
			{"id": "dup", "type": "agent"}
		],
		"edges": []
	}`

	req := httptest.NewRequest(http.MethodPut, "/api/v1/editor", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if result["valid"] == true {
		t.Error("expected valid=false for duplicate IDs")
	}
}

func TestAPI_EditorNodes_Add(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	body := `{"id": "junie", "type": "agent", "role": "developer", "model": "qwen3:4b"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/editor/nodes", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_EditorNode_Update(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	body := `{"model": "gpt-4o", "position": {"x": 300, "y": 200}}`

	req := httptest.NewRequest(http.MethodPut, "/api/v1/editor/nodes/lumi", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_EditorNode_Delete(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/editor/nodes/mango", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_EditorEdge_Add(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	body := `{"id": "e-test", "from": "lumi", "to": "mango", "type": "review"}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/editor/edges", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_EditorEdge_Delete(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	// First get current state to find an edge ID
	req := httptest.NewRequest(http.MethodGet, "/api/v1/editor", nil)
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	var state editor.EditorState
	json.Unmarshal(w.Body.Bytes(), &state)

	if len(state.Edges) == 0 {
		t.Skip("no edges to delete")
	}

	edgeID := state.Edges[0].ID

	// Delete the edge
	req2 := httptest.NewRequest(http.MethodDelete, "/api/v1/editor/edges/"+edgeID, nil)
	w2 := httptest.NewRecorder()
	srv.mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w2.Code, w2.Body.String())
	}
}

func TestAPI_EditorSave(t *testing.T) {
	srv, cleanup := newTestAPI(t)
	defer cleanup()

	body := `{
		"nodes": [
			{"id": "lumi", "type": "agent", "role": "lead", "model": "glm-5.1:cloud", "primary": true},
			{"id": "mango", "type": "agent", "role": "coder", "model": "deepseek-v4-pro:cloud"}
		],
		"edges": []
	}`

	req := httptest.NewRequest(http.MethodPost, "/api/v1/editor/save", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if result["valid"] != true {
		t.Errorf("expected valid=true, got %v", result["valid"])
	}
	if result["yaml"] == nil {
		t.Error("expected yaml in response")
	}
}