// Package v2 implements the Natural Gates Workflow System.
// Each phase has a natural gate — a real condition that must be met before
// the phase exits, not just an iteration count. The system tracks assumptions
// and confidence as first-class state, supports multi-agent delegation through
// NATS, and has real feedback checkpoints that pause work until approval.
package v2

import (
	"context"
	"fmt"
	"log"
	"time"
)

// Engine is the core Natural Gates workflow engine.
// It runs a phase graph with cyclic transitions (e.g., FEEDBACK_POST → EXECUTION for fixes),
// stateful gates, external event waiting, and multi-agent delegation.
type Engine struct {
	phases        []Phase
	phaseMap      map[string]int
	gates         map[string]Gate
	state         *WorkflowState
	eventEmitter  EventEmitter
	delegation    *DelegationManager
	config        *WorkflowConfig
	externalEvent chan ExternalEvent // events from Discord/NATS during feedback gates
}

// Phase is the core abstraction. Each phase encapsulates its own gate logic,
// tool permissions, and interaction patterns.
type Phase interface {
	Name() string
	Description() string
	Enter(ctx context.Context, state *WorkflowState) error
	RunIteration(ctx context.Context, state *WorkflowState, llmResponse string) (PhaseAction, error)
	CheckGate(state *WorkflowState) GateResult
	Exit(ctx context.Context, state *WorkflowState) error
	AllowedTools() []string
	MaxIterations() int
}

// PhaseAction is what the engine should do after processing an LLM response.
type PhaseAction struct {
	Type         PhaseActionType
	ToolRequest  *ToolRequest
	FinalContent string
	Continue     bool
}

type PhaseActionType int

const (
	ActionContinue PhaseActionType = iota
	ActionToolCall
	ActionFinal
	ActionPhaseComplete
	ActionWaitExternal
)

// Gate is the enforcement mechanism. Gates are pluggable.
type Gate interface {
	Name() string
	Evaluate(state *WorkflowState) GateResult
}

// GateResult is the output of a gate evaluation.
type GateResult struct {
	Passed  bool
	Score   float64
	Reason  string
	Missing []string
}

// EventEmitter handles event emission for observability.
type EventEmitter interface {
	Emit(eventType string, payload map[string]any)
}

// ExternalEvent is an event from Discord or NATS during feedback gates.
type ExternalEvent struct {
	Type          string // "approval", "review", "task_complete", "agent_status"
	CorrelationID string
	Source        string // "discord", "nats"
	Data          map[string]any
}

// NewEngineWithState creates an engine with an existing workflow state (for resumption).
func NewEngineWithState(config *WorkflowConfig, state *WorkflowState, emitter EventEmitter, delegation *DelegationManager) *Engine {
	e := NewEngine(config, emitter, delegation)
	e.state = state
	// Set the current phase index based on the state's current phase
	for i, phase := range e.phases {
		if phase.Name() == state.CurrentPhase() {
			e.state.CurrentPhaseIdx = i
			break
		}
	}
	return e
}

// NewEngine creates a new Natural Gates workflow engine.
func NewEngine(config *WorkflowConfig, emitter EventEmitter, delegation *DelegationManager) *Engine {
	e := &Engine{
		phases:        make([]Phase, 0),
		phaseMap:      make(map[string]int),
		gates:         make(map[string]Gate),
		state:         NewWorkflowState(config),
		eventEmitter:  emitter,
		delegation:    delegation,
		config:        config,
		externalEvent: make(chan ExternalEvent, 100),
	}

	// Register phases from config
	for _, phaseCfg := range config.Phases {
		phase := NewPhaseFromConfig(phaseCfg)
		e.RegisterPhase(phase)
	}

	return e
}

// RegisterPhase adds a phase to the engine.
func (e *Engine) RegisterPhase(p Phase) {
	idx := len(e.phases)
	e.phases = append(e.phases, p)
	e.phaseMap[p.Name()] = idx
}

// RegisterGate associates a gate with a phase name.
func (e *Engine) RegisterGate(phaseName string, g Gate) {
	e.gates[phaseName] = g
}

// GetExternalEventChannel returns the channel for receiving external events.
func (e *Engine) GetExternalEventChannel() chan<- ExternalEvent {
	return e.externalEvent
}

// Run executes the workflow through all phases.
// It handles phase transitions, gate checks, external event waiting, and delegation.
func (e *Engine) Run(ctx context.Context) (*WorkflowState, error) {
	e.state.Status = StatusInProgress
	e.state.StartedAt = time.Now().UTC().Format(time.RFC3339)
	e.emitEvent("workflow.started", map[string]any{"workflow": e.config.Name})

	for e.state.CurrentPhaseIdx < len(e.phases) {
		phase := e.phases[e.state.CurrentPhaseIdx]
		phaseName := phase.Name()

		e.emitEvent("phase.entered", map[string]any{"phase": phaseName})
		e.state.SetPhaseStatus(phaseName, PhaseStatusInProgress)

		// Enter the phase
		if err := phase.Enter(ctx, e.state); err != nil {
			return e.state, fmt.Errorf("phase %s enter failed: %w", phaseName, err)
		}

		// Run phase iterations
		completed := false
		for iter := 0; iter < phase.MaxIterations(); iter++ {
			e.state.IncrementPhaseIteration(phaseName)

			// Check for external events (for feedback gates)
			select {
			case evt := <-e.externalEvent:
				e.handleExternalEvent(evt, phaseName)
			default:
			}

			// The actual LLM interaction happens in the caller (wake handler)
			// which calls Engine.ProcessLLMResponse. For now, we check the gate
			// after each iteration.
			gate, ok := e.gates[phaseName]
			if !ok {
				// No gate for this phase — check if phase signaled completion
				if ps, ok := e.state.PhaseStates[phaseName]; ok && ps.Status == PhaseStatusCompleted {
					completed = true
					break
				}
				continue
			}

			result := gate.Evaluate(e.state)
			e.emitEvent("phase.gate_check", map[string]any{
				"phase":   phaseName,
				"passed":  result.Passed,
				"score":   result.Score,
				"reason":  result.Reason,
			})

			if result.Passed {
				e.state.SetPhaseStatus(phaseName, PhaseStatusCompleted)
				e.state.SetPhaseGateResult(phaseName, result)
				completed = true
				break
			}

			// Gate not met — inject guidance for the LLM
			if result.Reason != "" {
				e.state.AddSystemMessage(phaseName, result.Reason)
			}
		}

		if !completed {
			// Phase didn't complete within its iteration budget
			e.emitEvent("phase.fallback", map[string]any{
				"phase":     phaseName,
				"reason":    "max_iterations_reached",
			})
			e.state.SetPhaseStatus(phaseName, PhaseStatusFallback)

			// Check if this is a blocking phase
			if phaseCfg := e.config.GetPhase(phaseName); phaseCfg != nil && phaseCfg.Fallback.Blocks {
				e.state.Status = StatusBlocked
				e.emitEvent("workflow.blocked", map[string]any{
					"phase":   phaseName,
					"reason":  "blocking phase fallback",
				})
				return e.state, fmt.Errorf("blocking phase %s failed", phaseName)
			}
		}

		// Exit the phase
		if err := phase.Exit(ctx, e.state); err != nil {
			log.Printf("[V2] phase %s exit error: %v", phaseName, err)
		}

		e.emitEvent("phase.exited", map[string]any{"phase": phaseName})

		// Check if workflow is paused (e.g., waiting for approval)
		if e.state.Status == StatusPaused {
			e.emitEvent("workflow.paused", map[string]any{
				"phase":  phaseName,
				"reason": e.state.PauseReason,
			})
			// Persist state and wait for external event
			e.WaitForResume(ctx)
		}

		// Check for phase cycling (e.g., FEEDBACK_POST → EXECUTION for fixes)
		if nextPhase := e.state.NextPhase; nextPhase != "" {
			idx, ok := e.phaseMap[nextPhase]
			if ok {
				e.state.CurrentPhaseIdx = idx
				e.state.NextPhase = ""
				continue
			}
		}

		e.state.CurrentPhaseIdx++
	}

	// All phases complete
	e.state.Status = StatusCompleted
	e.state.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	e.emitEvent("workflow.completed", map[string]any{
		"workflow": e.config.Name,
	})

	return e.state, nil
}

// WaitForResume blocks until an external event resumes the workflow.
// This is used during feedback gates that pause for approval.
func (e *Engine) WaitForResume(ctx context.Context) {
	for {
		select {
		case evt := <-e.externalEvent:
			if evt.Type == "approval" || evt.Type == "review" {
				e.handleExternalEvent(evt, e.state.CurrentPhase())
				if e.state.Status == StatusInProgress {
					return // resumed
				}
			}
		case <-ctx.Done():
			return // context cancelled
		case <-time.After(time.Hour * 8):
			// Max wait — mark as blocked
			e.state.Status = StatusBlocked
			e.state.PauseReason = "max wait time exceeded"
			return
		}
	}
}

// handleExternalEvent processes an external event (approval, review, task completion).
func (e *Engine) handleExternalEvent(evt ExternalEvent, phaseName string) {
	switch evt.Type {
	case "approval":
		decision, _ := evt.Data["decision"].(string)
		switch decision {
		case "approved":
			e.state.SetFeedbackPreStatus("approved", evt.Source)
			e.state.Status = StatusInProgress
			e.state.PauseReason = ""
		case "changes_requested":
			e.state.SetFeedbackPreStatus("changes_requested", evt.Source)
			e.state.NextPhase = "PLAN"
			e.state.Status = StatusInProgress
			e.state.PauseReason = ""
		case "rejected":
			e.state.SetFeedbackPreStatus("rejected", evt.Source)
			e.state.NextPhase = "PLAN"
			e.state.Status = StatusInProgress
			e.state.PauseReason = ""
		}
	case "review":
		decision, _ := evt.Data["decision"].(string)
		reviewer, _ := evt.Data["reviewer"].(string)
		switch decision {
		case "approved":
			e.state.SetReviewStatus(reviewer, "approved", evt.Data)
			if e.state.AllReviewersApproved() {
				e.state.Status = StatusInProgress
				e.state.PauseReason = ""
			}
		case "changes_requested":
			e.state.SetReviewStatus(reviewer, "changes_requested", evt.Data)
			e.state.NextPhase = "EXECUTION"
			e.state.Status = StatusInProgress
			e.state.PauseReason = ""
		}
	case "task_complete":
		taskID, _ := evt.Data["task_id"].(string)
		status, _ := evt.Data["status"].(string)
		e.state.UpdateTaskStatus(taskID, status, evt.Data)
	case "agent_status":
		agentName, _ := evt.Data["agent"].(string)
		availability, _ := evt.Data["availability"].(string)
		e.state.UpdateAgentAvailability(agentName, availability)
	}
}

// emitEvent sends an event through the event emitter.
func (e *Engine) emitEvent(eventType string, payload map[string]any) {
	if e.eventEmitter != nil {
		e.eventEmitter.Emit(eventType, payload)
	}
}

// ProcessLLMResponse processes an LLM response and returns the action to take.
// This is called by the wake handler after getting a response from the LLM.
func (e *Engine) ProcessLLMResponse(response string) (PhaseAction, error) {
	phase := e.phases[e.state.CurrentPhaseIdx]
	return phase.RunIteration(context.Background(), e.state, response)
}

// GetState returns the current workflow state.
func (e *Engine) GetState() *WorkflowState {
	return e.state
}