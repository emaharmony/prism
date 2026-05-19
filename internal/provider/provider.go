// Package provider defines the LLM provider interface for Prism.
//
// Provider implementations live in sub-packages:
//   - provider/mock     — deterministic mock for testing
//   - provider/ollama    — local Ollama instances (/api/generate)
//   - provider/openai    — OpenAI and OpenAI-compatible APIs
//   - provider/chain     — provider chaining with tier-based fallback
//
// Import the sub-package directly to use a provider:
//
//	import "github.com/emaharmony/prism/internal/provider/mock"
//	p := mock.New()
package provider

import "context"

// Provider is the interface for LLM generation backends.
type Provider interface {
	Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error)
}

// GenerateRequest is the input to a provider's Generate call.
type GenerateRequest struct {
	RunID         string  `json:"run_id"`
	CorrelationID string  `json:"correlation_id"`
	Agent         string  `json:"agent"`
	Project       string  `json:"project"`
	Task          string  `json:"task"`
	Prompt        string  `json:"prompt"`
	Model         string  `json:"model"`
	Temperature   float64 `json:"temperature"`
	MaxTokens     int     `json:"max_tokens"`
}

// GenerateResponse is the result from a provider's Generate call.
type GenerateResponse struct {
	Text          string         `json:"text"`
	Model         string         `json:"model"`
	Provider      string         `json:"provider"`
	LatencyMS     int64          `json:"latency_ms"`
	PromptTokens  int            `json:"prompt_tokens"`
	OutputTokens   int            `json:"output_tokens"`
	Raw           map[string]any `json:"raw,omitempty"`
}

// ProviderTier classifies providers by cost.
type ProviderTier string

const (
	TierFree ProviderTier = "free"
	TierPaid ProviderTier = "paid"
)

// TokenChunk represents a batch of tokens from a streaming LLM response.
type TokenChunk struct {
	Tokens   []string
	Index    int
	Finished bool
	Error    error
}

// StreamingProvider extends Provider with streaming generation capability.
type StreamingProvider interface {
	Provider
	GenerateStream(ctx context.Context, req GenerateRequest) (<-chan TokenChunk, error)
}

// NamedProvider is an optional interface for providers that have a name.
type NamedProvider interface {
	Provider
	Name() string
}

// TieredProvider is an optional interface for cost classification.
type TieredProvider interface {
	Provider
	Tier() ProviderTier
}