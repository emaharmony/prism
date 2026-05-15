package gate

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds registered gate implementations.
type Registry struct {
	mu    sync.RWMutex
	gates map[string]Gate
}

// NewRegistry creates an empty gate registry.
func NewRegistry() *Registry {
	return &Registry{
		gates: make(map[string]Gate),
	}
}

// Register adds a gate implementation to the registry.
// Returns an error if a gate with the same name already exists.
func (r *Registry) Register(g Gate) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := g.Name()
	if _, ok := r.gates[name]; ok {
		return fmt.Errorf("gate %q already registered", name)
	}
	r.gates[name] = g
	return nil
}

// Resolve looks up a gate by name.
// Returns an error if the gate is not found.
func (r *Registry) Resolve(name string) (Gate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	g, ok := r.gates[name]
	if !ok {
		return nil, fmt.Errorf("unknown gate %q", name)
	}
	return g, nil
}

// List returns all registered gate names, sorted alphabetically.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.gates))
	for name := range r.gates {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
