package gate

import (
	"context"
	"fmt"
)

// EventEmitter is a function that emits a Prism event.
// Matches the pattern used in validation and mutation packages.
type EventEmitter func(eventType, source string, payload map[string]any)

// Evaluator orchestrates gate evaluation: resolve gate → validate → evaluate → return result.
type Evaluator struct {
	registry *Registry
	emitter  EventEmitter
}

// NewEvaluator creates a new gate evaluator.
func NewEvaluator(registry *Registry, emitter EventEmitter) *Evaluator {
	return &Evaluator{
		registry: registry,
		emitter:  emitter,
	}
}

// emit sends an event through the emitter if one is set.
func (e *Evaluator) emit(eventType string, payload map[string]any) {
	if e.emitter != nil {
		e.emitter(eventType, "prism-gate", payload)
	}
}

// Evaluate resolves the gate, validates input, runs evaluation, emits events, and returns the result.
//
// Events emitted:
//   - prism.gate.requested — before evaluation starts
//   - prism.gate.evaluated — after evaluation completes (always)
//   - prism.gate.allowed — if decision is allowed
//   - prism.gate.denied — if decision is denied
//   - prism.gate.approval_required — if decision is requires_approval
//   - prism.gate.failed — if evaluation itself fails (error case)
func (e *Evaluator) Evaluate(ctx context.Context, input GateInput) (*GateResult, error) {
	// Emit requested event
	e.emit(EventGateRequested, map[string]any{
		"gate":   input.Gate,
		"domain": input.Domain,
		"action": input.Action,
	})

	// Resolve the gate by name
	g, err := e.registry.Resolve(input.Gate)
	if err != nil {
		e.emit(EventGateFailed, map[string]any{
			"gate":   input.Gate,
			"domain": input.Domain,
			"action": input.Action,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("gate evaluation failed: %w", err)
	}

	// Validate input
	if err := g.ValidateInput(input); err != nil {
		e.emit(EventGateFailed, map[string]any{
			"gate":   input.Gate,
			"domain": input.Domain,
			"action": input.Action,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("gate input validation failed: %w", err)
	}

	// Check context cancellation before evaluation
	if err := ctx.Err(); err != nil {
		e.emit(EventGateFailed, map[string]any{
			"gate":   input.Gate,
			"domain": input.Domain,
			"action": input.Action,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("gate evaluation cancelled: %w", err)
	}

	// Run evaluation
	result, err := g.Evaluate(ctx, input)
	if err != nil {
		e.emit(EventGateFailed, map[string]any{
			"gate":   input.Gate,
			"domain": input.Domain,
			"action": input.Action,
			"error":  err.Error(),
		})
		return nil, fmt.Errorf("gate evaluation failed: %w", err)
	}

	// Emit evaluated event (always)
	e.emit(EventGateEvaluated, map[string]any{
		"gate":      input.Gate,
		"domain":    input.Domain,
		"action":    input.Action,
		"decision":  string(result.Decision),
		"risk_score": result.RiskScore,
	})

	// Emit decision-specific event
	switch result.Decision {
	case DecisionAllowed:
		e.emit(EventGateAllowed, map[string]any{
			"gate":   input.Gate,
			"domain": input.Domain,
			"action": input.Action,
		})
	case DecisionDenied:
		e.emit(EventGateDenied, map[string]any{
			"gate":   input.Gate,
			"domain": input.Domain,
			"action": input.Action,
			"reason": result.Reason,
		})
	case DecisionRequiresApproval:
		e.emit(EventGateApprovalRequired, map[string]any{
			"gate":        input.Gate,
			"domain":      input.Domain,
			"action":      input.Action,
			"approval_id": result.ApprovalID,
		})
	}

	return result, nil
}
