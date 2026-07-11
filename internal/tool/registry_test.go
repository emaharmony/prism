package tool

import (
	"context"
	"testing"
)

// mockTool is a simple tool implementation for testing the registry.
type mockTool struct {
	name        string
	description string
}

func (m *mockTool) Name() string        { return m.name }
func (m *mockTool) Description() string { return m.description }
func (m *mockTool) Schema() ToolSchema  { return ToolSchema{} }
func (m *mockTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	return ToolResult{Success: true, Output: map[string]any{"echo": m.name}}, nil
}

func TestRegistryRegisterAndList(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register(&mockTool{name: "test_tool", description: "A test tool"})
	if err != nil {
		t.Fatalf("failed to register tool: %v", err)
	}

	err = reg.Register(&mockTool{name: "another_tool", description: "Another test tool"})
	if err != nil {
		t.Fatalf("failed to register second tool: %v", err)
	}

	names := reg.List()
	if len(names) != 2 {
		t.Errorf("expected 2 tools, got %d", len(names))
	}
}

func TestRegistryDuplicateRegistration(t *testing.T) {
	reg := NewRegistry()

	err := reg.Register(&mockTool{name: "test_tool", description: "First"})
	if err != nil {
		t.Fatalf("first registration should succeed: %v", err)
	}

	err = reg.Register(&mockTool{name: "test_tool", description: "Second"})
	if err == nil {
		t.Error("expected error on duplicate registration, got nil")
	}
}

func TestRegistryResolve(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&mockTool{name: "echo", description: "Echo tool"})

	tool, err := reg.Resolve("echo")
	if err != nil {
		t.Fatalf("failed to resolve echo: %v", err)
	}
	if tool.Name() != "echo" {
		t.Errorf("expected tool name 'echo', got %s", tool.Name())
	}
}

func TestRegistryResolveUnknown(t *testing.T) {
	reg := NewRegistry()

	_, err := reg.Resolve("nonexistent")
	if err == nil {
		t.Error("expected error resolving unknown tool, got nil")
	}
}

func TestRegistryValidate(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg, ".", 1024*1024)

	// echo requires "text" parameter
	err := reg.Validate("echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Errorf("valid echo input should pass: %v", err)
	}

	err = reg.Validate("echo", map[string]any{})
	if err == nil {
		t.Error("echo without required 'text' parameter should fail validation")
	}
}

func TestRegistryExecute(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg, ".", 1024*1024)

	result, err := reg.Execute(context.Background(), "echo", map[string]any{"text": "hello"})
	if err != nil {
		t.Fatalf("execute echo should succeed: %v", err)
	}
	if !result.Success {
		t.Error("echo result should be successful")
	}
	if result.Output["text"] != "hello" {
		t.Errorf("expected echo output 'hello', got %v", result.Output["text"])
	}
}
