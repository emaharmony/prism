package v2

import (
	"context"
	"fmt"
	"time"
)

// DelegationManager handles multi-agent task delegation via NATS.
type DelegationManager struct {
	natsSubject      string // e.g., "prism.agent.openclaw"
	completionSubject string // e.g., "prism.workflow.task.complete"
	timeouts         map[string]time.Duration
	maxRetries       int
}

// NewDelegationManager creates a new delegation manager.
func NewDelegationManager(natsSubject, completionSubject string) *DelegationManager {
	return &DelegationManager{
		natsSubject:       natsSubject,
		completionSubject: completionSubject,
		timeouts: map[string]time.Duration{
			"prism":       30 * time.Minute,
			"mango":       15 * time.Minute,
			"junie":       20 * time.Minute,
			"custom":      20 * time.Minute,
			"lumi":        10 * time.Minute, // consultation
		},
		maxRetries: 2,
	}
}

// TaskPacket is the structured task sent to an agent via NATS.
type TaskPacket struct {
	Type             string            `json:"type"` // "task_delegation"
	TargetAgent      string            `json:"target_agent"`
	TaskID           string            `json:"task_id"`
	Description      string            `json:"description"`
	Context          TaskContext       `json:"context"`
	ExpectedDeliverable string         `json:"expected_deliverable"`
	ValidationChecklist []string        `json:"validation_checklist"`
	Priority         string            `json:"priority"`
	Deadline         string            `json:"deadline"`
}

type TaskContext struct {
	Files      []string `json:"files,omitempty"`
	Decisions  []string `json:"decisions,omitempty"`
	Constraints []string `json:"constraints,omitempty"`
}

// TaskCompletion is the response from a delegated task.
type TaskCompletion struct {
	TaskID         string                 `json:"task_id"`
	Status         string                 `json:"status"` // completed|failed
	OutputSummary  string                 `json:"output_summary"`
	Artifacts      CompletionArtifacts    `json:"artifacts"`
	ReviewNotes    string                 `json:"review_notes,omitempty"`
}

type CompletionArtifacts struct {
	FilePaths     []string `json:"file_paths,omitempty"`
	CommitHashes  []string `json:"commit_hashes,omitempty"`
	PRURLs        []string `json:"pr_urls,omitempty"`
}

// DelegateTask sends a task packet to an agent via NATS.
// The actual NATS publishing is done by the caller (wake handler).
func (dm *DelegationManager) DelegateTask(ctx context.Context, task PlanTask, state *WorkflowState) (*DelegationState, error) {
	timeout := dm.timeouts[task.Agent]
	if timeout == 0 {
		timeout = dm.timeouts["custom"]
	}

	deadline := time.Now().Add(timeout).UTC().Format(time.RFC3339)

	packet := TaskPacket{
		Type:             "task_delegation",
		TargetAgent:      task.Agent,
		TaskID:           task.ID,
		Description:      task.Description,
		ExpectedDeliverable: task.SuccessCriteria,
		Priority:         task.RiskLevel,
		Deadline:         deadline,
	}

	// Create delegation state
	delegationID := fmt.Sprintf("DEL-%s-%d", task.ID, time.Now().Unix())
	delegation := DelegationState{
		DelegationID: delegationID,
		TaskID:       task.ID,
		Agent:        task.Agent,
		Status:       "sent",
		SentAt:       time.Now().UTC().Format(time.RFC3339),
	}

	state.mu.Lock()
	state.Delegations = append(state.Delegations, delegation)
	state.mu.Unlock()

	// Update task status
	_ = packet // will be sent via NATS by caller
	state.UpdateTaskStatus(task.ID, "in_progress", nil)

	return &delegation, nil
}

// HandleTaskCompletion processes a task completion from NATS.
func (dm *DelegationManager) HandleTaskCompletion(completion TaskCompletion, state *WorkflowState) {
	// Update delegation state
	state.mu.Lock()
	for i, del := range state.Delegations {
		if del.TaskID == completion.TaskID {
			state.Delegations[i].Status = completion.Status
			state.Delegations[i].CompletedAt = time.Now().UTC().Format(time.RFC3339)
			state.Delegations[i].ResultSummary = completion.OutputSummary
			break
		}
	}
	state.mu.Unlock()

	// Update task status
	artifactsData := map[string]any{
		"artifacts": map[string]any{
			"file_paths":  completion.Artifacts.FilePaths,
			"commit_hash": firstOrEmpty(completion.Artifacts.CommitHashes),
			"pr_url":     firstOrEmpty(completion.Artifacts.PRURLs),
		},
	}
	state.UpdateTaskStatus(completion.TaskID, completion.Status, artifactsData)
}

// CheckTimeouts checks for timed-out delegations and escalates.
func (dm *DelegationManager) CheckTimeouts(state *WorkflowState) []string {
	var timedOut []string
	state.mu.RLock()
	defer state.mu.RUnlock()

	for _, del := range state.Delegations {
		if del.Status == "sent" || del.Status == "acknowledged" || del.Status == "in_progress" {
			timeout := dm.timeouts[del.Agent]
			if timeout == 0 {
				timeout = dm.timeouts["custom"]
			}
			sentTime, _ := time.Parse(time.RFC3339, del.SentAt)
			if time.Since(sentTime) > timeout {
				timedOut = append(timedOut, del.TaskID)
			}
		}
	}
	return timedOut
}

// firstOrEmpty returns the first element or empty string.
func firstOrEmpty(slice []string) string {
	if len(slice) > 0 {
		return slice[0]
	}
	return ""
}