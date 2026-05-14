package validation

import (
	"strings"
	"testing"
)

func TestProfileValidation(t *testing.T) {
	// Valid profile
	p := Profile{
		Name:           "test_profile",
		Command:        "go",
		Args:           []string{"version"},
		TimeoutSeconds: 30,
	}
	if err := p.Validate(); err != nil {
		t.Errorf("expected valid profile, got error: %v", err)
	}

	// Missing name
	p2 := Profile{
		Command:        "go",
		TimeoutSeconds: 30,
	}
	if err := p2.Validate(); err == nil {
		t.Error("expected error for missing name")
	}

	// Missing command
	p3 := Profile{
		Name:           "test",
		TimeoutSeconds: 30,
	}
	if err := p3.Validate(); err == nil {
		t.Error("expected error for missing command")
	}

	// Zero timeout
	p4 := Profile{
		Name:    "test",
		Command: "go",
	}
	if err := p4.Validate(); err == nil {
		t.Error("expected error for zero timeout")
	}
}

func TestRegistryList(t *testing.T) {
	r := NewRegistry()
	profiles := r.List()
	if len(profiles) == 0 {
		t.Fatal("expected at least one built-in profile")
	}

	found := false
	for _, name := range profiles {
		if name == "go_test_all" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected go_test_all profile in registry")
	}
}

func TestRegistryResolve(t *testing.T) {
	r := NewRegistry()

	p, err := r.Resolve("go_test_all")
	if err != nil {
		t.Fatalf("expected to resolve go_test_all, got: %v", err)
	}
	if p.Name != "go_test_all" {
		t.Errorf("expected name go_test_all, got %s", p.Name)
	}
	if p.Command != "go" {
		t.Errorf("expected command go, got %s", p.Command)
	}
	if p.TimeoutSeconds <= 0 {
		t.Errorf("expected positive timeout, got %d", p.TimeoutSeconds)
	}
}

func TestRegistryResolveUnknown(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("nonexistent_profile")
	if err == nil {
		t.Error("expected error for unknown profile")
	}
	if !strings.Contains(err.Error(), "unknown validation profile") {
		t.Errorf("expected 'unknown validation profile' in error, got: %v", err)
	}
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	r := NewRegistry()
	err := r.Register(Profile{
		Name:           "go_test_all",
		Command:        "echo",
		TimeoutSeconds: 10,
	})
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistryRegisterInvalid(t *testing.T) {
	r := NewRegistry()
	err := r.Register(Profile{
		Name: "bad_profile",
		// missing command and timeout
	})
	if err == nil {
		t.Error("expected error for invalid profile")
	}
}

func TestCommandSafety(t *testing.T) {
	tests := []struct {
		cmd  string
		safe bool
	}{
		{"go test ./...", true},
		{"echo hello", true},
		{"ls -la", true},
		{"cat file.txt", true},
		{"npm test", true},
		{"python script.py", true},

		// Dangerous
		{"cat file | grep foo", false},
		{"echo foo > bar", false},
		{"echo foo >> bar", false},
		{"cmd1 && cmd2", false},
		{"cmd1 || cmd2", false},
		{"cmd1; cmd2", false},
		{"cmd1 & cmd2", false},
		{"echo `whoami`", false},
		{"echo $(whoami)", false},
		{"echo ${PATH}", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := IsSafeCommandString(tt.cmd)
			if result != tt.safe {
				t.Errorf("IsSafeCommandString(%q) = %v, want %v", tt.cmd, result, tt.safe)
			}
		})
	}
}

func TestPathTraversalBlocked(t *testing.T) {
	// Create a temp dir to use as project root
	tmpDir := t.TempDir()

	// Test with a directory outside the project root
	valid, err := IsWithinProjectRoot(tmpDir, "../../etc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if valid {
		t.Error("expected path traversal to be blocked")
	}
}

func TestPathWithinRootAllowed(t *testing.T) {
	tmpDir := t.TempDir()

	valid, err := IsWithinProjectRoot(tmpDir, "subdir")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Error("expected subdir within project root to be allowed")
	}
}

func TestPathExactRootAllowed(t *testing.T) {
	tmpDir := t.TempDir()

	valid, err := IsWithinProjectRoot(tmpDir, ".")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !valid {
		t.Error("expected exact project root to be allowed")
	}
}
