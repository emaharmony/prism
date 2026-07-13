package workflow

import (
	"context"
	"fmt"
)

// ErrWorkflowStop is returned when a workflow.stop step halts the workflow.
// The output map contains the stop reason.
var ErrWorkflowStop = fmt.Errorf("workflow stopped")

// ErrWorkflowPaused is returned when a step requires approval that's not yet granted.
var ErrWorkflowPaused = fmt.Errorf("workflow paused")

// executeStep dispatches to the appropriate handler based on step type.
func (r *Runner) executeStep(ctx context.Context, step Step, input map[string]any, stepOutputs map[string]map[string]any, runID, correlationID string) (map[string]any, error) {
	switch step.Type {
	case StepTypeToolExecute:
		return r.executeToolStep(ctx, step, input)
	case StepTypeGateEvaluate:
		return r.executeGateStep(ctx, step)
	case StepTypeDispatchRun:
		return r.executeDispatchStep(ctx, step, stepOutputs, runID, correlationID)
	case StepTypeWorkflowStop:
		return r.executeStopStep(step)
	default:
		return nil, fmt.Errorf("unsupported step type %q", step.Type)
	}
}

// executeToolStep runs a tool through the tool executor function.
func (r *Runner) executeToolStep(ctx context.Context, step Step, input map[string]any) (map[string]any, error) {
	if r.handlers.ToolExecute == nil {
		return nil, fmt.Errorf("tool executor not configured")
	}

	// Merge step input with workflow input (step input takes precedence)
	toolInput := mergeInputs(input, step.Input)

	return r.handlers.ToolExecute(ctx, step.Tool, toolInput)
}

// executeGateStep evaluates a gate through the gate evaluator function.
func (r *Runner) executeGateStep(ctx context.Context, step Step) (map[string]any, error) {
	if r.handlers.GateEvaluate == nil {
		return nil, fmt.Errorf("gate evaluator not configured")
	}

	gateInput := step.Input
	if gateInput == nil {
		gateInput = map[string]any{
			"gate":   step.Gate,
			"domain": step.Gate,
			"action": "evaluate",
		}
	} else {
		// Ensure gate name is present
		if _, ok := gateInput["gate"]; !ok {
			gateInput["gate"] = step.Gate
		}
		if _, ok := gateInput["domain"]; !ok {
			gateInput["domain"] = step.Gate
		}
	}

	result, err := r.handlers.GateEvaluate(ctx, step.Gate, gateInput)
	if err != nil {
		return nil, err
	}

	// If gate requires approval, pause the workflow
	if decision, ok := result["decision"]; ok {
		if fmt.Sprintf("%v", decision) == "requires_approval" {
			return result, ErrWorkflowPaused
		}
	}

	return result, nil
}

// executeDispatchStep runs a dispatch through the dispatcher function.
func (r *Runner) executeDispatchStep(ctx context.Context, step Step, stepOutputs map[string]map[string]any, runID, correlationID string) (map[string]any, error) {
	if r.handlers.DispatchRun == nil {
		return nil, fmt.Errorf("dispatcher not configured")
	}

	// Determine gate decision from step outputs or step input
	dispatchInput := step.Input
	if dispatchInput == nil {
		dispatchInput = map[string]any{}
	}

	// Copy over gate decision from previous step outputs
	for _, stepOutput := range stepOutputs {
		if decision, ok := stepOutput["decision"]; ok {
			dispatchInput["gate_decision"] = decision
			break
		}
	}

	// Set adapter name, action, and run metadata
	dispatchInput["adapter"] = step.Adapter
	if step.Action != "" {
		dispatchInput["action"] = step.Action
	}
	dispatchInput["run_id"] = runID
	dispatchInput["correlation_id"] = correlationID

	return r.handlers.DispatchRun(ctx, step.Adapter, dispatchInput)
}

// executeStopStep halts the workflow.
func (r *Runner) executeStopStep(step Step) (map[string]any, error) {
	reason := "workflow stopped"
	if step.Input != nil {
		if r, ok := step.Input["reason"].(string); ok {
			reason = r
		}
	}

	return map[string]any{"reason": reason}, ErrWorkflowStop
}

// mergeInputs merges base input with step input. Step input takes precedence.
func mergeInputs(base, step map[string]any) map[string]any {
	if base == nil && step == nil {
		return nil
	}
	if base == nil {
		return step
	}
	if step == nil {
		return base
	}

	merged := make(map[string]any, len(base)+len(step))
	for k, v := range base {
		merged[k] = v
	}
	for k, v := range step {
		merged[k] = v
	}
	return merged
}
