package stage

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestWALWriter_WriteEntry(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewWALWriter(tmpDir, "run_test123")
	if err != nil {
		t.Fatalf("NewWALWriter() error = %v", err)
	}
	defer wal.Close()

	err = wal.StageEntered("connection", 0)
	if err != nil {
		t.Fatalf("StageEntered() error = %v", err)
	}

	err = wal.StageCompleted("connection", 0, true)
	if err != nil {
		t.Fatalf("StageCompleted() error = %v", err)
	}
}

func TestWALWriter_MutationApplied(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewWALWriter(tmpDir, "run_test456")
	if err != nil {
		t.Fatalf("NewWALWriter() error = %v", err)
	}
	defer wal.Close()

	err = wal.MutationApplied("sha256:abc123", "/path/to/file.go")
	if err != nil {
		t.Fatalf("MutationApplied() error = %v", err)
	}
}

func TestWALReader_ReadEntries(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewWALWriter(tmpDir, "run_test789")
	if err != nil {
		t.Fatalf("NewWALWriter() error = %v", err)
	}

	wal.StageEntered("connection", 0)
	wal.StageCompleted("connection", 0, true)
	wal.StageEntered("llm", 1)
	wal.StageCompleted("llm", 1, true)
	wal.Close()

	reader := NewWALReader(tmpDir)
	entries, err := reader.ReadEntries()
	if err != nil {
		t.Fatalf("ReadEntries() error = %v", err)
	}
	if len(entries) != 4 {
		t.Errorf("expected 4 entries, got %d", len(entries))
	}

	// Check entry types
	if entries[0].Type != "wal.stage.entered" {
		t.Errorf("entry[0].Type = %q, want wal.stage.entered", entries[0].Type)
	}
	if entries[1].Type != "wal.stage.completed" {
		t.Errorf("entry[1].Type = %q, want wal.stage.completed", entries[1].Type)
	}
}

func TestWALReader_LastCompletedStage(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewWALWriter(tmpDir, "run_test999")
	if err != nil {
		t.Fatalf("NewWALWriter() error = %v", err)
	}

	wal.StageEntered("connection", 0)
	wal.StageCompleted("connection", 0, true)
	wal.StageEntered("llm", 1)
	// LLM stage didn't complete (crash!)
	wal.Close()

	reader := NewWALReader(tmpDir)
	stage, index, err := reader.LastCompletedStage()
	if err != nil {
		t.Fatalf("LastCompletedStage() error = %v", err)
	}
	if stage != "connection" {
		t.Errorf("stage = %q, want connection", stage)
	}
	if index != 0 {
		t.Errorf("index = %d, want 0", index)
	}
}

func TestWALReader_MutationKeys(t *testing.T) {
	tmpDir := t.TempDir()
	wal, err := NewWALWriter(tmpDir, "run_test_keys")
	if err != nil {
		t.Fatalf("NewWALWriter() error = %v", err)
	}

	wal.MutationApplied("sha256:abc123", "/path/to/file1.go")
	wal.MutationApplied("sha256:def456", "/path/to/file2.go")
	wal.Close()

	reader := NewWALReader(tmpDir)
	keys, err := reader.MutationKeys()
	if err != nil {
		t.Fatalf("MutationKeys() error = %v", err)
	}
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}
	if !keys["sha256:abc123"] {
		t.Error("key sha256:abc123 not found")
	}
	if !keys["sha256:def456"] {
		t.Error("key sha256:def456 not found")
	}
}

func TestWALReader_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	reader := NewWALReader(tmpDir) // No WAL file

	entries, err := reader.ReadEntries()
	if err != nil {
		t.Fatalf("ReadEntries() error = %v", err)
	}
	if entries != nil {
		t.Errorf("expected nil entries for missing file, got %d", len(entries))
	}
}

func TestComputeMutationKey(t *testing.T) {
	key1 := ComputeMutationKey("run_1", "approval", "/path/to/file.go", []byte("content"))
	key2 := ComputeMutationKey("run_1", "approval", "/path/to/file.go", []byte("content"))
	key3 := ComputeMutationKey("run_1", "approval", "/path/to/file.go", []byte("different content"))

	// Same inputs = same key
	if key1 != key2 {
		t.Error("same inputs should produce same key")
	}

	// Different content = different key
	if key1 == key3 {
		t.Error("different content should produce different key")
	}

	// Key should start with sha256:
	if len(key1) < 8 {
		t.Errorf("key too short: %q", key1)
	}
}

func TestComputeMutationKeyWithTimestamp(t *testing.T) {
	ts1 := time.Now()
	ts2 := time.Now().Add(1 * time.Second)

	key1 := ComputeMutationKeyWithTimestamp("run_1", "approval", "/path/file.go", []byte("content"), ts1)
	key2 := ComputeMutationKeyWithTimestamp("run_1", "approval", "/path/file.go", []byte("content"), ts2)

	// Same content, different timestamp = different key
	if key1 == key2 {
		t.Error("different timestamps should produce different keys")
	}
}

func TestIsMutationApplied(t *testing.T) {
	entries := []WALEntry{
		{Type: "wal.mutation.applied", Payload: map[string]any{"mutation_key": "sha256:abc123"}},
		{Type: "wal.stage.completed", Payload: map[string]any{"stage": "llm"}},
		{Type: "wal.mutation.applied", Payload: map[string]any{"mutation_key": "sha256:def456"}},
	}

	if !IsMutationApplied(entries, "sha256:abc123") {
		t.Error("sha256:abc123 should be applied")
	}
	if !IsMutationApplied(entries, "sha256:def456") {
		t.Error("sha256:def456 should be applied")
	}
	if IsMutationApplied(entries, "sha256:nonexistent") {
		t.Error("nonexistent key should not be applied")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		retryable bool
	}{
		{"nil", nil, false},
		{"explicit retryable", NewRetryableError(fmt.Errorf("503")), true},
		{"explicit non-retryable", NewNonRetryableError(fmt.Errorf("forbidden")), false},
		{"503", fmt.Errorf("server returned 503"), true},
		{"429", fmt.Errorf("rate limit 429"), true},
		{"timeout", fmt.Errorf("connection timeout"), true},
		{"context deadline", context.DeadlineExceeded, true},
		{"context canceled", context.Canceled, true},
		{"generic error", fmt.Errorf("something went wrong"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.retryable {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

func TestRetryWithBackoff_Success(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond, // Fast for tests
		MaxDelay:   10 * time.Millisecond,
		Jitter:     0,
	}

	result, err := RetryWithBackoff(context.Background(), config, func(a int) (string, error) {
		return "success", nil
	})
	if err != nil {
		t.Fatalf("RetryWithBackoff() error = %v", err)
	}
	if result != "success" {
		t.Errorf("result = %q, want success", result)
	}
}

func TestRetryWithBackoff_RetrySuccess(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
		Jitter:     0,
	}

	result, err := RetryWithBackoff(context.Background(), config, func(attempt int) (string, error) {
		if attempt < 2 {
			return "", NewRetryableError(fmt.Errorf("transient error"))
		}
		return fmt.Sprintf("success on attempt %d", attempt), nil
	})
	if err != nil {
		t.Fatalf("RetryWithBackoff() error = %v", err)
	}
	_ = result // verify success
}

func TestRetryWithBackoff_NonRetryableError(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   10 * time.Millisecond,
		Jitter:     0,
	}

	attempt := 0
	_, err := RetryWithBackoff(context.Background(), config, func(a int) (string, error) {
		attempt++
		return "", NewNonRetryableError(fmt.Errorf("mutation failed"))
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if attempt != 1 {
		t.Errorf("non-retryable error should only attempt once, got %d attempts", attempt)
	}
}

func TestRetryWithBackoff_ExhaustedRetries(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 2,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   5 * time.Millisecond,
		Jitter:     0,
	}

	_, err := RetryWithBackoff(context.Background(), config, func(a int) (string, error) {
		return "", NewRetryableError(fmt.Errorf("always fails"))
	})
	if err == nil {
		t.Fatal("expected error after retries exhausted")
	}
}

func TestRetryWithBackoff_ContextCancellation(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 10,
		BaseDelay:  1 * time.Millisecond,
		MaxDelay:   5 * time.Millisecond,
		Jitter:     0,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := RetryWithBackoff(ctx, config, func(a int) (string, error) {
		return "", NewRetryableError(fmt.Errorf("slow fail"))
	})
	if err != context.DeadlineExceeded {
		t.Errorf("expected context error, got %v", err)
	}
}

func TestCalculateDelay(t *testing.T) {
	config := RetryConfig{
		MaxRetries: 5,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Jitter:     0,
	}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 1 * time.Second},
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 30 * time.Second}, // capped at MaxDelay
	}

	for _, tt := range tests {
		got := calculateDelay(tt.attempt, config)
		if got != tt.want {
			t.Errorf("calculateDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}