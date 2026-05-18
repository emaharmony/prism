package adapter

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteAdapterArtifact persists an adapter result to the run directory.
func WriteAdapterArtifact(runDir string, adapterName, action string, result *Result) error {
	dir := filepath.Join(runDir, "adapter")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create adapter artifact dir: %w", err)
	}

	filename := fmt.Sprintf("%s_%s.json", adapterName, action)
	path := filepath.Join(dir, filename)

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal adapter result: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write adapter artifact: %w", err)
	}

	return nil
}