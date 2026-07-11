// Package provider defines the provider registry — a mapping from model IDs
// to their Provider instances and metadata. This enables dynamic provider
// selection from OpenClaw config or other sources.
package provider

import (
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

// ProviderRegistry maps model IDs to their Provider instances and metadata.
// Thread-safe. Use Register() to add providers, Get() to look them up.
type ProviderRegistry struct {
	mu        sync.RWMutex
	providers map[string]Provider       // model ID → provider
	models    map[string]ModelInfo      // model ID → metadata
	chains    map[string]*ChainProvider // chain name → chain
}

// NewProviderRegistry creates an empty registry.
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]Provider),
		models:    make(map[string]ModelInfo),
		chains:    make(map[string]*ChainProvider),
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
