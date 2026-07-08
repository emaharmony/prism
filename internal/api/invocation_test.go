package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/session"
	"github.com/emaharmony/prism/internal/task"
)

// fakeChatProvider implements both provider.Provider and provider.ChatProvider
// so it can be registered in a provider.ProviderRegistry and resolved via
// GetChatProvider, without depending on a real LLM backend.
type fakeChatProvider struct {
	content string
	err     error
}

func (f *fakeChatProvider) Generate(ctx context.Context, req provider.GenerateRequest) (provider.GenerateResponse, error) {
	return provider.GenerateResponse{Text: f.content}, f.err
}

func (f *fakeChatProvider) ChatGenerate(ctx context.Context, req provider.ChatGenerateRequest) (provider.ChatGenerateResponse, error) {
	if f.err != nil {
		return provider.ChatGenerateResponse{}, f.err
	}
	return provider.ChatGenerateResponse{Content: f.content, Model: req.Model, Provider: "fake"}, nil
}

var (
	_ provider.Provider     = (*fakeChatProvider)(nil)
	_ provider.ChatProvider = (*fakeChatProvider)(nil)
)

func newInvocationTestAPI(t *testing.T, agents []orchestrator.AgentConfig, chatProv *fakeChatProvider) (*Server, func()) {
	t.Helper()
	dir := t.TempDir()

	store, err := task.NewStore(filepath.Join(dir, "tasks.db"))
	if err != nil {
		t.Fatalf("task store: %v", err)
	}
	sessions, err := session.NewManager(filepath.Join(dir, "sessions.db"), 50, 30*time.Minute, 4, "truncate")
	if err != nil {
		t.Fatalf("session manager: %v", err)
	}

	cfg := &orchestrator.Config{
		Prism:    orchestrator.PrismConfig{DataDir: dir},
		Agents:   agents,
		Sessions: orchestrator.SessionConfig{MaxContextMessages: 50, CompactionStrategy: "truncate"},
	}
	orch, err := orchestrator.New(cfg)
	if err != nil {
		t.Fatalf("orchestrator: %v", err)
	}

	// A nil chatProv means "simulate a deployment where invocation was never
	// wired up" — leave Providers nil rather than an empty registry, so the
	// synchronous "not configured" guard (as opposed to an async
	// model-not-found failure) is what gets exercised.
	var providers *provider.ProviderRegistry
	if chatProv != nil {
		providers = provider.NewProviderRegistry()
		for _, a := range agents {
			providers.Register(a.Model, chatProv, provider.ModelInfo{ProviderName: a.Provider})
		}
	}

	server := NewServer(Config{
		Addr:      ":0",
		Orch:      orch,
		Store:     store,
		Sessions:  sessions,
		Providers: providers,
		NATS:      nil,
	})

	cleanup := func() {
		store.Close()
		sessions.Close()
	}
	return server, cleanup
}

func invokableAgent(id string) orchestrator.AgentConfig {
	return orchestrator.AgentConfig{
		ID:              id,
		Role:            "stream-clip-judge",
		Provider:        "fake",
		Model:           "fake-model",
		InvocableViaAPI: true,
	}
}

func postInvoke(t *testing.T, s *Server, agentID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/agents/"+agentID+"/invoke", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	return w
}

func waitForInvocation(t *testing.T, s *Server, agentID, invocationID string) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/agents/%s/invocations/%s", agentID, invocationID), nil)
		w := httptest.NewRecorder()
		s.mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("poll invocation: expected 200, got %d: %s", w.Code, w.Body.String())
		}
		var resp map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("poll invocation: parse response: %v", err)
		}
		if resp["status"] != "pending" {
			return resp
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("invocation %s did not complete within deadline", invocationID)
	return nil
}

func TestAPI_AgentInvoke_NotInvocable(t *testing.T) {
	agents := []orchestrator.AgentConfig{{ID: "lumi", Role: "lead", Provider: "fake", Model: "fake-model"}}
	s, cleanup := newInvocationTestAPI(t, agents, &fakeChatProvider{content: `{"ok":true}`})
	defer cleanup()

	w := postInvoke(t, s, "lumi", `{"prompt":"hello"}`)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_AgentInvoke_AgentNotFound(t *testing.T) {
	s, cleanup := newInvocationTestAPI(t, nil, &fakeChatProvider{content: `{}`})
	defer cleanup()

	w := postInvoke(t, s, "ghost", `{"prompt":"hello"}`)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_AgentInvoke_MissingPrompt(t *testing.T) {
	agents := []orchestrator.AgentConfig{invokableAgent("clippy")}
	s, cleanup := newInvocationTestAPI(t, agents, &fakeChatProvider{content: `{}`})
	defer cleanup()

	w := postInvoke(t, s, "clippy", `{"prompt":""}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_AgentInvoke_SuccessJSONResult(t *testing.T) {
	agents := []orchestrator.AgentConfig{invokableAgent("clippy")}
	s, cleanup := newInvocationTestAPI(t, agents, &fakeChatProvider{
		content: `{"clip_worthy":true,"confidence":0.87,"title":"Big reaction","reason":"chat spike"}`,
	})
	defer cleanup()

	w := postInvoke(t, s, "clippy", `{"prompt":"chat velocity 4.2x baseline"}`)
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var accepted map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &accepted); err != nil {
		t.Fatalf("parse accepted response: %v", err)
	}
	invocationID, _ := accepted["invocation_id"].(string)
	if invocationID == "" {
		t.Fatalf("expected invocation_id in response, got %v", accepted)
	}
	if accepted["status"] != "pending" {
		t.Errorf("expected initial status pending, got %v", accepted["status"])
	}

	final := waitForInvocation(t, s, "clippy", invocationID)
	if final["status"] != "completed" {
		t.Fatalf("expected status completed, got %v (full: %v)", final["status"], final)
	}
	result, ok := final["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result object, got %v", final["result"])
	}
	if result["clip_worthy"] != true {
		t.Errorf("expected clip_worthy=true, got %v", result["clip_worthy"])
	}
	if result["title"] != "Big reaction" {
		t.Errorf("expected title 'Big reaction', got %v", result["title"])
	}
}

func TestAPI_AgentInvoke_NonJSONResultWrapped(t *testing.T) {
	agents := []orchestrator.AgentConfig{invokableAgent("clippy")}
	s, cleanup := newInvocationTestAPI(t, agents, &fakeChatProvider{content: "not json at all"})
	defer cleanup()

	w := postInvoke(t, s, "clippy", `{"prompt":"hello"}`)
	var accepted map[string]any
	json.Unmarshal(w.Body.Bytes(), &accepted)
	invocationID := accepted["invocation_id"].(string)

	final := waitForInvocation(t, s, "clippy", invocationID)
	if final["status"] != "completed" {
		t.Fatalf("expected status completed, got %v", final["status"])
	}
	result := final["result"].(map[string]any)
	if result["text"] != "not json at all" {
		t.Errorf("expected wrapped text result, got %v", result)
	}
}

func TestAPI_AgentInvoke_ProviderErrorFails(t *testing.T) {
	agents := []orchestrator.AgentConfig{invokableAgent("clippy")}
	s, cleanup := newInvocationTestAPI(t, agents, &fakeChatProvider{err: fmt.Errorf("provider unavailable")})
	defer cleanup()

	w := postInvoke(t, s, "clippy", `{"prompt":"hello"}`)
	var accepted map[string]any
	json.Unmarshal(w.Body.Bytes(), &accepted)
	invocationID := accepted["invocation_id"].(string)

	final := waitForInvocation(t, s, "clippy", invocationID)
	if final["status"] != "failed" {
		t.Fatalf("expected status failed, got %v", final["status"])
	}
	if final["error"] == nil || final["error"] == "" {
		t.Errorf("expected a non-empty error message, got %v", final["error"])
	}
}

func TestAPI_AgentInvocationDetail_NotFound(t *testing.T) {
	agents := []orchestrator.AgentConfig{invokableAgent("clippy")}
	s, cleanup := newInvocationTestAPI(t, agents, &fakeChatProvider{content: `{}`})
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/agents/clippy/invocations/does-not-exist", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestAPI_AgentInvoke_NoProvidersConfigured(t *testing.T) {
	agents := []orchestrator.AgentConfig{invokableAgent("clippy")}
	s, cleanup := newInvocationTestAPI(t, agents, nil)
	defer cleanup()

	w := postInvoke(t, s, "clippy", `{"prompt":"hello"}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}
