package policy

import (
	"fmt"
	"sync"
)

// Registry stores and resolves policy rules.
type Registry struct {
	mu    sync.RWMutex
	rules []PolicyRule
}

// NewRegistry creates an empty policy registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register adds a policy rule to the registry.
// Returns an error if a rule with the same ID already exists.
func (r *Registry) Register(rule PolicyRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.rules {
		if existing.ID == rule.ID {
			return fmt.Errorf("policy rule %q already registered", rule.ID)
		}
	}

	r.rules = append(r.rules, rule)
	return nil
}

// Rules returns a copy of all registered rules in registration order.
func (r *Registry) Rules() []PolicyRule {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]PolicyRule, len(r.rules))
	copy(result, r.rules)
	return result
}

// FindByID returns a rule by its ID, or an error if not found.
func (r *Registry) FindByID(id string) (PolicyRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, rule := range r.rules {
		if rule.ID == id {
			return rule, nil
		}
	}
	return PolicyRule{}, fmt.Errorf("policy rule %q not found", id)
}