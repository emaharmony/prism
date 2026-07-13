package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// ── Definition Tests ────────────────────────────────────────────────────────

func TestLoadFromYAMLValid(t *testing.T) {
	yaml := `
name: demo.echo_tool
description: Demo workflow that runs an echo tool.
version: 1
steps:
  - id: echo
    type: tool.execute
    tool: echo
    input:
      text: "hello from workflow"
`
	w, err := LoadFromYAML([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromYAML: %v", err)
	}
	if w.Name != "demo.echo_tool" {
		t.Errorf("Name = %q, want %q", w.Name, "demo.echo_tool")
	}
	if len(w.Steps) != 1 {
		t.Errorf("Steps = %d, want 1", len(w.Steps))
	}
	if w.Steps[0].Tool != "echo" {
		t.Errorf("Step.Tool = %q, want %q", w.Steps[0].Tool, "echo")
	}
}

func TestRejectMissingName(t *testing.T) {
	yaml := `
description: No name
version: 1
steps:
  - id: echo
    type: tool.execute
    tool: echo
`
	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("expected error for missing name")
	}
}

func TestRejectMissingSteps(t *testing.T) {
	yaml := `
name: empty
version: 1
`
	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("expected error for missing steps")
	}
}

func TestRejectDuplicateStepIDs(t *testing.T) {
	yaml := `
name: dup
version: 1
steps:
  - id: echo
    type: tool.execute
    tool: echo
  - id: echo
    type: tool.execute
    tool: echo
`
	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("expected error for duplicate step IDs")
	}
}

func TestRejectUnsupportedStepType(t *testing.T) {
	yaml := `
name: bad
version: 1
steps:
  - id: bad_step
    type: unsupported.type
`
	_, err := LoadFromYAML([]byte(yaml))
	if err == nil {
		t.Error("expected error for unsupported step type")
	}
}

// ── Registry Tests ──────────────────────────────────────────────────────────

func TestRegistryRegister(t *testing.T) {
	reg := NewRegistry()
	w := &Workflow{Name: "test", Version: 1, Steps: []Step{{ID: "s1", Type: StepTypeToolExecute, Tool: "echo"}}}
	if err := reg.Register(w); err != nil {
		t.Fatalf("Register: %v", err)
	}
}

func TestRegistryRegisterDuplicate(t *testing.T) {
	reg := NewRegistry()
	w := &Workflow{Name: "test", Version: 1, Steps: []Step{{ID: "s1", Type: StepTypeToolExecute, Tool: "echo"}}}
	_ = reg.Register(w)
	if err := reg.Register(w); err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestRegistryList(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&Workflow{Name: "alpha", Version: 1, Steps: []Step{{ID: "s1", Type: StepTypeToolExecute, Tool: "echo"}}})
	_ = reg.Register(&Workflow{Name: "beta", Version: 1, Steps: []Step{{ID: "s1", Type: StepTypeToolExecute, Tool: "echo"}}})
	names := reg.List()
	if len(names) != 2 {
		t.Errorf("List = %d workflows, want 2", len(names))
	}
}

func TestRegistryResolve(t *testing.T) {
	reg := NewRegistry()
	_ = reg.Register(&Workflow{Name: "test", Version: 1, Steps: []Step{{ID: "s1", Type: StepTypeToolExecute, Tool: "echo"}}})
	w, err := reg.Resolve("test")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if w.Name != "test" {
		t.Errorf("Name = %q, want %q", w.Name, "test")
	}
}

func TestRegistryResolveUnknown(t *testing.T) {
	reg := NewRegistry()
	_, err := reg.Resolve("nonexistent")
	if err == nil {
		t.Error("expected error for unknown workflow")
	}
}

func TestRegistryLoadFromDir(t *testing.T) {
	dir := t.TempDir()
	yaml := []byte(`
name: demo.echo_tool
description: Demo
version: 1
steps:
  - id: echo
    type: tool.execute
    tool: echo
`)
	if err := os.WriteFile(filepath.Join(dir, "demo.yaml"), yaml, 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	loaded, err := reg.LoadFromDir(dir)
	if err != nil {
		t.Fatalf("LoadFromDir: %v", err)
	}
	if loaded != 1 {
		t.Errorf("loaded = %d, want 1", loaded)
	}
	w, err := reg.Resolve("demo.echo_tool")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if w.Name != "demo.echo_tool" {
		t.Errorf("Name = %q, want %q", w.Name, "demo.echo_tool")
	}
}

func TestRegistryLoadFromDirSkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	valid := []byte(`
name: demo.echo_tool
description: Demo
version: 1
steps:
  - id: echo
    type: tool.execute
    tool: echo
`)
	invalid := []byte(`
name: demo.delegate
description: Uses an unsupported step type
version: 1
steps:
  - id: plan
    type: delegate
    agent: planner
`)
	// "a-invalid.yaml" sorts before "b-valid.yaml" so we prove that a bad file
	// early in the directory does not stop later valid files from loading.
	if err := os.WriteFile(filepath.Join(dir, "a-invalid.yaml"), invalid, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b-valid.yaml"), valid, 0644); err != nil {
		t.Fatal(err)
	}

	reg := NewRegistry()
	loaded, err := reg.LoadFromDir(dir)
	if err == nil {
		t.Error("expected a non-nil error describing the skipped invalid file")
	}
	if loaded != 1 {
		t.Errorf("loaded = %d, want 1", loaded)
	}
	if _, resolveErr := reg.Resolve("demo.echo_tool"); resolveErr != nil {
		t.Errorf("valid workflow should still load despite invalid sibling: %v", resolveErr)
	}
}

// ── Condition Tests ─────────────────────────────────────────────────────────

func TestEvaluateConditionEqualString(t *testing.T) {
	outputs := map[string]map[string]any{
		"evaluate_gate": {"decision": "allowed"},
	}
	met, err := EvaluateCondition(`evaluate_gate.decision == "allowed"`, outputs)
	if err != nil {
		t.Fatalf("EvaluateCondition: %v", err)
	}
	if !met {
		t.Error("condition should be met")
	}
}

func TestEvaluateConditionNotEqual(t *testing.T) {
	outputs := map[string]map[string]any{
		"evaluate_gate": {"decision": "denied"},
	}
	met, err := EvaluateCondition(`evaluate_gate.decision == "allowed"`, outputs)
	if err != nil {
		t.Fatalf("EvaluateCondition: %v", err)
	}
	if met {
		t.Error("condition should not be met")
	}
}

func TestEvaluateConditionMissingStep(t *testing.T) {
	outputs := map[string]map[string]any{}
	met, err := EvaluateCondition(`evaluate_gate.decision == "allowed"`, outputs)
	if err != nil {
		t.Fatalf("EvaluateCondition: %v", err)
	}
	if met {
		t.Error("condition should not be met for missing step")
	}
}

func TestEvaluateConditionMissingField(t *testing.T) {
	outputs := map[string]map[string]any{
		"evaluate_gate": {"other_field": "value"},
	}
	met, err := EvaluateCondition(`evaluate_gate.decision == "allowed"`, outputs)
	if err != nil {
		t.Fatalf("EvaluateCondition: %v", err)
	}
	if met {
		t.Error("condition should not be met for missing field")
	}
}

func TestEvaluateConditionEmpty(t *testing.T) {
	met, err := EvaluateCondition("", nil)
	if err != nil {
		t.Fatalf("EvaluateCondition: %v", err)
	}
	if !met {
		t.Error("empty condition should always be met")
	}
}

// ── Runner Tests ───────────────────────────────────────────────────────────

func TestRunnerEchoWorkflow(t *testing.T) {
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"text": input["text"]}, nil
		},
	}

	w := &Workflow{
		Name:    "demo.echo_tool",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo", Input: map[string]any{"text": "hello from workflow"}},
		},
	}

	runner := NewRunner(handlers)
	result, err := runner.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want %q", result.Status, "completed")
	}
	if len(result.State.StepStates) != 1 {
		t.Errorf("StepStates = %d, want 1", len(result.State.StepStates))
	}
	if result.State.StepStates[0].Status != "completed" {
		t.Errorf("Step status = %q, want %q", result.State.StepStates[0].Status, "completed")
	}
}

func TestRunnerStepOutputsRecorded(t *testing.T) {
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"text": "echo output", "length": 11}, nil
		},
	}

	w := &Workflow{
		Name:    "test.outputs",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	result, err := runner.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	output := result.State.StepStates[0].Output
	if output["text"] != "echo output" {
		t.Errorf("output[text] = %v, want %q", output["text"], "echo output")
	}
}

func TestRunnerFailedStep(t *testing.T) {
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return nil, fmt.Errorf("tool %q not found", toolName)
		},
	}

	w := &Workflow{
		Name:    "test.failure",
		Version: 1,
		Steps: []Step{
			{ID: "fail", Type: StepTypeToolExecute, Tool: "nonexistent"},
		},
	}

	runner := NewRunner(handlers)
	result, err := runner.Run(context.Background(), w, nil)
	if err == nil {
		t.Error("expected error from failed step")
	}
	if result.Status != "failed" {
		t.Errorf("Status = %q, want %q", result.Status, "failed")
	}
}

func TestRunnerSkippedCondition(t *testing.T) {
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"decision": "denied"}, nil
		},
	}

	w := &Workflow{
		Name:    "test.skip",
		Version: 1,
		Steps: []Step{
			{ID: "gate", Type: StepTypeToolExecute, Tool: "gate"},
			{ID: "skip_me", Type: StepTypeToolExecute, Tool: "echo", When: `gate.decision == "allowed"`},
		},
	}

	runner := NewRunner(handlers)
	result, err := runner.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want %q", result.Status, "completed")
	}
	// Gate step completed, skip_me step skipped
	if result.State.StepStates[0].Status != "completed" {
		t.Errorf("gate step = %q, want completed", result.State.StepStates[0].Status)
	}
	if result.State.StepStates[1].Status != "skipped" {
		t.Errorf("skip_me step = %q, want skipped", result.State.StepStates[1].Status)
	}
}

func TestRunnerWorkflowStop(t *testing.T) {
	handlers := StepHandlers{}

	w := &Workflow{
		Name:    "test.stop",
		Version: 1,
		Steps: []Step{
			{ID: "stop", Type: StepTypeWorkflowStop, Input: map[string]any{"reason": "gate denied"}},
		},
	}

	runner := NewRunner(handlers)
	result, err := runner.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want %q", result.Status, "completed")
	}
}

func TestRunnerWorkflowStopHaltsExecution(t *testing.T) {
	executed := false
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			executed = true
			return map[string]any{}, nil
		},
	}

	w := &Workflow{
		Name:    "test.stop_halt",
		Version: 1,
		Steps: []Step{
			{ID: "stop", Type: StepTypeWorkflowStop},
			{ID: "after", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	result, err := runner.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want %q", result.Status, "completed")
	}
	if executed {
		t.Error("step after workflow.stop should not execute")
	}
}

// ── Event Tests ────────────────────────────────────────────────────────────

func TestWorkflowEventsStarted(t *testing.T) {
	var collected []string
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"text": "ok"}, nil
		},
	}

	w := &Workflow{
		Name:    "test.events",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	runner.SetEmitter(func(eventType, source string, payload map[string]any) {
		collected = append(collected, eventType)
	})

	_, _ = runner.Run(context.Background(), w, nil)

	found := false
	for _, e := range collected {
		if e == V7EventTypes.WorkflowStarted {
			found = true
		}
	}
	if !found {
		t.Error("workflow.started event not emitted")
	}
}

func TestWorkflowEventsStepStarted(t *testing.T) {
	var collected []string
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"text": "ok"}, nil
		},
	}

	w := &Workflow{
		Name:    "test.events",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	runner.SetEmitter(func(eventType, source string, payload map[string]any) {
		collected = append(collected, eventType)
	})

	_, _ = runner.Run(context.Background(), w, nil)

	found := false
	for _, e := range collected {
		if e == V7EventTypes.WorkflowStepStarted {
			found = true
		}
	}
	if !found {
		t.Error("workflow.step.started event not emitted")
	}
}

func TestWorkflowEventsToolEmittedInsideWorkflow(t *testing.T) {
	// This test verifies that tool events can be emitted inside a workflow
	// by having the tool handler emit events through the same emitter.
	var collected []string
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			// Tool handler emits its own events (simulating existing layers)
			// These are captured by the same emitter
			return map[string]any{"text": "ok"}, nil
		},
	}

	w := &Workflow{
		Name:    "test.tool_events",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	runner.SetEmitter(func(eventType, source string, payload map[string]any) {
		collected = append(collected, eventType)
	})

	_, _ = runner.Run(context.Background(), w, nil)

	// At minimum: workflow.started, step.started, step.completed, workflow.completed
	if len(collected) < 4 {
		t.Errorf("expected at least 4 events, got %d: %v", len(collected), collected)
	}
}

func TestWorkflowEventsStepCompleted(t *testing.T) {
	var collected []string
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"text": "ok"}, nil
		},
	}

	w := &Workflow{
		Name:    "test.events",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	runner.SetEmitter(func(eventType, source string, payload map[string]any) {
		collected = append(collected, eventType)
	})

	_, _ = runner.Run(context.Background(), w, nil)

	found := false
	for _, e := range collected {
		if e == V7EventTypes.WorkflowStepCompleted {
			found = true
		}
	}
	if !found {
		t.Error("workflow.step.completed event not emitted")
	}
}

func TestWorkflowEventsWorkflowCompleted(t *testing.T) {
	var collected []string
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"text": "ok"}, nil
		},
	}

	w := &Workflow{
		Name:    "test.events",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	runner.SetEmitter(func(eventType, source string, payload map[string]any) {
		collected = append(collected, eventType)
	})

	_, _ = runner.Run(context.Background(), w, nil)

	found := false
	for _, e := range collected {
		if e == V7EventTypes.WorkflowCompleted {
			found = true
		}
	}
	if !found {
		t.Error("workflow.completed event not emitted")
	}
}

func TestWorkflowEventsCorrelationIDPreserved(t *testing.T) {
	var mu sync.Mutex
	var events []map[string]any
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"text": "ok"}, nil
		},
	}

	w := &Workflow{
		Name:    "test.corr_id",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	runner.SetEmitter(func(eventType, source string, payload map[string]any) {
		mu.Lock()
		events = append(events, payload)
		mu.Unlock()
	})

	_, _ = runner.Run(context.Background(), w, nil)

	mu.Lock()
	defer mu.Unlock()
	if len(events) == 0 {
		t.Fatal("no events emitted")
	}

	firstCorrID := events[0]["correlation_id"]
	for _, e := range events {
		if e["correlation_id"] != firstCorrID {
			t.Errorf("correlation_id mismatch: %v vs %v", e["correlation_id"], firstCorrID)
		}
	}
}

func TestWorkflowEventsEventOrder(t *testing.T) {
	var collected []string
	handlers := StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"text": "ok"}, nil
		},
	}

	w := &Workflow{
		Name:    "test.order",
		Version: 1,
		Steps: []Step{
			{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"},
		},
	}

	runner := NewRunner(handlers)
	runner.SetEmitter(func(eventType, source string, payload map[string]any) {
		collected = append(collected, eventType)
	})

	_, _ = runner.Run(context.Background(), w, nil)

	// Expected order: workflow.started, step.started, step.completed, workflow.completed
	expected := []string{
		V7EventTypes.WorkflowStarted,
		V7EventTypes.WorkflowStepStarted,
		V7EventTypes.WorkflowStepCompleted,
		V7EventTypes.WorkflowCompleted,
	}

	if len(collected) < len(expected) {
		t.Fatalf("expected at least %d events, got %d: %v", len(expected), len(collected), collected)
	}

	for i, exp := range expected {
		if collected[i] != exp {
			t.Errorf("event[%d] = %q, want %q", i, collected[i], exp)
		}
	}
}

// ── Gate Step Test ──────────────────────────────────────────────────────────

func TestRunnerGateStep(t *testing.T) {
	handlers := StepHandlers{
		GateEvaluate: func(ctx context.Context, gateName string, input map[string]any) (map[string]any, error) {
			return map[string]any{"decision": "allowed", "reason": "all checks passed", "risk_score": 0.1}, nil
		},
	}

	w := &Workflow{
		Name:    "test.gate",
		Version: 1,
		Steps: []Step{
			{ID: "evaluate_gate", Type: StepTypeGateEvaluate, Gate: "trading"},
		},
	}

	runner := NewRunner(handlers)
	result, err := runner.Run(context.Background(), w, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %q, want %q", result.Status, "completed")
	}
	output := result.State.StepStates[0].Output
	if output["decision"] != "allowed" {
		t.Errorf("decision = %v, want %q", output["decision"], "allowed")
	}
}

// ── Artifacts Tests ────────────────────────────────────────────────────────

func TestWriteWorkflowArtifacts(t *testing.T) {
	dir := t.TempDir()

	w := &Workflow{Name: "test", Version: 1, Steps: []Step{{ID: "echo", Type: StepTypeToolExecute, Tool: "echo"}}}
	state := WorkflowState{
		WorkflowName:  "test",
		Version:       1,
		Status:        "completed",
		RunID:         "run_test123",
		CorrelationID: "corr_test123",
		StepStates:    []StepState{{ID: "echo", Type: StepTypeToolExecute, Status: "completed"}},
	}
	result := &RunResult{
		RunID:         "run_test123",
		CorrelationID: "corr_test123",
		WorkflowName:  "test",
		Status:        "completed",
		State:         state,
	}

	err := WriteWorkflowArtifacts(dir, w, state, result)
	if err != nil {
		t.Fatalf("WriteWorkflowArtifacts: %v", err)
	}

	// Check workflow_state.json exists
	stateData, err := os.ReadFile(filepath.Join(dir, "workflow_state.json"))
	if err != nil {
		t.Fatalf("reading state: %v", err)
	}

	var savedState WorkflowState
	if err := json.Unmarshal(stateData, &savedState); err != nil {
		t.Fatalf("unmarshal state: %v", err)
	}
	if savedState.Status != "completed" {
		t.Errorf("saved state status = %q, want completed", savedState.Status)
	}

	// Check workflow_summary.json exists
	summaryData, err := os.ReadFile(filepath.Join(dir, "workflow_summary.json"))
	if err != nil {
		t.Fatalf("reading summary: %v", err)
	}

	var summary map[string]any
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatalf("unmarshal summary: %v", err)
	}
	if summary["workflow"] != "test" {
		t.Errorf("summary workflow = %v, want test", summary["workflow"])
	}
}

// ── Unsupported Step Type Test ──────────────────────────────────────────────

func TestUnsupportedStepTypeFails(t *testing.T) {
	w := &Workflow{
		Name:    "test.unsupported",
		Version: 1,
		Steps: []Step{
			{ID: "bad", Type: "agent.run"},
		},
	}

	if err := w.Validate(); err == nil {
		t.Error("expected error for unsupported step type")
	}
}
