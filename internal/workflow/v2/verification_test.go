package v2

import (
	"context"
	"strings"
	"testing"
	"time"
)

// execVerifyConfig is a single-EXECUTION-phase loop whose task_completion gate
// passes trivially (empty plan) so phase completion hinges purely on the
// verification step — letting these tests exercise verification in isolation.
func execVerifyConfig(blocking bool) *WorkflowConfig {
	return &WorkflowConfig{
		Name:    "verify-loop",
		Version: 1,
		Global:  GlobalConfig{MaxTotalIterations: 10},
		Phases: []PhaseConfig{
			{
				Name: "EXECUTION", Type: "execution", MaxIterations: 5,
				AllowedTools: []string{"write_file"},
				Gate:         GateConfig{Type: "task_completion", Mode: "partial_allowed"},
				Verification: &VerificationConfig{Profile: "go_test_all", Blocking: blocking},
			},
		},
	}
}

func finalLLM(_ context.Context, _ []Message) (string, int, int, error) {
	return `{"type":"final","content":"done"}`, 1, 1, nil
}

// driveExec runs the single-phase verify config with an empty plan seeded so the
// task_completion gate passes, isolating verification behaviour.
func driveExec(t *testing.T, engine *Engine) *WorkflowState {
	t.Helper()
	engine.GetState().Plan = &PlanGraph{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	state, err := engine.Drive(ctx, finalLLM, noopTool, DriveOptions{
		SystemPrompt: "s", UserPrompt: "u", SkipPushRequirement: true,
	})
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	return state
}

// A blocking verification failure must keep the phase iterating (feeding the
// failure back) until verification passes, then complete.
func TestDriveBlocksUntilVerificationPasses(t *testing.T) {
	engine := NewEngine(execVerifyConfig(true), nil, nil)

	calls := 0
	engine.SetVerificationRunner(func(_ context.Context, profile string) VerificationOutcome {
		calls++
		if profile != "go_test_all" {
			t.Errorf("unexpected profile %q", profile)
		}
		if calls == 1 {
			return VerificationOutcome{Ran: true, Passed: false, ExitCode: 1, Summary: "FAIL: build broken"}
		}
		return VerificationOutcome{Ran: true, Passed: true, ExitCode: 0, Summary: "ok"}
	})

	state := driveExec(t, engine)

	if calls < 2 {
		t.Fatalf("expected verification to run at least twice (fail then pass), ran %d", calls)
	}
	if state.Verification == nil || !state.Verification.Passed {
		t.Fatalf("expected verification recorded as passed, got %+v", state.Verification)
	}
	if state.Verification.Attempts < 2 {
		t.Fatalf("expected >=2 attempts recorded, got %d", state.Verification.Attempts)
	}
	if ps := state.PhaseStates["EXECUTION"]; ps == nil || ps.Status != PhaseStatusCompleted {
		t.Fatalf("EXECUTION should complete after verification passes: %+v", ps)
	}
}

// A non-blocking verification failure is recorded but does not stop the phase.
func TestDriveNonBlockingVerificationRecordsButProceeds(t *testing.T) {
	engine := NewEngine(execVerifyConfig(false), nil, nil)

	calls := 0
	engine.SetVerificationRunner(func(_ context.Context, _ string) VerificationOutcome {
		calls++
		return VerificationOutcome{Ran: true, Passed: false, ExitCode: 2, Summary: "tests failed"}
	})

	state := driveExec(t, engine)

	if calls != 1 {
		t.Fatalf("expected verification to run exactly once for non-blocking, ran %d", calls)
	}
	if state.Verification == nil || state.Verification.Passed {
		t.Fatalf("expected a recorded failing verification, got %+v", state.Verification)
	}
	if ps := state.PhaseStates["EXECUTION"]; ps == nil || ps.Status != PhaseStatusCompleted {
		t.Fatalf("EXECUTION should still complete on non-blocking failure: %+v", ps)
	}
}

// With no runner wired, verification is a no-op and the phase completes normally.
func TestDriveVerificationSkippedWhenNoRunner(t *testing.T) {
	engine := NewEngine(execVerifyConfig(true), nil, nil)
	state := driveExec(t, engine)
	if state.Verification != nil {
		t.Fatalf("expected no verification record without a runner, got %+v", state.Verification)
	}
	if ps := state.PhaseStates["EXECUTION"]; ps == nil || ps.Status != PhaseStatusCompleted {
		t.Fatalf("EXECUTION should complete when verification is skipped: %+v", ps)
	}
}

// A profile that could not run (Ran=false) must never block the loop.
func TestDriveVerificationNonRunnableDoesNotBlock(t *testing.T) {
	engine := NewEngine(execVerifyConfig(true), nil, nil)
	engine.SetVerificationRunner(func(_ context.Context, _ string) VerificationOutcome {
		return VerificationOutcome{Ran: false}
	})
	state := driveExec(t, engine)
	if state.Verification != nil {
		t.Fatalf("expected no record when profile could not run, got %+v", state.Verification)
	}
	if ps := state.PhaseStates["EXECUTION"]; ps == nil || ps.Status != PhaseStatusCompleted {
		t.Fatalf("EXECUTION should complete when verification cannot run: %+v", ps)
	}
}

func TestDefaultConfigHasExecutionVerification(t *testing.T) {
	cfg := DefaultConfig()
	exec := cfg.GetPhase("EXECUTION")
	if exec == nil || exec.Verification == nil {
		t.Fatalf("EXECUTION phase should declare verification")
	}
	if exec.Verification.Profile != "go_test_all" || !exec.Verification.Blocking {
		t.Fatalf("unexpected verification config: %+v", exec.Verification)
	}
}

func TestVerificationConfigRoundTrips(t *testing.T) {
	out, err := MarshalConfigYAML(DefaultConfig())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	cfg, err := parseConfigBytes(out, "x.yaml")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	exec := cfg.GetPhase("EXECUTION")
	if exec == nil || exec.Verification == nil || exec.Verification.Profile != "go_test_all" || !exec.Verification.Blocking {
		t.Fatalf("verification did not round-trip: %+v", exec)
	}
}

func TestValidateConfigRejectsEmptyVerificationProfile(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GetPhase("EXECUTION").Verification = &VerificationConfig{Profile: "  "}
	errs := ValidateConfig(cfg)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "verification.profile is required") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected validation error for empty verification profile, got %v", errs)
	}
}
