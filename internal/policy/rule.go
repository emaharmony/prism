package policy

// MatchSpec defines when a policy rule applies.
// Fields use exact match. Missing fields are wildcards (match anything).
type MatchSpec struct {
	Action       string `json:"action" yaml:"action"`
	ResourceType string `json:"resource.type,omitempty" yaml:"resource.type,omitempty"`
	ResourceName string `json:"resource.name,omitempty" yaml:"resource.name,omitempty"`
	ContextMode  string `json:"context.mode,omitempty" yaml:"context.mode,omitempty"`
}

// Matches returns true if the request matches this spec.
// Missing fields in the spec act as wildcards.
func (m MatchSpec) Matches(req PolicyRequest) bool {
	if m.Action != "" && m.Action != req.Action {
		return false
	}
	if m.ResourceType != "" && m.ResourceType != req.Resource.Type {
		return false
	}
	if m.ResourceName != "" && m.ResourceName != req.Resource.Name {
		return false
	}
	if m.ContextMode != "" && m.ContextMode != req.Context.Mode {
		return false
	}
	return true
}

// PolicyRule is a declarative policy rule.
type PolicyRule struct {
	ID          string    `json:"id" yaml:"id"`
	Description string    `json:"description" yaml:"description"`
	Match       MatchSpec `json:"match" yaml:"match"`
	Decision    Decision  `json:"decision" yaml:"decision"`
	Reason      string    `json:"reason" yaml:"reason"`
	Severity    Severity  `json:"severity,omitempty" yaml:"severity,omitempty"`
}
