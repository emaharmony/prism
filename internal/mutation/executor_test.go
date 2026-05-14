package mutation

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/emaharmony/prism/internal/approval"
)

func TestExecutorApplyApprovedWrites(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "output.txt", "hello world", policy)
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	var events []string
	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	})

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", a.ApprovalID, "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}

	// Verify file was written
	absPath := filepath.Join(tmpDir, "output.txt")
	data, readErr := os.ReadFile(absPath)
	if readErr != nil {
		t.Fatalf("expected file to exist: %v", readErr)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}

	// Verify events emitted
	hasGranted := false
	hasValidated := false
	hasApplied := false
	for _, evt := range events {
		switch evt {
		case "prism.approval.granted":
			hasGranted = true
		case "prism.mutation.validated":
			hasValidated = true
		case "prism.mutation.applied":
			hasApplied = true
		}
	}
	if !hasGranted {
		t.Error("expected prism.approval.granted event")
	}
	if !hasValidated {
		t.Error("expected prism.mutation.validated event")
	}
	if !hasApplied {
		t.Error("expected prism.mutation.applied event")
	}
}

func TestExecutorDeniedApprovalDoesNotWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "output.txt", "hello world", policy)
	a.Deny("ema", "not needed")
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", a.ApprovalID, "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for denied approval")
	}

	// Verify file was NOT written
	absPath := filepath.Join(tmpDir, "output.txt")
	if _, readErr := os.Stat(absPath); !os.IsNotExist(readErr) {
		t.Error("expected file to NOT exist")
	}
}

func TestExecutorUnsafePathDoesNotWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}

	// Path traversal
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "../outside.txt", "malicious", policy)
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	var events []string
	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	})

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", a.ApprovalID, "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for path traversal")
	}

	hasFailed := false
	for _, evt := range events {
		if evt == "prism.mutation.failed" {
			hasFailed = true
		}
	}
	if !hasFailed {
		t.Error("expected prism.mutation.failed event")
	}
}

func TestExecutorAbsolutePathDoesNotWrite(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "/etc/passwd", "malicious", policy)
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", a.ApprovalID, "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for absolute path")
	}
}

func TestExecutorFailedMutationEmitsFailed(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}
	// Trying to write to a directory that doesn't exist, but path goes outside
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "", "", policy)
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	var events []string
	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	})

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", a.ApprovalID, "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for empty path")
	}

	hasFailed := false
	for _, evt := range events {
		if evt == "prism.mutation.failed" {
			hasFailed = true
		}
	}
	if !hasFailed {
		t.Error("expected prism.mutation.failed event for empty path")
	}
}

func TestExecutorDenyApproval(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "test.txt", "content", policy)
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	var events []string
	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	})

	err := executor.DenyApproval("run_01KM", a.ApprovalID, "ema", "not needed")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify approval was saved with denied status
	loaded, err := store.Load("run_01KM", a.ApprovalID)
	if err != nil {
		t.Fatalf("failed to reload: %v", err)
	}
	if loaded.Status != approval.StatusDenied {
		t.Errorf("expected status denied, got %q", loaded.Status)
	}

	// Verify event emitted
	hasDenied := false
	for _, evt := range events {
		if evt == "prism.approval.denied" {
			hasDenied = true
		}
	}
	if !hasDenied {
		t.Error("expected prism.approval.denied event")
	}
}

func TestExecutorContentSizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	// Create content larger than 1MB
	largeContent := make([]byte, MaxContentSize+1)
	for i := range largeContent {
		largeContent[i] = 'x'
	}

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "big.txt", string(largeContent), policy)
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", a.ApprovalID, "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for oversized content")
	}
}

func TestExecutorApprovalNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	executor := NewExecutor(tmpDir, store)

	var events []string
	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	})

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", "appr_nonexistent", "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for nonexistent approval")
	}

	hasFailed := false
	for _, evt := range events {
		if evt == "prism.mutation.failed" {
			hasFailed = true
		}
	}
	if !hasFailed {
		t.Error("expected prism.mutation.failed event")
	}
}

func TestExecutorWriteToSubdirectory(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "subdir/output.txt", "hello world", policy)
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", a.ApprovalID, "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Message)
	}

	absPath := filepath.Join(tmpDir, "subdir", "output.txt")
	data, err := os.ReadFile(absPath)
	if err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("expected 'hello world', got %q", string(data))
	}
}

func TestExecutorCannotOverwriteDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)

	// Create a directory
	dirPath := filepath.Join(tmpDir, "mydir")
	os.MkdirAll(dirPath, 0755)

	policy := approval.PolicyDecision{Decision: "requires_approval", Reason: "test"}
	a := approval.NewApproval("run_01KM", "corr_01KM", "test-cli", "prism", "write_file", "mydir", "content", policy)
	store.Save(a)

	executor := NewExecutor(tmpDir, store)

	result, err := executor.ApplyWithRun(context.Background(), "run_01KM", a.ApprovalID, "ema")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure when overwriting directory")
	}
}

func TestEmptyApply(t *testing.T) {
	tmpDir := t.TempDir()
	store := approval.NewStore(tmpDir)
	executor := NewExecutor(tmpDir, store)

	// Apply without runID should fail
	_, err := executor.Apply(context.Background(), "appr_test", "ema")
	if err == nil {
		t.Error("expected error from Apply without runID")
	}
}
