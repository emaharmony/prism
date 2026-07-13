package policy

// Subject identifies who is requesting the action.
type Subject struct {
	Type string `json:"type" yaml:"type"` // e.g., "agent", "user", "system"
	ID   string `json:"id" yaml:"id"`     // e.g., "lumi", "ema"
}

// Resource identifies what the action targets.
type Resource struct {
	Type string `json:"type" yaml:"type"` // e.g., "tool", "file", "gate"
	Name string `json:"name" yaml:"name"` // e.g., "read_file", "trading_gate"
}

// Context provides additional context for the policy evaluation.
type Context struct {
	Project string `json:"project,omitempty" yaml:"project,omitempty"`
	Path    string `json:"path,omitempty" yaml:"path,omitempty"`
	Mode    string `json:"mode,omitempty" yaml:"mode,omitempty"` // e.g., "local", "live"
}

// PolicyRequest is the input to the policy evaluator.
type PolicyRequest struct {
	Subject  Subject  `json:"subject" yaml:"subject"`
	Action   string   `json:"action" yaml:"action"` // e.g., "tool.execute", "mutation.apply"
	Resource Resource `json:"resource" yaml:"resource"`
	Context  Context  `json:"context" yaml:"context"`
}
