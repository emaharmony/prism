// Package tool provides plan management tools for the V32 Plan-First Pipeline.
package tool

import (
	"context"
	"fmt"

	"github.com/emaharmony/prism/internal/plan"
)

// PlanManager interface for tool access to plan state.
// The Executor provides the real implementation; tests can mock it.
type PlanManager interface {
	CreatePlan(p plan.Plan) error
	LoadPlans() ([]plan.Plan, error)
	ApprovePlan(id, approvedBy string) error
	CompletePlan(id string) error
	AbandonPlan(id string) error
}

// --- PlanCreate ---

type PlanCreateTool struct {
	Mgr PlanManager
}

func (t *PlanCreateTool) Name() string { return "plan_create" }
func (t *PlanCreateTool) Description() string {
	return "Create a new task plan. Every code change should have a plan first. The approval level is determined automatically based on the scope — bugs and improvements auto-proceed, architecture changes require approval."
}
func (t *PlanCreateTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"title":        {Type: "string", Description: "Short description of what you're doing", Required: true},
			"description":  {Type: "string", Description: "Full description of the task"},
			"reasoning":    {Type: "string", Description: "Why you're doing this"},
			"scope":        {Type: "string", Description: "What is explicitly OUT of scope"},
			"deliverables": {Type: "array", Description: "Expected outputs"},
		},
		Output: ParamSpec{Type: "string", Description: "Confirmation with plan ID and approval level"},
	}
}
func (t *PlanCreateTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	title, _ := getString(input, "title")
	if title == "" {
		return ToolResult{Success: false, Error: "title is required"}, nil
	}

	description, _ := getString(input, "description")
	reasoning, _ := getString(input, "reasoning")
	scope, _ := getString(input, "scope")

	var deliverables []string
	if d, ok := input["deliverables"].([]any); ok {
		for _, v := range d {
			if s, ok := v.(string); ok {
				deliverables = append(deliverables, s)
			}
		}
	}

	// Determine approval level
	approvalLevel := plan.NeedsApproval(title, scope)

	p := plan.Plan{
		Title:         title,
		Description:   description,
		Reasoning:     reasoning,
		Scope:         scope,
		Deliverables:  deliverables,
		ApprovalLevel: approvalLevel,
	}

	if err := t.Mgr.CreatePlan(p); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to create plan: %v", err)}, nil
	}

	status := "auto_proceed"
	if approvalLevel == plan.ApprovalRequired || approvalLevel == plan.ApprovalCritical {
		status = "pending_approval"
	}

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"plan_id":        p.ID,
			"title":          p.Title,
			"status":         status,
			"approval_level": string(approvalLevel),
			"message":        fmt.Sprintf("Plan created: %s — %s (approval: %s)", p.ID, p.Title, approvalLevel),
		},
	}, nil
}

// --- PlanList ---

type PlanListTool struct {
	Mgr PlanManager
}

func (t *PlanListTool) Name() string { return "plan_list" }
func (t *PlanListTool) Description() string {
	return "List all task plans. Shows ID, title, status, and approval level."
}
func (t *PlanListTool) Schema() ToolSchema {
	return ToolSchema{
		Input:  map[string]ParamSpec{},
		Output: ParamSpec{Type: "string", Description: "List of all plans"},
	}
}
func (t *PlanListTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	plans, err := t.Mgr.LoadPlans()
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to load plans: %v", err)}, nil
	}

	if len(plans) == 0 {
		return ToolResult{
			Success: true,
			Output: map[string]any{
				"message": "No plans found. Create one with plan_create.",
				"count":   0,
			},
		}, nil
	}

	var result string
	for _, p := range plans {
		result += fmt.Sprintf("- %s: %s [%s] (approval: %s)\n", p.ID, p.Title, p.Status, p.ApprovalLevel)
	}
	return ToolResult{
		Success: true,
		Output: map[string]any{
			"message": result,
			"count":   len(plans),
		},
	}, nil
}

// --- PlanApprove ---

type PlanApproveTool struct {
	Mgr PlanManager
}

func (t *PlanApproveTool) Name() string { return "plan_approve" }
func (t *PlanApproveTool) Description() string {
	return "Approve a plan so code execution can proceed. Only use for plans that require approval (architecture, direction changes)."
}
func (t *PlanApproveTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"id":           {Type: "string", Description: "Plan ID (e.g., P-001)", Required: true},
			"approved_by":  {Type: "string", Description: "Who approved this plan"},
		},
		Output: ParamSpec{Type: "string", Description: "Confirmation message"},
	}
}
func (t *PlanApproveTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	id, _ := getString(input, "id")
	if id == "" {
		return ToolResult{Success: false, Error: "id is required"}, nil
	}

	approvedBy, _ := getString(input, "approved_by")
	if approvedBy == "" {
		approvedBy = "agent"
	}

	if err := t.Mgr.ApprovePlan(id, approvedBy); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to approve plan: %v", err)}, nil
	}

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"plan_id": id,
			"message": fmt.Sprintf("Plan %s approved by %s", id, approvedBy),
		},
	}, nil
}

// --- PlanComplete ---

type PlanCompleteTool struct {
	Mgr PlanManager
}

func (t *PlanCompleteTool) Name() string { return "plan_complete" }
func (t *PlanCompleteTool) Description() string {
	return "Mark a plan as completed. Use when all deliverables are done."
}
func (t *PlanCompleteTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"id": {Type: "string", Description: "Plan ID (e.g., P-001)", Required: true},
		},
		Output: ParamSpec{Type: "string", Description: "Confirmation message"},
	}
}
func (t *PlanCompleteTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	id, _ := getString(input, "id")
	if id == "" {
		return ToolResult{Success: false, Error: "id is required"}, nil
	}

	if err := t.Mgr.CompletePlan(id); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to complete plan: %v", err)}, nil
	}

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"plan_id": id,
			"message": fmt.Sprintf("Plan %s completed", id),
		},
	}, nil
}

// --- PlanAbandon ---

type PlanAbandonTool struct {
	Mgr PlanManager
}

func (t *PlanAbandonTool) Name() string { return "plan_abandon" }
func (t *PlanAbandonTool) Description() string {
	return "Mark a plan as abandoned (no longer pursuing). Use when scope or direction changes."
}
func (t *PlanAbandonTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"id": {Type: "string", Description: "Plan ID (e.g., P-001)", Required: true},
		},
		Output: ParamSpec{Type: "string", Description: "Confirmation message"},
	}
}
func (t *PlanAbandonTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	id, _ := getString(input, "id")
	if id == "" {
		return ToolResult{Success: false, Error: "id is required"}, nil
	}

	if err := t.Mgr.AbandonPlan(id); err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("failed to abandon plan: %v", err)}, nil
	}

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"plan_id": id,
			"message": fmt.Sprintf("Plan %s abandoned", id),
		},
	}, nil
}

// RegisterPlanTools registers all plan tools with the tool registry.
func RegisterPlanTools(registry *Registry, planMgr *plan.Manager) {
	mgr := &planManagerAdapter{Mgr: planMgr}
	registry.Register(&PlanCreateTool{Mgr: mgr})
	registry.Register(&PlanListTool{Mgr: mgr})
	registry.Register(&PlanApproveTool{Mgr: mgr})
	registry.Register(&PlanCompleteTool{Mgr: mgr})
	registry.Register(&PlanAbandonTool{Mgr: mgr})
}

// planManagerAdapter wraps plan.Manager to satisfy the PlanManager interface.
type planManagerAdapter struct {
	Mgr *plan.Manager
}

func (a *planManagerAdapter) CreatePlan(p plan.Plan) error      { return a.Mgr.CreatePlan(p) }
func (a *planManagerAdapter) LoadPlans() ([]plan.Plan, error)   { return a.Mgr.LoadPlans() }
func (a *planManagerAdapter) ApprovePlan(id, by string) error  { return a.Mgr.ApprovePlan(id, by) }
func (a *planManagerAdapter) CompletePlan(id string) error      { return a.Mgr.CompletePlan(id) }
func (a *planManagerAdapter) AbandonPlan(id string) error       { return a.Mgr.AbandonPlan(id) }