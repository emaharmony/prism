package main

import (
	"strings"
	"testing"
)

func TestWatchModelAppliesPhaseLifecycle(t *testing.T) {
	m := newWatchModel()
	m.apply("workflow.started", map[string]any{"workflow": "gated-loop"})
	m.apply("phase.entered", map[string]any{"phase": "EXECUTION"})
	m.apply("phase.tokens", map[string]any{"phase": "EXECUTION", "total": float64(250000), "max": float64(1000000)})
	m.apply("phase.gate_check", map[string]any{"phase": "EXECUTION", "passed": true, "score": float64(0.92)})
	m.apply("phase.verification", map[string]any{"phase": "EXECUTION", "profile": "go_test_all", "passed": true, "exit_code": float64(0)})

	if m.workflow != "gated-loop" || m.status != "running" {
		t.Fatalf("unexpected header state: workflow=%q status=%q", m.workflow, m.status)
	}
	pv := m.phases["EXECUTION"]
	if pv == nil || pv.status != "passed" || !pv.gatePassed || pv.gateScore != 0.92 {
		t.Fatalf("EXECUTION phase not updated: %+v", pv)
	}
	if m.tokTotal != 250000 || m.tokMax != 1000000 {
		t.Fatalf("token totals wrong: %d/%d", m.tokTotal, m.tokMax)
	}
	out := m.render()
	for _, want := range []string{"gated-loop", "EXECUTION", "gate 0.92", "go_test_all", "Budget", "25%"} {
		if !strings.Contains(out, want) {
			t.Fatalf("render missing %q:\n%s", want, out)
		}
	}
}

func TestWatchModelDelegationAndStuck(t *testing.T) {
	m := newWatchModel()
	m.apply("task.delegated", map[string]any{"task_id": "T2", "agent": "mango"})
	if m.delegations["T2"] != "mango:sent" {
		t.Fatalf("delegation not recorded: %v", m.delegations)
	}
	m.apply("delegation.retry", map[string]any{"tasks": []any{"T2"}})
	if m.delegations["T2"] != "mango:retrying" {
		t.Fatalf("retry not reflected: %v", m.delegations)
	}
	m.apply("delegation.timeout", map[string]any{"tasks": []any{"T2"}})
	if m.delegations["T2"] != "mango:timed_out" {
		t.Fatalf("timeout not reflected: %v", m.delegations)
	}

	m.apply("phase.entered", map[string]any{"phase": "EXECUTION"})
	m.apply("phase.stuck", map[string]any{"phase": "EXECUTION", "tool": "read_file", "repeats": float64(6)})
	if m.phases["EXECUTION"].status != "stuck" {
		t.Fatalf("stuck status not set: %+v", m.phases["EXECUTION"])
	}
	out := m.render()
	if !strings.Contains(out, "Delegations") || !strings.Contains(out, "timed_out") {
		t.Fatalf("delegations not rendered:\n%s", out)
	}
}

func TestTokenMeter(t *testing.T) {
	full := tokenMeter(1000000, 1000000)
	if !strings.Contains(full, "100%") || !strings.Contains(full, "█") {
		t.Fatalf("full meter wrong: %q", full)
	}
	half := tokenMeter(500000, 1000000)
	if !strings.Contains(half, "50%") {
		t.Fatalf("half meter wrong: %q", half)
	}
	none := tokenMeter(1234, 0)
	if !strings.Contains(none, "no ceiling") {
		t.Fatalf("uncapped meter wrong: %q", none)
	}
	// Over-budget clamps to 100%, never panics on the bar width.
	over := tokenMeter(2000000, 1000000)
	if !strings.Contains(over, "100%") {
		t.Fatalf("over-budget meter should clamp: %q", over)
	}
}

func TestWatchModelPausedAndCompleted(t *testing.T) {
	m := newWatchModel()
	m.apply("phase.entered", map[string]any{"phase": "FEEDBACK_PRE"})
	m.apply("workflow.paused", map[string]any{"phase": "FEEDBACK_PRE"})
	if m.status != "paused" || m.phases["FEEDBACK_PRE"].status != "paused" {
		t.Fatalf("paused not reflected: status=%q phase=%+v", m.status, m.phases["FEEDBACK_PRE"])
	}
	m.apply("workflow.completed", map[string]any{"workflow": "gated-loop"})
	if m.status != "completed" {
		t.Fatalf("completed not reflected: %q", m.status)
	}
}
