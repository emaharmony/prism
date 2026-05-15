package trading

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteTradeProposalArtifact persists the trade proposal to the run directory.
// Creates: <runDir>/trade_proposal.json
func WriteTradeProposalArtifact(runDir string, proposal *TradeProposal) error {
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	data, err := json.MarshalIndent(proposal, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal trade proposal: %w", err)
	}

	path := filepath.Join(runDir, "trade_proposal.json")
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write trade proposal artifact: %w", err)
	}

	return nil
}
