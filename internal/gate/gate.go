package gate

import "context"

// GateInput is the generic input shape for any gate evaluation.
type GateInput struct {
	Gate    string         `json:"gate"`
	Domain  string         `json:"domain"`
	Action  string         `json:"action"`
	Payload map[string]any `json:"payload"`
}

// Gate is the interface every domain adapter must implement.
type Gate interface {
	// Name returns the gate's registered name (e.g., "trading").
	Name() string

	// Domain returns the domain this gate handles (e.g., "trading").
	Domain() string

	// Evaluate runs the gate's policy checks and returns a GateResult.
	Evaluate(ctx context.Context, input GateInput) (*GateResult, error)

	// ValidateInput checks that the input is well-formed for this gate.
	ValidateInput(input GateInput) error
}
