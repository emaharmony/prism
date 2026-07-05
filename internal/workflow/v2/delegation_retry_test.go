package v2

import (
	"context"
	"testing"
	"time"
)

func TestRetryDelegationAccounting(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	st := NewWorkflowState(&WorkflowConfig{Name: "x"})
	st.Plan = &PlanGraph{Tasks: []PlanTask{{ID: "T2", Agent: "mango", Description: "d"}}}
	old := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	st.Delegations = []DelegationState{{TaskID: "T2", Agent: "mango", Status: "sent", SentAt: old}}

	// maxRetries defaults to 2: two retries succeed, the third is exhausted.
	if pkt, ok := dm.RetryDelegation("T2", st); !ok || pkt.TaskID != "T2" {
		t.Fatalf("first retry should succeed, got ok=%v pkt=%+v", ok, pkt)
	}
	if st.Delegations[0].RetryCount != 1 || st.Delegations[0].Status != "sent" {
		t.Fatalf("retry should bump count and re-arm: %+v", st.Delegations[0])
	}
	if _, ok := dm.RetryDelegation("T2", st); !ok || st.Delegations[0].RetryCount != 2 {
		t.Fatalf("second retry should succeed, count=%d", st.Delegations[0].RetryCount)
	}
	if _, ok := dm.RetryDelegation("T2", st); ok {
		t.Fatal("third retry should be exhausted")
	}
	if _, ok := dm.RetryDelegation("UNKNOWN", st); ok {
		t.Fatal("unknown task should not retry")
	}
}

// On timeout the driver re-dispatches (republishes) rather than failing while
// retries remain.
func TestDriveRetriesTimedOutDelegation(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	cfg := delegConfig()
	cfg.Phases[0].MaxIterations = 3
	cfg.Phases[0].Gate = GateConfig{Type: "task_completion", Mode: "partial_allowed"}
	em := &captureEmitter{}
	engine := NewEngine(cfg, em, dm)

	var published []TaskPacket
	engine.SetTaskPublisher(func(p TaskPacket) error { published = append(published, p); return nil })

	st := engine.GetState()
	st.Plan = &PlanGraph{Tasks: []PlanTask{
		{ID: "T1", Agent: "mango", Status: "completed"},
		{ID: "T2", Agent: "mango", Status: "in_progress"},
	}}
	old := time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	st.Delegations = []DelegationState{{TaskID: "T2", Agent: "mango", Status: "sent", SentAt: old}}

	llm := func(_ context.Context, _ []Message) (string, int, int, error) {
		return `{"type":"final","content":"done"}`, 1, 1, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := engine.Drive(ctx, llm, noopTool, DriveOptions{SystemPrompt: "s", UserPrompt: "u", SkipPushRequirement: true}); err != nil {
		t.Fatalf("Drive error: %v", err)
	}

	if len(published) == 0 || published[0].TaskID != "T2" {
		t.Fatalf("expected the timed-out task to be republished, got %+v", published)
	}
	if !em.has("delegation.retry") {
		t.Fatalf("expected delegation.retry event, got %v", em.events)
	}
	if st.Delegations[0].RetryCount != 1 || st.Delegations[0].Status != "sent" {
		t.Fatalf("delegation should be re-armed once, got %+v", st.Delegations[0])
	}
}
