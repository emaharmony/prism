package main

import (
	"testing"

	"github.com/emaharmony/prizm/internal/orchestrator"
	"github.com/emaharmony/prizm/internal/provider"
)

func TestResolveContextMode(t *testing.T) {
	tests := []struct {
		name       string
		globalMode string
		agentMode  string
		want       string
	}{
		{"default both empty", "", "", "full"},
		{"global open_book", "open_book", "", "open_book"},
		{"global full", "full", "", "full"},
		{"agent override takes priority", "full", "open_book", "open_book"},
		{"agent override to full", "open_book", "full", "full"},
		{"agent override when global empty", "", "open_book", "open_book"},
		{"agent override when global empty full", "", "full", "full"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &orchestrator.Config{
				Prizm: orchestrator.PrizmConfig{
					ContextMode: tt.globalMode,
				},
			}
			agentCfg := &orchestrator.AgentConfig{
				ContextMode: tt.agentMode,
			}
			got := resolveContextMode(cfg, agentCfg)
			if got != tt.want {
				t.Errorf("resolveContextMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveAgentLoop(t *testing.T) {
	tests := []struct {
		name       string
		globalLoop string
		agentLoop  string
		want       string
	}{
		{"default both empty", "", "", "classic"},
		{"global agentic", "agentic", "", "agentic"},
		{"global classic", "classic", "", "classic"},
		{"agent override takes priority", "classic", "agentic", "agentic"},
		{"agent override to classic", "agentic", "classic", "classic"},
		{"agent override when global empty", "", "agentic", "agentic"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &orchestrator.Config{
				Prizm: orchestrator.PrizmConfig{
					AgentLoop: tt.globalLoop,
				},
			}
			agentCfg := &orchestrator.AgentConfig{
				AgentLoop: tt.agentLoop,
			}
			got := resolveAgentLoop(cfg, agentCfg)
			if got != tt.want {
				t.Errorf("resolveAgentLoop() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetModelContextTokens(t *testing.T) {
	tests := []struct {
		model string
		want  int
		found bool
	}{
		{"glm-5.1:cloud", 202752, true},
		{"deepseek-v4-pro:cloud", 131072, true},
		{"unknown-model", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got, ok := getModelContextTokens(tt.model)
			if ok != tt.found {
				t.Errorf("getModelContextTokens(%q) found = %v, want %v", tt.model, ok, tt.found)
			}
			if ok && got != tt.want {
				t.Errorf("getModelContextTokens(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}

func TestEstimateTokenCount(t *testing.T) {
	msg := []provider.ChatMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "Hello!"},
		{Role: "assistant", Content: "Hi there!"},
	}
	count := estimateTokenCount(msg)
	if count <= 0 {
		t.Errorf("estimateTokenCount() = %d, want > 0", count)
	}
	if count > 100 {
		t.Errorf("estimateTokenCount() = %d, seems too high for ~50 chars", count)
	}
}

func TestCompressToolResults(t *testing.T) {
	longResult := make([]byte, 2000)
	for i := range longResult {
		longResult[i] = 'x'
	}

	messages := []provider.ChatMessage{
		{Role: "user", Content: "Hello"},
		{Role: "tool", Content: string(longResult), ToolID: "1"},
		{Role: "tool", Content: string(longResult), ToolID: "2"},
		{Role: "tool", Content: "short result", ToolID: "3"},
		{Role: "tool", Content: "plan data", ToolID: "4"},
	}

	compressed := compressToolResults(messages, 2, 200)
	if compressed != 2 {
		t.Errorf("compressToolResults() compressed %d messages, want 2", compressed)
	}
	// First tool result should be truncated
	if len(messages[1].Content) > 300 {
		t.Errorf("first tool result not compressed: %d chars", len(messages[1].Content))
	}
	// Recent results should be unchanged
	if messages[3].Content != "short result" {
		t.Errorf("recent tool result was modified: %s", messages[3].Content)
	}
}