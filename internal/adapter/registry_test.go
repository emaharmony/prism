package adapter

import (
	"fmt"
	"testing"
)

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	a := &testAdapter{name: "echo", version: "1.0"}
	if err := r.Register(a); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Duplicate registration should fail
	if err := r.Register(a); err == nil {
		t.Error("Register() duplicate should return error")
	}
}

func TestRegistry_Register_InvalidName(t *testing.T) {
	r := NewRegistry()

	tests := []struct {
		name string
	}{
		{"my.adapter"},
		{"my_adapter"},
		{"MyAdapter"},
		{""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &testAdapter{name: tt.name, version: "1.0"}
			if err := r.Register(a); err == nil {
				t.Errorf("Register(%q) should return error for invalid name", tt.name)
			}
		})
	}
}

func TestRegistry_Resolve(t *testing.T) {
	r := NewRegistry()
	a := &testAdapter{name: "trading", version: "1.0"}
	r.Register(a)

	resolved, err := r.Resolve("trading")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if resolved.Name() != "trading" {
		t.Errorf("Resolve() Name = %q, want %q", resolved.Name(), "trading")
	}

	// Unknown adapter should fail
	_, err = r.Resolve("unknown")
	if err == nil {
		t.Error("Resolve() unknown should return error")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&testAdapter{name: "echo", version: "1.0"})
	r.Register(&testAdapter{name: "trading", version: "1.0"})

	names := r.List()
	if len(names) != 2 {
		t.Errorf("List() length = %d, want 2", len(names))
	}

	// Verify both names exist (order not guaranteed)
	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["echo"] || !found["trading"] {
		t.Errorf("List() names = %v, want echo and trading", names)
	}
}

func TestRegistry_Capabilities(t *testing.T) {
	r := NewRegistry()
	caps := []Capability{
		{Action: "evaluate", Description: "Evaluate decisions"},
		{Action: "execute", Description: "Execute actions"},
	}
	r.Register(&testAdapter{name: "trading", version: "1.0", caps: caps})

	result, err := r.Capabilities("trading")
	if err != nil {
		t.Fatalf("Capabilities() error = %v", err)
	}
	if len(result) != 2 {
		t.Errorf("Capabilities() length = %d, want 2", len(result))
	}

	// Unknown adapter
	_, err = r.Capabilities("unknown")
	if err == nil {
		t.Error("Capabilities() unknown should return error")
	}
}

func TestRegistry_ValidateInput(t *testing.T) {
	r := NewRegistry()
	r.Register(&testValidatingAdapter{
		testAdapter: testAdapter{name: "validating", version: "1.0"},
	})

	// Valid action should pass
	err := r.ValidateInput("validating", "ok", map[string]any{})
	if err != nil {
		t.Errorf("ValidateInput() valid action error = %v", err)
	}

	// Invalid action should fail (InputValidator returns error)
	err = r.ValidateInput("validating", "fail", map[string]any{})
	if err == nil {
		t.Error("ValidateInput() invalid action should return error")
	}
}

func TestRegistry_ValidateInput_NoValidator(t *testing.T) {
	r := NewRegistry()
	r.Register(&testAdapter{name: "basic", version: "1.0"})

	// Adapter without InputValidator should always pass
	err := r.ValidateInput("basic", "anything", map[string]any{})
	if err != nil {
		t.Errorf("ValidateInput() no validator error = %v", err)
	}
}

func TestRegistry_ValidateInput_UnknownAdapter(t *testing.T) {
	r := NewRegistry()
	err := r.ValidateInput("unknown", "action", map[string]any{})
	if err == nil {
		t.Error("ValidateInput() unknown adapter should return error")
	}
}

func TestRegistry_Concurrent(t *testing.T) {
	r := NewRegistry()

	// Register multiple adapters concurrently
	done := make(chan error, 10)
	for i := 0; i < 5; i++ {
		go func(n int) {
			done <- r.Register(&testAdapter{name: fmt.Sprintf("adapter-%d", n), version: "1.0"})
		}(i)
	}
	for i := 0; i < 5; i++ {
		go func(n int) {
			_, err := r.Resolve(fmt.Sprintf("adapter-%d", n))
			done <- err
		}(i)
	}

	for i := 0; i < 10; i++ {
		if err := <-done; err != nil {
			// Resolve errors for not-yet-registered adapters are expected
			t.Logf("concurrent op %d: %v", i, err)
		}
	}

	names := r.List()
	if len(names) != 5 {
		t.Errorf("List() after concurrent = %d, want 5", len(names))
	}
}