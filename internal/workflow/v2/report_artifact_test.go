package v2

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteReportArtifact(t *testing.T) {
	state := NewWorkflowState(DefaultConfig())
	state.RunID = "gl-123"
	state.Verification = &VerificationRecord{Profile: "go_test_all", Passed: true, ExitCode: 0, Attempts: 1}
	state.Plan = &PlanGraph{Tasks: []PlanTask{{ID: "T1", Description: "do it", Status: "completed"}}}

	base := t.TempDir()
	path, err := WriteReportArtifact(state, base)
	if err != nil {
		t.Fatalf("WriteReportArtifact: %v", err)
	}

	want := filepath.Join(base, "gl-123", "REPORT.md")
	if path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read artifact: %v", err)
	}
	content := string(data)
	for _, frag := range []string{"Natural Gates Workflow", "gl-123", "go_test_all", "T1"} {
		if !strings.Contains(content, frag) {
			t.Fatalf("artifact missing %q:\n%s", frag, content)
		}
	}
}

func TestWriteReportArtifactDefaultsRunID(t *testing.T) {
	state := NewWorkflowState(DefaultConfig()) // no RunID
	base := t.TempDir()
	path, err := WriteReportArtifact(state, base)
	if err != nil {
		t.Fatalf("WriteReportArtifact: %v", err)
	}
	if filepath.Base(filepath.Dir(path)) != "gated-loop" {
		t.Fatalf("expected gated-loop default dir, got %q", path)
	}
}

func TestWriteReportArtifactNilState(t *testing.T) {
	if _, err := WriteReportArtifact(nil, t.TempDir()); err == nil {
		t.Fatal("expected error for nil state")
	}
}
