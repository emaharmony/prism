package task

import (
	"fmt"
	"testing"
	"time"
)

func TestStore_CreateAndGet(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	task := &Task{
		ID:           "task-001",
		Type:         "code_implementation",
		Status:       StatusCreated,
		DelegatedBy:  "lumi",
		DelegatedTo:  "mango",
		Description:  "Implement the X feature",
		Context:      map[string]any{"file": "internal/stage/llm.go"},
		Priority:     "high",
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err = store.Create(task)
	if err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	got, err := store.Get("task-001")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}

	if got.ID != "task-001" {
		t.Errorf("expected ID task-001, got %s", got.ID)
	}
	if got.Status != StatusCreated {
		t.Errorf("expected status created, got %s", got.Status)
	}
	if got.DelegatedBy != "lumi" {
		t.Errorf("expected delegated_by lumi, got %s", got.DelegatedBy)
	}
	if got.DelegatedTo != "mango" {
		t.Errorf("expected delegated_to mango, got %s", got.DelegatedTo)
	}
	if got.Type != "code_implementation" {
		t.Errorf("expected type code_implementation, got %s", got.Type)
	}
	if got.Description != "Implement the X feature" {
		t.Errorf("expected description, got %s", got.Description)
	}
	if got.Context["file"] != "internal/stage/llm.go" {
		t.Errorf("expected context file, got %v", got.Context)
	}
}

func TestStore_UpdateStatus(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	task := &Task{
		ID:          "task-002",
		Type:        "review",
		Status:      StatusCreated,
		DelegatedBy: "lumi",
		DelegatedTo: "mango",
		Description: "Review the PR",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := store.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	// Transition to in_progress
	if err := store.UpdateStatus("task-002", StatusInProgress, nil); err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	got, err := store.Get("task-002")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got.Status != StatusInProgress {
		t.Errorf("expected status in_progress, got %s", got.Status)
	}

	// Complete with result
	result := map[string]any{"lines_changed": 42, "tests_passed": true}
	if err := store.UpdateStatus("task-002", StatusCompleted, result); err != nil {
		t.Fatalf("failed to complete task: %v", err)
	}

	got, err = store.Get("task-002")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got.Status != StatusCompleted {
		t.Errorf("expected status completed, got %s", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected completed_at to be set")
	}
	if got.Result["lines_changed"] != float64(42) {
		t.Errorf("expected result lines_changed=42, got %v", got.Result["lines_changed"])
	}
}

func TestStore_ListByAgent(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	for i, taskType := range []string{"code", "review", "research"} {
		task := &Task{
			ID:           fmt.Sprintf("task-%d", i+1),
			Type:         taskType,
			Status:       StatusCreated,
			DelegatedBy:  "lumi",
			DelegatedTo:  "mango",
			Description:  "Task " + fmt.Sprintf("%d", i+1),
			CreatedAt:    now.Add(time.Duration(i) * time.Minute),
			UpdatedAt:    now.Add(time.Duration(i) * time.Minute),
		}
		if err := store.Create(task); err != nil {
			t.Fatalf("failed to create task %d: %v", i+1, err)
		}
	}

	tasks, err := store.ListByAgent("mango")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 3 {
		t.Errorf("expected 3 tasks for mango, got %d", len(tasks))
	}

	// Should not find tasks for unknown agent
	tasks, err = store.ListByAgent("unknown")
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected 0 tasks for unknown agent, got %d", len(tasks))
	}
}

func TestStore_ListByStatus(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	task1 := &Task{
		ID: "task-created", Type: "code", Status: StatusCreated,
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "Created task", CreatedAt: now, UpdatedAt: now,
	}
	task2 := &Task{
		ID: "task-completed", Type: "review", Status: StatusCompleted,
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "Completed task", CreatedAt: now, UpdatedAt: now,
	}

	if err := store.Create(task1); err != nil {
		t.Fatalf("failed to create task1: %v", err)
	}
	if err := store.Create(task2); err != nil {
		t.Fatalf("failed to create task2: %v", err)
	}

	created, err := store.ListByStatus(StatusCreated)
	if err != nil {
		t.Fatalf("failed to list created tasks: %v", err)
	}
	if len(created) != 1 {
		t.Errorf("expected 1 created task, got %d", len(created))
	}

	completed, err := store.ListByStatus(StatusCompleted)
	if err != nil {
		t.Fatalf("failed to list completed tasks: %v", err)
	}
	if len(completed) != 1 {
		t.Errorf("expected 1 completed task, got %d", len(completed))
	}
}

func TestStore_InvalidStatus(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	task := &Task{
		ID: "task-invalid", Type: "code", Status: "unknown_status",
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "Invalid task", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}

	err = store.Create(task)
	if err == nil {
		t.Error("expected error for invalid status")
	}
}

func TestStore_ParentTask(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	parent := &Task{
		ID: "parent-task", Type: "feature", Status: StatusInProgress,
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "Parent task", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Create(parent); err != nil {
		t.Fatalf("failed to create parent: %v", err)
	}

	child := &Task{
		ID: "child-task", ParentID: "parent-task", Type: "subtask", Status: StatusCreated,
		DelegatedBy: "mango", DelegatedTo: "junie",
		Description: "Child task", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Create(child); err != nil {
		t.Fatalf("failed to create child: %v", err)
	}

	got, err := store.Get("child-task")
	if err != nil {
		t.Fatalf("failed to get child: %v", err)
	}
	if got.ParentID != "parent-task" {
		t.Errorf("expected parent_id parent-task, got %s", got.ParentID)
	}
}

func TestStore_CancelTask(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	task := &Task{
		ID: "task-cancel", Type: "code", Status: StatusCreated,
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "Task to cancel", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	if err := store.UpdateStatus("task-cancel", StatusCancelled, nil); err != nil {
		t.Fatalf("failed to cancel task: %v", err)
	}

	got, err := store.Get("task-cancel")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	if got.Status != StatusCancelled {
		t.Errorf("expected status cancelled, got %s", got.Status)
	}
	if got.CompletedAt == nil {
		t.Error("expected completed_at to be set for cancelled task")
	}
}

func TestStore_NilContext(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	task := &Task{
		ID: "task-nil-ctx", Type: "code", Status: StatusCreated,
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "Task with nil context", CreatedAt: now, UpdatedAt: now,
		// Context and Result are nil
	}
	if err := store.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	got, err := store.Get("task-nil-ctx")
	if err != nil {
		t.Fatalf("failed to get task: %v", err)
	}
	// Nil context should deserialize as nil or empty map
	if got.Context != nil && len(got.Context) != 0 {
		t.Errorf("expected nil or empty context, got %v", got.Context)
	}
}

func TestStore_DuplicateID(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	now := time.Now()
	task := &Task{
		ID: "task-dup", Type: "code", Status: StatusCreated,
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "First", CreatedAt: now, UpdatedAt: now,
	}
	if err := store.Create(task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}

	task2 := &Task{
		ID: "task-dup", Type: "code", Status: StatusCreated,
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "Duplicate", CreatedAt: now, UpdatedAt: now,
	}
	err = store.Create(task2)
	if err == nil {
		t.Error("expected error for duplicate ID")
	}
}

func TestStore_GetNonexistent(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	_, err = store.Get("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent task")
	}
}

func TestStore_EmptyID(t *testing.T) {
	store, err := NewStore(t.TempDir() + "/tasks.db")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()

	task := &Task{
		ID: "", Type: "code", Status: StatusCreated,
		DelegatedBy: "lumi", DelegatedTo: "mango",
		Description: "No ID", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	err = store.Create(task)
	if err == nil {
		t.Error("expected error for empty ID")
	}
}