package agent

import (
	"testing"
)

func TestAgentValidation_Valid(t *testing.T) {
	a := &Agent{
		Name:    "coder",
		Version: "1.0.0",
		Role:    "implementation",
		Capabilities: []AgentCapability{
			{Action: "write_code", Description: "Write code"},
		},
	}
	if err := a.Validate(); err != nil {
		t.Errorf("valid agent failed validation: %v", err)
	}
}

func TestAgentValidation_NoName(t *testing.T) {
	a := &Agent{
		Version: "1.0.0",
		Role:    "implementation",
		Capabilities: []AgentCapability{
			{Action: "write_code", Description: "Write code"},
		},
	}
	if err := a.Validate(); err == nil {
		t.Error("expected validation error for missing name")
	}
}

func TestAgentValidation_InvalidName(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"my.agent"},   // dot not allowed
		{"my agent"},   // space not allowed
		{"agent/1"},    // slash not allowed
		{""},           // empty not allowed
	}
	for _, tt := range tests {
		a := &Agent{
			Name:    tt.name,
			Version: "1.0.0",
			Role:    "test",
			Capabilities: []AgentCapability{
				{Action: "test", Description: "Test"},
			},
		}
		if err := a.Validate(); err == nil {
			t.Errorf("expected validation error for name %q", tt.name)
		}
	}
}

func TestAgentValidation_NoCapabilities(t *testing.T) {
	a := &Agent{
		Name:         "coder",
		Version:      "1.0.0",
		Role:         "implementation",
		Capabilities: []AgentCapability{},
	}
	if err := a.Validate(); err == nil {
		t.Error("expected validation error for no capabilities")
	}
}

func TestRegistryRegister(t *testing.T) {
	reg := NewRegistry()
	a := &Agent{
		Name:    "planner",
		Version: "1.0.0",
		Role:    "planning",
		Capabilities: []AgentCapability{
			{Action: "plan_task", Description: "Plan tasks"},
		},
	}
	if err := reg.Register(a); err != nil {
		t.Fatalf("register failed: %v", err)
	}

	resolved, err := reg.Resolve("planner")
	if err != nil {
		t.Fatalf("resolve failed: %v", err)
	}
	if resolved.Name != "planner" {
		t.Errorf("resolved name = %q, want planner", resolved.Name)
	}
}

func TestRegistryRegister_Duplicate(t *testing.T) {
	reg := NewRegistry()
	a := &Agent{
		Name:    "coder",
		Version: "1.0.0",
		Role:    "implementation",
		Capabilities: []AgentCapability{
			{Action: "write_code", Description: "Write code"},
		},
	}
	reg.Register(a) //nolint:errcheck // first registration
	if err := reg.Register(a); err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistryResolve_NotFound(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Resolve("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Agent{Name: "coder", Version: "1.0.0", Role: "impl", Capabilities: []AgentCapability{{Action: "write", Description: "Write"}}})   //nolint:errcheck
	reg.Register(&Agent{Name: "planner", Version: "1.0.0", Role: "plan", Capabilities: []AgentCapability{{Action: "plan", Description: "Plan"}}})   //nolint:errcheck
	reg.Register(&Agent{Name: "reviewer", Version: "1.0.0", Role: "review", Capabilities: []AgentCapability{{Action: "review", Description: "Review"}}}) //nolint:errcheck

	names := reg.List()
	if len(names) != 3 {
		t.Fatalf("list count = %d, want 3", len(names))
	}
	// List should be sorted
	if names[0] != "coder" || names[1] != "planner" || names[2] != "reviewer" {
		t.Errorf("list order = %v, want [coder planner reviewer]", names)
	}
}

func TestRegistryCapabilities(t *testing.T) {
	reg := NewRegistry()
	reg.Register(&Agent{
		Name:    "coder",
		Version: "1.0.0",
		Role:    "implementation",
		Capabilities: []AgentCapability{
			{Action: "write_code", Description: "Write code"},
			{Action: "fix_code", Description: "Fix bugs"},
		},
	}) //nolint:errcheck

	caps, err := reg.Capabilities("coder")
	if err != nil {
		t.Fatalf("capabilities failed: %v", err)
	}
	if len(caps) != 2 {
		t.Errorf("capabilities count = %d, want 2", len(caps))
	}
}

func TestIsValidAgentName(t *testing.T) {
	tests := []struct {
		name  string
		valid bool
	}{
		{"coder", true},
		{"code-reviewer", true},
		{"agent123", true},
		{"my.agent", false},
		{"my agent", false},
		{"", false},
		{"Agent", true}, // uppercase allowed
	}
	for _, tt := range tests {
		got := isValidAgentName(tt.name)
		if got != tt.valid {
			t.Errorf("isValidAgentName(%q) = %v, want %v", tt.name, got, tt.valid)
		}
	}
}