package v2

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DiscordFormatter formats workflow messages for Discord.
// These are posted to #manager-room for feedback gates and reports.
type DiscordFormatter struct{}

// FormatPlanForApproval formats a plan for the FEEDBACK_PRE gate.
// Posted to Discord for Lumi/Ema to review and approve.
func FormatPlanForApproval(state *WorkflowState) string {
	var sb strings.Builder

	sb.WriteString("## 📋 Plan Review Required\n\n")
	sb.WriteString(fmt.Sprintf("**Workflow:** %s\n", state.RunID))
	sb.WriteString(fmt.Sprintf("**Requested:** %s\n\n", time.Now().UTC().Format(time.RFC3339)))

	// Assumptions summary
	if len(state.Assumptions) > 0 {
		sb.WriteString("### Key Assumptions\n")
		open := 0
		for _, a := range state.Assumptions {
			if a.Status != "addressed" {
				open++
				emoji := "🔴"
				if a.Confidence >= 0.7 {
					emoji = "🟢"
				} else if a.Confidence >= 0.4 {
					emoji = "🟡"
				}
				sb.WriteString(fmt.Sprintf("- %s [%s] %s (conf: %.1f)\n", emoji, a.Criticality, a.Statement, a.Confidence))
			}
		}
		if open == 0 {
			sb.WriteString("All assumptions resolved ✅\n")
		}
		sb.WriteString("\n")
	}

	// Confidence summary
	sb.WriteString("### Confidence Matrix\n")
	minConf := state.GetMinConfidence()
	confEmoji := "🟢"
	if minConf < 0.5 {
		confEmoji = "🔴"
	} else if minConf < 0.7 {
		confEmoji = "🟡"
	}
	sb.WriteString(fmt.Sprintf("Overall: %s %.0f%%\n", confEmoji, minConf*100))
	for domain, cd := range state.ConfidenceMatrix {
		dEmoji := "🟢"
		if cd.Score < 0.5 {
			dEmoji = "🔴"
		} else if cd.Score < 0.7 {
			dEmoji = "🟡"
		}
		sb.WriteString(fmt.Sprintf("- %s %s: %.0f%%\n", dEmoji, domain, cd.Score*100))
	}
	sb.WriteString("\n")

	// Plan tasks
	if state.Plan != nil && len(state.Plan.Tasks) > 0 {
		sb.WriteString("### Task Breakdown\n")
		for _, task := range state.Plan.Tasks {
			agentEmoji := "🤖"
			switch task.Agent {
			case "prism":
				agentEmoji = "⚡"
			case "mango":
				agentEmoji = "🥭"
			case "junie":
				agentEmoji = "🔧"
			case "lumi":
				agentEmoji = "✨"
			}
			riskBadge := ""
			switch task.RiskLevel {
			case "high":
				riskBadge = " ⚠️ HIGH RISK"
			case "medium":
				riskBadge = " 🟡 medium"
			}
			sb.WriteString(fmt.Sprintf("- **%s** %s → %s%s\n", task.ID, agentEmoji, task.Description, riskBadge))
			if task.SuccessCriteria != "" {
				sb.WriteString(fmt.Sprintf("  Done when: %s\n", task.SuccessCriteria))
			}
			if len(task.DependsOn) > 0 {
				sb.WriteString(fmt.Sprintf("  Depends on: %s\n", strings.Join(task.DependsOn, ", ")))
			}
		}
		sb.WriteString("\n")
	}

	// Risk mitigations
	if state.Plan != nil && len(state.Plan.RiskMitigations) > 0 {
		sb.WriteString("### Risk Mitigations\n")
		for _, rm := range state.Plan.RiskMitigations {
			sb.WriteString(fmt.Sprintf("- **%s**: %s → task %s\n", rm.Domain, rm.Strategy, rm.TaskID))
		}
		sb.WriteString("\n")
	}

	// Approval instructions
	sb.WriteString("### Approval\n")
	sb.WriteString("Reply with one of:\n")
	sb.WriteString("- `approve " + state.RunID + "` — approve the plan\n")
	sb.WriteString("- `changes " + state.RunID + " : {feedback}` — request changes\n")
	sb.WriteString("- `reject " + state.RunID + " : {reason}` — reject the plan\n")

	return sb.String()
}

// FormatReviewPackage formats a review package for the FEEDBACK_POST gate.
func FormatReviewPackage(state *WorkflowState, gitDiff string) string {
	var sb strings.Builder

	sb.WriteString("## 🔍 Post-Execution Review Required\n\n")
	sb.WriteString(fmt.Sprintf("**Workflow:** %s\n\n", state.RunID))

	// Completed tasks
	if state.Plan != nil {
		sb.WriteString("### Completed Tasks\n")
		for _, task := range state.Plan.Tasks {
			statusEmoji := "✅"
			if task.Status == "failed" {
				statusEmoji = "❌"
			} else if task.Status == "pending" {
				statusEmoji = "⏳"
			}
			sb.WriteString(fmt.Sprintf("- %s **%s** → %s (%s)\n", statusEmoji, task.ID, task.Description, task.Status))
			if task.Artifacts != nil {
				if task.Artifacts.CommitHash != "" {
					sb.WriteString(fmt.Sprintf("  Commit: `%s`\n", task.Artifacts.CommitHash))
				}
				if task.Artifacts.BranchName != "" {
					sb.WriteString(fmt.Sprintf("  Branch: `%s`\n", task.Artifacts.BranchName))
				}
				if task.Artifacts.PRURL != "" {
					sb.WriteString(fmt.Sprintf("  PR: %s\n", task.Artifacts.PRURL))
				}
				if len(task.Artifacts.FilePaths) > 0 {
					sb.WriteString(fmt.Sprintf("  Files: %s\n", strings.Join(task.Artifacts.FilePaths, ", ")))
				}
			}
		}
		sb.WriteString("\n")
	}

	// Git diff (truncated)
	if gitDiff != "" {
		sb.WriteString("### Changes (git diff --stat)\n")
		sb.WriteString("```\n")
		if len(gitDiff) > 2000 {
			sb.WriteString(gitDiff[:2000])
			sb.WriteString("\n... [truncated]\n")
		} else {
			sb.WriteString(gitDiff)
		}
		sb.WriteString("```\n\n")
	}

	// Review instructions
	sb.WriteString("### Review\n")
	sb.WriteString("Reviewers needed: **Mango** (required) and **Lumi**\n")
	sb.WriteString("6 dimensions: code_quality, task_completion, regression_check, test_coverage, documentation, git_hygiene\n\n")
	sb.WriteString("Reply with:\n")
	sb.WriteString("- `review_approve " + state.RunID + "` — all dimensions pass\n")
	sb.WriteString("- `review_approve " + state.RunID + " notes: {observations}` — approve with notes\n")
	sb.WriteString("- `review_changes " + state.RunID + " : {specific issues}` — send back for fixes\n")

	return sb.String()
}

// FormatFinalReport formats the REPORT phase output for Discord.
func FormatFinalReport(state *WorkflowState) string {
	var sb strings.Builder

	sb.WriteString("## 🌟 Natural Gates Workflow Complete\n\n")
	sb.WriteString(fmt.Sprintf("**Workflow:** %s\n", state.RunID))
	sb.WriteString(fmt.Sprintf("**Status:** %s\n", state.Status))
	sb.WriteString(fmt.Sprintf("**Duration:** %s → %s\n\n", state.StartedAt, state.CompletedAt))

	// Phase summary
	sb.WriteString("### Phase Summary\n")
	for _, phaseCfg := range DefaultConfig().Phases {
		if ps, ok := state.PhaseStates[phaseCfg.Name]; ok {
			emoji := "✅"
			if ps.Status == PhaseStatusFallback {
				emoji = "⚠️"
			} else if ps.Status == PhaseStatusFailed {
				emoji = "❌"
			}
			sb.WriteString(fmt.Sprintf("%s %s — %d iterations", emoji, phaseCfg.Name, ps.Iterations))
			if ps.GateResult != nil {
				sb.WriteString(fmt.Sprintf(" (gate: %.2f)", ps.GateResult.Score))
			}
			sb.WriteString("\n")
		}
	}
	sb.WriteString("\n")

	// Assumptions resolved
	if len(state.Assumptions) > 0 {
		resolved := 0
		for _, a := range state.Assumptions {
			if a.Status == "addressed" {
				resolved++
			}
		}
		sb.WriteString(fmt.Sprintf("### Assumptions: %d/%d resolved\n", resolved, len(state.Assumptions)))
	}
	sb.WriteString("\n")

	// Confidence
	minConf := state.GetMinConfidence()
	sb.WriteString(fmt.Sprintf("### Final Confidence: %.0f%%\n", minConf*100))
	for domain, cd := range state.ConfidenceMatrix {
		sb.WriteString(fmt.Sprintf("- %s: %.0f%%\n", domain, cd.Score*100))
	}
	sb.WriteString("\n")

	// Plan results
	if state.Plan != nil && len(state.Plan.Tasks) > 0 {
		sb.WriteString("### Task Results\n")
		for _, task := range state.Plan.Tasks {
			emoji := "✅"
			if task.Status == "failed" {
				emoji = "❌"
			}
			sb.WriteString(fmt.Sprintf("%s **%s** → %s (%s)\n", emoji, task.ID, task.Description, task.Status))
		}
		sb.WriteString("\n")
	}

	// Feedback summary
	if state.Feedback != nil {
		if state.Feedback.PreExecution != nil && state.Feedback.PreExecution.Status == "approved" {
			sb.WriteString(fmt.Sprintf("Plan approved by %s\n", state.Feedback.PreExecution.Approver))
		}
		if state.Feedback.PostExecution != nil {
			sb.WriteString("Post-execution review:\n")
			for reviewer, rs := range state.Feedback.PostExecution.Reviewers {
				sb.WriteString(fmt.Sprintf("- %s: %s\n", reviewer, rs.Status))
			}
		}
	}

	// Report sections
	if state.Report != nil && state.Report.Content != "" {
		sb.WriteString("\n### Report Content\n")
		sb.WriteString(state.Report.Content)
	}

	return sb.String()
}

// ParseApprovalCommand parses a Discord approval command.
// Returns: decision (approved|changes_requested|rejected), workflowID, notes.
func ParseApprovalCommand(text string) (decision, workflowID, notes string) {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)

	if strings.HasPrefix(lower, "approve ") {
		rest := strings.TrimPrefix(lower, "approve ")
		// Check for "with notes:" suffix
		if idx := strings.Index(rest, " with notes:"); idx != -1 {
			workflowID = strings.TrimSpace(rest[:idx])
			notes = strings.TrimSpace(text[idx+13:])
		} else {
			workflowID = strings.TrimSpace(rest)
		}
		return "approved", workflowID, notes
	}

	if strings.HasPrefix(lower, "changes ") {
		rest := strings.TrimPrefix(lower, "changes ")
		if idx := strings.Index(rest, " :"); idx != -1 {
			workflowID = strings.TrimSpace(rest[:idx])
			notes = strings.TrimSpace(text[idx+2:])
		} else {
			workflowID = strings.TrimSpace(rest)
		}
		return "changes_requested", workflowID, notes
	}

	if strings.HasPrefix(lower, "reject ") {
		rest := strings.TrimPrefix(lower, "reject ")
		if idx := strings.Index(rest, " :"); idx != -1 {
			workflowID = strings.TrimSpace(rest[:idx])
			notes = strings.TrimSpace(text[idx+2:])
		} else {
			workflowID = strings.TrimSpace(rest)
		}
		return "rejected", workflowID, notes
	}

	return "", "", ""
}

// ParseReviewCommand parses a Discord review command.
// Returns: decision (approved|changes_requested), workflowID, reviewer, notes.
func ParseReviewCommand(text string) (decision, workflowID, reviewer, notes string) {
	text = strings.TrimSpace(text)
	lower := strings.ToLower(text)

	if strings.HasPrefix(lower, "review_approve ") {
		rest := strings.TrimPrefix(lower, "review_approve ")
		if idx := strings.Index(rest, " notes:"); idx != -1 {
			workflowID = strings.TrimSpace(rest[:idx])
			notes = strings.TrimSpace(text[idx+7:])
		} else {
			workflowID = strings.TrimSpace(rest)
		}
		// Reviewer is inferred from the message sender (handled by caller)
		return "approved", workflowID, "", notes
	}

	if strings.HasPrefix(lower, "review_changes ") {
		rest := strings.TrimPrefix(lower, "review_changes ")
		if idx := strings.Index(rest, " :"); idx != -1 {
			workflowID = strings.TrimSpace(rest[:idx])
			notes = strings.TrimSpace(text[idx+2:])
		} else {
			workflowID = strings.TrimSpace(rest)
		}
		return "changes_requested", workflowID, "", notes
	}

	return "", "", "", ""
}

// WorkflowStateJSON returns the full state as formatted JSON (for debugging/dashboard).
func WorkflowStateJSON(state *WorkflowState) string {
	state.mu.RLock()
	defer state.mu.RUnlock()
	data, _ := json.MarshalIndent(state, "", "  ")
	return string(data)
}