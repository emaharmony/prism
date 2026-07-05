package v2

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// rollbackConfig is the single-EXECUTION verify loop with auto-rollback on.
func rollbackConfig(maxAttempts int) *WorkflowConfig {
	cfg := execVerifyConfig(true)
	cfg.Global.AutoRollback = true
	cfg.Global.MaxVerificationAttempts = maxAttempts
	return cfg
}

// driveExecErr is driveExec without failing the test on a Drive error (blocked
// runs return one).
func driveExecErr(t *testing.T, engine *Engine) (*WorkflowState, error) {
	t.Helper()
	engine.GetState().Plan = &PlanGraph{}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return engine.Drive(ctx, finalLLM, noopTool, DriveOptions{
		SystemPrompt: "s", UserPrompt: "u", SkipPushRequirement: true,
	})
}

// Blocking verification failing MaxVerificationAttempts times must fire the
// rollback runner, emit workflow.rollback, and finalize as rolled_back.
func TestDriveRollsBackAfterVerificationAttemptsExhausted(t *testing.T) {
	emitter := &captureEmitter{}
	engine := NewEngine(rollbackConfig(2), emitter, nil)

	verifyCalls := 0
	engine.SetVerificationRunner(func(_ context.Context, _ string) VerificationOutcome {
		verifyCalls++
		return VerificationOutcome{Ran: true, Passed: false, ExitCode: 1, Summary: "FAIL"}
	})
	rollbackCalls := 0
	rollbackReason := ""
	engine.SetRollbackRunner(func(_ context.Context, reason string) error {
		rollbackCalls++
		rollbackReason = reason
		return nil
	})

	state, err := driveExecErr(t, engine)
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if verifyCalls != 2 {
		t.Fatalf("expected exactly 2 verification attempts before rollback, got %d", verifyCalls)
	}
	if rollbackCalls != 1 {
		t.Fatalf("expected exactly one rollback, got %d", rollbackCalls)
	}
	if !strings.Contains(rollbackReason, "failed 2 times") {
		t.Fatalf("unexpected rollback reason: %q", rollbackReason)
	}
	if state.Status != StatusRolledBack {
		t.Fatalf("expected status rolled_back, got %s", state.Status)
	}
	if state.Rollback == nil || state.Rollback.Error != "" {
		t.Fatalf("expected clean rollback record, got %+v", state.Rollback)
	}
	if !emitter.has("workflow.rollback") {
		t.Fatalf("expected workflow.rollback event, got %v", emitter.events)
	}
	if ps := state.PhaseStates["EXECUTION"]; ps == nil || ps.Status != PhaseStatusFallback {
		t.Fatalf("EXECUTION should be fallback after rollback: %+v", ps)
	}
}

// Without AutoRollback the same failing verification must NOT roll back —
// preserving the pre-V57 fix-forward behavior.
func TestDriveNoRollbackWhenDisabled(t *testing.T) {
	cfg := rollbackConfig(2)
	cfg.Global.AutoRollback = false
	engine := NewEngine(cfg, nil, nil)
	engine.SetVerificationRunner(func(_ context.Context, _ string) VerificationOutcome {
		return VerificationOutcome{Ran: true, Passed: false, ExitCode: 1, Summary: "FAIL"}
	})
	rollbackCalls := 0
	engine.SetRollbackRunner(func(_ context.Context, _ string) error {
		rollbackCalls++
		return nil
	})

	state, err := driveExecErr(t, engine)
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if rollbackCalls != 0 {
		t.Fatalf("rollback must not fire when auto_rollback is off, fired %d times", rollbackCalls)
	}
	if state.Status == StatusRolledBack {
		t.Fatal("run must not finalize rolled_back when auto_rollback is off")
	}
}

// With AutoRollback on but no runner wired, the run must not deadlock or panic
// — rollback is skipped and the run finalizes normally.
func TestDriveRollbackSkippedWithoutRunner(t *testing.T) {
	engine := NewEngine(rollbackConfig(1), nil, nil)
	engine.SetVerificationRunner(func(_ context.Context, _ string) VerificationOutcome {
		return VerificationOutcome{Ran: true, Passed: false, ExitCode: 1, Summary: "FAIL"}
	})
	state, err := driveExecErr(t, engine)
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if state.Rollback != nil {
		t.Fatalf("no rollback record expected without a runner, got %+v", state.Rollback)
	}
	if state.Status == StatusRolledBack {
		t.Fatal("run must not be rolled_back without a runner")
	}
}

// A run that ends with red verification (attempt cap not reached, iterations
// exhausted instead) still rolls back at finalization.
func TestDriveRollsBackAtEndWithFailingVerification(t *testing.T) {
	engine := NewEngine(rollbackConfig(99), nil, nil) // cap never reached mid-run
	engine.SetVerificationRunner(func(_ context.Context, _ string) VerificationOutcome {
		return VerificationOutcome{Ran: true, Passed: false, ExitCode: 1, Summary: "FAIL"}
	})
	rollbackCalls := 0
	engine.SetRollbackRunner(func(_ context.Context, reason string) error {
		rollbackCalls++
		if !strings.Contains(reason, "failing verification") {
			t.Errorf("unexpected reason: %q", reason)
		}
		return nil
	})
	state, err := driveExecErr(t, engine)
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if rollbackCalls != 1 {
		t.Fatalf("expected end-of-run rollback, got %d calls", rollbackCalls)
	}
	if state.Status != StatusRolledBack {
		t.Fatalf("expected rolled_back, got %s", state.Status)
	}
}

// A failed rollback records the error and must NOT mark the run rolled_back.
func TestDriveRollbackFailureRecorded(t *testing.T) {
	engine := NewEngine(rollbackConfig(1), nil, nil)
	engine.SetVerificationRunner(func(_ context.Context, _ string) VerificationOutcome {
		return VerificationOutcome{Ran: true, Passed: false, ExitCode: 1, Summary: "FAIL"}
	})
	engine.SetRollbackRunner(func(_ context.Context, _ string) error {
		return errors.New("reset refused")
	})
	state, err := driveExecErr(t, engine)
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if state.Rollback == nil || state.Rollback.Error == "" {
		t.Fatalf("expected rollback record with error, got %+v", state.Rollback)
	}
	if state.Status == StatusRolledBack {
		t.Fatal("failed rollback must not report rolled_back status")
	}
}

// A passing run with AutoRollback on must never roll back.
func TestDriveNoRollbackOnPassingRun(t *testing.T) {
	engine := NewEngine(rollbackConfig(2), nil, nil)
	engine.SetVerificationRunner(func(_ context.Context, _ string) VerificationOutcome {
		return VerificationOutcome{Ran: true, Passed: true, ExitCode: 0, Summary: "ok"}
	})
	rollbackCalls := 0
	engine.SetRollbackRunner(func(_ context.Context, _ string) error {
		rollbackCalls++
		return nil
	})
	state, err := driveExecErr(t, engine)
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if rollbackCalls != 0 {
		t.Fatalf("rollback fired on a passing run (%d times)", rollbackCalls)
	}
	if state.Status != StatusCompleted {
		t.Fatalf("expected completed, got %s", state.Status)
	}
}

func TestValidateConfigAutoRollbackNeedsBlockingVerification(t *testing.T) {
	cfg := execVerifyConfig(false) // verification present but NOT blocking
	cfg.Global.AutoRollback = true
	errs := ValidateConfig(cfg)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "auto_rollback requires") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected auto_rollback validation error, got %v", errs)
	}

	// With blocking verification it validates clean.
	cfg2 := rollbackConfig(3)
	for _, e := range ValidateConfig(cfg2) {
		if strings.Contains(e, "auto_rollback") {
			t.Fatalf("unexpected auto_rollback error on valid config: %v", e)
		}
	}
}
