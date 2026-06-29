package v2

import (
	"strings"
	"testing"
)

func TestFormatReviewerListDynamic(t *testing.T) {
	if got := formatReviewerList(nil); got != "the configured reviewers" {
		t.Fatalf("empty should be generic, got %q", got)
	}
	if got := formatReviewerList([]string{"ema"}); got != "**ema** (required)" {
		t.Fatalf("single reviewer wrong: %q", got)
	}
	got := formatReviewerList([]string{"alice", "bob"})
	if got != "**alice** (required) and **bob**" {
		t.Fatalf("two reviewers wrong: %q", got)
	}
}

func TestFormatReviewPackageUsesConfiguredReviewers(t *testing.T) {
	state := NewWorkflowState(DefaultConfig())
	state.RunID = "gl-1"
	out := FormatReviewPackage(state, "", []string{"alice", "carol"})
	if !strings.Contains(out, "**alice** (required) and **carol**") {
		t.Fatalf("expected configured reviewers, got:\n%s", out)
	}
	// No hardcoded personas leak in.
	if strings.Contains(out, "Mango") || strings.Contains(out, "Lumi") {
		t.Fatalf("review package still contains hardcoded personas:\n%s", out)
	}
}

func TestFormatPlanForApprovalNamesApprovers(t *testing.T) {
	state := NewWorkflowState(DefaultConfig())
	state.RunID = "gl-1"
	out := FormatPlanForApproval(state, []string{"dave"})
	if !strings.Contains(out, "**Approvers:** dave") {
		t.Fatalf("expected approver name, got:\n%s", out)
	}
	// Empty approvers: no Approvers line, no panic.
	if strings.Contains(FormatPlanForApproval(state, nil), "**Approvers:**") {
		t.Fatalf("empty approvers should omit the line")
	}
}

func TestAgentGlyphDefaultsAndOverrides(t *testing.T) {
	if agentGlyph("anybody") != "🤖" {
		t.Fatalf("unknown agent should default to robot")
	}
	AgentGlyphs["coder"] = "🔧"
	defer delete(AgentGlyphs, "coder")
	if agentGlyph("coder") != "🔧" {
		t.Fatalf("override not honored")
	}
}
