package v2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SaveWorkflowState persists the full workflow state to disk as JSON.
func SaveWorkflowState(state *WorkflowState, dir string) error {
	if dir == "" {
		dir = "runs/natural-gates"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create state dir: %w", err)
	}

	state.mu.RLock()
	defer state.mu.RUnlock()

	path := filepath.Join(dir, fmt.Sprintf("workflow-%s.json", state.RunID))
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal state: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}

// LoadWorkflowState reconstructs workflow state from disk.
func LoadWorkflowState(path string) (*WorkflowState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read state file: %w", err)
	}
	var state WorkflowState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("unmarshal state: %w", err)
	}
	return &state, nil
}

// SaveCurrentWorkflowState saves state to a known "current" path for easy resumption.
func SaveCurrentWorkflowState(state *WorkflowState, dir string) error {
	if dir == "" {
		dir = "runs/natural-gates"
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	path := filepath.Join(dir, "current_workflow.json")
	state.mu.RLock()
	defer state.mu.RUnlock()
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadCurrentWorkflowState loads the current workflow state.
func LoadCurrentWorkflowState(dir string) (*WorkflowState, error) {
	if dir == "" {
		dir = "runs/natural-gates"
	}
	path := filepath.Join(dir, "current_workflow.json")
	return LoadWorkflowState(path)
}

// AutoSave periodically saves workflow state.
func AutoSave(state *WorkflowState, dir string, interval time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			if err := SaveCurrentWorkflowState(state, dir); err != nil {
				// Log error but continue
				continue
			}
		case <-stop:
			// Final save on stop
			SaveCurrentWorkflowState(state, dir)
			return
		}
	}
}

// jsonUnmarshal is a helper to avoid importing encoding/json in every file.
func jsonUnmarshal(data []byte, target interface{}) error {
	return json.Unmarshal(data, target)
}
