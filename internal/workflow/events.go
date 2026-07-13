package workflow

// V7EventTypes defines the event types emitted by the workflow runtime.
var V7EventTypes = struct {
	WorkflowStarted       string
	WorkflowCompleted     string
	WorkflowFailed        string
	WorkflowPaused        string
	WorkflowResumed       string
	WorkflowStepStarted   string
	WorkflowStepCompleted string
	WorkflowStepFailed    string
	WorkflowStepSkipped   string
}{
	WorkflowStarted:       "prism.workflow.started",
	WorkflowCompleted:     "prism.workflow.completed",
	WorkflowFailed:        "prism.workflow.failed",
	WorkflowPaused:        "prism.workflow.paused",
	WorkflowResumed:       "prism.workflow.resumed",
	WorkflowStepStarted:   "prism.workflow.step.started",
	WorkflowStepCompleted: "prism.workflow.step.completed",
	WorkflowStepFailed:    "prism.workflow.step.failed",
	WorkflowStepSkipped:   "prism.workflow.step.skipped",
}
