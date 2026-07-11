package workflow

// WorkflowState tracks the execution state of a workflow run.
type WorkflowState struct {
	WorkflowName  string      `json:"workflow_name"`
	Version       int         `json:"version"`
	Status        string      `json:"status"` // started, completed, failed, paused
	RunID         string      `json:"run_id"`
	CorrelationID string      `json:"correlation_id"`
	CurrentStep   *string     `json:"current_step"`
	StepStates    []StepState `json:"steps"`
}

// StepState tracks the execution state of a single workflow step.
type StepState struct {
	ID     string         `json:"id"`
	Type   string         `json:"type"`
	Status string         `json:"status"` // started, completed, failed, skipped
	Output map[string]any `json:"output,omitempty"`
}
