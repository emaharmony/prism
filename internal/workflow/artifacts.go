package workflow

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WriteWorkflowArtifacts persists workflow run artifacts to the run directory.
func WriteWorkflowArtifacts(runDir string, w *Workflow, state WorkflowState, result *RunResult) error {
	if runDir == "" {
		return nil
	}

	if err := os.MkdirAll(runDir, 0755); err != nil {
		return fmt.Errorf("workflow: failed to create run directory: %w", err)
	}

	// Write workflow state
	stateData, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("workflow: failed to marshal state: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "workflow_state.json"), stateData, 0644); err != nil {
		return fmt.Errorf("workflow: failed to write state: %w", err)
	}

	// Write workflow summary
	summary := buildWorkflowSummary(w, state, result)
	summaryData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("workflow: failed to marshal summary: %w", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, "workflow_summary.json"), summaryData, 0644); err != nil {
		return fmt.Errorf("workflow: failed to write summary: %w", err)
	}

	return nil
}

// AppendWorkflowEvents appends workflow events to the events log.
func AppendWorkflowEvents(runDir string, events []map[string]any) error {
	if runDir == "" || len(events) == 0 {
		return nil
	}

	eventsPath := filepath.Join(runDir, "events.jsonl")
	f, err := os.OpenFile(eventsPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("workflow: failed to open events file: %w", err)
	}
	defer f.Close()

	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		f.Write(data)
		f.Write([]byte("\n"))
	}

	return nil
}

// UpdateSummaryWithWorkflow adds workflow information to an existing run summary.
func UpdateSummaryWithWorkflow(runDir string, result *RunResult) error {
	if runDir == "" {
		return nil
	}

	summaryPath := filepath.Join(runDir, "summary.json")
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		// No existing summary, skip
		return nil
	}

	var summary map[string]any
	if err := json.Unmarshal(data, &summary); err != nil {
		return nil
	}

	summary["workflow"] = map[string]any{
		"name":    result.WorkflowName,
		"status":  result.Status,
		"run_id":  result.RunID,
	}

	updated, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return nil
	}

	return os.WriteFile(summaryPath, updated, 0644)
}

// buildWorkflowSummary creates a summary map from workflow state and result.
func buildWorkflowSummary(w *Workflow, state WorkflowState, result *RunResult) map[string]any {
	summary := map[string]any{
		"workflow":      w.Name,
		"version":      w.Version,
		"status":       state.Status,
		"run_id":       state.RunID,
		"correlation_id": state.CorrelationID,
		"step_count":   len(state.StepStates),
	}

	steps := make([]map[string]any, 0, len(state.StepStates))
	for _, s := range state.StepStates {
		step := map[string]any{
			"id":     s.ID,
			"type":   s.Type,
			"status": s.Status,
		}
		steps = append(steps, step)
	}
	summary["steps"] = steps

	return summary
}