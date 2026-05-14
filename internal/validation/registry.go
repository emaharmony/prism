package validation

import (
	"fmt"
	"sort"
	"sync"
)

// Registry holds all registered validation profiles.
type Registry struct {
	mu       sync.RWMutex
	profiles map[string]Profile
}

// NewEmptyRegistry creates a new validation profile registry without built-in profiles.
// Useful for tests that need custom profile sets.
func NewEmptyRegistry() *Registry {
	return &Registry{
		profiles: make(map[string]Profile),
	}
}

// NewRegistry creates a new validation profile registry with V5 built-in profiles.
func NewRegistry() *Registry {
	r := &Registry{
		profiles: make(map[string]Profile),
	}
	RegisterBuiltins(r)
	return r
}

// Register adds a profile to the registry. Returns an error if a profile
// with the same name already exists or if the profile fails validation.
func (r *Registry) Register(p Profile) error {
	if err := p.Validate(); err != nil {
		return fmt.Errorf("cannot register profile: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.profiles[p.Name]; ok {
		return fmt.Errorf("profile %q already registered", p.Name)
	}
	r.profiles[p.Name] = p
	return nil
}

// Unregister removes a profile from the registry by name.
func (r *Registry) Unregister(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.profiles, name)
}

// Resolve returns the profile with the given name, or an error if not found.
func (r *Registry) Resolve(name string) (Profile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.profiles[name]
	if !ok {
		return Profile{}, fmt.Errorf("unknown validation profile %q", name)
	}
	return p, nil
}

// List returns all registered profile names, sorted alphabetically.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.profiles))
	for name := range r.profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// RegisterBuiltins registers all V5 built-in validation profiles.
func RegisterBuiltins(r *Registry) {
	// echo_test — quick test profile for integration tests (runs instantly)
	r.Register(Profile{
		Name:            "echo_test",
		Description:    "Quick echo test for integration testing",
		Command:        "echo",
		Args:           []string{"hello"},
		WorkingDir:     ".",
		TimeoutSeconds: 5,
		AllowedExitCodes: []int{0},
	})

	// go_test_all — runs all Go tests in the project
	r.Register(Profile{
		Name:            "go_test_all",
		Description:    "Run all Go tests in the project",
		Command:        "go",
		Args:           []string{"test", "./..."},
		WorkingDir:     ".",
		TimeoutSeconds: 120,
		AllowedExitCodes: []int{0},
	})
}
