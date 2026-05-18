package tool

import (
	"context"
	"fmt"

	"github.com/emaharmony/prism/internal/policy"
)

// Executor runs a tool through the full lifecycle: policy check → event
// emission → execution → event emission. It records every decision in the
// event log for auditability.
type Executor struct {
	Registry        *Registry
	Policy          PolicyConfig
	PolicyEvaluator PolicyEvaluatorFunc // V8: central policy engine (optional)
	// Emit is called to record events. If nil, events are silently dropped.
	Emit func(eventType, source string, payload map[string]any)
}

// PolicyEvaluatorFunc evaluates a V8 policy request and returns a decision.
// If nil, V8 policy evaluation is skipped (backward compatible).
type PolicyEvaluatorFunc func(action string, resource policy.Resource, context policy.Context) policy.PolicyDecision

// NewExecutor creates an executor with the given registry and policy config.
func NewExecutor(registry *Registry, policy PolicyConfig) *Executor {
	return &Executor{
		Registry: registry,
		Policy:  policy,
	}
}

// SetEmitter sets the event emission callback.
func (e *Executor) SetEmitter(emit func(eventType, source string, payload map[string]any)) {
	e.Emit = emit
}

// SetPolicyEvaluator sets the V8 central policy evaluator.
// When set, policy evaluation runs before local tool policy.
func (e *Executor) SetPolicyEvaluator(fn PolicyEvaluatorFunc) {
	e.PolicyEvaluator = fn
}

// ExecuteWithPolicy evaluates policy for a tool call, emits events for the
// decision, and executes the tool if approved.
//
// V8 adds a central policy evaluation BEFORE local tool policy.
// Policy decides permission; local validators still enforce input safety.
// If V8 policy denies, local policy is never reached.
// If V8 policy allows, local policy still enforces path safety, etc.
// If V8 policy is not configured, local policy runs as before (backward compatible).
func (e *Executor) ExecuteWithPolicy(ctx context.Context, toolName, agent, project, correlationID string, input map[string]any) (ToolResult, error) {
	// Emit tool.requested
	e.emitEvent("prism.tool.requested", map[string]any{
		"tool_name":      toolName,
		"agent":           agent,
		"project":         project,
		"correlation_id":  correlationID,
		"input":           input,
	})

	// V8: Evaluate central policy first (if configured)
	if e.PolicyEvaluator != nil {
		v8Decision := e.PolicyEvaluator(
			"tool.execute",
			policy.Resource{Type: "tool", Name: toolName},
			policy.Context{Project: project},
		)

		e.emitEvent("prism.policy.checked", map[string]any{
			"tool_name":       toolName,
			"policy_decision":  string(v8Decision.Decision),
			"policy_rule_id":   v8Decision.RuleID,
			"policy_reason":   v8Decision.Reason,
		})

		switch v8Decision.Decision {
		case policy.DecisionDenied:
			e.emitEvent("prism.tool.denied", map[string]any{
				"tool_name":       toolName,
				"agent":           agent,
				"project":         project,
				"correlation_id":  correlationID,
				"policy_decision":  string(v8Decision.Decision),
				"policy_reason":   v8Decision.Reason,
				"policy_source":   "v8",
			})
			return ToolResult{
				Success: false,
				Output:  nil,
				Error:   fmt.Sprintf("tool call denied by policy: %s", v8Decision.Reason),
			}, nil
		case policy.DecisionRequiresApproval:
			// V8 says requires_approval — fall through to local policy
			// which will handle the approval workflow
				default:
			// V8 says allowed — local policy still validates safety
		}
	}

	// Evaluate policy
	policyResult := EvaluatePolicy(e.Policy, toolName, input)

	// Handle requires_approval — this is not a denial, it's a request for human approval
	if policyResult.Decision == PolicyRequiresApproval {
		e.emitEvent("prism.tool.approved", map[string]any{
			"tool_name":       toolName,
			"agent":           agent,
			"project":         project,
			"correlation_id":  correlationID,
			"policy_decision": string(policyResult.Decision),
			"policy_reason":   policyResult.Reason,
		})

		// Execute the tool (which will return approval_id)
		result, err := e.Registry.Execute(ctx, toolName, input)
		if err != nil {
			return ToolResult{
				Success: false,
				Output:  nil,
				Error:   err.Error(),
			}, err
		}

		// Mark as pending_approval status
		result.Output["policy_decision"] = string(PolicyRequiresApproval)
		result.Output["policy_reason"] = policyResult.Reason

		return result, nil
	}

	// Emit denied event
	if policyResult.Decision == PolicyDenied {
		e.emitEvent("prism.tool.denied", map[string]any{
			"tool_name":       toolName,
			"agent":           agent,
			"project":         project,
			"correlation_id":  correlationID,
			"policy_decision": string(policyResult.Decision),
			"policy_reason":   policyResult.Reason,
		})
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   fmt.Sprintf("tool call denied: %s", policyResult.Reason),
		}, nil
	}

	e.emitEvent("prism.tool.approved", map[string]any{
		"tool_name":       toolName,
		"agent":           agent,
		"project":         project,
		"correlation_id":  correlationID,
		"policy_decision": string(policyResult.Decision),
		"policy_reason":   policyResult.Reason,
	})

	// Emit tool.started
	e.emitEvent("prism.tool.started", map[string]any{
		"tool_name":      toolName,
		"agent":          agent,
		"project":        project,
		"correlation_id": correlationID,
	})

	// Resolve and execute the tool
	result, err := e.Registry.Execute(ctx, toolName, input)
	if err != nil {
		e.emitEvent("prism.tool.failed", map[string]any{
			"tool_name":      toolName,
			"agent":          agent,
			"project":        project,
			"correlation_id": correlationID,
			"error":          err.Error(),
		})
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   err.Error(),
		}, err
	}

	// Emit tool.completed
	e.emitEvent("prism.tool.completed", map[string]any{
		"tool_name":      toolName,
		"agent":          agent,
		"project":        project,
		"correlation_id": correlationID,
		"success":        result.Success,
	})

	return result, nil
}

// emitEvent calls the configured emitter if it exists.
func (e *Executor) emitEvent(eventType string, payload map[string]any) {
	if e.Emit != nil {
		e.Emit(eventType, "prism-tool-executor", payload)
	}
}