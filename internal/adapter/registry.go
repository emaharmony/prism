package adapter

import (
	"fmt"
	"sync"
)

// Registry manages registered adapters.
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]Adapter
}

// NewRegistry creates an empty adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		adapters: make(map[string]Adapter),
	}
}

// Register adds an adapter to the registry.
// Returns an error if an adapter with the same name is already registered
// or if the adapter name violates the naming rule.
func (r *Registry) Register(a Adapter) error {
	if err := ValidateAdapterName(a.Name()); err != nil {
		return fmt.Errorf("invalid adapter name: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.adapters[a.Name()]; exists {
		return fmt.Errorf("adapter %q already registered", a.Name())
	}

	r.adapters[a.Name()] = a
	return nil
}

// Resolve returns an adapter by name.
func (r *Registry) Resolve(name string) (Adapter, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("adapter %q not found", name)
	}
	return a, nil
}

// List returns all registered adapter names in registration order.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return in stable order
	names := make([]string, 0, len(r.adapters))
	for name := range r.adapters {
		names = append(names, name)
	}
	return names
}

// Capabilities returns all capabilities for a registered adapter.
func (r *Registry) Capabilities(name string) ([]Capability, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.adapters[name]
	if !ok {
		return nil, fmt.Errorf("adapter %q not found", name)
	}
	return a.Capabilities(), nil
}

// ValidateInput checks input validity if the adapter implements InputValidator.
// Returns nil if the adapter does not implement InputValidator.
func (r *Registry) ValidateInput(adapterName, action string, input map[string]any) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	a, ok := r.adapters[adapterName]
	if !ok {
		return fmt.Errorf("adapter %q not found", adapterName)
	}

	if v, ok := a.(InputValidator); ok {
		return v.ValidateInput(action, input)
	}
	return nil
}