package trading

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/emaharmony/prism/internal/gate"
)

// TradeProposal represents a trade proposal from the stock AI application.
type TradeProposal struct {
	Domain           string         `json:"domain"`            // always "trading"
	Action           string         `json:"action"`            // always "trade_proposal"
	Mode             string         `json:"mode"`              // "paper" or "live"
	Symbol           string         `json:"symbol"`            // e.g., "AAPL"
	Side             string         `json:"side"`              // "buy", "sell", or "hold"
	Quantity         int            `json:"quantity"`           // non-negative
	OrderType        string         `json:"order_type"`         // required for buy/sell
	EstimatedPrice   float64        `json:"estimated_price"`
	EstimatedNotional float64       `json:"estimated_notional"` // non-negative
	Strategy         string         `json:"strategy"`
	Confidence       float64        `json:"confidence"`         // 0.0–1.0
	Reason           string         `json:"reason"`
	Portfolio        *PortfolioInfo `json:"portfolio,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// PortfolioInfo holds the current portfolio state at the time of proposal.
type PortfolioInfo struct {
	CashAvailable    float64 `json:"cash_available"`
	CurrentPosition  int     `json:"current_position"`
	DailyRealizedPnL float64 `json:"daily_realized_pnl"`
}

// Validate performs validation on the trade proposal.
// Returns a descriptive error for each validation failure.
func (p *TradeProposal) Validate() error {
	if p.Symbol == "" {
		return errors.New("symbol is required")
	}

	validSides := map[string]bool{"buy": true, "sell": true, "hold": true}
	if !validSides[p.Side] {
		return fmt.Errorf("side must be buy, sell, or hold, got %q", p.Side)
	}

	if p.Quantity < 0 {
		return fmt.Errorf("quantity must be non-negative, got %d", p.Quantity)
	}

	if p.Side == "buy" || p.Side == "sell" {
		if p.OrderType == "" {
			return fmt.Errorf("order_type is required for %s orders", p.Side)
		}
	}

	if p.EstimatedNotional < 0 {
		return fmt.Errorf("estimated_notional must be non-negative, got %.2f", p.EstimatedNotional)
	}

	if p.Confidence < 0 || p.Confidence > 1 {
		return fmt.Errorf("confidence must be between 0.0 and 1.0, got %.2f", p.Confidence)
	}

	if p.Mode != "paper" && p.Mode != "live" {
		return fmt.Errorf("mode must be paper or live, got %q", p.Mode)
	}

	// Validate portfolio if provided
	if p.Portfolio != nil {
		if p.Portfolio.CashAvailable < 0 {
			return fmt.Errorf("portfolio cash_available must be non-negative, got %.2f", p.Portfolio.CashAvailable)
		}
		if p.Portfolio.CurrentPosition < 0 {
			return fmt.Errorf("portfolio current_position must be non-negative, got %d", p.Portfolio.CurrentPosition)
		}
	}

	return nil
}

// FromGateInput extracts a TradeProposal from a generic GateInput payload.
func FromGateInput(input gate.GateInput) (*TradeProposal, error) {
	data, err := json.Marshal(input.Payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal gate input payload: %w", err)
	}

	var proposal TradeProposal
	if err := json.Unmarshal(data, &proposal); err != nil {
		return nil, fmt.Errorf("failed to unmarshal trade proposal: %w", err)
	}

	return &proposal, nil
}
