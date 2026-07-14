// Package provider defines the provider registry — a mapping from model IDs
// to their Provider instances and metadata. This enables dynamic provider
// selection from OpenClaw config or other sources.
package provider

import (
	"context"
	"fmt"
	"sync"
)

// ModelCost holds per-token pricing for a model.
type ModelCost struct {
	InputPer1K  float64 `json:"input_per_1k"`
	OutputPer1K float64 `json:"output_per_1k"`
	CacheRead   float64 `json:"cache_read_per_1k"`
	CacheWrite  float64 `json:"cache_write_per_1k"`
}

// ModelInfo holds metadata about a specific model.
type ModelInfo struct {
	ID            string    `json:"id"`
	Name          string    `json:"name,omitempty"`
	ContextWindow int       `json:"context_window"`
	InputTypes    []string  `json:"input_types"`
	Reasoning     bool      `json:"reasoning"`
	Cost          ModelCost `json:"cost"`
	ProviderName  string    `json:"provider_name"` // e.g., "ollama", "openai"
}

// UsageRecord is one per-call token-usage sample handed to a UsageRecorder.
// Provider and Model reflect the backend that actually served the response
// (after failover), not the configured primary.
type UsageRecord struct {
	Agent            string
	Provider         string
	Model            string
	Source           string // optional; inferred from RunID when empty
	RunID            string
	ConversationID   string
	PromptTokens     int
	CompletionTokens int
}

// UsageRecorder receives token usage for each successful chat generation routed
// through a per-agent provider. Implemented by internal/usage; kept as an
// interface here so the provider package stays free of a storage dependency.
type UsageRecorder interface {
	RecordUsage(u UsageRecord)
}

// ProviderRegistry maps model IDs to their Provider instances and metadata.
// Thread-safe. Use Register() to add providers, Get() to look them up.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]Provider       // model ID → provider
	models    map[string]ModelInfo      // model ID → metadata
	chains    map[string]*ChainProvider // chain name → chain
	agents    map[string]Provider       // agent ID → per-agent failover provider
	recorder  UsageRecorder             // optional token-usage sink (nil = off)
}

// NewProviderRegistry creates an empty registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
		models:    make(map[string]ModelInfo),
		chains:    make(map[string]*ChainProvider),
		agents:    make(map[string]Provider),
	}
}

// Register adds a provider and model metadata to the registry.
func (r *ProviderRegistry) Register(modelID string, p Provider, info ModelInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()

	info.ID = modelID
	r.providers[modelID] = p
	r.models[modelID] = info
}

// RegisterAgent associates an agent with its own resolved provider. This lets
// agents sharing the same primary model use different fallback chains.
func (r *ProviderRegistry) RegisterAgent(agentID string, p Provider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.agents[agentID] = p
}

// SetUsageRecorder installs a token-usage sink. Chat providers resolved via
// GetChatProviderForAgent are then wrapped so every successful ChatGenerate is
// recorded. Passing nil disables recording.
func (r *ProviderRegistry) SetUsageRecorder(rec UsageRecorder) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recorder = rec
}

// GetForAgent returns the per-agent provider when one is registered, falling
// back to the model provider for callers that do not use agent failover.
func (r *ProviderRegistry) GetForAgent(agentID, modelID string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if p, ok := r.agents[agentID]; ok {
		return p, nil
	}
	p, ok := r.providers[modelID]
	if !ok {
		return nil, fmt.Errorf("model %q not found in registry; available: %v", modelID, r.listModelsLocked())
	}
	return p, nil
}

// GetChatProviderForAgent resolves a native-chat capable per-agent provider.
// When a usage recorder is installed, the returned provider is wrapped so each
// successful ChatGenerate is recorded (attributed to the actual serving
// provider/model after failover).
func (r *ProviderRegistry) GetChatProviderForAgent(agentID, modelID string) (ChatProvider, error) {
	p, err := r.GetForAgent(agentID, modelID)
	if err != nil {
		return nil, err
	}
	chatProv, ok := p.(ChatProvider)
	if !ok {
		return nil, fmt.Errorf("provider for %s does not support chat generation with tool calling", modelID)
	}
	r.mu.RLock()
	rec := r.recorder
	r.mu.RUnlock()
	if rec != nil {
		return &recordingChatProvider{inner: chatProv, agentID: agentID, modelID: modelID, rec: rec}, nil
	}
	return chatProv, nil
}

// recordingChatProvider decorates a ChatProvider to report token usage after
// each successful call. It attributes usage to resp.Provider/resp.Model when the
// backend set them (failover annotates the serving target), falling back to the
// configured model ID otherwise.
type recordingChatProvider struct {
	inner   ChatProvider
	agentID string
	modelID string
	rec     UsageRecorder
}

func (p *recordingChatProvider) ChatGenerate(ctx context.Context, req ChatGenerateRequest) (ChatGenerateResponse, error) {
	resp, err := p.inner.ChatGenerate(ctx, req)
	if err != nil {
		return resp, err
	}
	model := resp.Model
	if model == "" {
		model = p.modelID
	}
	agent := req.Agent
	if agent == "" {
		agent = p.agentID
	}
	p.rec.RecordUsage(UsageRecord{
		Agent:            agent,
		Provider:         resp.Provider,
		Model:            model,
		RunID:            req.RunID,
		PromptTokens:     resp.PromptTokens,
		CompletionTokens: resp.OutputTokens,
	})
	return resp, nil
}

// Get returns the provider for a model ID.
func (r *ProviderRegistry) Get(modelID string) (Provider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[modelID]
	if !ok {
		return nil, fmt.Errorf("model %q not found in registry; available: %v", modelID, r.listModelsLocked())
	}
	return p, nil
}

// GetChatProvider returns a ChatProvider if the model's provider supports native tool calling.
// Returns an error if the model is not found or the provider doesn't implement ChatProvider.
// Use this when you want to use /api/chat with structured messages and tool calling.
func (r *ProviderRegistry) GetChatProvider(modelID string) (ChatProvider, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.providers[modelID]
	if !ok {
		return nil, fmt.Errorf("model %q not found in registry; available: %v", modelID, r.listModelsLocked())
	}
	chatProv, ok := p.(ChatProvider)
	if !ok {
		return nil, fmt.Errorf("provider for %s (%s) does not support chat generation with tool calling",
			modelID, r.models[modelID].ProviderName)
	}
	return chatProv, nil
}

// ModelInfo returns metadata for a model ID.
func (r *ProviderRegistry) ModelInfo(modelID string) (ModelInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.models[modelID]
	if !ok {
		return ModelInfo{}, fmt.Errorf("model %q not found in registry", modelID)
	}
	return info, nil
}

// ListModels returns all registered model IDs.
func (r *ProviderRegistry) ListModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.listModelsLocked()
}

// ListModelsByProvider returns model IDs for a specific provider.
func (r *ProviderRegistry) ListModelsByProvider(providerName string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var models []string
	for id, info := range r.models {
		if info.ProviderName == providerName {
			models = append(models, id)
		}
	}
	return models
}

// AllModelInfo returns all model metadata.
func (r *ProviderRegistry) AllModelInfo() map[string]ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]ModelInfo, len(r.models))
	for k, v := range r.models {
		result[k] = v
	}
	return result
}

// HasModel returns true if a model ID is registered.
func (r *ProviderRegistry) HasModel(modelID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, ok := r.providers[modelID]
	return ok
}

// listModelsLocked returns model IDs (caller must hold r.mu).
func (r *ProviderRegistry) listModelsLocked() []string {
	models := make([]string, 0, len(r.providers))
	for id := range r.providers {
		models = append(models, id)
	}
	return models
}
