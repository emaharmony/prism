package provider

import (
	"context"
	"fmt"
	"testing"
)

// mockP is a minimal Provider implementation for registry tests.
type mockP struct{}

func (m *mockP) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	return GenerateResponse{Text: "mock", Model: req.Model, Provider: "mock"}, nil
}

func (m *mockP) Name() string { return "mock" }

func (m *mockP) Tier() ProviderTier { return TierFree }

func TestProviderRegistry_RegisterAndGet(t *testing.T) {
	reg := NewProviderRegistry()
	var p Provider = &mockP{}

	reg.Register("mock-model", p, ModelInfo{
		ID:            "mock-model",
		ContextWindow: 4096,
		ProviderName:  "mock",
		Cost: ModelCost{
			InputPer1K:  0.0,
			OutputPer1K: 0.0,
		},
	})

	got, err := reg.Get("mock-model")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if got == nil {
		t.Fatal("expected provider, got nil")
	}
}

func TestProviderRegistry_GetNotFound(t *testing.T) {
	reg := NewProviderRegistry()

	_, err := reg.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent model")
	}
}

func TestProviderRegistry_ModelInfo(t *testing.T) {
	reg := NewProviderRegistry()
	var p Provider = &mockP{}

	reg.Register("gpt-4o", p, ModelInfo{
		ID:            "gpt-4o",
		ContextWindow: 128000,
		InputTypes:    []string{"text", "image"},
		Reasoning:     true,
		ProviderName:  "openai",
		Cost: ModelCost{
			InputPer1K:  0.0025,
			OutputPer1K: 0.01,
		},
	})

	info, err := reg.ModelInfo("gpt-4o")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if info.ContextWindow != 128000 {
		t.Errorf("expected context window 128000, got %d", info.ContextWindow)
	}
	if info.ProviderName != "openai" {
		t.Errorf("expected provider openai, got %s", info.ProviderName)
	}
	if info.Cost.InputPer1K != 0.0025 {
		t.Errorf("expected input cost 0.0025, got %f", info.Cost.InputPer1K)
	}
}

func TestProviderRegistry_ListModels(t *testing.T) {
	reg := NewProviderRegistry()
	var p Provider = &mockP{}

	reg.Register("model-a", p, ModelInfo{ID: "model-a", ProviderName: "mock"})
	reg.Register("model-b", p, ModelInfo{ID: "model-b", ProviderName: "mock"})

	models := reg.ListModels()
	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}
}

func TestProviderRegistry_ListModelsByProvider(t *testing.T) {
	reg := NewProviderRegistry()
	var p Provider = &mockP{}

	reg.Register("ollama-model", p, ModelInfo{ID: "ollama-model", ProviderName: "ollama"})
	reg.Register("openai-model", p, ModelInfo{ID: "openai-model", ProviderName: "openai"})

	ollamaModels := reg.ListModelsByProvider("ollama")
	if len(ollamaModels) != 1 {
		t.Fatalf("expected 1 ollama model, got %d", len(ollamaModels))
	}
	if ollamaModels[0] != "ollama-model" {
		t.Errorf("expected ollama-model, got %s", ollamaModels[0])
	}

	openaiModels := reg.ListModelsByProvider("openai")
	if len(openaiModels) != 1 {
		t.Fatalf("expected 1 openai model, got %d", len(openaiModels))
	}
}

func TestProviderRegistry_HasModel(t *testing.T) {
	reg := NewProviderRegistry()
	var p Provider = &mockP{}

	reg.Register("test-model", p, ModelInfo{ID: "test-model"})

	if !reg.HasModel("test-model") {
		t.Error("expected HasModel to return true")
	}
	if reg.HasModel("nonexistent") {
		t.Error("expected HasModel to return false for nonexistent model")
	}
}

func TestProviderRegistry_AllModelInfo(t *testing.T) {
	reg := NewProviderRegistry()
	var p Provider = &mockP{}

	reg.Register("model-a", p, ModelInfo{ID: "model-a", ContextWindow: 4096})
	reg.Register("model-b", p, ModelInfo{ID: "model-b", ContextWindow: 8192})

	all := reg.AllModelInfo()
	if len(all) != 2 {
		t.Fatalf("expected 2 models, got %d", len(all))
	}
	if all["model-a"].ContextWindow != 4096 {
		t.Errorf("expected model-a context window 4096, got %d", all["model-a"].ContextWindow)
	}
}

func TestProviderRegistry_ConcurrentAccess(t *testing.T) {
	reg := NewProviderRegistry()
	var p Provider = &mockP{}

	done := make(chan bool)
	for i := 0; i < 100; i++ {
		go func(i int) {
			reg.Register(fmt.Sprintf("model-%d", i), p, ModelInfo{ID: fmt.Sprintf("model-%d", i)})
			done <- true
		}(i)
	}

	for i := 0; i < 100; i++ {
		<-done
	}

	models := reg.ListModels()
	if len(models) != 100 {
		t.Errorf("expected 100 models after concurrent registration, got %d", len(models))
	}
}