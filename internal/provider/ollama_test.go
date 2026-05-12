package provider_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/emaharmony/prism/internal/provider"
)

// ---------- compile-time interface check ----------

func TestOllamaProviderImplementsInterface(t *testing.T) {
	var _ provider.Provider = provider.NewOllamaProvider("")
	var _ provider.Provider = provider.NewOllamaProvider("http://custom:1234")
}

// ---------- connection refused (no server running) ----------

func TestOllamaProviderConnectionRefused(t *testing.T) {
	// Use a URL that is guaranteed to have nothing listening.
	// 127.0.0.1:1 should be unbound at test time.
	p := provider.NewOllamaProvider("http://127.0.0.1:1")
	req := provider.GenerateRequest{
		RunID:   "run_conn",
		Agent:   "lumi",
		Project: "prism",
		Task:    "connection refused test",
		Prompt:  "hi",
		Model:   "llama3.2",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.Generate(ctx, req)
	if err == nil {
		t.Fatal("expected error from connection refused, got nil")
	}
	// Should mention "ollama:" in the error chain — verify it doesn't panic.
	if !strings.Contains(err.Error(), "ollama:") && !strings.Contains(err.Error(), "connection refused") {
		t.Logf("error message (acceptable): %v", err)
	}
	t.Logf("connection refused error (expected): %v", err)
}

// ---------- success via httptest mock ----------

func TestOllamaProviderSuccess(t *testing.T) {
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the request path
		if r.URL.Path != "/api/generate" {
			t.Errorf("expected /api/generate, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Return a valid Ollama response
		resp := map[string]any{
			"model":             "llama3.2",
			"response":          "Hello! How can I help you today?",
			"prompt_eval_count": 42,
			"eval_count":        18,
			"total_duration":    int64(1_500_000_000), // 1.5s in ns
			"done":              true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockSrv.Close()

	p := provider.NewOllamaProvider(mockSrv.URL)
	req := provider.GenerateRequest{
		RunID:       "run_success",
		Agent:       "lumi",
		Project:     "prism",
		Task:        "greet",
		Prompt:      "Say hello",
		Model:       "llama3.2",
		Temperature: 0.7,
		MaxTokens:   256,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := p.Generate(ctx, req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}

	if resp.Text != "Hello! How can I help you today?" {
		t.Errorf("unexpected text: %s", resp.Text)
	}
	if resp.Model != "llama3.2" {
		t.Errorf("expected model llama3.2, got %s", resp.Model)
	}
	if resp.Provider != provider.OllamaProviderName {
		t.Errorf("expected provider %s, got %s", provider.OllamaProviderName, resp.Provider)
	}
	if resp.PromptTokens != 42 {
		t.Errorf("expected 42 prompt tokens, got %d", resp.PromptTokens)
	}
	if resp.OutputTokens != 18 {
		t.Errorf("expected 18 output tokens, got %d", resp.OutputTokens)
	}
	if resp.LatencyMS < 0 {
		t.Errorf("expected non-negative latency, got %d", resp.LatencyMS)
	}
	if resp.Raw == nil {
		t.Error("expected non-nil Raw")
	}
}

// ---------- timeout via short context deadline ----------

func TestOllamaProviderTimeout(t *testing.T) {
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate a very slow response — longer than the context deadline
		time.Sleep(500 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]any{
			"model":    "llama3.2",
			"response": "too late",
			"done":     true,
		})
	}))
	defer mockSrv.Close()

	p := provider.NewOllamaProvider(mockSrv.URL)
	req := provider.GenerateRequest{
		RunID:   "run_timeout",
		Agent:   "lumi",
		Project: "prism",
		Task:    "timeout test",
		Prompt:  "hi",
		Model:   "llama3.2",
	}

	// Set a very short deadline so the context expires before the handler responds.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := p.Generate(ctx, req)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "deadline") && !strings.Contains(errStr, "timeout") {
		t.Errorf("expected deadline/timeout error, got: %v", err)
	}
	t.Logf("timeout error (expected): %v", err)
}

// ---------- error JSON from Ollama (non-zero HTTP, or error field) ----------

func TestOllamaProviderHTTPError(t *testing.T) {
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error": "internal server error"}`))
	}))
	defer mockSrv.Close()

	p := provider.NewOllamaProvider(mockSrv.URL)
	req := provider.GenerateRequest{
		RunID:   "run_http_err",
		Agent:   "lumi",
		Project: "prism",
		Task:    "http error test",
		Prompt:  "hi",
		Model:   "llama3.2",
	}

	_, err := p.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error from non-200 response, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to mention status 500, got: %v", err)
	}
}

func TestOllamaProviderModelError(t *testing.T) {
	// Ollama returns HTTP 200 but with an "error" field when model is missing.
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"error": "model 'nonexistent-model' not found, try pulling it first",
		})
	}))
	defer mockSrv.Close()

	p := provider.NewOllamaProvider(mockSrv.URL)
	req := provider.GenerateRequest{
		RunID:   "run_model_err",
		Agent:   "lumi",
		Project: "prism",
		Task:    "model error test",
		Prompt:  "hi",
		Model:   "nonexistent-model",
	}

	_, err := p.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error from missing model, got nil")
	}
	if !strings.Contains(err.Error(), "model") {
		t.Errorf("expected error to mention model, got: %v", err)
	}
	t.Logf("model error (expected): %v", err)
}

// ---------- default base URL when empty ----------

func TestOllamaProviderDefaultURL(t *testing.T) {
	p := provider.NewOllamaProvider("")
	if p.BaseURL != provider.DefaultOllamaBaseURL {
		t.Errorf("expected default URL %s, got %s", provider.DefaultOllamaBaseURL, p.BaseURL)
	}
	if p.HTTPClient == nil {
		t.Error("expected non-nil HTTPClient")
	}
}

// ---------- custom base URL ----------

func TestOllamaProviderCustomURL(t *testing.T) {
	custom := "http://ollama.internal:11434"
	p := provider.NewOllamaProvider(custom)
	if p.BaseURL != custom {
		t.Errorf("expected base URL %s, got %s", custom, p.BaseURL)
	}
}

// ---------- token fallback when Ollama returns 0 ----------

func TestOllamaProviderTokenFallback(t *testing.T) {
	mockSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{
			"model":             "llama3.2",
			"response":          "Short reply",
			"prompt_eval_count": 0,
			"eval_count":        0,
			"done":              true,
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer mockSrv.Close()

	p := provider.NewOllamaProvider(mockSrv.URL)
	req := provider.GenerateRequest{
		RunID:   "run_fallback",
		Prompt:  "some prompt text here for counting",
		Model:   "llama3.2",
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// PromptTokens should fall back to len(prompt)/4
	if resp.PromptTokens <= 0 {
		t.Errorf("expected positive prompt tokens via fallback, got %d", resp.PromptTokens)
	}
	// OutputTokens should fall back to len(text)/4
	if resp.OutputTokens <= 0 {
		t.Errorf("expected positive output tokens via fallback, got %d", resp.OutputTokens)
	}
	t.Logf("fallback prompt_tokens=%d output_tokens=%d", resp.PromptTokens, resp.OutputTokens)
}
