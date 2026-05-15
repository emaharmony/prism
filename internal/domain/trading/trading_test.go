package trading

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emaharmony/prism/internal/gate"
)

// --- Schema Validation Tests ---

func validProposal() *TradeProposal {
	return &TradeProposal{
		Domain:            "trading",
		Action:            "trade_proposal",
		Mode:              "paper",
		Symbol:            "AAPL",
		Side:              "buy",
		Quantity:          10,
		OrderType:         "market",
		EstimatedPrice:    150.50,
		EstimatedNotional: 1505.00,
		Strategy:          "momentum",
		Confidence:        0.85,
		Reason:            "strong upward trend",
	}
}

func TestValidate_ValidProposal(t *testing.T) {
	p := validProposal()
	if err := p.Validate(); err != nil {
		t.Errorf("expected valid proposal to pass, got: %v", err)
	}
}

func TestValidate_MissingSymbol(t *testing.T) {
	p := validProposal()
	p.Symbol = ""
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for missing symbol")
	}
	if !strings.Contains(err.Error(), "symbol") {
		t.Errorf("expected error about symbol, got: %v", err)
	}
}

func TestValidate_InvalidSide(t *testing.T) {
	p := validProposal()
	p.Side = "short"
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for invalid side")
	}
	if !strings.Contains(err.Error(), "side") {
		t.Errorf("expected error about side, got: %v", err)
	}
}

func TestValidate_NegativeQuantity(t *testing.T) {
	p := validProposal()
	p.Quantity = -5
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for negative quantity")
	}
	if !strings.Contains(err.Error(), "quantity") {
		t.Errorf("expected error about quantity, got: %v", err)
	}
}

func TestValidate_InvalidConfidence(t *testing.T) {
	tests := []struct {
		name       string
		confidence float64
	}{
		{"below zero", -0.1},
		{"above one", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := validProposal()
			p.Confidence = tt.confidence
			err := p.Validate()
			if err == nil {
				t.Fatal("expected error for invalid confidence")
			}
			if !strings.Contains(err.Error(), "confidence") {
				t.Errorf("expected error about confidence, got: %v", err)
			}
		})
	}
}

func TestValidate_InvalidMode(t *testing.T) {
	p := validProposal()
	p.Mode = "production"
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("expected error about mode, got: %v", err)
	}
}

func TestValidate_OrderTypeRequiredForBuy(t *testing.T) {
	p := validProposal()
	p.Side = "buy"
	p.OrderType = ""
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for missing order_type on buy")
	}
	if !strings.Contains(err.Error(), "order_type") {
		t.Errorf("expected error about order_type, got: %v", err)
	}
}

func TestValidate_OrderTypeRequiredForSell(t *testing.T) {
	p := validProposal()
	p.Side = "sell"
	p.OrderType = ""
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for missing order_type on sell")
	}
	if !strings.Contains(err.Error(), "order_type") {
		t.Errorf("expected error about order_type, got: %v", err)
	}
}

func TestValidate_OrderTypeNotRequiredForHold(t *testing.T) {
	p := validProposal()
	p.Side = "hold"
	p.OrderType = ""
	if err := p.Validate(); err != nil {
		t.Errorf("expected hold without order_type to pass, got: %v", err)
	}
}

func TestValidate_NegativeEstimatedNotional(t *testing.T) {
	p := validProposal()
	p.EstimatedNotional = -100
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for negative estimated_notional")
	}
	if !strings.Contains(err.Error(), "estimated_notional") {
		t.Errorf("expected error about estimated_notional, got: %v", err)
	}
}

func TestValidate_InvalidPortfolio(t *testing.T) {
	p := validProposal()
	p.Portfolio = &PortfolioInfo{
		CashAvailable:   -50,
		CurrentPosition: 0,
	}
	err := p.Validate()
	if err == nil {
		t.Fatal("expected error for negative cash_available")
	}
	if !strings.Contains(err.Error(), "cash_available") {
		t.Errorf("expected error about cash_available, got: %v", err)
	}
}

func TestValidate_HoldSide(t *testing.T) {
	p := validProposal()
	p.Side = "hold"
	p.OrderType = ""
	p.Quantity = 0
	if err := p.Validate(); err != nil {
		t.Errorf("expected valid hold proposal to pass, got: %v", err)
	}
}

// --- FromGateInput Tests ---

func TestFromGateInput_ValidPayload(t *testing.T) {
	input := gate.GateInput{
		Gate:   "trading",
		Domain: "trading",
		Action: "trade_proposal",
		Payload: map[string]any{
			"domain":             "trading",
			"action":             "trade_proposal",
			"mode":               "paper",
			"symbol":             "AAPL",
			"side":               "buy",
			"quantity":           10,
			"order_type":         "market",
			"estimated_price":    150.50,
			"estimated_notional": 1505.00,
			"strategy":           "momentum",
			"confidence":         0.85,
			"reason":             "strong upward trend",
		},
	}

	proposal, err := FromGateInput(input)
	if err != nil {
		t.Fatalf("expected valid extraction, got: %v", err)
	}

	if proposal.Symbol != "AAPL" {
		t.Errorf("expected AAPL, got %s", proposal.Symbol)
	}
	if proposal.Side != "buy" {
		t.Errorf("expected buy, got %s", proposal.Side)
	}
	if proposal.EstimatedNotional != 1505.00 {
		t.Errorf("expected 1505.00, got %.2f", proposal.EstimatedNotional)
	}
}

// --- Policy Evaluation Tests ---

func defaultEvaluator() *PolicyEvaluator {
	return NewPolicyEvaluator(DefaultConfig())
}

func TestPolicy_BlocklistedSymbolDenied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Symbols.Blocklist = []string{"GME", "AMC", "MULN"}
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Symbol = "GME"

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionDenied {
		t.Fatalf("expected denied, got %s", decision)
	}
	if len(checks) < 1 {
		t.Fatal("expected at least one check")
	}
	if !strings.Contains(checks[0].Details, "blocklisted") {
		t.Errorf("expected blocklist detail, got: %s", checks[0].Details)
	}
}

func TestPolicy_ConfidenceBelowThresholdDenied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.MinConfidence = 0.65
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Confidence = 0.30

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionDenied {
		t.Fatalf("expected denied, got %s", decision)
	}

	// Find the confidence check
	found := false
	for _, c := range checks {
		if c.Name == "min_confidence" && c.Status == "denied" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected confidence check to be denied")
	}
}

func TestPolicy_NotionalOverHardMaxDenied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.MaxTradeNotional = 500
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.EstimatedNotional = 600

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionDenied {
		t.Fatalf("expected denied, got %s", decision)
	}

	found := false
	for _, c := range checks {
		if c.Name == "max_trade_notional" && c.Status == "denied" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected max_trade_notional check to be denied")
	}
}

func TestPolicy_DailyLossExceededDenied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Limits.MaxDailyLoss = 100
	cfg.Limits.MaxTradeNotional = 10000 // raise so notional check doesn't fire first
	cfg.Approvals.RequireAboveNotional = 10000
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Portfolio = &PortfolioInfo{
		DailyRealizedPnL: -150,
	}

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionDenied {
		t.Fatalf("expected denied, got %s", decision)
	}

	found := false
	for _, c := range checks {
		if c.Name == "max_daily_loss" && c.Status == "denied" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected max_daily_loss check to be denied")
	}
}

func TestPolicy_LiveModeRequiresApproval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Approvals.RequireForLive = true
	cfg.Limits.MaxTradeNotional = 10000 // raise so notional check doesn't fire first
	cfg.Approvals.RequireAboveNotional = 10000
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Mode = "live"

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionRequiresApproval {
		t.Fatalf("expected requires_approval, got %s", decision)
	}

	found := false
	for _, c := range checks {
		if c.Name == "live_mode_approval" && c.Status == "requires_approval" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected live_mode_approval check to require approval")
	}
}

func TestPolicy_PaperModeUnderLimitsAllowed(t *testing.T) {
	cfg := DefaultConfig()
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Mode = "paper"
	proposal.EstimatedNotional = 200

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionAllowed {
		t.Fatalf("expected allowed, got %s", decision)
	}
	if len(checks) == 0 {
		t.Fatal("expected check results")
	}
}

func TestPolicy_AboveApprovalThresholdRequiresApproval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Approvals.RequireAboveNotional = 250
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Mode = "paper"
	proposal.EstimatedNotional = 300

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionRequiresApproval {
		t.Fatalf("expected requires_approval, got %s", decision)
	}

	found := false
	for _, c := range checks {
		if c.Name == "notional_approval_threshold" && c.Status == "requires_approval" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected notional_approval_threshold to require approval")
	}
}

func TestPolicy_SellWithRequireForShortRequiresApproval(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Approvals.RequireForShort = true
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Side = "sell"
	proposal.OrderType = "market"
	proposal.EstimatedNotional = 100 // under approval threshold

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionRequiresApproval {
		t.Fatalf("expected requires_approval for sell, got %s", decision)
	}

	found := false
	for _, c := range checks {
		if c.Name == "short_sell_approval" && c.Status == "requires_approval" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected short_sell_approval to require approval")
	}
}

func TestPolicy_SellWithRequireForShortDisabledAllowed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Approvals.RequireForShort = false
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Side = "sell"
	proposal.OrderType = "market"
	proposal.EstimatedNotional = 100

	checks, decision := pe.Evaluate(proposal)

	if decision != gate.DecisionAllowed {
		t.Fatalf("expected allowed when RequireForShort is false, got %s", decision)
	}
	_ = checks
}

func TestPolicy_AllowlistEnforcement(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Symbols.Allowlist = []string{"AAPL", "MSFT"}
	pe := NewPolicyEvaluator(cfg)

	// Allowed symbol
	proposal := validProposal()
	proposal.Symbol = "AAPL"
	proposal.EstimatedNotional = 100

	checks, decision := pe.Evaluate(proposal)
	if decision != gate.DecisionAllowed {
		t.Errorf("expected AAPL to be allowed, got %s", decision)
	}
	_ = checks

	// Not on allowlist
	proposal2 := validProposal()
	proposal2.Symbol = "GOOGL"
	proposal2.EstimatedNotional = 100

	checks2, decision2 := pe.Evaluate(proposal2)
	if decision2 != gate.DecisionDenied {
		t.Fatalf("expected GOOGL to be denied, got %s", decision2)
	}
	found := false
	for _, c := range checks2 {
		if c.Name == "symbol_allowlist" && c.Status == "denied" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected symbol_allowlist to be denied for GOOGL")
	}
}

func TestPolicy_EmptyAllowlistAllowsAll(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Symbols.Allowlist = nil
	pe := NewPolicyEvaluator(cfg)

	proposal := validProposal()
	proposal.Symbol = "ANYSTOCK"
	proposal.EstimatedNotional = 100

	_, decision := pe.Evaluate(proposal)
	if decision != gate.DecisionAllowed {
		t.Errorf("expected ANYSTOCK to be allowed with empty allowlist, got %s", decision)
	}
}

// --- Risk Score Tests ---

func TestCalculateRiskScore_PaperLowNotional(t *testing.T) {
	pe := defaultEvaluator()
	proposal := validProposal()
	proposal.Mode = "paper"
	proposal.EstimatedNotional = 50
	proposal.Confidence = 0.95

	score := pe.calculateRiskScore(proposal)
	if score < 0 || score > 1 {
		t.Errorf("risk score should be 0-1, got %.4f", score)
	}
	// Low notional + high confidence + paper => should be low risk
	if score > 0.3 {
		t.Errorf("expected low risk score, got %.4f", score)
	}
}

func TestCalculateRiskScore_LiveHighNotional(t *testing.T) {
	pe := defaultEvaluator()
	proposal := validProposal()
	proposal.Mode = "live"
	proposal.EstimatedNotional = 500
	proposal.Confidence = 0.5

	score := pe.calculateRiskScore(proposal)
	if score < 0 || score > 1 {
		t.Errorf("risk score should be 0-1, got %.4f", score)
	}
	// High notional + low confidence + live mode => should be high risk
	if score < 0.6 {
		t.Errorf("expected high risk score, got %.4f", score)
	}
}

func TestCalculateRiskScore_ZeroConfidence(t *testing.T) {
	pe := defaultEvaluator()
	proposal := validProposal()
	proposal.Mode = "live"
	proposal.EstimatedNotional = 250
	proposal.Confidence = 0.0

	score := pe.calculateRiskScore(proposal)
	// 0.0 confidence => 1.0 inverse * 0.35 = 0.35; live = +0.25; notional 250/500 * 0.4 = 0.2 => total 0.8
	if score < 0.7 || score > 1.0 {
		t.Errorf("expected high risk score for zero confidence, got %.4f", score)
	}
}

// --- Gate Integration Tests ---

func TestTradingGate_ImplementsGateInterface(t *testing.T) {
	cfg := DefaultConfig()
	g := NewTradingGate(cfg)

	var _ gate.Gate = g // compile-time check

	if g.Name() != "trading" {
		t.Errorf("expected Name() to return 'trading', got %q", g.Name())
	}
	if g.Domain() != "trading" {
		t.Errorf("expected Domain() to return 'trading', got %q", g.Domain())
	}
}

func TestTradingGate_EvaluateAllowed(t *testing.T) {
	cfg := DefaultConfig()
	g := NewTradingGate(cfg)

	input := gate.GateInput{
		Gate:   "trading",
		Domain: "trading",
		Action: "trade_proposal",
		Payload: map[string]any{
			"domain":             "trading",
			"action":             "trade_proposal",
			"mode":               "paper",
			"symbol":             "AAPL",
			"side":               "buy",
			"quantity":           10,
			"order_type":         "market",
			"estimated_price":    150.50,
			"estimated_notional": 200.00,
			"strategy":           "momentum",
			"confidence":         0.85,
			"reason":             "strong upward trend",
		},
	}

	result, err := g.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Decision != gate.DecisionAllowed {
		t.Errorf("expected allowed, got %s", result.Decision)
	}
	if result.RiskScore < 0 || result.RiskScore > 1 {
		t.Errorf("risk score should be 0-1, got %.4f", result.RiskScore)
	}
	if result.GateName != "trading" {
		t.Errorf("expected gate_name 'trading', got %q", result.GateName)
	}
	if result.Domain != "trading" {
		t.Errorf("expected domain 'trading', got %q", result.Domain)
	}
	if len(result.Checks) == 0 {
		t.Error("expected check results")
	}
	// Events should include Allowed
	foundAllowed := false
	for _, e := range result.Events {
		if e == EventTradeAllowed {
			foundAllowed = true
			break
		}
	}
	if !foundAllowed {
		t.Error("expected EventTradeAllowed in events")
	}
}

func TestTradingGate_EvaluateDenied(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Symbols.Blocklist = []string{"GME"}
	g := NewTradingGate(cfg)

	input := gate.GateInput{
		Gate:   "trading",
		Domain: "trading",
		Action: "trade_proposal",
		Payload: map[string]any{
			"domain":             "trading",
			"action":             "trade_proposal",
			"mode":               "paper",
			"symbol":             "GME",
			"side":               "buy",
			"quantity":           100,
			"order_type":         "market",
			"estimated_price":    25.00,
			"estimated_notional": 2500.00,
			"strategy":           "momentum",
			"confidence":         0.90,
			"reason":             "meme stock run",
		},
	}

	result, err := g.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Decision != gate.DecisionDenied {
		t.Errorf("expected denied, got %s", result.Decision)
	}
	if result.Reason == "" {
		t.Error("expected non-empty reason")
	}

	foundDenied := false
	for _, e := range result.Events {
		if e == EventTradeDenied {
			foundDenied = true
			break
		}
	}
	if !foundDenied {
		t.Error("expected EventTradeDenied in events")
	}
}

func TestTradingGate_EvaluateRequiresApproval(t *testing.T) {
	cfg := DefaultConfig()
	g := NewTradingGate(cfg)

	input := gate.GateInput{
		Gate:   "trading",
		Domain: "trading",
		Action: "trade_proposal",
		Payload: map[string]any{
			"domain":             "trading",
			"action":             "trade_proposal",
			"mode":               "live",
			"symbol":             "AAPL",
			"side":               "buy",
			"quantity":           10,
			"order_type":         "market",
			"estimated_price":    150.50,
			"estimated_notional": 200.00,
			"strategy":           "momentum",
			"confidence":         0.85,
			"reason":             "strong upward trend",
		},
	}

	result, err := g.Evaluate(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Decision != gate.DecisionRequiresApproval {
		t.Errorf("expected requires_approval, got %s", result.Decision)
	}
	if result.ApprovalID == "" {
		t.Error("expected non-empty approval ID")
	}
	if !strings.HasPrefix(result.ApprovalID, "appr_") {
		t.Errorf("expected approval ID to start with 'appr_', got %q", result.ApprovalID)
	}

	foundApproval := false
	for _, e := range result.Events {
		if e == EventTradeApprovalRequired {
			foundApproval = true
			break
		}
	}
	if !foundApproval {
		t.Error("expected EventTradeApprovalRequired in events")
	}
}

func TestTradingGate_ValidateInput_InvalidPayload(t *testing.T) {
	cfg := DefaultConfig()
	g := NewTradingGate(cfg)

	input := gate.GateInput{
		Gate:   "trading",
		Domain: "trading",
		Action: "trade_proposal",
		Payload: map[string]any{
			"domain": "trading",
			// missing required fields
		},
	}

	err := g.ValidateInput(input)
	if err == nil {
		t.Fatal("expected validation error for invalid input")
	}
}

func TestTradingGate_ValidateInput_ValidPayload(t *testing.T) {
	cfg := DefaultConfig()
	g := NewTradingGate(cfg)

	input := gate.GateInput{
		Gate:   "trading",
		Domain: "trading",
		Action: "trade_proposal",
		Payload: map[string]any{
			"domain":             "trading",
			"action":             "trade_proposal",
			"mode":               "paper",
			"symbol":             "AAPL",
			"side":               "buy",
			"quantity":           10,
			"order_type":         "market",
			"estimated_price":    150.50,
			"estimated_notional": 200.00,
			"strategy":           "momentum",
			"confidence":         0.85,
			"reason":             "strong upward trend",
		},
	}

	err := g.ValidateInput(input)
	if err != nil {
		t.Errorf("expected valid input to pass validation, got: %v", err)
	}
}

// --- Artifact Tests ---

func TestWriteTradeProposalArtifact(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "prism-trading-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	proposal := validProposal()
	if err := WriteTradeProposalArtifact(tmpDir, proposal); err != nil {
		t.Fatalf("failed to write artifact: %v", err)
	}

	artifactPath := filepath.Join(tmpDir, "trade_proposal.json")
	data, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatalf("failed to read artifact: %v", err)
	}

	var written TradeProposal
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatalf("failed to unmarshal artifact: %v", err)
	}

	if written.Symbol != "AAPL" {
		t.Errorf("expected AAPL, got %s", written.Symbol)
	}
	if written.Side != "buy" {
		t.Errorf("expected buy, got %s", written.Side)
	}
	if written.Confidence != 0.85 {
		t.Errorf("expected 0.85, got %.2f", written.Confidence)
	}
}

// --- Unused import guard ---
// This ensures the testing package and all imported packages are used.
var _ = context.Canceled
