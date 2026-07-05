package v2

import (
	"strings"
	"testing"
)

func TestFinalReportIncludesVerificationAndDelegations(t *testing.T) {
	state := NewWorkflowState(DefaultConfig())
	state.RunID = "gl-1"
	state.Verification = &VerificationRecord{Profile: "go_test_all", Passed: true, ExitCode: 0, Attempts: 2}
	state.Delegations = []DelegationState{
		{TaskID: "T2", Agent: "mango", Status: "completed"},
		{TaskID: "T3", Agent: "junie", Status: "timed_out", RetryCount: 2},
	}

	out := FormatFinalReport(state)

	for _, want := range []string{"### Verification", "go_test_all", "passed", "### Delegations", "T2", "mango", "T3", "timed_out", "2 retries"} {
		if !strings.Contains(out, want) {
			t.Fatalf("final report missing %q.\n---\n%s", want, out)
		}
	}
}

func TestFinalReportFailedVerificationShown(t *testing.T) {
	state := NewWorkflowState(DefaultConfig())
	state.Verification = &VerificationRecord{Profile: "go_test_all", Passed: false, ExitCode: 1, Attempts: 3}
	out := FormatFinalReport(state)
	if !strings.Contains(out, "FAILED") || !strings.Contains(out, "exit 1") {
		t.Fatalf("expected failed verification in report:\n%s", out)
	}
}
