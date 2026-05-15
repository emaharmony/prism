package gate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// mockGate is a simple mock implementation of the Gate interface for testing.
type mockGate struct {
	name     string
	domain   string
	decision Decision
	err      error
}

func (m *mockGate) Name() string    { return m.name }
func (m *mockGate) Domain() string  { return m.domain }

func (m *mockGate) Evaluate(_ context.Context, input GateInput) (*GateResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &GateResult{
		Decision:  m.decision,
		Reason:    fmt.Sprintf("mock %s result", m.decision),
		RiskScore: 0.5,
		Checks: []CheckResult{
			{Name: "mock_check", Status: string(m.decision), Details: "mock detail"},
		},
		Events:   []string{EventGateEvaluated},
		GateName: m.name,
		Domain:   input.Domain,
		Action:   input.Action,
	}, nil
}

func (m *mockGate) ValidateInput(_ GateInput) error {
	return nil
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()
	g := &mockGate{name: "test-gate", domain: "test"}

	if err := r.Register(g); err != nil {
		t.Fatalf("Register() failed: %v", err)
	}

	names := r.List()
	if len(names) != 1 || names[0] != "test-gate" {
		t.Fatalf("List() = %v, want [test-gate]", names)
	}
}

func TestRegistry_Resolve(t *testing.T) {
	r := NewRegistry()
	g := &mockGate{name: "test-gate", domain: "test"}
	r.Register(g)

	resolved, err := r.Resolve("test-gate")
	if err != nil {
		t.Fatalf("Resolve() failed: %v", err)
	}
	if resolved.Name() != "test-gate" {
		t.Fatalf("Resolve().Name() = %q, want %q", resolved.Name(), "test-gate")
	}
}

func TestRegistry_ResolveUnknownGate(t *testing.T) {
	r := NewRegistry()
	_, err := r.Resolve("nonexistent")
	if err == nil {
		t.Fatal("Resolve() should fail for unknown gate")
	}
}

func TestRegistry_DuplicateRegistration(t *testing.T) {
	r := NewRegistry()
	g1 := &mockGate{name: "test-gate", domain: "test"}
	g2 := &mockGate{name: "test-gate", domain: "test"}

	if err := r.Register(g1); err != nil {
		t.Fatalf("First Register() failed: %v", err)
	}
	if err := r.Register(g2); err == nil {
		t.Fatal("Second Register() should fail for duplicate name")
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockGate{name: "gate-b", domain: "test"})
	r.Register(&mockGate{name: "gate-a", domain: "test"})
	r.Register(&mockGate{name: "gate-c", domain: "test"})

	names := r.List()
	if len(names) != 3 {
		t.Fatalf("List() length = %d, want 3", len(names))
	}
	if names[0] != "gate-a" || names[1] != "gate-b" || names[2] != "gate-c" {
		t.Fatalf("List() = %v, want sorted [gate-a gate-b gate-c]", names)
	}
}

func TestEvaluator_RequestedEvent(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockGate{name: "test-gate", domain: "test", decision: DecisionAllowed})

	var events []string
	emitter := func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	}

	eval := NewEvaluator(r, emitter)
	input := GateInput{Gate: "test-gate", Domain: "test", Action: "check"}
	_, err := eval.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if len(events) == 0 || events[0] != EventGateRequested {
		t.Fatalf("First event = %v, want %s", events, EventGateRequested)
	}
}

func TestEvaluator_EvaluatedEvent(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockGate{name: "test-gate", domain: "test", decision: DecisionAllowed})

	var events []string
	emitter := func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	}

	eval := NewEvaluator(r, emitter)
	input := GateInput{Gate: "test-gate", Domain: "test", Action: "check"}
	_, err := eval.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	found := false
	for _, evt := range events {
		if evt == EventGateEvaluated {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Events %v should include %s", events, EventGateEvaluated)
	}
}

func TestEvaluator_AllowedEvent(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockGate{name: "test-gate", domain: "test", decision: DecisionAllowed})

	var events []string
	emitter := func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	}

	eval := NewEvaluator(r, emitter)
	result, err := eval.Evaluate(context.Background(), GateInput{Gate: "test-gate", Domain: "test", Action: "check"})
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if result.Decision != DecisionAllowed {
		t.Fatalf("Decision = %q, want %q", result.Decision, DecisionAllowed)
	}

	found := false
	for _, evt := range events {
		if evt == EventGateAllowed {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Events %v should include %s", events, EventGateAllowed)
	}
}

func TestEvaluator_DeniedEvent(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockGate{name: "test-gate", domain: "test", decision: DecisionDenied})

	var events []string
	emitter := func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	}

	eval := NewEvaluator(r, emitter)
	result, err := eval.Evaluate(context.Background(), GateInput{Gate: "test-gate", Domain: "test", Action: "check"})
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if result.Decision != DecisionDenied {
		t.Fatalf("Decision = %q, want %q", result.Decision, DecisionDenied)
	}

	found := false
	for _, evt := range events {
		if evt == EventGateDenied {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Events %v should include %s", events, EventGateDenied)
	}
}

func TestEvaluator_ApprovalRequiredEvent(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockGate{name: "test-gate", domain: "test", decision: DecisionRequiresApproval})

	var events []string
	emitter := func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	}

	eval := NewEvaluator(r, emitter)
	result, err := eval.Evaluate(context.Background(), GateInput{Gate: "test-gate", Domain: "test", Action: "check"})
	if err != nil {
		t.Fatalf("Evaluate() failed: %v", err)
	}

	if result.Decision != DecisionRequiresApproval {
		t.Fatalf("Decision = %q, want %q", result.Decision, DecisionRequiresApproval)
	}

	found := false
	for _, evt := range events {
		if evt == EventGateApprovalRequired {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Events %v should include %s", events, EventGateApprovalRequired)
	}
}

func TestEvaluator_FailedEvent(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockGate{name: "test-gate", domain: "test", decision: DecisionAllowed, err: errors.New("mock failure")})

	var events []string
	emitter := func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	}

	eval := NewEvaluator(r, emitter)
	_, err := eval.Evaluate(context.Background(), GateInput{Gate: "test-gate", Domain: "test", Action: "check"})
	if err == nil {
		t.Fatal("Evaluate() should fail for mock error")
	}

	found := false
	for _, evt := range events {
		if evt == EventGateFailed {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Events %v should include %s", events, EventGateFailed)
	}
}

func TestEvaluator_UnknownGateFails(t *testing.T) {
	r := NewRegistry()

	var events []string
	emitter := func(eventType, source string, payload map[string]any) {
		events = append(events, eventType)
	}

	eval := NewEvaluator(r, emitter)
	_, err := eval.Evaluate(context.Background(), GateInput{Gate: "nonexistent", Domain: "test", Action: "check"})
	if err == nil {
		t.Fatal("Evaluate() should fail for unknown gate")
	}

	found := false
	for _, evt := range events {
		if evt == EventGateFailed {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Events %v should include %s for unknown gate", events, EventGateFailed)
	}
}

func TestDecision_StringValues(t *testing.T) {
	tests := []struct {
		d    Decision
		want string
	}{
		{DecisionAllowed, "allowed"},
		{DecisionDenied, "denied"},
		{DecisionRequiresApproval, "requires_approval"},
	}

	for _, tt := range tests {
		if string(tt.d) != tt.want {
			t.Errorf("Decision %s = %q, want %q", tt.d, string(tt.d), tt.want)
		}
	}
}

func TestEvaluator_NilEmitter(t *testing.T) {
	// Should not panic with nil emitter
	r := NewRegistry()
	r.Register(&mockGate{name: "test-gate", domain: "test", decision: DecisionAllowed})

	eval := NewEvaluator(r, nil)
	_, err := eval.Evaluate(context.Background(), GateInput{Gate: "test-gate", Domain: "test", Action: "check"})
	if err != nil {
		t.Fatalf("Evaluate() with nil emitter failed: %v", err)
	}
}

func TestEvaluator_CancelledContext(t *testing.T) {
	r := NewRegistry()
	r.Register(&mockGate{name: "test-gate", domain: "test", decision: DecisionAllowed})

	emitter := func(eventType, source string, payload map[string]any) {}

	eval := NewEvaluator(r, emitter)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := eval.Evaluate(ctx, GateInput{Gate: "test-gate", Domain: "test", Action: "check"})
	if err == nil {
		t.Fatal("Evaluate() should fail for cancelled context")
	}
}

func TestWriteGateArtifacts(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "runs", "test-run-123")
	input := GateInput{Gate: "test-gate", Domain: "test", Action: "check"}
	result := GateResult{
		Decision:  DecisionAllowed,
		Reason:    "mock result",
		RiskScore: 0.5,
		Checks: []CheckResult{
			{Name: "check1", Status: "passed", Details: "detail"},
		},
		Events:   []string{EventGateEvaluated, EventGateAllowed},
		GateName: "test-gate",
		Domain:   "test",
		Action:   "check",
	}

	if err := WriteGateArtifacts(runDir, input, result); err != nil {
		t.Fatalf("WriteGateArtifacts() failed: %v", err)
	}

	artifactPath := filepath.Join(runDir, "gate_result.json")
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("Reading artifact file failed: %v", err)
	}

	var artifact struct {
		Input  GateInput  `json:"input"`
		Result GateResult `json:"result"`
	}
	if err := json.Unmarshal(data, &artifact); err != nil {
		t.Fatalf("Unmarshal artifact failed: %v", err)
	}

	if artifact.Input.Gate != "test-gate" {
		t.Errorf("Input.Gate = %q, want test-gate", artifact.Input.Gate)
	}
	if artifact.Result.Decision != DecisionAllowed {
		t.Errorf("Result.Decision = %q, want %q", artifact.Result.Decision, DecisionAllowed)
	}
	if len(artifact.Result.Checks) != 1 {
		t.Errorf("Result.Checks length = %d, want 1", len(artifact.Result.Checks))
	}
	if artifact.Result.Checks[0].Name != "check1" {
		t.Errorf("Result.Checks[0].Name = %q, want check1", artifact.Result.Checks[0].Name)
	}
}

func TestGateResult_JSONRoundtrip(t *testing.T) {
	result := GateResult{
		Decision:   DecisionRequiresApproval,
		Reason:     "high risk",
		RiskScore:  0.9,
		ApprovalID: "appr_123",
		Checks: []CheckResult{
			{Name: "risk_check", Status: "requires_approval", Details: "risk score too high"},
			{Name: "compliance", Status: "passed", Details: "all clear"},
		},
		Events:   []string{EventGateRequested, EventGateEvaluated, EventGateApprovalRequired},
		GateName: "trading",
		Domain:   "trading",
		Action:   "place_order",
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var decoded GateResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.Decision != DecisionRequiresApproval {
		t.Errorf("Decision = %q, want %q", decoded.Decision, DecisionRequiresApproval)
	}
	if decoded.ApprovalID != "appr_123" {
		t.Errorf("ApprovalID = %q, want appr_123", decoded.ApprovalID)
	}
	if decoded.RiskScore != 0.9 {
		t.Errorf("RiskScore = %f, want 0.9", decoded.RiskScore)
	}
	if len(decoded.Checks) != 2 {
		t.Errorf("Checks length = %d, want 2", len(decoded.Checks))
	}
	if decoded.GateName != "trading" {
		t.Errorf("GateName = %q, want trading", decoded.GateName)
	}
	if decoded.Domain != "trading" {
		t.Errorf("Domain = %q, want trading", decoded.Domain)
	}
	if decoded.Action != "place_order" {
		t.Errorf("Action = %q, want place_order", decoded.Action)
	}
}
