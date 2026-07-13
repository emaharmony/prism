package tool

import (
	"context"
	"fmt"
	"sync"
)

// Registry stores and resolves tools by name. It is safe for concurrent use.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// NewRegistry creates an empty tool registry.
func NewRegistry() *Registry {
	return &Registry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry. Returns an error if a tool with the
// same name is already registered.
func (r *Registry) Register(t Tool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := t.Name()
	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool %q already registered", name)
	}
	r.tools[name] = t
	return nil
}

// List returns all registered tool names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// ListWithDescriptions returns tool names with their descriptions, useful for
// building LLM prompts that tell the model what each tool does.
func (r *Registry) ListWithDescriptions() []ToolInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	infos := make([]ToolInfo, 0, len(r.tools))
	for _, t := range r.tools {
		infos = append(infos, ToolInfo{
			Name:        t.Name(),
			Description: t.Description(),
			Schema:      t.Schema(),
		})
	}
	return infos
}

// ToolInfo holds tool metadata for prompts and documentation.
type ToolInfo struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Schema      ToolSchema `json:"schema"`
}

// Resolve returns a tool by name. Returns an error if the tool is not found.
func (r *Registry) Resolve(name string) (Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("tool %q not found", name)
	}
	return t, nil
}

// Validate checks whether a tool exists in the registry and whether its input
// conforms to the tool's schema (required fields present).
func (r *Registry) Validate(name string, input map[string]any) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.tools[name]
	if !ok {
		return fmt.Errorf("tool %q not found", name)
	}

	schema := t.Schema()
	for paramName, spec := range schema.Input {
		if spec.Required {
			val, exists := input[paramName]
			if !exists || val == nil {
				return fmt.Errorf("required parameter %q missing for tool %q", paramName, name)
			}
		}
	}
	return nil
}

// Execute resolves a tool, validates input, and runs it.
func (r *Registry) Execute(ctx context.Context, name string, input map[string]any) (ToolResult, error) {
	if err := r.Validate(name, input); err != nil {
		return ToolResult{}, err
	}

	t, err := r.Resolve(name)
	if err != nil {
		return ToolResult{}, err
	}

	return t.Execute(ctx, input)
}
