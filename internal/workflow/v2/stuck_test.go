package v2

import (
	"context"
	"testing"
	"time"
)

// A model that requests the identical tool call forever (never making progress)
// must trip stuck-loop detection and abort the phase, rather than burning the
// full iteration budget.
func TestDriveAbortsOnStuckToolLoop(t *testing.T) {
	cfg := &WorkflowConfig{
		Name: "stuck", Version: 1,
		Global: GlobalConfig{MaxTotalIterations: 50, MaxRepeatedToolCalls: 4},
		Phases: []PhaseConfig{{
			Name: "PROBE", Type: "probe", MaxIterations: 30,
			AllowedTools: []string{"read_file"},
			Gate:         GateConfig{Type: "assumption_threshold", Threshold: 2.0},
		}},
	}
	em := &captureEmitter{}
	engine := NewEngine(cfg, em, nil)

	toolCalls := 0
	tool := func(_ context.Context, _ string, _ *ToolRequest) (string, error) {
		toolCalls++
		return "ok", nil
	}
	llm := func(_ context.Context, _ []Message) (string, int, int, error) {
		return `{"type":"tool_request","tool":"read_file","input":{"path":"x"}}`, 1, 1, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := engine.Drive(ctx, llm, tool, DriveOptions{SystemPrompt: "s", UserPrompt: "u"})
	if err != nil {
		t.Fatalf("Drive returned error: %v", err)
	}
	if !em.has("phase.stuck") {
		t.Fatalf("expected phase.stuck event, got %v", em.events)
	}
	// The phase aborts at the cap (4); the 4th identical call is detected before
	// execution, so the tool runs strictly fewer times than the iteration budget.
	if toolCalls >= 30 {
		t.Fatalf("stuck detection did not bound tool calls: ran %d", toolCalls)
	}
}

// Distinct tool calls must NOT be flagged as stuck.
func TestToolSignatureDistinguishesInputs(t *testing.T) {
	a := toolSignature(&ToolRequest{Tool: "read_file", Input: map[string]any{"path": "a"}})
	b := toolSignature(&ToolRequest{Tool: "read_file", Input: map[string]any{"path": "b"}})
	if a == b {
		t.Fatalf("different inputs should produce different signatures: %q == %q", a, b)
	}
	c := toolSignature(&ToolRequest{Tool: "read_file", Input: map[string]any{"path": "a"}})
	if a != c {
		t.Fatalf("identical calls should produce identical signatures: %q != %q", a, c)
	}
}
