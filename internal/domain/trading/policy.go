package trading

import (
	"fmt"
	"math"

	"github.com/emaharmony/prism/internal/gate"
)

// PolicyEvaluator evaluates deterministic trading rules against a proposal.
type PolicyEvaluator struct {
	config *Config
}

// NewPolicyEvaluator creates a policy evaluator with the given config.
func NewPolicyEvaluator(config *Config) *PolicyEvaluator {
	return &PolicyEvaluator{config: config}
}

// Evaluate checks the proposal against all policy rules.
// Returns slice of CheckResult and the final Decision.
//
// Evaluation order (first denial wins):
//   1. Blocklisted symbol → denied
//   2. Confidence below min_confidence → denied
//   3. Notional exceeds max_trade_notional → denied
//   4. Daily realized PnL <= -max_daily_loss → denied
//   5. Allowlist check: if allowlist is non-empty, symbol must be in it → denied if not
//   6. Live mode → requires_approval (if RequireForLive is true)
//   7. Notional exceeds approval threshold → requires_approval
//   8. Short/sell with RequireForShort → requires_approval
//   9. Paper mode, all checks pass → allowed
func (pe *PolicyEvaluator) Evaluate(proposal *TradeProposal) ([]gate.CheckResult, gate.Decision) {
	var checks []gate.CheckResult

	// 1. Blocklisted symbol → denied
	if pe.isBlocklisted(proposal.Symbol) {
		checks = append(checks, gate.CheckResult{
			Name:    "symbol_blocklist",
			Status:  "denied",
			Details: fmt.Sprintf("symbol %q is blocklisted", proposal.Symbol),
		})
		return checks, gate.DecisionDenied
	}
	checks = append(checks, gate.CheckResult{
		Name:   "symbol_blocklist",
		Status: "passed",
	})

	// 2. Confidence below min_confidence → denied
	if proposal.Confidence < pe.config.Limits.MinConfidence {
		checks = append(checks, gate.CheckResult{
			Name:    "min_confidence",
			Status:  "denied",
			Details: fmt.Sprintf("confidence %.2f is below minimum %.2f", proposal.Confidence, pe.config.Limits.MinConfidence),
		})
		return checks, gate.DecisionDenied
	}
	checks = append(checks, gate.CheckResult{
		Name:   "min_confidence",
		Status: "passed",
	})

	// 3. Notional exceeds max_trade_notional → denied
	if proposal.EstimatedNotional > pe.config.Limits.MaxTradeNotional {
		checks = append(checks, gate.CheckResult{
			Name:    "max_trade_notional",
			Status:  "denied",
			Details: fmt.Sprintf("notional %.2f exceeds maximum %.2f", proposal.EstimatedNotional, pe.config.Limits.MaxTradeNotional),
		})
		return checks, gate.DecisionDenied
	}
	checks = append(checks, gate.CheckResult{
		Name:   "max_trade_notional",
		Status: "passed",
	})

	// 4. Daily realized PnL <= -max_daily_loss → denied
	if proposal.Portfolio != nil && proposal.Portfolio.DailyRealizedPnL <= -pe.config.Limits.MaxDailyLoss {
		checks = append(checks, gate.CheckResult{
			Name:    "max_daily_loss",
			Status:  "denied",
			Details: fmt.Sprintf("daily realized PnL %.2f has exceeded max daily loss %.2f", proposal.Portfolio.DailyRealizedPnL, pe.config.Limits.MaxDailyLoss),
		})
		return checks, gate.DecisionDenied
	}
	checks = append(checks, gate.CheckResult{
		Name:   "max_daily_loss",
		Status: "passed",
	})

	// 5. Allowlist check: if allowlist is non-empty, symbol must be in it → denied if not
	if len(pe.config.Symbols.Allowlist) > 0 && !pe.isAllowlisted(proposal.Symbol) {
		checks = append(checks, gate.CheckResult{
			Name:    "symbol_allowlist",
			Status:  "denied",
			Details: fmt.Sprintf("symbol %q is not on the allowlist", proposal.Symbol),
		})
		return checks, gate.DecisionDenied
	}
	checks = append(checks, gate.CheckResult{
		Name:   "symbol_allowlist",
		Status: "passed",
	})

	// 6. Live mode → requires_approval (if RequireForLive is true)
	if proposal.Mode == "live" && pe.config.Approvals.RequireForLive {
		checks = append(checks, gate.CheckResult{
			Name:    "live_mode_approval",
			Status:  "requires_approval",
			Details: "live trading requires human approval",
		})
		return checks, gate.DecisionRequiresApproval
	}
	checks = append(checks, gate.CheckResult{
		Name:   "live_mode_approval",
		Status: "passed",
	})

	// 7. Notional exceeds approval threshold → requires_approval
	if proposal.EstimatedNotional > pe.config.Approvals.RequireAboveNotional {
		checks = append(checks, gate.CheckResult{
			Name:    "notional_approval_threshold",
			Status:  "requires_approval",
			Details: fmt.Sprintf("notional %.2f exceeds approval threshold %.2f", proposal.EstimatedNotional, pe.config.Approvals.RequireAboveNotional),
		})
		return checks, gate.DecisionRequiresApproval
	}
	checks = append(checks, gate.CheckResult{
		Name:   "notional_approval_threshold",
		Status: "passed",
	})

	// 8. Short/sell with RequireForShort → requires_approval
	if proposal.Side == "sell" && pe.config.Approvals.RequireForShort {
		checks = append(checks, gate.CheckResult{
			Name:    "short_sell_approval",
			Status:  "requires_approval",
			Details: "sell orders require human approval",
		})
		return checks, gate.DecisionRequiresApproval
	}
	checks = append(checks, gate.CheckResult{
		Name:   "short_sell_approval",
		Status: "passed",
	})

	// 9. Paper mode, all checks pass → allowed
	return checks, gate.DecisionAllowed
}

// calculateRiskScore computes a 0.0–1.0 risk score for the proposal.
// Higher = riskier. Factors: notional vs max, confidence inverse, live mode.
func (pe *PolicyEvaluator) calculateRiskScore(proposal *TradeProposal) float64 {
	score := 0.0

	// Factor 1: notional vs max_trade_notional (0–0.4)
	if pe.config.Limits.MaxTradeNotional > 0 {
		notionalRatio := proposal.EstimatedNotional / pe.config.Limits.MaxTradeNotional
		notionalRatio = math.Min(notionalRatio, 1.0)
		score += notionalRatio * 0.4
	}

	// Factor 2: confidence inverse — lower confidence = higher risk (0–0.35)
	if proposal.Confidence <= 1.0 {
		confidenceInverse := 1.0 - proposal.Confidence
		score += confidenceInverse * 0.35
	}

	// Factor 3: live mode penalty (0.25)
	if proposal.Mode == "live" {
		score += 0.25
	}

	return math.Min(score, 1.0)
}

// isBlocklisted checks whether a symbol is on the blocklist.
func (pe *PolicyEvaluator) isBlocklisted(symbol string) bool {
	for _, blocked := range pe.config.Symbols.Blocklist {
		if blocked == symbol {
			return true
		}
	}
	return false
}

// isAllowlisted checks whether a symbol is on the allowlist.
func (pe *PolicyEvaluator) isAllowlisted(symbol string) bool {
	for _, allowed := range pe.config.Symbols.Allowlist {
		if allowed == symbol {
			return true
		}
	}
	return false
}
