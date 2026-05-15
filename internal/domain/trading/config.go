package trading

// Config holds the deterministic trading policy rules.
type Config struct {
	Symbols   SymbolConfig   `json:"symbols" yaml:"symbols"`
	Limits    LimitConfig    `json:"limits" yaml:"limits"`
	Approvals ApprovalConfig `json:"approvals" yaml:"approvals"`
}

// SymbolConfig controls which symbols are allowed or blocked.
type SymbolConfig struct {
	Allowlist []string `json:"allowlist" yaml:"allowlist"` // empty = allow all
	Blocklist []string `json:"blocklist" yaml:"blocklist"`
}

// LimitConfig defines hard limits for trade proposals.
type LimitConfig struct {
	MaxTradeNotional   float64 `json:"max_trade_notional" yaml:"max_trade_notional"`     // default 500
	MaxPositionNotional float64 `json:"max_position_notional" yaml:"max_position_notional"` // default 2000
	MaxDailyLoss       float64 `json:"max_daily_loss" yaml:"max_daily_loss"`            // default 100
	MinConfidence      float64 `json:"min_confidence" yaml:"min_confidence"`          // default 0.65
}

// ApprovalConfig controls when human approval is required.
type ApprovalConfig struct {
	RequireForLive      bool    `json:"require_for_live" yaml:"require_for_live"`           // default true
	RequireAboveNotional float64 `json:"require_above_notional" yaml:"require_above_notional"` // default 250
	RequireForShort     bool    `json:"require_for_short" yaml:"require_for_short"`        // default true
}

// DefaultConfig returns the default trading policy configuration.
func DefaultConfig() *Config {
	return &Config{
		Symbols: SymbolConfig{
			Allowlist: nil, // empty = allow all
			Blocklist: nil,
		},
		Limits: LimitConfig{
			MaxTradeNotional:  500,
			MaxPositionNotional: 2000,
			MaxDailyLoss:      100,
			MinConfidence:     0.65,
		},
		Approvals: ApprovalConfig{
			RequireForLive:      true,
			RequireAboveNotional: 250,
			RequireForShort:     true,
		},
	}
}
