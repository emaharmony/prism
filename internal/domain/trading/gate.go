package trading

import (
	"context"
	"fmt"

	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/gate"
)

// TradingGate implements the gate.Gate interface for the trading domain.
type TradingGate struct {
	config *Config
	policy *PolicyEvaluator
}

// NewTradingGate creates a trading gate with the given configuration.
func NewTradingGate(config *Config) *TradingGate {
	return &TradingGate{
		config: config,
		policy: NewPolicyEvaluator(config),
	}
}

// Name returns the gate's registered name.
func (g *TradingGate) Name() string { return "trading" }

// Domain returns the domain this gate handles.
func (g *TradingGate) Domain() string { return "trading" }

// ValidateInput checks that the GateInput payload is a valid trade proposal.
func (g *TradingGate) ValidateInput(input gate.GateInput) error {
	proposal, err := FromGateInput(input)
	if err != nil {
		return fmt.Errorf("invalid trade proposal payload: %w", err)
	}

	if err := proposal.Validate(); err != nil {
		return fmt.Errorf("invalid trade proposal: %w", err)
	}

	return nil
}

// Evaluate runs the trading gate's policy checks and returns a GateResult.
func (g *TradingGate) Evaluate(ctx context.Context, input gate.GateInput) (*gate.GateResult, error) {
	// 1. Extract TradeProposal from input
	proposal, err := FromGateInput(input)
	if err != nil {
		return nil, fmt.Errorf("failed to extract trade proposal: %w", err)
	}

	// 2. Run policy evaluation
	checks, decision := g.policy.Evaluate(proposal)

	// 3. Calculate risk score
	riskScore := g.policy.calculateRiskScore(proposal)

	// Build the reason from the last check result
	reason := ""
	for i := len(checks) - 1; i >= 0; i-- {
		if checks[i].Status == "denied" || checks[i].Status == "requires_approval" {
			reason = checks[i].Details
			break
		}
	}
	if reason == "" {
		reason = "all policy checks passed"
	}

	// 4. If requires_approval, generate approval ID
	var approvalID string
	if decision == gate.DecisionRequiresApproval {
		approvalID = "appr_" + event.NewID()
	}

	// Collect event types based on decision
	var events []string
	switch decision {
	case gate.DecisionAllowed:
		events = []string{EventTradeProposed, EventRiskEvaluated, EventTradeAllowed}
	case gate.DecisionDenied:
		events = []string{EventTradeProposed, EventRiskEvaluated, EventTradeDenied}
	case gate.DecisionRequiresApproval:
		events = []string{EventTradeProposed, EventRiskEvaluated, EventTradeApprovalRequired}
	}

	// 5. Build and return GateResult
	return &gate.GateResult{
		Decision:   decision,
		Reason:     reason,
		RiskScore:  riskScore,
		ApprovalID: approvalID,
		Checks:     checks,
		Events:     events,
		GateName:   g.Name(),
		Domain:     g.Domain(),
		Action:     input.Action,
	}, nil
}
