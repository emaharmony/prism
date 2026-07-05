package v2

import (
	"context"
	"errors"
	"testing"
)

// A transient failure on a read-only tool is retried until it succeeds.
func TestExecuteToolRetriesIdempotentTransient(t *testing.T) {
	engine := NewEngine(execVerifyConfig(false), nil, nil)
	calls := 0
	tool := func(_ context.Context, _ string, _ *ToolRequest) (string, error) {
		calls++
		if calls == 1 {
			return "", errors.New("dial tcp: connection refused")
		}
		return "ok", nil
	}
	out, err := engine.executeTool(context.Background(), "EXECUTION", &ToolRequest{Tool: "read_file"}, tool)
	if err != nil {
		t.Fatalf("expected eventual success, got %v", err)
	}
	if out != "ok" || calls != 2 {
		t.Fatalf("expected one retry then success, got out=%q calls=%d", out, calls)
	}
}

// A deterministic (non-transient) failure on a read-only tool is NOT retried.
func TestExecuteToolDoesNotRetryDeterministic(t *testing.T) {
	engine := NewEngine(execVerifyConfig(false), nil, nil)
	calls := 0
	tool := func(_ context.Context, _ string, _ *ToolRequest) (string, error) {
		calls++
		return "", errors.New("no such file or directory")
	}
	_, err := engine.executeTool(context.Background(), "EXECUTION", &ToolRequest{Tool: "read_file"}, tool)
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("deterministic error should not retry, got %d calls", calls)
	}
}

// Mutations must NEVER be retried, even on a transient-looking error.
func TestExecuteToolNeverRetriesMutations(t *testing.T) {
	engine := NewEngine(execVerifyConfig(false), nil, nil)
	for _, mut := range []string{"git_commit", "git_push", "write_file", "create_directory", "git_add"} {
		calls := 0
		tool := func(_ context.Context, _ string, _ *ToolRequest) (string, error) {
			calls++
			return "", errors.New("timeout") // transient-looking, but must not retry a mutation
		}
		_, _ = engine.executeTool(context.Background(), "EXECUTION", &ToolRequest{Tool: mut}, tool)
		if calls != 1 {
			t.Fatalf("mutation %q must run exactly once, ran %d", mut, calls)
		}
	}
}
