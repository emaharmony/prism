package adapter

import (
	"context"
	"fmt"
	"testing"
)

// testAdapter is a minimal Adapter implementation for testing.
type testAdapter struct {
	name    string
	version string
	caps    []Capability
}

func (t *testAdapter) Name() string               { return t.name }
func (t *testAdapter) Version() string            { return t.version }
func (t *testAdapter) Capabilities() []Capability { return t.caps }
func (t *testAdapter) Execute(ctx context.Context, action string, input map[string]any) (*Result, error) {
	return &Result{Success: true, Output: input}, nil
}
func (t *testAdapter) Health(ctx context.Context) (*HealthResult, error) {
	return &HealthResult{Ready: true, Message: "ok"}, nil
}

// testValidatingAdapter implements InputValidator for testing.
type testValidatingAdapter struct {
	testAdapter
}

func (v *testValidatingAdapter) ValidateInput(action string, input map[string]any) error {
	if action == "fail" {
		return errInvalidAction
	}
	return nil
}

var errInvalidAction = fmt.Errorf("invalid action")

func TestValidateAdapterName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"simple", "trading", false},
		{"with-hyphen", "paper-trading", false},
		{"alphanumeric", "adapter123", false},
		{"single-char", "a", false},
		{"with-dot", "my.adapter", true},
		{"with-underscore", "my_adapter", true},
		{"with-space", "my adapter", true},
		{"uppercase", "MyAdapter", true},
		{"empty", "", true},
		{"starts-with-hyphen", "-bad", true},
		{"ends-with-hyphen", "bad-", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAdapterName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAdapterName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestAdapterInterface(t *testing.T) {
	a := &testAdapter{name: "test", version: "1.0"}
	if a.Name() != "test" {
		t.Errorf("Name() = %q, want %q", a.Name(), "test")
	}
	if a.Version() != "1.0" {
		t.Errorf("Version() = %q, want %q", a.Version(), "1.0")
	}
	ctx := context.Background()
	result, err := a.Execute(ctx, "echo", map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !result.Success {
		t.Error("Execute() Success = false, want true")
	}
	if result.Output["key"] != "value" {
		t.Errorf("Execute() Output[\"key\"] = %v, want %v", result.Output["key"], "value")
	}
	health, err := a.Health(ctx)
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if !health.Ready {
		t.Error("Health() Ready = false, want true")
	}
}

func TestResult(t *testing.T) {
	r := &Result{
		Success:  true,
		Output:   map[string]any{"key": "value"},
		Metadata: map[string]any{"duration_ms": 42},
	}
	if !r.Success {
		t.Error("Result.Success = false, want true")
	}
	if r.Output["key"] != "value" {
		t.Errorf("Result.Output[\"key\"] = %v, want %v", r.Output["key"], "value")
	}
}

func TestHealthResult(t *testing.T) {
	h := &HealthResult{
		Ready:   true,
		Message: "adapter is healthy",
		Details: map[string]any{"uptime_seconds": 3600},
	}
	if !h.Ready {
		t.Error("HealthResult.Ready = false, want true")
	}
	if h.Message != "adapter is healthy" {
		t.Errorf("HealthResult.Message = %q, want %q", h.Message, "adapter is healthy")
	}
}

func TestCapability(t *testing.T) {
	c := Capability{
		Action:           "evaluate",
		Description:      "Evaluate a trading decision",
		RequiresApproval: true,
	}
	if c.Action != "evaluate" {
		t.Errorf("Capability.Action = %q, want %q", c.Action, "evaluate")
	}
	if !c.RequiresApproval {
		t.Error("Capability.RequiresApproval = false, want true")
	}
}
