package v2

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// driver.go wires the Natural Gates phase/gate abstractions to a live LLM and
// tool executor. Engine.Run() deliberately contains no LLM calls (it expects an
// external caller to drive ProcessLLMResponse); Drive() is that caller, made
// reusable so any trigger (scheduled wake, interactive prompt, API) shares one
// implementation instead of re-coding the loop.

// Message is a single chat message exchanged with the model.
type Message struct {
	Role    string // system | user | assistant
	Content string
}

// LLMFunc calls the model with the running transcript and returns the response
// text plus token usage. The caller owns provider/model selection.
type LLMFunc func(ctx context.Context, messages []Message) (text string, promptTokens, completionTokens int, err error)

// ToolFunc executes a tool request for the given phase and returns a result
// string to feed back to the model. A non-nil error means the tool failed.
type ToolFunc func(ctx context.Context, phase string, req *ToolRequest) (result string, err error)

// DriveOptions configure a single Drive run.
type DriveOptions struct {
	SystemPrompt string
	UserPrompt   string
	StateDir     string        // where to persist workflow state (autosave + pauses)
	RepoPath     string        // repo path, surfaced to reviewers in feedback events
	GetBranch    func() string // current git branch for EXECUTION branch protection; nil disables
}

// codeMutationTools are blocked while on a protected branch during EXECUTION.
var codeMutationTools = map[string]bool{
	"write_file": true, "git_add": true, "git_commit": true, "git_push": true,
}

// Drive runs the workflow end-to-end against a live LLM and tool executor.
// It honours phase gates, feedback pauses, iterate/loop-back via NextPhase, and
// EXECUTION safety enforcement (branch protection, read budget, commit/push).
func (e *Engine) Drive(ctx context.Context, llm LLMFunc, tool ToolFunc, opts DriveOptions) (*WorkflowState, error) {
	e.state.Status = StatusInProgress
	if e.state.StartedAt == "" {
		e.state.StartedAt = time.Now().UTC().Format(time.RFC3339)
	}
	e.emitEvent("workflow.started", map[string]any{"workflow": e.config.Name})

	messages := []Message{
		{Role: "system", Content: opts.SystemPrompt},
		{Role: "user", Content: opts.UserPrompt},
	}

	maxTotal := e.config.Global.MaxTotalIterations
	if maxTotal <= 0 {
		maxTotal = 60
	}
	totalIter := 0

	// EXECUTION enforcement state.
	var hasWritten, hasCommitted, hasPushed bool
	readsInPhase := 0

	for e.state.CurrentPhaseIdx >= 0 && e.state.CurrentPhaseIdx < len(e.phases) {
		phase := e.phases[e.state.CurrentPhaseIdx]
		phaseName := phase.Name()

		e.emitEvent("phase.entered", map[string]any{"phase": phaseName})
		e.state.SetPhaseStatus(phaseName, PhaseStatusInProgress)
		if err := phase.Enter(ctx, e.state); err != nil {
			return e.state, fmt.Errorf("phase %s enter: %w", phaseName, err)
		}
		readsInPhase = 0
		if phaseName == "EXECUTION" {
			// Re-entry (after changes requested) must re-commit/re-push fresh work.
			hasCommitted, hasPushed = false, false
		}

		// Feedback phases pause for human/sub-agent input instead of calling the LLM.
		if e.state.Status == StatusPaused {
			pausePayload := map[string]any{
				"phase":     phaseName,
				"reason":    e.state.PauseReason,
				"run_id":    e.state.RunID,
				"repo_path": opts.RepoPath,
			}
			if cfg := e.config.GetPhase(phaseName); cfg != nil {
				if len(cfg.Gate.Approvers) > 0 {
					pausePayload["approvers"] = cfg.Gate.Approvers
				}
				if len(cfg.Gate.RequiredReviewers) > 0 {
					pausePayload["required_reviewers"] = cfg.Gate.RequiredReviewers
				}
			}
			// Include the formatted plan/review package so a sub-agent reviewer
			// has the full context without re-deriving it.
			switch phaseName {
			case "FEEDBACK_PRE":
				pausePayload["package"] = FormatPlanForApproval(e.state)
			case "FEEDBACK_POST":
				pausePayload["package"] = FormatReviewPackage(e.state, "")
			}
			e.emitEvent("workflow.paused", pausePayload)
			// A separate feedback.requested event lets Discord/dashboard/sub-agent
			// reviewers know to act and reply on prism.workflow.feedback.response.
			e.emitEvent("feedback.requested", pausePayload)
			if opts.StateDir != "" {
				_ = SaveCurrentWorkflowState(e.state, opts.StateDir)
			}
			e.WaitForResume(ctx)
			if e.state.Status == StatusBlocked {
				return e.state, fmt.Errorf("workflow blocked at %s: %s", phaseName, e.state.PauseReason)
			}
			e.recordGate(phaseName)
			if err := phase.Exit(ctx, e.state); err != nil {
				log.Printf("[V2] phase %s exit: %v", phaseName, err)
			}
			e.emitEvent("phase.exited", map[string]any{"phase": phaseName})
			if e.advance(phaseName) {
				continue
			}
			break
		}

		messages = append(messages, Message{Role: "system", Content: e.phaseGuidance(phase)})

		completed := false
		for iter := 0; iter < phase.MaxIterations() && totalIter < maxTotal; iter++ {
			totalIter++
			e.state.IncrementPhaseIteration(phaseName)

			// Drain any external events (e.g. agent status) without blocking.
			select {
			case evt := <-e.externalEvent:
				e.handleExternalEvent(evt, phaseName)
			default:
			}

			// Inject pending gate guidance.
			for _, msg := range e.state.GetSystemMessages(phaseName) {
				messages = append(messages, Message{Role: "system", Content: msg})
			}

			text, pt, ct, err := llm(ctx, messages)
			e.state.AddTokens(pt, ct)
			if err != nil {
				return e.state, fmt.Errorf("llm call in %s: %w", phaseName, err)
			}
			messages = append(messages, Message{Role: "assistant", Content: text})

			action, _ := phase.RunIteration(ctx, e.state, text)

			switch action.Type {
			case ActionToolCall:
				req := action.ToolRequest
				if deny := e.enforceExecution(phaseName, req, opts, &hasWritten, &readsInPhase); deny != "" {
					messages = append(messages, Message{Role: "user", Content: deny})
					continue
				}
				result, terr := tool(ctx, phaseName, req)
				switch req.Tool {
				case "write_file":
					if terr == nil {
						hasWritten = true
					}
				case "git_commit":
					if terr == nil {
						hasCommitted = true
					}
				case "git_push":
					if terr == nil {
						hasPushed = true
					}
				}
				if terr != nil {
					messages = append(messages, Message{Role: "user", Content: fmt.Sprintf("Tool %q failed: %v. Try a different approach.", req.Tool, terr)})
					continue
				}
				messages = append(messages, Message{Role: "user", Content: truncate(fmt.Sprintf("Tool %q result:\n%s", req.Tool, result), 3000)})

			case ActionFinal, ActionPhaseComplete:
				if reject := e.enforceCommitPush(phaseName, hasWritten, hasCommitted, hasPushed); reject != "" {
					messages = append(messages, Message{Role: "system", Content: reject})
					iter-- // don't burn the budget on an enforcement bounce
					continue
				}
				if e.gatePasses(phaseName) {
					completed = true
				} else {
					e.injectGateReason(phaseName, &messages)
				}
				if completed {
					break
				}

			case ActionContinue:
				// Opportunistic gate check — PROBE/RESEARCH/PLAN parse state each turn.
				if e.gatePasses(phaseName) {
					completed = true
					break
				}
			}
		}

		if !completed {
			if e.gatePasses(phaseName) {
				completed = true
			} else {
				e.emitEvent("phase.fallback", map[string]any{"phase": phaseName, "reason": "max_iterations_reached"})
				e.state.SetPhaseStatus(phaseName, PhaseStatusFallback)
				if cfg := e.config.GetPhase(phaseName); cfg != nil && cfg.Fallback.Blocks {
					e.state.Status = StatusBlocked
					e.emitEvent("workflow.blocked", map[string]any{"phase": phaseName})
					return e.state, fmt.Errorf("blocking phase %s did not pass its gate", phaseName)
				}
			}
		}

		if err := phase.Exit(ctx, e.state); err != nil {
			log.Printf("[V2] phase %s exit: %v", phaseName, err)
		}
		e.emitEvent("phase.exited", map[string]any{"phase": phaseName})

		if !e.advance(phaseName) {
			break
		}
		if totalIter >= maxTotal {
			log.Printf("[V2] total iteration budget (%d) exhausted", maxTotal)
			break
		}
	}

	e.state.Status = StatusCompleted
	e.state.CompletedAt = time.Now().UTC().Format(time.RFC3339)
	e.emitEvent("workflow.completed", map[string]any{"workflow": e.config.Name})
	if opts.StateDir != "" {
		_ = SaveCurrentWorkflowState(e.state, opts.StateDir)
	}
	return e.state, nil
}

// advance moves to the next phase, honouring NextPhase loop-back (iterate).
// Returns false when there is no further phase to run.
func (e *Engine) advance(fromPhase string) bool {
	if next := e.state.NextPhase; next != "" {
		if idx, ok := e.phaseMap[next]; ok {
			e.state.NextPhase = ""
			e.emitEvent("workflow.iteration", map[string]any{"from": fromPhase, "to": next})
			e.state.CurrentPhaseIdx = idx
			return true
		}
		e.state.NextPhase = ""
	}
	e.state.CurrentPhaseIdx++
	return e.state.CurrentPhaseIdx < len(e.phases)
}

// gatePasses evaluates the registered gate for a phase and records the result.
func (e *Engine) gatePasses(phaseName string) bool {
	gate, ok := e.gates[phaseName]
	if !ok {
		return true // no gate → phase completes on its own signal
	}
	result := gate.Evaluate(e.state)
	e.emitEvent("phase.gate_check", map[string]any{
		"phase": phaseName, "passed": result.Passed, "score": result.Score, "reason": result.Reason,
	})
	if result.Passed {
		e.state.SetPhaseStatus(phaseName, PhaseStatusCompleted)
		e.state.SetPhaseGateResult(phaseName, result)
	}
	return result.Passed
}

// recordGate evaluates and records a gate result without changing phase status
// (used after a feedback pause resumes).
func (e *Engine) recordGate(phaseName string) {
	if gate, ok := e.gates[phaseName]; ok {
		result := gate.Evaluate(e.state)
		e.state.SetPhaseGateResult(phaseName, result)
		e.emitEvent("phase.gate_check", map[string]any{
			"phase": phaseName, "passed": result.Passed, "score": result.Score, "reason": result.Reason,
		})
	}
}

// injectGateReason queues the gate's reason as guidance for the next iteration.
func (e *Engine) injectGateReason(phaseName string, messages *[]Message) {
	if gate, ok := e.gates[phaseName]; ok {
		if result := gate.Evaluate(e.state); result.Reason != "" {
			*messages = append(*messages, Message{Role: "system", Content: "GATE NOT MET: " + result.Reason})
		}
	}
}

// enforceExecution applies EXECUTION-phase safety (branch protection + read budget).
// Returns a non-empty denial message when the tool call must be blocked.
func (e *Engine) enforceExecution(phaseName string, req *ToolRequest, opts DriveOptions, hasWritten *bool, readsInPhase *int) string {
	if phaseName != "EXECUTION" {
		return ""
	}
	if codeMutationTools[req.Tool] && opts.GetBranch != nil {
		branch := opts.GetBranch()
		if branch == "main" || branch == "master" {
			return fmt.Sprintf("BRANCH PROTECTION: you are on %q. Create a feature branch (git_checkout create=true) before writing or committing.", branch)
		}
	}
	if req.Tool == "read_file" || req.Tool == "list_dir" || req.Tool == "search_files" {
		*readsInPhase++
		if *readsInPhase > 3 && !*hasWritten {
			return "READ BUDGET EXCEEDED: you have read enough. Use write_file to make changes now."
		}
	}
	return ""
}

// enforceCommitPush rejects a premature final/complete in EXECUTION when work
// has been written but not committed and pushed.
func (e *Engine) enforceCommitPush(phaseName string, hasWritten, hasCommitted, hasPushed bool) string {
	if phaseName != "EXECUTION" {
		return ""
	}
	if hasWritten && !hasCommitted {
		return "COMMIT REQUIRED: you wrote files but have not committed. Use git_add then git_commit before finishing."
	}
	if hasCommitted && !hasPushed {
		return "PUSH REQUIRED: you committed but have not pushed. Use git_push before finishing."
	}
	return ""
}

// phaseGuidance builds the per-phase system instruction injected on entry.
func (e *Engine) phaseGuidance(phase Phase) string {
	signal := map[string]string{
		"PROBE":     "PROBE_COMPLETE",
		"RESEARCH":  "RESEARCH_COMPLETE",
		"PLAN":      "PLAN_COMPLETE",
		"EXECUTION": "EXECUTION_COMPLETE",
		"REPORT":    "REPORT_COMPLETE",
	}[phase.Name()]

	var b strings.Builder
	fmt.Fprintf(&b, "[PHASE: %s] %s\n", phase.Name(), phase.Description())
	if tools := phase.AllowedTools(); len(tools) > 0 {
		fmt.Fprintf(&b, "Allowed tools: %s\n", strings.Join(tools, ", "))
	}
	b.WriteString("Respond with PURE JSON: a tool request {\"type\":\"tool_request\",\"tool\":\"...\",\"input\":{...}} or {\"type\":\"final\",\"content\":\"...\"}.\n")
	if signal != "" {
		fmt.Fprintf(&b, "When this phase is done, include the token %s in your response.\n", signal)
	}
	switch phase.Name() {
	case "PROBE":
		b.WriteString("Declare assumptions as: ASSUMPTION: {statement} | confidence: {0.0-1.0} | criticality: {blocker|high|medium|low}\n")
	case "RESEARCH":
		b.WriteString("Declare confidence as: CONFIDENCE: {domain} | {0.0-1.0} | reason: {why}\n")
	case "PLAN":
		b.WriteString("Declare tasks as: TASK: {id} | description: {what} | agent: {who} | success: {criteria}\n")
	case "REPORT":
		b.WriteString("Include sections: ## Change Summary, ## Proof of Work, ## Impact, ## Next Steps, ## Learnings\n")
	}
	return b.String()
}

// truncate shortens long strings for transcript injection.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-200] + fmt.Sprintf("\n... [truncated %d chars] ...\n", len(s)-max) + s[len(s)-200:]
}
