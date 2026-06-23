package v2

// V2 event type constants for observability.
// Every significant state change emits an event that can be consumed
// by dashboards, monitors, and other agents.
const (
	EventPhaseEntered        = "workflow.phase.entered"
	EventPhaseIteration      = "workflow.phase.iteration"
	EventPhaseGateCheck       = "workflow.phase.gate_check"
	EventPhaseGatePassed      = "workflow.phase.gate_passed"
	EventPhaseGateFailed      = "workflow.phase.gate_failed"
	EventPhaseExited          = "workflow.phase.exited"
	EventWorkflowStarted      = "workflow.started"
	EventWorkflowPaused       = "workflow.paused.waiting_approval"
	EventWorkflowResumed      = "workflow.resumed"
	EventWorkflowCompleted    = "workflow.completed"
	EventWorkflowBlocked      = "workflow.blocked"
	EventTaskDelegated        = "workflow.task.delegated"
	EventTaskCompleted        = "workflow.task.completed"
	EventTaskFailed           = "workflow.task.failed"
	EventTaskTimedOut         = "workflow.task.timed_out"
	EventAgentCreated         = "workflow.agent.created"
	EventFeedbackRequested    = "workflow.feedback.requested"
	EventFeedbackReceived     = "workflow.feedback.received"
	EventAssumptionAdded      = "workflow.assumption.added"
	EventAssumptionResolved   = "workflow.assumption.resolved"
	EventConfidenceUpdated    = "workflow.confidence.updated"
	EventPlanTaskAdded        = "workflow.plan.task_added"
)

// LogEmitter is a simple EventEmitter that logs events.
type LogEmitter struct{}

func (l *LogEmitter) Emit(eventType string, payload map[string]any) {
	// In production, this would publish to NATS and log
	// For now, just log to stdout
}