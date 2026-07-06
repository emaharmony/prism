package v2

import (
	"os"
	"path/filepath"
	"testing"
)

// TestConfigYAMLRoundTrip verifies that MarshalConfigYAML produces snake_case
// YAML that LoadConfig reads back into an equivalent config — the dashboard
// editor's save → reload path.
func TestConfigYAMLRoundTrip(t *testing.T) {
	orig := DefaultConfig()

	yamlBytes, err := MarshalConfigYAML(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "gated-loop.yaml")
	if err := os.WriteFile(path, yamlBytes, 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	loaded, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.Name != orig.Name {
		t.Errorf("name: got %q want %q", loaded.Name, orig.Name)
	}
	if len(loaded.Phases) != len(orig.Phases) {
		t.Fatalf("phase count: got %d want %d", len(loaded.Phases), len(orig.Phases))
	}
	for i := range orig.Phases {
		op, lp := orig.Phases[i], loaded.Phases[i]
		if op.Name != lp.Name || op.Type != lp.Type || op.MaxIterations != lp.MaxIterations {
			t.Errorf("phase %d mismatch: %+v vs %+v", i, op, lp)
		}
		if op.Gate.Type != lp.Gate.Type {
			t.Errorf("phase %d gate type: got %q want %q", i, lp.Gate.Type, op.Gate.Type)
		}
	}
	// FEEDBACK_PRE must keep its approvers through the round-trip.
	fp := loaded.GetPhase("FEEDBACK_PRE")
	if fp == nil || len(fp.Gate.Approvers) == 0 {
		t.Fatalf("FEEDBACK_PRE approvers lost in round-trip: %+v", fp)
	}

	exec := loaded.GetPhase("EXECUTION")
	if exec == nil {
		t.Fatalf("EXECUTION phase missing")
	}
	hasCreateDirectory := false
	for _, tool := range exec.AllowedTools {
		if tool == "create_directory" {
			hasCreateDirectory = true
			break
		}
	}
	if !hasCreateDirectory {
		t.Fatalf("EXECUTION allowed tools should include create_directory: %v", exec.AllowedTools)
	}
}

func TestValidateConfigCatchesErrors(t *testing.T) {
	bad := &WorkflowConfig{
		Name: "",
		Phases: []PhaseConfig{
			{Name: "X", Type: "bogus", Gate: GateConfig{Type: "nope"}},
			{Name: "X", Type: "plan"}, // duplicate name
		},
	}
	errs := ValidateConfig(bad)
	if len(errs) < 3 {
		t.Fatalf("expected >=3 validation errors (name, type, gate, dup), got %d: %v", len(errs), errs)
	}

	if errs := ValidateConfig(DefaultConfig()); len(errs) != 0 {
		t.Fatalf("DefaultConfig should be valid, got: %v", errs)
	}
}

func TestDefaultConfigAllowsReferenceImageCollection(t *testing.T) {
	cfg := DefaultConfig()
	for _, phaseName := range []string{"PROBE", "RESEARCH"} {
		phase := cfg.GetPhase(phaseName)
		if phase == nil {
			t.Fatalf("phase %s missing", phaseName)
		}
		found := false
		for _, tool := range phase.AllowedTools {
			if tool == "collect_reference_images" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%s should allow collect_reference_images: %v", phaseName, phase.AllowedTools)
		}
	}
}
