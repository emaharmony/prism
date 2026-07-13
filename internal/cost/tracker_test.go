package cost

import (
	"testing"
)

func TestCostTracker_Track(t *testing.T) {
	tracker := NewCostTracker("run_123")

	tracker.Track("ollama", "llama3", "analyst", &TokenUsage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
		EstimatedCostUsd: 0.0,
	})

	tracker.Track("openai", "gpt-4o", "analyst", &TokenUsage{
		PromptTokens:     200,
		CompletionTokens: 100,
		TotalTokens:      300,
		EstimatedCostUsd: 0.00075,
	})

	report := tracker.Report()

	if report.TotalTokens != 450 {
		t.Errorf("expected 450 total tokens, got %d", report.TotalTokens)
	}
	if report.PromptTokens != 300 {
		t.Errorf("expected 300 prompt tokens, got %d", report.PromptTokens)
	}
	if report.CompletionTokens != 150 {
		t.Errorf("expected 150 completion tokens, got %d", report.CompletionTokens)
	}
	if report.EstimatedCostUsd != 0.00075 {
		t.Errorf("expected $0.00075, got $%f", report.EstimatedCostUsd)
	}
	if report.EventCount != 2 {
		t.Errorf("expected 2 events, got %d", report.EventCount)
	}
}

func TestCostTracker_ByProvider(t *testing.T) {
	tracker := NewCostTracker("run_123")

	tracker.Track("openai", "gpt-4o", "agent1", &TokenUsage{
		TotalTokens:      1000,
		EstimatedCostUsd: 0.0025,
	})

	tracker.Track("openai", "gpt-4o", "agent2", &TokenUsage{
		TotalTokens:      500,
		EstimatedCostUsd: 0.00125,
	})

	tracker.Track("anthropic", "claude-sonnet-4-20250514", "agent1", &TokenUsage{
		TotalTokens:      2000,
		EstimatedCostUsd: 0.006,
	})

	report := tracker.Report()

	if len(report.ByProvider) != 2 {
		t.Errorf("expected 2 providers, got %d", len(report.ByProvider))
	}
	if report.ByProvider["openai"] != 0.00375 {
		t.Errorf("expected openai cost $0.00375, got $%f", report.ByProvider["openai"])
	}
	if report.ByProvider["anthropic"] != 0.006 {
		t.Errorf("expected anthropic cost $0.006, got $%f", report.ByProvider["anthropic"])
	}
}

func TestCostTracker_ByModel(t *testing.T) {
	tracker := NewCostTracker("run_123")

	tracker.Track("openai", "gpt-4o", "agent1", &TokenUsage{
		TotalTokens:      1000,
		EstimatedCostUsd: 0.0025,
	})

	tracker.Track("openai", "gpt-4o-mini", "agent1", &TokenUsage{
		TotalTokens:      2000,
		EstimatedCostUsd: 0.0003,
	})

	report := tracker.Report()

	if len(report.ByModel) != 2 {
		t.Errorf("expected 2 models, got %d", len(report.ByModel))
	}
	if report.ByModel["gpt-4o"] != 0.0025 {
		t.Errorf("expected gpt-4o cost $0.0025, got $%f", report.ByModel["gpt-4o"])
	}
}

func TestCostTracker_ByAgent(t *testing.T) {
	tracker := NewCostTracker("run_123")

	tracker.Track("openai", "gpt-4o", "analyst", &TokenUsage{
		TotalTokens:      1000,
		EstimatedCostUsd: 0.0025,
	})

	tracker.Track("openai", "gpt-4o", "reviewer", &TokenUsage{
		TotalTokens:      500,
		EstimatedCostUsd: 0.00125,
	})

	report := tracker.Report()

	if report.ByAgent["analyst"] != 1000 {
		t.Errorf("expected analyst 1000 tokens, got %d", report.ByAgent["analyst"])
	}
	if report.ByAgent["reviewer"] != 500 {
		t.Errorf("expected reviewer 500 tokens, got %d", report.ByAgent["reviewer"])
	}
}

func TestCostTracker_NilUsage(t *testing.T) {
	tracker := NewCostTracker("run_123")
	tracker.Track("ollama", "llama3", "agent", nil) // Should not panic

	report := tracker.Report()
	if report.TotalTokens != 0 {
		t.Errorf("expected 0 tokens for nil usage, got %d", report.TotalTokens)
	}
}

func TestCostTracker_EmptyReport(t *testing.T) {
	tracker := NewCostTracker("run_123")
	report := tracker.Report()

	if report.RunID != "run_123" {
		t.Errorf("expected run_id 'run_123', got %q", report.RunID)
	}
	if report.TotalTokens != 0 {
		t.Errorf("expected 0 tokens, got %d", report.TotalTokens)
	}
	if report.EventCount != 0 {
		t.Errorf("expected 0 events, got %d", report.EventCount)
	}
}

func TestEstimateCost_KnownModel(t *testing.T) {
	cost := EstimateCost("gpt-4o", 1000)
	if cost != 0.0025 {
		t.Errorf("expected $0.0025 for 1K gpt-4o tokens, got $%f", cost)
	}

	cost = EstimateCost("gpt-4o", 5000)
	if cost != 0.0125 {
		t.Errorf("expected $0.0125 for 5K gpt-4o tokens, got $%f", cost)
	}
}

func TestEstimateCost_LocalModel(t *testing.T) {
	cost := EstimateCost("llama3", 10000)
	if cost != 0.0 {
		t.Errorf("expected $0 for local model, got $%f", cost)
	}
}

func TestEstimateCost_UnknownModel(t *testing.T) {
	cost := EstimateCost("future-model-x", 1000)
	if cost != 0.0 {
		t.Errorf("expected $0 for unknown model, got $%f", cost)
	}
}

func TestEstimateCost_AnthropicModel(t *testing.T) {
	cost := EstimateCost("claude-sonnet-4-20250514", 1000)
	if cost != 0.003 {
		t.Errorf("expected $0.003 for 1K claude-sonnet tokens, got $%f", cost)
	}
}

func TestEstimateCost_GeminiModel(t *testing.T) {
	cost := EstimateCost("gemini-2.0-flash", 1000)
	if cost != 0.0001 {
		t.Errorf("expected $0.0001 for 1K gemini-2.0-flash tokens, got $%f", cost)
	}
}
