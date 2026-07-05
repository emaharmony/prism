package v2

import (
	"context"
	"strings"
	"testing"
	"time"
)

type captureEmitter struct{ events []string }

func (c *captureEmitter) Emit(t string, _ map[string]any) { c.events = append(c.events, t) }
func (c *captureEmitter) has(t string) bool {
	for _, e := range c.events {
		if e == t {
			return true
		}
	}
	return false
}

// A phase whose gate can never pass (no assumptions declared) would iterate to
// its budget; with a token ceiling set, the run must stop early and finalize
// gracefully rather than burn the full iteration budget.
func TestDriveStopsOnTokenBudget(t *testing.T) {
	cfg := &WorkflowConfig{
		Name: "budget", Version: 1,
		Global: GlobalConfig{MaxTotalIterations: 50, MaxTotalTokens: 5},
		Phases: []PhaseConfig{{
			Name: "PROBE", Type: "probe", MaxIterations: 20,
			Gate: GateConfig{Type: "assumption_threshold", Threshold: 2.0},
		}},
	}
	em := &captureEmitter{}
	engine := NewEngine(cfg, em, nil)
	llm := func(_ context.Context, _ []Message) (string, int, int, error) {
		return `{"type":"final","content":"x"}`, 3, 3, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state, err := engine.Drive(ctx, llm, noopTool, DriveOptions{SystemPrompt: "s", UserPrompt: "u"})
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if !em.has("workflow.budget_exhausted") {
		t.Fatalf("expected workflow.budget_exhausted event, got %v", em.events)
	}
	if used := state.GetTotalPromptTokens() + state.GetTotalCompletionTokens(); used < 5 {
		t.Fatalf("expected at least the budget in tokens, got %d", used)
	}
	if state.Status != StatusCompleted {
		t.Fatalf("budget-exhausted run should finalize as completed, got %s", state.Status)
	}
}

// A phase with its own token cap must stop iterating (phase.budget_exhausted)
// while the RUN continues to later phases — per-phase caps are softer than the
// run-wide ceiling.
func TestDrivePhaseTokenBudgetStopsPhaseNotRun(t *testing.T) {
	cfg := &WorkflowConfig{
		Name: "phase-budget", Version: 1,
		Global: GlobalConfig{MaxTotalIterations: 50},
		Phases: []PhaseConfig{
			{
				Name: "PROBE", Type: "probe", MaxIterations: 20, MaxTokens: 5,
				Gate: GateConfig{Type: "assumption_threshold", Threshold: 2.0},
			},
			{
				// Second phase with a trivially-passing gate proves the run survives.
				Name: "EXECUTION", Type: "execution", MaxIterations: 5,
				Gate: GateConfig{Type: "task_completion", Mode: "partial_allowed"},
			},
		},
	}
	em := &captureEmitter{}
	engine := NewEngine(cfg, em, nil)
	engine.GetState().Plan = &PlanGraph{}
	llm := func(_ context.Context, _ []Message) (string, int, int, error) {
		return `{"type":"final","content":"x"}`, 3, 3, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	state, err := engine.Drive(ctx, llm, noopTool, DriveOptions{SystemPrompt: "s", UserPrompt: "u", SkipPushRequirement: true})
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if !em.has("phase.budget_exhausted") {
		t.Fatalf("expected phase.budget_exhausted event, got %v", em.events)
	}
	if em.has("workflow.budget_exhausted") {
		t.Fatal("per-phase cap must not trip the run-wide budget event")
	}
	if got := state.GetPhaseTokens("PROBE"); got < 5 {
		t.Fatalf("PROBE should have accumulated at least its cap, got %d", got)
	}
	// The run continued past the capped phase.
	if ps := state.PhaseStates["EXECUTION"]; ps == nil || ps.Iterations == 0 {
		t.Fatalf("EXECUTION should have run after PROBE hit its cap: %+v", ps)
	}
	if state.Status != StatusCompleted {
		t.Fatalf("run should finalize as completed, got %s", state.Status)
	}
}

func TestValidateConfigRejectsNegativePhaseMaxTokens(t *testing.T) {
	cfg := execVerifyConfig(false)
	cfg.Phases[0].MaxTokens = -1
	errs := ValidateConfig(cfg)
	found := false
	for _, e := range errs {
		if strings.Contains(e, "max_tokens must be >= 0") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected max_tokens validation error, got %v", errs)
	}
}

func TestBudgetExceededTimeAndZero(t *testing.T) {
	e := NewEngine(execVerifyConfig(false), nil, nil)
	e.config.Global.MaxTotalTime = "1h"
	if r := e.budgetExceeded(time.Now().Add(-time.Second)); r == "" {
		t.Fatal("expected a past deadline to trip the time budget")
	}
	if r := e.budgetExceeded(time.Time{}); r != "" {
		t.Fatalf("a zero deadline should never trip: %q", r)
	}
}

func TestBudgetExceededTokensOnly(t *testing.T) {
	e := NewEngine(execVerifyConfig(false), nil, nil)
	e.config.Global.MaxTotalTokens = 10
	e.state.AddTokens(6, 6) // 12 >= 10
	if r := e.budgetExceeded(time.Time{}); r == "" {
		t.Fatal("expected token ceiling to trip")
	}
	e2 := NewEngine(execVerifyConfig(false), nil, nil)
	e2.config.Global.MaxTotalTokens = 0 // disabled
	e2.state.AddTokens(1000, 1000)
	if r := e2.budgetExceeded(time.Time{}); r != "" {
		t.Fatalf("zero token ceiling should be unlimited: %q", r)
	}
}
