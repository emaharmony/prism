package approval

import "testing"

func TestApprovalRequiresDecisionActor(t *testing.T) {
	tests := []struct {
		name   string
		decide func(*Approval) error
	}{
		{name: "approve empty", decide: func(a *Approval) error { return a.Approve("") }},
		{name: "approve whitespace", decide: func(a *Approval) error { return a.Approve("  \t") }},
		{name: "deny empty", decide: func(a *Approval) error { return a.Deny("", "reason") }},
		{name: "deny whitespace", decide: func(a *Approval) error { return a.Deny("  \t", "reason") }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := NewApproval("run", "corr", "requester", "project", MutationWriteFile, "file.txt", "content", PolicyDecision{Decision: DecisionRequiresApproval})
			if err := tt.decide(a); err == nil {
				t.Fatal("expected an empty decision actor to be rejected")
			}
			if a.Status != StatusPending {
				t.Fatalf("rejected decision changed status to %q", a.Status)
			}
			if a.ApprovedAt != nil || a.DeniedAt != nil {
				t.Fatal("rejected decision recorded a decision timestamp")
			}
		})
	}
}
