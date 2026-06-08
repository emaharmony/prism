package state

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestManagerActiveTask(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// No active task initially
	task, err := m.LoadActiveTask()
	if err != nil {
		t.Fatalf("LoadActiveTask failed: %v", err)
	}
	if task != nil {
		t.Fatalf("expected nil task, got %+v", task)
	}

	// Save an active task
	now := time.Now()
	expected := &ActiveTask{
		Task:      "V32 Phase 1: Adaptive Context",
		Plan:      "Create state package, workspace/state/ directory, context injection",
		Scope:     "State files only — no guard rail yet",
		Branch:    "feat/v32-adaptive-context",
		Status:    "executing",
		StartedAt: now,
	}
	if err := m.SaveActiveTask(expected); err != nil {
		t.Fatalf("SaveActiveTask failed: %v", err)
	}

	// Load it back
	task, err = m.LoadActiveTask()
	if err != nil {
		t.Fatalf("LoadActiveTask failed: %v", err)
	}
	if task.Task != expected.Task {
		t.Errorf("task.Task = %q, want %q", task.Task, expected.Task)
	}
	if task.Branch != expected.Branch {
		t.Errorf("task.Branch = %q, want %q", task.Branch, expected.Branch)
	}
	if task.Status != expected.Status {
		t.Errorf("task.Status = %q, want %q", task.Status, expected.Status)
	}

	// Clear it
	if err := m.ClearActiveTask(); err != nil {
		t.Fatalf("ClearActiveTask failed: %v", err)
	}

	// Should be nil now
	task, err = m.LoadActiveTask()
	if err != nil {
		t.Fatalf("LoadActiveTask after clear failed: %v", err)
	}
	if task != nil {
		t.Fatalf("expected nil task after clear, got %+v", task)
	}

	// Clear again (should be no-op)
	if err := m.ClearActiveTask(); err != nil {
		t.Fatalf("ClearActiveTask (already clear) failed: %v", err)
	}
}

func TestManagerDecisions(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// No decisions initially
	decisions, err := m.LoadDecisions()
	if err != nil {
		t.Fatalf("LoadDecisions failed: %v", err)
	}
	if len(decisions) != 0 {
		t.Fatalf("expected 0 decisions, got %d", len(decisions))
	}

	// Record a decision
	d1 := Decision{
		Decision:     "Use local qwen3.5:9b for guard rail",
		Reasoning:    "Always faster than cloud, no latency",
		Alternatives: "Cloud model (higher capability but slower)",
		Author:       "ema",
	}
	if err := m.RecordDecision(d1); err != nil {
		t.Fatalf("RecordDecision failed: %v", err)
	}

	// Record another
	d2 := Decision{
		Decision:  "Auto-PR for bugs AND improvements",
		Reasoning: "System should auto-patch itself",
		Author:    "ema",
	}
	if err := m.RecordDecision(d2); err != nil {
		t.Fatalf("RecordDecision failed: %v", err)
	}

	// Load and verify
	decisions, err = m.LoadDecisions()
	if err != nil {
		t.Fatalf("LoadDecisions failed: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}
	if decisions[0].ID != "D-001" {
		t.Errorf("decisions[0].ID = %q, want D-001", decisions[0].ID)
	}
	if decisions[1].ID != "D-002" {
		t.Errorf("decisions[1].ID = %q, want D-002", decisions[1].ID)
	}
	if decisions[0].Author != "ema" {
		t.Errorf("decisions[0].Author = %q, want ema", decisions[0].Author)
	}
	if decisions[0].Timestamp.IsZero() {
		t.Error("decisions[0].Timestamp should not be zero")
	}
}

func TestManagerBlocked(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// No blocked items initially
	items, err := m.LoadBlocked()
	if err != nil {
		t.Fatalf("LoadBlocked failed: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected 0 blocked items, got %d", len(items))
	}

	// Add a blocked item
	b1 := BlockedItem{
		Item:      "P-005: Add openclaw-lumi to Astraea's listen_to_agents",
		WaitingOn: "Windows access / Codex",
		TaskRef:   "P-005",
	}
	if err := m.AddBlocked(b1); err != nil {
		t.Fatalf("AddBlocked failed: %v", err)
	}

	// Add another
	b2 := BlockedItem{
		Item:      "Mango review of P-008",
		WaitingOn: "Mango availability",
		TaskRef:   "P-008",
	}
	if err := m.AddBlocked(b2); err != nil {
		t.Fatalf("AddBlocked failed: %v", err)
	}

	// Load and verify
	items, err = m.LoadBlocked()
	if err != nil {
		t.Fatalf("LoadBlocked failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 blocked items, got %d", len(items))
	}
	if items[0].ID != "B-001" {
		t.Errorf("items[0].ID = %q, want B-001", items[0].ID)
	}
	if items[0].Since.IsZero() {
		t.Error("items[0].Since should not be zero")
	}

	// Unblock one
	if err := m.Unblock("B-001"); err != nil {
		t.Fatalf("Unblock failed: %v", err)
	}

	items, err = m.LoadBlocked()
	if err != nil {
		t.Fatalf("LoadBlocked after unblock failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1 blocked item after unblock, got %d", len(items))
	}
	if items[0].ID != "B-002" {
		t.Errorf("items[0].ID = %q, want B-002", items[0].ID)
	}
}

func TestManagerWorkingContext(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// No context initially
	ctx, err := m.LoadContext()
	if err != nil {
		t.Fatalf("LoadContext failed: %v", err)
	}
	if ctx != nil {
		t.Fatalf("expected nil context, got %+v", ctx)
	}

	// Save a working context
	expected := &WorkingContext{
		Branch:       "feat/v32-adaptive-context",
		LastAction:   "Created internal/state package",
		LastActionAt: time.Now(),
		OpenFiles:    []string{"internal/state/state.go", "internal/state/state_test.go"},
		PR:           "#43",
		Notes:        "Phase 1 of V32 — state persistence and context injection",
	}
	if err := m.SaveContext(expected); err != nil {
		t.Fatalf("SaveContext failed: %v", err)
	}

	// Load it back
	ctx, err = m.LoadContext()
	if err != nil {
		t.Fatalf("LoadContext failed: %v", err)
	}
	if ctx.Branch != expected.Branch {
		t.Errorf("ctx.Branch = %q, want %q", ctx.Branch, expected.Branch)
	}
	if ctx.PR != expected.PR {
		t.Errorf("ctx.PR = %q, want %q", ctx.PR, expected.PR)
	}
	if len(ctx.OpenFiles) != len(expected.OpenFiles) {
		t.Errorf("len(ctx.OpenFiles) = %d, want %d", len(ctx.OpenFiles), len(expected.OpenFiles))
	}
}

func TestFormatActiveTask(t *testing.T) {
	task := &ActiveTask{
		Task:      "V32 Phase 1",
		Plan:      "Create state package",
		Scope:     "State files only",
		Branch:    "feat/v32",
		Status:    "executing",
		StartedAt: time.Date(2026, 6, 7, 18, 0, 0, 0, time.UTC),
	}
	result := FormatActiveTask(task)
	if !strings.Contains(result, "V32 Phase 1") {
		t.Error("FormatActiveTask should contain task name")
	}
	if !strings.Contains(result, "executing") {
		t.Error("FormatActiveTask should contain status")
	}

	// Nil task
	result = FormatActiveTask(nil)
	if result != "" {
		t.Errorf("FormatActiveTask(nil) = %q, want empty string", result)
	}
}

func TestFormatDecisions(t *testing.T) {
	decisions := []Decision{
		{
			ID:        "D-001",
			Decision:  "Use local model",
			Reasoning: "Faster",
			Author:    "ema",
			Timestamp: time.Date(2026, 6, 7, 17, 56, 0, 0, time.UTC),
		},
	}
	result := FormatDecisions(decisions, 0)
	if !strings.Contains(result, "D-001") {
		t.Error("FormatDecisions should contain decision ID")
	}
	if !strings.Contains(result, "Use local model") {
		t.Error("FormatDecisions should contain decision text")
	}

	// Empty decisions
	result = FormatDecisions(nil, 0)
	if result != "" {
		t.Errorf("FormatDecisions(nil) = %q, want empty string", result)
	}
}

func TestFormatBlocked(t *testing.T) {
	items := []BlockedItem{
		{
			ID:        "B-001",
			Item:      "P-005",
			WaitingOn: "Windows access",
			Since:     time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
		},
	}
	result := FormatBlocked(items)
	if !strings.Contains(result, "B-001") {
		t.Error("FormatBlocked should contain item ID")
	}
	if !strings.Contains(result, "Windows access") {
		t.Error("FormatBlocked should contain waiting_on")
	}

	// Empty items
	result = FormatBlocked(nil)
	if result != "" {
		t.Errorf("FormatBlocked(nil) = %q, want empty string", result)
	}
}

func TestFormatContext(t *testing.T) {
	ctx := &WorkingContext{
		Branch:       "feat/v32",
		LastAction:   "Created state package",
		LastActionAt: time.Now(),
		PR:           "#43",
		OpenFiles:    []string{"state.go"},
	}
	result := FormatContext(ctx)
	if !strings.Contains(result, "feat/v32") {
		t.Error("FormatContext should contain branch")
	}
	if !strings.Contains(result, "#43") {
		t.Error("FormatContext should contain PR")
	}

	// Nil context
	result = FormatContext(nil)
	if result != "" {
		t.Errorf("FormatContext(nil) = %q, want empty string", result)
	}
}

func TestFormatStateForPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// No state — should return empty string
	result := m.FormatStateForPrompt()
	if result != "" {
		t.Errorf("FormatStateForPrompt with no state = %q, want empty string", result)
	}

	// Add some state
	m.SaveActiveTask(&ActiveTask{
		Task:   "V32 Phase 1",
		Plan:   "State persistence and context injection",
		Status: "executing",
	})
	m.RecordDecision(Decision{
		Decision:  "Local guard rail",
		Author:    "ema",
	})

	result = m.FormatStateForPrompt()
	if !strings.Contains(result, "V32 Phase 1") {
		t.Error("FormatStateForPrompt should contain active task")
	}
	if !strings.Contains(result, "Local guard rail") {
		t.Error("FormatStateForPrompt should contain decisions")
	}
}

func TestEnsureDir(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// State dir shouldn't exist yet
	if _, err := os.Stat(m.stateDir); !os.IsNotExist(err) {
		t.Fatal("state dir should not exist yet")
	}

	// EnsureDir should create it
	if err := m.EnsureDir(); err != nil {
		t.Fatalf("EnsureDir failed: %v", err)
	}

	// Now it should exist
	if _, err := os.Stat(m.stateDir); os.IsNotExist(err) {
		t.Fatal("state dir should exist after EnsureDir")
	}
}

func TestSaveActiveTaskAutoTimestamps(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	task := &ActiveTask{
		Task:   "Test task",
		Status: "planning",
	}
	if err := m.SaveActiveTask(task); err != nil {
		t.Fatalf("SaveActiveTask failed: %v", err)
	}

	// StartedAt should be auto-set
	if task.StartedAt.IsZero() {
		t.Error("StartedAt should be auto-set")
	}
	// UpdatedAt should be auto-set
	if task.UpdatedAt.IsZero() {
		t.Error("UpdatedAt should be auto-set")
	}

	// Save again — StartedAt should NOT change, UpdatedAt should
	originalStarted := task.StartedAt
	task.Status = "executing"
	time.Sleep(10 * time.Millisecond)
	if err := m.SaveActiveTask(task); err != nil {
		t.Fatalf("SaveActiveTask (update) failed: %v", err)
	}
	if !task.StartedAt.Equal(originalStarted) {
		t.Error("StartedAt should not change on update")
	}
}

func TestRecordDecisionAutoFields(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	d := Decision{
		Decision: "Auto-ID test",
		Author:   "lumi",
	}
	if err := m.RecordDecision(d); err != nil {
		t.Fatalf("RecordDecision failed: %v", err)
	}

	decisions, _ := m.LoadDecisions()
	if len(decisions) != 1 {
		t.Fatalf("expected 1 decision, got %d", len(decisions))
	}
	if decisions[0].ID != "D-001" {
		t.Errorf("auto ID = %q, want D-001", decisions[0].ID)
	}
	if decisions[0].Timestamp.IsZero() {
		t.Error("Timestamp should be auto-set")
	}
}

func TestCorruptedFileRecovery(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)
	m.EnsureDir()

	// Write corrupted JSON
	os.WriteFile(filepath.Join(m.stateDir, "active-task.json"), []byte("{invalid json"), 0644)

	task, err := m.LoadActiveTask()
	if err == nil {
		t.Error("expected error loading corrupted file")
	}
	if task != nil {
		t.Error("expected nil task for corrupted file")
	}
}

func TestConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)

	// Simulate concurrent writes
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(i int) {
			task := &ActiveTask{
				Task:   fmt.Sprintf("Concurrent task %d", i),
				Status: "executing",
			}
			m.SaveActiveTask(task)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}

	// Should be able to load the final result
	task, err := m.LoadActiveTask()
	if err != nil {
		t.Fatalf("LoadActiveTask after concurrent writes failed: %v", err)
	}
	if task == nil {
		t.Fatal("expected non-nil task after concurrent writes")
	}
}

func TestFormatStateForPromptAtomicConsistency(t *testing.T) {
	// FormatStateForPrompt should produce a consistent snapshot
	// even under concurrent writes. This tests that the atomic
	// RLock-based read sees a coherent view.
	tmpDir := t.TempDir()
	m := NewManager(tmpDir)
	m.EnsureDir()

	// Set up initial state
	m.SaveActiveTask(&ActiveTask{
		Task:   "Atomic test",
		Status: "executing",
	})
	m.RecordDecision(Decision{
		Decision: "Use atomic reads",
		Author:   "mango",
	})
	m.AddBlocked(BlockedItem{
		Item:      "Review pending",
		WaitingOn: "Ema approval",
	})
	m.SaveContext(&WorkingContext{
		Branch:     "feat/v32-fix",
		LastAction: "Added atomic read",
	})

	// Read state atomically
	result := m.FormatStateForPrompt()

	// All four state components should appear in the same read
	if !strings.Contains(result, "Atomic test") {
		t.Error("atomic read should contain active task")
	}
	if !strings.Contains(result, "Use atomic reads") {
		t.Error("atomic read should contain decision")
	}
	if !strings.Contains(result, "Review pending") {
		t.Error("atomic read should contain blocked item")
	}
	if !strings.Contains(result, "feat/v32-fix") {
		t.Error("atomic read should contain context branch")
	}

	// Concurrent writes should not cause FormatStateForPrompt to panic
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			m.SaveActiveTask(&ActiveTask{
				Task:   fmt.Sprintf("Concurrent %d", i),
				Status: "executing",
			})
		}(i)
	}
	// Read concurrently during writes
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.FormatStateForPrompt()
		}()
	}
	wg.Wait()
}