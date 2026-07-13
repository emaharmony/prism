package policy

// V8 Policy Event Types
//
// Policy events are emitted during evaluation and persist to events.jsonl.

const (
	EventTypePolicyRequested        = "prism.policy.requested"
	EventTypePolicyEvaluated        = "prism.policy.evaluated"
	EventTypePolicyAllowed          = "prism.policy.allowed"
	EventTypePolicyDenied           = "prism.policy.denied"
	EventTypePolicyApprovalRequired = "prism.policy.approval_required"
	EventTypePolicyFailed           = "prism.policy.failed"
)
