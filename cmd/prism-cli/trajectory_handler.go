package main

import (
	"time"

	"github.com/emaharmony/prism/internal/trajectory"
)

// startTrajectory creates a TrajectoryRun at the start of an agent run.
// The returned run should be finalized with finalizeTrajectory before saving.
func startTrajectory(runID, agentID, sessionKey, model, provider, trigger, triggerDetail string) trajectory.TrajectoryRun {
	return trajectory.TrajectoryRun{
		ID:            runID,
		AgentID:       agentID,
		SessionKey:    sessionKey,
		Model:         model,
		Provider:      provider,
		Trigger:       trigger,
		TriggerDetail: triggerDetail,
		Status:        "pending",
		StartedAt:     time.Now(),
	}
}

// finalizeTrajectory updates a trajectory run with completion data and saves it.
func finalizeTrajectory(store *trajectory.Store, run *trajectory.TrajectoryRun, status string, durationMs int64, promptTokens, outputTokens int, errMsg string) {
	if store == nil || run == nil {
		return
	}
	run.Status = status
	run.EndedAt = time.Now()
	run.DurationMs = durationMs
	run.PromptTokens = promptTokens
	run.OutputTokens = outputTokens
	run.Error = errMsg
	store.Save(*run)
}