// Package provider defines the LLM provider interface for Prism V2.
// Providers implement Generate() — Prism owns the lifecycle, events,
// prompt assembly, and failure handling. The model only generates text.
package provider

import "context"

// Provider is the interface for LLM generation backends.
// Implementations include MockProvider (testing) and OllamaProvider (production).
type Provider interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
}

// GenerateRequest is the input to a provider's Generate call.
// All fields are set by Prism's lifecycle orchestrator.
type GenerateRequest struct {
	RunID         string `json:"run_id"`
	CorrelationID string `json:"correlation_id"`
	Agent         string `json:"agent"`
	Project       string `json:"project"`
	Task          string `json:"task"`
	Prompt        string `json:"prompt"`
	Model         string `json:"model"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int `json:"max_tokens"`
}

// GenerateResponse is the result from a provider's Generate call.
type GenerateResponse struct {
	Text         string         `json:"text"`
	Model        string         `json:"model"`
	Provider     string         `json:"provider"`
	LatencyMS    int64          `json:"latency_ms"`
	PromptTokens int            `json:"prompt_tokens"`
	OutputTokens int            `json:"output_tokens"`
	Raw          map[string]any `json:"raw,omitempty"`
}
