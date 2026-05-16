package workflow

import (
	"fmt"
	"os"
	"path/filepath"
)

// Registry stores and resolves workflow definitions by name.
type Registry struct {
	workflows map[string]*Workflow
}

// NewRegistry creates an empty workflow registry.
func NewRegistry() *Registry {
	return &Registry{
		workflows: make(map[string]*Workflow),
	}
}

// Register adds a workflow to the registry.
// Returns an error if a workflow with the same name is already registered.
func (r *Registry) Register(w *Workflow) error {
	if w == nil {
		return fmt.Errorf("workflow: cannot register nil workflow")
	}
	if w.Name == "" {
		return fmt.Errorf("workflow: cannot register workflow with empty name")
	}
	if _, exists := r.workflows[w.Name]; exists {
		return fmt.Errorf("workflow: %q already registered", w.Name)
	}
	r.workflows[w.Name] = w
	return nil
}

// List returns all registered workflow names in sorted order.
func (r *Registry) List() []string {
	names := make([]string, 0, len(r.workflows))
	for name := range r.workflows {
		names = append(names, name)
	}
	return names
}

// Resolve returns a workflow by name.
// Returns an error if the workflow is not found.
func (r *Registry) Resolve(name string) (*Workflow, error) {
	w, ok := r.workflows[name]
	if !ok {
		return nil, fmt.Errorf("workflow: %q not found", name)
	}
	return w, nil
}

// LoadFromDir loads all workflow YAML files from a directory into the registry.
// Returns the number of workflows loaded and any errors encountered.
func (r *Registry) LoadFromDir(dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("workflow: failed to read directory %q: %w", dir, err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := filepath.Ext(entry.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return loaded, fmt.Errorf("workflow: failed to read %q: %w", path, err)
		}

		w, err := LoadFromYAML(data)
		if err != nil {
			return loaded, fmt.Errorf("workflow: failed to parse %q: %w", path, err)
		}

		if err := r.Register(w); err != nil {
			return loaded, fmt.Errorf("workflow: failed to register %q from %q: %w", w.Name, path, err)
		}
		loaded++
	}

	return loaded, nil
}