package v2

import (
	"fmt"
	"strings"
)

// --- Gate Implementations ---

// AssumptionThresholdGate evaluates whether the weighted assumption score is below threshold.
type AssumptionThresholdGate struct {
	Threshold float64
	Weights   map[string]float64 // criticality → weight
}

func (g *AssumptionThresholdGate) Name() string { return "assumption_threshold" }

func (g *AssumptionThresholdGate) Evaluate(state *WorkflowState) GateResult {
	score := state.GetAssumptionScore(g.Weights)
	passed := score < g.Threshold

	result := GateResult{
		Passed: passed,
		Score:  score,
		Reason: "",
	}

	if !passed {
		var blockers []string
		for _, a := range state.Assumptions {
			if a.Status != "addressed" {
				if a.Criticality == "blocker" && a.Confidence < 0.5 {
					blockers = append(blockers, fmt.Sprintf("%s: %s (confidence: %.1f)", a.ID, a.Statement, a.Confidence))
				}
			}
		}
		result.Reason = fmt.Sprintf("Assumption score %.2f >= threshold %.2f. %d unresolved assumptions remain.", score, g.Threshold, len(blockers))
		if len(blockers) > 0 {
			result.Missing = blockers
			result.Reason += fmt.Sprintf(" Blockers: %s", strings.Join(blockers, "; "))
		}
		result.Reason += " Reduce assumptions by asking questions, reading docs, or searching memory."
	} else {
		result.Reason = fmt.Sprintf("Assumption score %.2f < threshold %.2f. All critical assumptions resolved.", score, g.Threshold)
	}

	return result
}

// ConfidenceThresholdGate evaluates whether all confidence domains meet the minimum threshold.
type ConfidenceThresholdGate struct {
	Threshold float64
	Domains   []string
}

func (g *ConfidenceThresholdGate) Name() string { return "confidence_threshold" }

func (g *ConfidenceThresholdGate) Evaluate(state *WorkflowState) GateResult {
	minConfidence := state.GetMinConfidence()
	passed := minConfidence >= g.Threshold

	result := GateResult{
		Passed: passed,
		Score:  minConfidence,
		Reason: "",
	}

	if !passed {
		// Find the weakest domains
		var weak []string
		for _, domain := range g.Domains {
			if cd, ok := state.ConfidenceMatrix[domain]; ok {
				if cd.Score < g.Threshold {
					weak = append(weak, fmt.Sprintf("%s: %.2f", domain, cd.Score))
				}
			}
		}
		result.Reason = fmt.Sprintf("Minimum confidence %.2f < threshold %.2f. Weakest domains: %s. Search more in these areas.", minConfidence, g.Threshold, strings.Join(weak, ", "))
		result.Missing = weak
	} else {
		result.Reason = fmt.Sprintf("All confidence domains >= %.2f. Minimum: %.2f.", g.Threshold, minConfidence)
	}

	return result
}

// PlanCompletenessGate evaluates whether the plan is complete enough to proceed.
type PlanCompletenessGate struct {
	Threshold float64
	Weights   map[string]float64 // factor → weight
}

func (g *PlanCompletenessGate) Name() string { return "plan_completeness" }

func (g *PlanCompletenessGate) Evaluate(state *WorkflowState) GateResult {
	if state.Plan == nil {
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  "No plan created. Use TASK: declarations to create a plan.",
			Missing: []string{"plan"},
		}
	}

	// Calculate completeness factors
	tasksIdentified := 0.0
	resourcesAssigned := 0.0
	dependenciesOrdered := 0.0
	successCriteria := 0.0
	riskMitigation := 0.0

	totalTasks := len(state.Plan.Tasks)
	if totalTasks > 0 {
		tasksIdentified = 1.0
		assigned := 0
		withSuccess := 0
		withDeps := 0
		for _, task := range state.Plan.Tasks {
			if task.Agent != "" {
				assigned++
			}
			if task.SuccessCriteria != "" {
				withSuccess++
			}
			if len(task.DependsOn) > 0 || totalTasks == 1 {
				withDeps++
			}
		}
		resourcesAssigned = float64(assigned) / float64(totalTasks)
		successCriteria = float64(withSuccess) / float64(totalTasks)
		dependenciesOrdered = float64(withDeps) / float64(totalTasks)
	}

	if len(state.Plan.RiskMitigations) > 0 {
		riskMitigation = 1.0
	}

	score := tasksIdentified*g.Weights["tasks_identified"] +
		resourcesAssigned*g.Weights["resources_assigned"] +
		dependenciesOrdered*g.Weights["dependencies_ordered"] +
		successCriteria*g.Weights["success_criteria"] +
		riskMitigation*g.Weights["risk_mitigation"]

	passed := score >= g.Threshold
	result := GateResult{
		Passed:  passed,
		Score:   score,
		Reason:  "",
	}

	if !passed {
		var missing []string
		if resourcesAssigned < 1.0 {
			missing = append(missing, fmt.Sprintf("%.0f%% of tasks have agents assigned", resourcesAssigned*100))
		}
		if successCriteria < 1.0 {
			missing = append(missing, fmt.Sprintf("%.0f%% of tasks have success criteria", successCriteria*100))
		}
		if riskMitigation == 0.0 {
			missing = append(missing, "no risk mitigations for low-confidence domains")
		}
		result.Reason = fmt.Sprintf("Plan completeness %.2f < threshold %.2f. Missing: %s", score, g.Threshold, strings.Join(missing, ", "))
		result.Missing = missing
	} else {
		result.Reason = fmt.Sprintf("Plan completeness %.2f >= %.2f. %d tasks, all assigned.", score, g.Threshold, totalTasks)
	}

	return result
}

// ApprovalGate evaluates whether the plan has been approved.
type ApprovalGate struct {
	RequiredApprovers []string
	Mode              string // require_any or require_both
}

func (g *ApprovalGate) Name() string { return "approval" }

func (g *ApprovalGate) Evaluate(state *WorkflowState) GateResult {
	if state.Feedback == nil || state.Feedback.PreExecution == nil {
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  "No approval received. Workflow is paused waiting for approval.",
			Missing: g.RequiredApprovers,
		}
	}

	status := state.Feedback.PreExecution.Status
	switch status {
	case "approved":
		return GateResult{
			Passed:  true,
			Score:   1.0,
			Reason:  fmt.Sprintf("Plan approved by %s", state.Feedback.PreExecution.Approver),
		}
	case "changes_requested":
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  fmt.Sprintf("Changes requested by %s. Revise the plan.", state.Feedback.PreExecution.Approver),
			Missing: []string{"revise plan"},
		}
	case "rejected":
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  fmt.Sprintf("Plan rejected by %s.", state.Feedback.PreExecution.Approver),
			Missing: []string{"new plan"},
		}
	default:
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  "Approval pending. Workflow is paused.",
			Missing: g.RequiredApprovers,
		}
	}
}

// TaskCompletionGate evaluates whether all planned tasks are complete.
type TaskCompletionGate struct {
	Mode string // all_tasks or partial_allowed
}

func (g *TaskCompletionGate) Name() string { return "task_completion" }

func (g *TaskCompletionGate) Evaluate(state *WorkflowState) GateResult {
	if state.Plan == nil {
		return GateResult{
			Passed: false,
			Score:  0.0,
			Reason: "No plan exists. Cannot evaluate task completion.",
		}
	}

	total := len(state.Plan.Tasks)
	if total == 0 {
		return GateResult{Passed: true, Score: 1.0, Reason: "No tasks in plan."}
	}

	completed := 0
	failed := 0
	for _, task := range state.Plan.Tasks {
		switch task.Status {
		case "completed":
			completed++
		case "failed":
			failed++
		}
	}

	score := float64(completed) / float64(total)
	passed := false
	reason := ""

	if g.Mode == "all_tasks" {
		passed = completed == total && failed == 0
		if !passed {
			reason = fmt.Sprintf("%d/%d tasks completed, %d failed.", completed, total, failed)
		} else {
			reason = fmt.Sprintf("All %d tasks completed.", total)
		}
	} else {
		// partial_allowed
		passed = score >= 0.8 // at least 80% complete
		reason = fmt.Sprintf("%d/%d tasks completed (%.0f%%).", completed, total, score*100)
	}

	return GateResult{
		Passed: passed,
		Score:  score,
		Reason: reason,
	}
}

// ReviewPassGate evaluates whether post-execution review has passed.
type ReviewPassGate struct {
	RequiredReviewers []string
	MaxWarn           int
}

func (g *ReviewPassGate) Name() string { return "review_pass" }

func (g *ReviewPassGate) Evaluate(state *WorkflowState) GateResult {
	if state.Feedback == nil || state.Feedback.PostExecution == nil {
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  "No review submitted. Workflow is paused waiting for review.",
			Missing: g.RequiredReviewers,
		}
	}

	// Check if all required reviewers have submitted
	var pending []string
	var hasFail bool
	var warnCount int

	for _, reviewer := range g.RequiredReviewers {
		rs, ok := state.Feedback.PostExecution.Reviewers[reviewer]
		if !ok || rs.Status == "pending" {
			pending = append(pending, reviewer)
			continue
		}
		if rs.Status == "approved" {
			// Check dimensions for fails
			for _, result := range rs.Dimensions {
				if result == "fail" {
					hasFail = true
				}
				if result == "warn" {
					warnCount++
				}
			}
		}
		if rs.Status == "changes_requested" {
			return GateResult{
				Passed:  false,
				Score:   0.0,
				Reason:  fmt.Sprintf("Reviewer %s requested changes.", reviewer),
				Missing: []string{reviewer + " changes"},
			}
		}
	}

	if len(pending) > 0 {
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  fmt.Sprintf("Waiting for review from: %s", strings.Join(pending, ", ")),
			Missing: pending,
		}
	}

	if hasFail {
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  "Review found failing dimensions. Fix issues and re-submit.",
			Missing: []string{"fix failing dimensions"},
		}
	}

	if warnCount > g.MaxWarn {
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  fmt.Sprintf("%d warnings exceed max of %d. Address warnings before approval.", warnCount, g.MaxWarn),
			Missing: []string{"reduce warnings"},
		}
	}

	return GateResult{
		Passed: true,
		Score:  1.0,
		Reason: "All reviewers approved. No failing dimensions.",
	}
}

// ReportCompletenessGate evaluates whether the report has all required sections.
type ReportCompletenessGate struct {
	RequiredSections []string
}

func (g *ReportCompletenessGate) Name() string { return "report_completeness" }

func (g *ReportCompletenessGate) Evaluate(state *WorkflowState) GateResult {
	if state.Report == nil {
		return GateResult{
			Passed:  false,
			Score:   0.0,
			Reason:  "No report created.",
			Missing: g.RequiredSections,
		}
	}

	var missing []string
	present := 0
	for _, section := range g.RequiredSections {
		if status, ok := state.Report.Sections[section]; ok && status == "present" {
			present++
		} else {
			missing = append(missing, section)
		}
	}

	score := float64(present) / float64(len(g.RequiredSections))
	passed := score == 1.0

	result := GateResult{
		Passed: passed,
		Score:  score,
	}

	if !passed {
		result.Reason = fmt.Sprintf("Report %d/%d sections present. Missing: %s", present, len(g.RequiredSections), strings.Join(missing, ", "))
		result.Missing = missing
	} else {
		result.Reason = "All report sections present."
	}

	return result
}