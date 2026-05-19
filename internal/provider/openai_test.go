package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIProvider_Name(t *testing.T) {
	p := NewOpenAIProvider("test-key")
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want openai", p.Name())
	}
}

func TestOpenAIProvider_Tier(t *testing.T) {
	p := NewOpenAIProvider("test-key")
	if p.Tier() != TierPaid {
		t.Errorf("Tier() = %q, want paid", p.Tier())
	}
}

func TestOpenAIProvider_Generate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"id": "chatcmpl-test",
			"object": "chat.completion",
			"model": "gpt-4",
			"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"}],
			"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
		}`))
	}))
	defer server.Close()

	p := NewOpenAIProviderWithBaseURL("test-key", server.URL)
	resp, err := p.Generate(context.Background(), GenerateRequest{
		Prompt: "Say hello",
		Model:  "gpt-4",
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if resp.Text != "Hello!" {
		t.Errorf("Text = %q, want Hello!", resp.Text)
	}
	if resp.Provider != "openai" {
		t.Errorf("Provider = %q, want openai", resp.Provider)
	}
	if resp.PromptTokens != 10 {
		t.Errorf("PromptTokens = %d, want 10", resp.PromptTokens)
	}
	if resp.OutputTokens != 5 {
		t.Errorf("OutputTokens = %d, want 5", resp.OutputTokens)
	}
}

func TestOpenAIProvider_Generate_RateLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	p := NewOpenAIProviderWithBaseURL("test-key", server.URL)
	_, err := p.Generate(context.Background(), GenerateRequest{
		Prompt: "test",
		Model:  "gpt-4",
	})
	if err == nil {
		t.Fatal("expected error for 429")
	}
	if !IsOpenAIErrorRetryable(err) {
		t.Error("429 error should be retryable")
	}
}

func TestOpenAIProvider_Generate_ServiceUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	p := NewOpenAIProviderWithBaseURL("test-key", server.URL)
	_, err := p.Generate(context.Background(), GenerateRequest{
		Prompt: "test",
		Model:  "gpt-4",
	})
	if err == nil {
		t.Fatal("expected error for 503")
	}
	if !IsOpenAIErrorRetryable(err) {
		t.Error("503 error should be retryable")
	}
}

func TestOpenAIProvider_Generate_ForbiddenError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error": {"message": "invalid api key"}}`))
	}))
	defer server.Close()

	p := NewOpenAIProviderWithBaseURL("test-key", server.URL)
	_, err := p.Generate(context.Background(), GenerateRequest{
		Prompt: "test",
		Model:  "gpt-4",
	})
	if err == nil {
		t.Fatal("expected error for 403")
	}
	if IsOpenAIErrorRetryable(err) {
		t.Error("403 error should NOT be retryable")
	}
}

func TestOpenAIProvider_Generate_NoChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id": "test", "object": "chat.completion", "model": "gpt-4", "choices": []}`))
	}))
	defer server.Close()

	p := NewOpenAIProviderWithBaseURL("test-key", server.URL)
	_, err := p.Generate(context.Background(), GenerateRequest{
		Prompt: "test",
		Model:  "gpt-4",
	})
	if err == nil {
		t.Fatal("expected error for empty choices")
	}
}

func TestOpenAIProvider_Generate_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewOpenAIProvider("test-key")
	_, err := p.Generate(ctx, GenerateRequest{
		Prompt: "test",
		Model:  "gpt-4",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestOpenAIProvider_CustomBaseURL(t *testing.T) {
	p := NewOpenAIProviderWithBaseURL("test-key", "https://api.together.ai/v1")
	if p.baseURL != "https://api.together.ai/v1" {
		t.Errorf("baseURL = %q, want https://api.together.ai/v1", p.baseURL)
	}
}

func TestOpenAIProvider_DefaultBaseURL(t *testing.T) {
	p := NewOpenAIProvider("test-key")
	if p.baseURL != "https://api.openai.com/v1" {
		t.Errorf("baseURL = %q, want https://api.openai.com/v1", p.baseURL)
	}
}

// Mock provider for chain tests
type testProvider struct {
	response GenerateResponse
	err      error
	tier     ProviderTier
}

func (m *testProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	if m.err != nil {
		return GenerateResponse{}, m.err
	}
	return m.response, nil
}

func (m *testProvider) Name() string { return "test" }
func (m *testProvider) Tier() ProviderTier { return m.tier }