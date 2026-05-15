package gate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteGateArtifacts persists the gate evaluation artifacts to the run directory.
// Creates: <runDir>/gate_result.json
func WriteGateArtifacts(runDir string, input GateInput, result GateResult) error {
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("failed to create run directory: %w", err)
	}

	artifact := struct {
		Input  GateInput  `json:"input"`
		Result GateResult `json:"result"`
	}{
		Input:  input,
		Result: result,
	}

	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal gate artifact: %w", err)
	}

	path := filepath.Join(runDir, "gate_result.json")
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write gate artifact: %w", err)
	}

	return nil
}
