package delegation

import (
	"fmt"

	"github.com/emaharmony/prism/internal/orchestrator"
)

// Capabilities defines what an agent can do.
// These are enforced during delegation to prevent unauthorized task routing.
const (
	// CapPlan allows an agent to plan and break down tasks.
	CapPlan = "plan"

	// CapDelegate allows an agent to delegate tasks to other agents.
	CapDelegate = "delegate"

	// CapReview allows an agent to review and approve/reject work.
	CapReview = "review"

	// CapApprove allows an agent to approve pushes, deployments, etc.
	CapApprove = "approve"

	// CapCode allows an agent to write and modify code.
	CapCode = "code"

	// CapTest allows an agent to write and run tests.
	CapTest = "test"

	// CapReport allows an agent to generate reports and summaries.
	CapReport = "report"

	// CapSearch allows an agent to search for information.
	CapSearch = "search"

	// CapSummarize allows an agent to summarize information.
	CapSummarize = "summarize"
)

// AllCapabilities is the set of all valid capability strings.
var AllCapabilities = map[string]bool{
	CapPlan:      true,
	CapDelegate:  true,
	CapReview:    true,
	CapApprove:   true,
	CapCode:      true,
	CapTest:      true,
	CapReport:    true,
	CapSearch:    true,
	CapSummarize: true,
}

// RoleDefaults maps role names to their default capabilities.
// When an agent has no explicit capabilities, these defaults are applied.
var RoleDefaults = map[string][]string{
	"orchestrator": {CapPlan, CapDelegate, CapReview, CapApprove},
	"lead":         {CapPlan, CapDelegate, CapReview},
	"developer":    {CapCode, CapTest, CapReport},
	"coder":        {CapCode, CapTest, CapReport},
	"researcher":   {CapSearch, CapSummarize, CapReport},
}

// CanDelegate checks whether the delegating agent has the delegate capability
// and whether the target agent has a capability matching the task type.
//
// Rules:
// 1. The delegating agent must have the "delegate" capability (or be primary).
// 2. The target agent must have a capability matching the task type.
//    - Task type "code_implementation" requires "code" capability.
//    - Task type "code" requires "code" capability.
//    - Task type "review" requires "review" capability.
//    - Task type "research" requires "search" capability.
//    - Task type "general" is always allowed (any agent can handle it).
func CanDelegate(delegator *orchestrator.AgentConfig, target *orchestrator.AgentConfig, taskType string) error {
	// Rule 1: Primary agents can always delegate (they're the orchestrator)
	if delegator.Primary {
		return nil
	}

	// Rule 1b: Check if delegator has the "delegate" capability
	if !hasCapability(delegator, CapDelegate) {
		return fmt.Errorf("delegation: agent %q cannot delegate (missing %q capability)", delegator.ID, CapDelegate)
	}

	// Rule 2: Check if target can handle the task type
	if !canHandleTaskType(target, taskType) {
		return fmt.Errorf("delegation: agent %q cannot handle task type %q (missing capability)", target.ID, taskType)
	}

	return nil
}

// canHandleTaskType checks whether an agent's capabilities cover the given task type.
func canHandleTaskType(agent *orchestrator.AgentConfig, taskType string) bool {
	// "general" tasks can be handled by any agent
	if taskType == "general" {
		return true
	}

	// Map task types to required capabilities
	requiredCap := taskTypeToCapability(taskType)
	if requiredCap == "" {
		// Unknown task type — allow it (permissive default)
		return true
	}

	return hasCapability(agent, requiredCap)
}

// taskTypeToCapability maps task types to their required capabilities.
func taskTypeToCapability(taskType string) string {
	switch taskType {
	case "code", "code_implementation", "code_fix", "code_refactor":
		return CapCode
	case "test", "test_write", "test_run":
		return CapTest
	case "review", "code_review":
		return CapReview
	case "plan", "plan_task", "breakdown":
		return CapPlan
	case "approve", "approve_push", "approve_deploy":
		return CapApprove
	case "report", "summary", "summarize":
		return CapReport
	case "search", "research", "research_search":
		return CapSearch
	default:
		return "" // Unknown task type — permissive default
	}
}

// hasCapability checks whether an agent has a specific capability.
// If the agent has no explicit capabilities, role defaults are used.
func hasCapability(agent *orchestrator.AgentConfig, cap string) bool {
	// Check explicit capabilities
	for _, c := range agent.Capabilities {
		if c == cap {
			return true
		}
	}

	// Check role defaults
	if defaults, ok := RoleDefaults[agent.Role]; ok {
		for _, c := range defaults {
			if c == cap {
				return true
			}
		}
	}

	return false
}