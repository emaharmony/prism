// Package plan manages task plans for the V32 Plan-First Pipeline.
//
// Every code task should follow: Plan → Guard Check → Approval → Execute → Review → PR
// This package tracks plans and enforces the "no code without a plan" rule.
//
// A plan is stored in the workspace state directory as plan.json and includes:
//   - What we're doing (task description)
//   - Why (reasoning)
//   - What we're NOT doing (scope boundary)
//   - Expected deliverables
//   - Approval status (pending, approved, auto-proceed)
//
// The guard rail (Phase 3) will check plan existence before allowing code execution.
// If no active plan exists, the guard blocks the action and requests a plan first.
package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PlanStatus represents the approval status of a plan.
type PlanStatus string

const (
	// StatusDraft means the plan is being written but not yet reviewed.
	StatusDraft PlanStatus = "draft"
	// StatusPendingApproval means the plan needs human approval before execution.
	StatusPendingApproval PlanStatus = "pending_approval"
	// StatusApproved means the plan has been approved and execution can proceed.
	StatusApproved PlanStatus = "approved"
	// StatusAutoProceed means the plan is for a small change that auto-proceeds.
	StatusAutoProceed PlanStatus = "auto_proceed"
	// StatusRejected means the plan was rejected and should not be executed.
	StatusRejected PlanStatus = "rejected"
	// StatusCompleted means the plan has been fully executed.
	StatusCompleted PlanStatus = "completed"
	// StatusAbandoned means the plan was abandoned without completion.
	StatusAbandoned PlanStatus = "abandoned"
)

// ApprovalLevel represents what level of approval a plan needs.
// This implements Ema's decision: only system-breaking or critical
// architecture/direction changes require approval. Everything else auto-proceeds.
type ApprovalLevel string

const (
	// ApprovalNone means no approval needed (trivial change).
	ApprovalNone ApprovalLevel = "none"
	// ApprovalAuto means auto-proceed with notification (bug fix, improvement).
	ApprovalAuto ApprovalLevel = "auto"
	// ApprovalRequired means Ema must approve (architecture change, direction change).
	ApprovalRequired ApprovalLevel = "required"
	// ApprovalCritical means system-breaking change — must stop and ask.
	ApprovalCritical ApprovalLevel = "critical"
)

// Plan represents a task plan with scope, deliverables, and approval status.
type Plan struct {
	ID          string        `json:"id"`           // Unique ID (e.g., "P-011")
	Title       string        `json:"title"`         // Short description
	Description string        `json:"description"`  // Full description of what we're doing
	Reasoning   string        `json:"reasoning"`     // Why we're doing it
	Scope       string        `json:"scope"`         // What's explicitly OUT of scope
	Deliverables []string     `json:"deliverables"`  // Expected outputs
	ApprovalLevel ApprovalLevel `json:"approval_level"` // What level of approval is needed
	Status      PlanStatus    `json:"status"`         // Current status
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	ApprovedBy  string        `json:"approved_by"`     // Who approved (empty if auto)
	ApprovedAt  *time.Time    `json:"approved_at"`     // When approved
	CompletedAt *time.Time    `json:"completed_at"`   // When completed
	Branch      string        `json:"branch"`          // Git branch
	PR          string        `json:"pr"`              // PR number
}

// HasPlan checks if an active (non-completed, non-abandoned) plan exists.
func HasPlan(plans []Plan) bool {
	for _, p := range plans {
		if p.Status != StatusCompleted && p.Status != StatusAbandoned && p.Status != StatusRejected {
			return true
		}
	}
	return false
}

// ActivePlan returns the first active plan, or nil if none exists.
func ActivePlan(plans []Plan) *Plan {
	for i := range plans {
		if plans[i].Status != StatusCompleted && plans[i].Status != StatusAbandoned && plans[i].Status != StatusRejected {
			return &plans[i]
		}
	}
	return nil
}

// CanProceed checks if a plan allows code execution to proceed.
// A plan can proceed if it's approved or auto-proceed.
// It cannot proceed if it's draft, pending approval, or rejected.
func CanProceed(plan *Plan) bool {
	if plan == nil {
		return false
	}
	return plan.Status == StatusApproved || plan.Status == StatusAutoProceed
}

// NeedsApproval determines the approval level for a plan based on its scope.
// This implements Ema's V32 decisions:
//   - bugs and improvements → auto-proceed with notification
//   - architecture/direction changes → required approval
//   - system-breaking changes → critical (stop and ask)
func NeedsApproval(title, scope string) ApprovalLevel {
	// Keywords that suggest architecture or direction changes
	architectureKeywords := []string{
		"architecture", "refactor", "redesign", "migrate", "replace",
		"direction", "strategy", "framework", "pipeline", "rewrite",
		"breaking", "incompatible", "deprecate", "remove",
	}

	// Keywords that suggest critical system-breaking changes
	criticalKeywords := []string{
		"database migration", "schema change", "breaking change",
		"security", "auth", "encryption", "credential", "secret",
		"production", "deploy", "release",
	}

	lower := strings.ToLower(fmt.Sprintf("%s %s", title, scope))

	for _, kw := range criticalKeywords {
		if contains(lower, kw) {
			return ApprovalCritical
		}
	}

	for _, kw := range architectureKeywords {
		if contains(lower, kw) {
			return ApprovalRequired
		}
	}

	// Default: auto-proceed (bugs and improvements)
	return ApprovalAuto
}

// Manager handles reading and writing plan files.
type Manager struct {
	stateDir string
	mu       sync.RWMutex
}

// NewManager creates a plan manager for the given workspace directory.
func NewManager(workspaceDir string) *Manager {
	return &Manager{
		stateDir: filepath.Join(workspaceDir, "state"),
	}
}

// EnsureDir creates the state directory if it doesn't exist.
func (m *Manager) EnsureDir() error {
	return os.MkdirAll(m.stateDir, 0755)
}

// LoadPlans reads all plans from state.
func (m *Manager) LoadPlans() ([]Plan, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data, err := os.ReadFile(filepath.Join(m.stateDir, "plans.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plans: %w", err)
	}

	var plans []Plan
	if err := json.Unmarshal(data, &plans); err != nil {
		return nil, fmt.Errorf("parse plans: %w", err)
	}
	return plans, nil
}

// SavePlans writes all plans to state.
func (m *Manager) SavePlans(plans []Plan) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.EnsureDir(); err != nil {
		return err
	}

	data, err := json.MarshalIndent(plans, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plans: %w", err)
	}

	return os.WriteFile(filepath.Join(m.stateDir, "plans.json"), data, 0644)
}

// CreatePlan adds a new plan.
func (m *Manager) CreatePlan(plan Plan) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.EnsureDir(); err != nil {
		return err
	}

	plans, err := m.loadPlansLocked()
	if err != nil {
		return err
	}

	if plan.ID == "" {
		plan.ID = fmt.Sprintf("P-%03d", len(plans)+1)
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now()
	}
	plan.UpdatedAt = time.Now()

	// If approval level not set, determine it from the title/scope
	if plan.ApprovalLevel == "" {
		plan.ApprovalLevel = NeedsApproval(plan.Title, plan.Scope)
	}

	// Set initial status based on approval level
	if plan.Status == "" {
		switch plan.ApprovalLevel {
		case ApprovalNone, ApprovalAuto:
			plan.Status = StatusAutoProceed
		case ApprovalRequired, ApprovalCritical:
			plan.Status = StatusPendingApproval
		}
	}

	plans = append(plans, plan)
	return m.savePlansLocked(plans)
}

// ApprovePlan marks a plan as approved.
func (m *Manager) ApprovePlan(id, approvedBy string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plans, err := m.loadPlansLocked()
	if err != nil {
		return err
	}

	for i := range plans {
		if plans[i].ID == id {
			plans[i].Status = StatusApproved
			plans[i].ApprovedBy = approvedBy
			now := time.Now()
			plans[i].ApprovedAt = &now
			plans[i].UpdatedAt = now
			return m.savePlansLocked(plans)
		}
	}

	return fmt.Errorf("plan %s not found", id)
}

// CompletePlan marks a plan as completed.
func (m *Manager) CompletePlan(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plans, err := m.loadPlansLocked()
	if err != nil {
		return err
	}

	for i := range plans {
		if plans[i].ID == id {
			plans[i].Status = StatusCompleted
			now := time.Now()
			plans[i].CompletedAt = &now
			plans[i].UpdatedAt = now
			return m.savePlansLocked(plans)
		}
	}

	return fmt.Errorf("plan %s not found", id)
}

// AbandonPlan marks a plan as abandoned.
func (m *Manager) AbandonPlan(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	plans, err := m.loadPlansLocked()
	if err != nil {
		return err
	}

	for i := range plans {
		if plans[i].ID == id {
			plans[i].Status = StatusAbandoned
			plans[i].UpdatedAt = time.Now()
			return m.savePlansLocked(plans)
		}
	}

	return fmt.Errorf("plan %s not found", id)
}

// loadPlansLocked reads plans from disk. Caller must hold m.mu.
func (m *Manager) loadPlansLocked() ([]Plan, error) {
	data, err := os.ReadFile(filepath.Join(m.stateDir, "plans.json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read plans: %w", err)
	}

	var plans []Plan
	if err := json.Unmarshal(data, &plans); err != nil {
		return nil, fmt.Errorf("parse plans: %w", err)
	}
	return plans, nil
}

// savePlansLocked writes plans to disk. Caller must hold m.mu.
func (m *Manager) savePlansLocked(plans []Plan) error {
	data, err := json.MarshalIndent(plans, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal plans: %w", err)
	}
	return os.WriteFile(filepath.Join(m.stateDir, "plans.json"), data, 0644)
}

// FormatPlanForPrompt formats a plan for LLM injection.
func FormatPlanForPrompt(plan *Plan) string {
	if plan == nil {
		return "No active plan."
	}
	result := fmt.Sprintf("## Active Plan: %s\n", plan.Title)
	result += fmt.Sprintf("- **ID:** %s\n", plan.ID)
	result += fmt.Sprintf("- **Status:** %s\n", plan.Status)
	result += fmt.Sprintf("- **Approval:** %s\n", plan.ApprovalLevel)
	if plan.Description != "" {
		result += fmt.Sprintf("- **Description:** %s\n", plan.Description)
	}
	if plan.Reasoning != "" {
		result += fmt.Sprintf("- **Why:** %s\n", plan.Reasoning)
	}
	if plan.Scope != "" {
		result += fmt.Sprintf("- **Out of scope:** %s\n", plan.Scope)
	}
	if len(plan.Deliverables) > 0 {
		result += "- **Deliverables:**\n"
		for _, d := range plan.Deliverables {
			result += fmt.Sprintf("  - %s\n", d)
		}
	}
	if plan.Branch != "" {
		result += fmt.Sprintf("- **Branch:** %s\n", plan.Branch)
	}
	if plan.PR != "" {
		result += fmt.Sprintf("- **PR:** %s\n", plan.PR)
	}
	if plan.ApprovedBy != "" {
		result += fmt.Sprintf("- **Approved by:** %s\n", plan.ApprovedBy)
	}
	result += fmt.Sprintf("- **Created:** %s\n", plan.CreatedAt.Format("2006-01-02 15:04"))
	return result
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}