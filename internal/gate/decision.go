package gate

// Decision is the result of a gate evaluation.
type Decision string

const (
	DecisionAllowed          Decision = "allowed"
	DecisionDenied           Decision = "denied"
	DecisionRequiresApproval Decision = "requires_approval"
)

// CheckResult holds the result of a single policy check.
type CheckResult struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "passed", "denied", "requires_approval"
	Details string `json:"details"`
}

// GateResult is the output of a gate evaluation.
type GateResult struct {
	Decision   Decision      `json:"decision"`
	Reason     string        `json:"reason"`
	RiskScore  float64       `json:"risk_score"`
	ApprovalID string        `json:"approval_id,omitempty"`
	Checks     []CheckResult `json:"checks"`
	Events     []string      `json:"events"` // event IDs emitted during evaluation
	GateName   string        `json:"gate_name"`
	Domain     string        `json:"domain"`
	Action     string        `json:"action"`
}
