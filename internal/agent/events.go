// V13 agent event types.
//
// These events track the lifecycle of agent delegation within a run.
// Every delegation (workflow step → agent) emits these events, making
// multi-agent orchestration fully observable and auditable.
//
// Event flow for a delegation:
//  1. agent.delegated  — workflow delegates a subtask to an agent
//  2. (agent executes, may emit tool events via V3)
//  3. agent.completed — agent finishes the subtask successfully
//     OR
//  3. agent.failed     — agent fails the subtask
//
// agent.registered is emitted when an agent is added to the registry
// at the start of a run.
package agent

import "github.com/emaharmony/prizm/internal/event"

// V13 agent event type constants.
const (
	// EventAgentRegistered is emitted when an agent is added to the registry.
	// Payload: agent_name, role, version, capabilities
	EventAgentRegistered = "agent.registered"

	// EventAgentDelegated is emitted when a subtask is delegated to an agent.
	// Payload: agent_name, delegated_by, subtask, step_id
	EventAgentDelegated = "agent.delegated"

	// EventAgentCompleted is emitted when an agent finishes a subtask.
	// Payload: agent_name, subtask, output (summary), duration_ms
	EventAgentCompleted = "agent.completed"

	// EventAgentFailed is emitted when an agent fails a subtask.
	// Payload: agent_name, subtask, error, duration_ms
	EventAgentFailed = "agent.failed"
)

// NewAgentRegisteredEvent creates an event for when an agent is registered.
func NewAgentRegisteredEvent(agentName, role, version string, capabilities []string) event.Event {
	return event.NewEvent(EventAgentRegistered, "agent-registry", map[string]any{
		"agent_name":   agentName,
		"role":         role,
		"version":      version,
		"capabilities": capabilities,
	})
}

// NewAgentDelegatedEvent creates an event for when a subtask is delegated.
func NewAgentDelegatedEvent(agentName, delegatedBy, subtask, stepID string) event.Event {
	return event.NewEvent(EventAgentDelegated, "workflow-runner", map[string]any{
		"agent_name":   agentName,
		"delegated_by": delegatedBy,
		"subtask":      subtask,
		"step_id":      stepID,
	})
}

// NewAgentCompletedEvent creates an event for when an agent completes a subtask.
func NewAgentCompletedEvent(agentName, subtask, output string, durationMs int64) event.Event {
	return event.NewEvent(EventAgentCompleted, "agent-"+agentName, map[string]any{
		"agent_name":  agentName,
		"subtask":     subtask,
		"output":      output,
		"duration_ms": durationMs,
	})
}

// NewAgentFailedEvent creates an event for when an agent fails a subtask.
func NewAgentFailedEvent(agentName, subtask, agentErr string, durationMs int64) event.Event {
	return event.NewEvent(EventAgentFailed, "agent-"+agentName, map[string]any{
		"agent_name":  agentName,
		"subtask":     subtask,
		"error":       agentErr,
		"duration_ms": durationMs,
	})
}
