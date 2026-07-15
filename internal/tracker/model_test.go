package tracker

import "testing"

func TestModelAppliesWorkflowLifecycleAndAliases(t *testing.T) {
	m := New()
	m.Apply("workflow.started", map[string]any{"workflow": "gated-loop"})
	m.Apply("workflow.phase.entered", map[string]any{"phase": "EXECUTION"})
	m.Apply("workflow.phase.tokens", map[string]any{"phase": "EXECUTION", "total": float64(250000), "max": float64(1000000)})
	m.Apply("workflow.phase.gate_check", map[string]any{"phase": "EXECUTION", "passed": true, "score": float64(0.92)})
	m.Apply("workflow.phase.verification", map[string]any{"phase": "EXECUTION", "profile": "go_test_all", "passed": true, "exit_code": float64(0)})

	s := m.Snapshot()
	if s.Workflow != "gated-loop" || s.Status != "running" || s.Current != "EXECUTION" {
		t.Fatalf("unexpected snapshot header: %+v", s)
	}
	if s.TokTotal != 250000 || s.TokMax != 1000000 {
		t.Fatalf("token totals wrong: %d/%d", s.TokTotal, s.TokMax)
	}
	if len(s.Phases) != 1 {
		t.Fatalf("phase count = %d, want 1", len(s.Phases))
	}
	pv := s.Phases[0]
	if pv.Name != "EXECUTION" || pv.Status != "passed" || !pv.GatePassed || pv.GateScore != 0.92 {
		t.Fatalf("phase not updated: %+v", pv)
	}
	if pv.VerifyText != "go_test_all pass(exit 0)" {
		t.Fatalf("verify text = %q", pv.VerifyText)
	}
	if s.LastEvent != "phase.verification" {
		t.Fatalf("alias was not normalized: %q", s.LastEvent)
	}
}

func TestModelDelegationRetryTimeoutAndSnapshotCopy(t *testing.T) {
	m := New()
	m.Apply("workflow.task.delegated", map[string]any{"task_id": "T2", "agent": "mango"})
	m.Apply("delegation.retry", map[string]any{"tasks": []any{"T2"}})
	m.Apply("delegation.timeout", map[string]any{"tasks": []string{"T2"}})

	s := m.Snapshot()
	if s.Delegations["T2"] != "mango:timed_out" {
		t.Fatalf("delegation status = %q", s.Delegations["T2"])
	}

	s.Delegations["T2"] = "mutated"
	if got := m.Snapshot().Delegations["T2"]; got != "mango:timed_out" {
		t.Fatalf("snapshot map was not copied: %q", got)
	}
}

func TestModelPausedCompletedAndStuck(t *testing.T) {
	m := New()
	m.Apply("phase.entered", map[string]any{"phase": "FEEDBACK_PRE"})
	m.Apply("workflow.paused.waiting_approval", map[string]any{"phase": "FEEDBACK_PRE"})

	s := m.Snapshot()
	if s.Status != "paused" || s.Phases[0].Status != "paused" {
		t.Fatalf("paused not reflected: %+v", s)
	}

	m.Apply("phase.stuck", map[string]any{"phase": "FEEDBACK_PRE", "tool": "read_file"})
	m.Apply("workflow.completed", map[string]any{})
	s = m.Snapshot()
	if s.Status != "completed" || s.Phases[0].Status != "stuck" || s.Phases[0].LastTool != "read_file" {
		t.Fatalf("final state wrong: %+v", s)
	}
}
