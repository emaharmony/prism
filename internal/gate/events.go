package gate

// Gate event type constants.
// These are emitted by the Evaluator during gate evaluation lifecycle.
const (
	EventGateRequested       = "prism.gate.requested"
	EventGateEvaluated       = "prism.gate.evaluated"
	EventGateAllowed         = "prism.gate.allowed"
	EventGateDenied          = "prism.gate.denied"
	EventGateApprovalRequired = "prism.gate.approval_required"
	EventGateFailed          = "prism.gate.failed"
)
