package v2

import (
	"context"
	"strings"
	"testing"
	"time"
)

func runValidationConfig() *WorkflowConfig {
	return &WorkflowConfig{
		Name:    "rv-loop",
		Version: 1,
		Global:  GlobalConfig{MaxTotalIterations: 10, MaxRepeatedToolCalls: 6},
		Phases: []PhaseConfig{
			{
				Name: "EXECUTION", Type: "execution", MaxIterations: 6,
				AllowedTools: []string{"write_file", "run_validation"},
				Gate:         GateConfig{Type: "task_completion", Mode: "partial_allowed"},
				Verification: &VerificationConfig{Profile: "go_test_all", Blocking: true},
			},
		},
	}
}

// The model can call run_validation mid-phase; the result is recorded and fed back.
func TestDriveModelCanSelfValidate(t *testing.T) {
	engine := NewEngine(runValidationConfig(), nil, nil)
	engine.GetState().Plan = &PlanGraph{}

	var calls, lastExit int
	engine.SetVerificationRunner(func(_ context.Context, profile string) VerificationOutcome {
		calls++
		return VerificationOutcome{Ran: true, Passed: true, ExitCode: lastExit, Summary: "ok " + profile}
	})

	step := 0
	llm := func(_ context.Context, _ []Message) (string, int, int, error) {
		step++
		if step == 1 {
			return `{"type":"tool_request","tool":"run_validation","input":{}}`, 1, 1, nil
		}
		return `{"type":"final","content":"done"}`, 1, 1, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state, err := engine.Drive(ctx, llm, noopTool, DriveOptions{SystemPrompt: "s", UserPrompt: "u", SkipPushRequirement: true})
	if err != nil {
		t.Fatalf("Drive error: %v", err)
	}
	// Called at least twice: once for the model's run_validation, once for the gate.
	if calls < 2 {
		t.Fatalf("expected verification runner to be called for tool + gate, got %d", calls)
	}
	if state.Verification == nil || !state.Verification.Passed {
		t.Fatalf("expected recorded passing verification, got %+v", state.Verification)
	}
	if ps := state.PhaseStates["EXECUTION"]; ps == nil || ps.Status != PhaseStatusCompleted {
		t.Fatalf("EXECUTION should complete: %+v", ps)
	}
}

func TestHandleRunValidationProfileResolution(t *testing.T) {
	engine := NewEngine(runValidationConfig(), nil, nil)

	// No runner wired.
	if msg := engine.handleRunValidation(context.Background(), "EXECUTION", &ToolRequest{Tool: "run_validation"}); !strings.Contains(msg, "not available") {
		t.Fatalf("expected unavailable message, got %q", msg)
	}

	var gotProfile string
	engine.SetVerificationRunner(func(_ context.Context, profile string) VerificationOutcome {
		gotProfile = profile
		return VerificationOutcome{Ran: true, Passed: false, ExitCode: 3, Summary: "boom"}
	})

	// Explicit profile in the call overrides the phase default.
	msg := engine.handleRunValidation(context.Background(), "EXECUTION", &ToolRequest{Tool: "run_validation", Input: map[string]any{"profile": "echo_test"}})
	if gotProfile != "echo_test" {
		t.Fatalf("expected explicit profile to be used, got %q", gotProfile)
	}
	if !strings.Contains(msg, "FAILED") || !strings.Contains(msg, "boom") {
		t.Fatalf("expected failure summary in message, got %q", msg)
	}

	// Falls back to the phase's configured profile when none is supplied.
	engine.handleRunValidation(context.Background(), "EXECUTION", &ToolRequest{Tool: "run_validation"})
	if gotProfile != "go_test_all" {
		t.Fatalf("expected fallback to phase profile go_test_all, got %q", gotProfile)
	}
}
