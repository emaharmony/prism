package policy

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WritePolicyArtifact writes a policy decision artifact to the run directory.
func WritePolicyArtifact(runDir string, decision PolicyDecision) error {
	if err := os.MkdirAll(filepath.Join(runDir, "policy"), 0755); err != nil {
		return fmt.Errorf("create policy dir: %w", err)
	}

	path := filepath.Join(runDir, "policy", decision.EvaluationID+".json")
	data, err := json.MarshalIndent(decision, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal policy decision: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write policy artifact: %w", err)
	}

	return nil
}

// LoadPolicyArtifact reads a policy decision artifact from the run directory.
func LoadPolicyArtifact(runDir string, evaluationID string) (*PolicyDecision, error) {
	path := filepath.Join(runDir, "policy", evaluationID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy artifact: %w", err)
	}

	var decision PolicyDecision
	if err := json.Unmarshal(data, &decision); err != nil {
		return nil, fmt.Errorf("parse policy artifact: %w", err)
	}

	return &decision, nil
}
