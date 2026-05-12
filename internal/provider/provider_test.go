package provider_test

import (
	"context"
	"testing"
	"time"

	"github.com/emaharmony/prism/internal/provider"
)

func TestMockProviderSuccess(t *testing.T) {
	p := provider.NewMockProvider()
	req := provider.GenerateRequest{
		RunID:       "run_test123",
		Agent:       "lumi",
		Project:     "prism",
		Task:        "Explain the lifecycle",
		Prompt:      "system prompt",
		Model:       "mock-model",
		Temperature: 0.2,
		MaxTokens:   2048,
	}

	resp, err := p.Generate(context.Background(), req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if resp.Provider != provider.MockProviderName {
		t.Errorf("expected provider %s, got %s", provider.MockProviderName, resp.Provider)
	}
	if resp.Text == "" {
		t.Error("expected non-empty text")
	}
	if resp.Model != "mock-model" {
		t.Errorf("expected model mock-model, got %s", resp.Model)
	}
	if resp.LatencyMS < 0 {
		t.Errorf("expected non-negative latency, got %d", resp.LatencyMS)
	}
	if resp.PromptTokens <= 0 {
		t.Errorf("expected positive prompt tokens, got %d", resp.PromptTokens)
	}
	if resp.OutputTokens <= 0 {
		t.Errorf("expected positive output tokens, got %d", resp.OutputTokens)
	}
}

func TestFailingMockProvider(t *testing.T) {
	p := provider.NewFailingMockProvider()
	req := provider.GenerateRequest{
		RunID:   "run_fail",
		Agent:   "lumi",
		Project: "prism",
		Task:    "should fail",
		Prompt:  "test",
		Model:   "mock-model",
	}

	_, err := p.Generate(context.Background(), req)
	if err == nil {
		t.Fatal("expected error from failing mock provider, got nil")
	}
}

func TestMockProviderContextCancellation(t *testing.T) {
	p := provider.NewMockProvider()
	req := provider.GenerateRequest{
		Task:   "cancel test",
		Prompt: "test",
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := p.Generate(ctx, req)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestProviderInterface(t *testing.T) {
	// Verify both MockProvider and FailingMockProvider implement Provider
	var _ provider.Provider = provider.NewMockProvider()
	var _ provider.Provider = provider.NewFailingMockProvider()
}

func TestMockProviderLatency(t *testing.T) {
	p := provider.NewMockProvider()
	req := provider.GenerateRequest{
		Task:   "latency test",
		Prompt: "test prompt content for counting",
	}

	start := time.Now()
	resp, err := p.Generate(context.Background(), req)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed < 10*time.Millisecond {
		t.Errorf("expected at least 10ms delay, got %v", elapsed)
	}
	if resp.LatencyMS <= 0 {
		t.Errorf("expected positive latency_ms, got %d", resp.LatencyMS)
	}
}