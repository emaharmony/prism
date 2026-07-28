// Package trajectory provides a flight recorder for agent runs.
//
// Every time Prism processes a message — Discord or scheduled — the system
// captures what model was used, what tools were called, what config was
// active, how long it took, and what the outcome was. One row per run,
// stored in SQLite, exportable as JSONL for debugging.
package trajectory

import (
	"time"
)

// TrajectoryRun is a single agent run's full metadata.
type TrajectoryRun struct {
	ID            string            `json:"id"`
	AgentID       string            `json:"agent_id"`
	SessionKey    string            `json:"session_key"`
	Model         string            `json:"model"`
	Provider      string            `json:"provider"`
	Trigger       string            `json:"trigger"`        // "discord", "scheduled", "cli"
	TriggerDetail string            `json:"trigger_detail"` // channel ID, action name, etc.
	Status        string            `json:"status"`         // "completed", "failed", "timed_out"
	ThinkLevel    string            `json:"think_level,omitempty"`
	SystemPromptHash string         `json:"system_prompt_hash,omitempty"`
	ToolCalls     []ToolCallRecord  `json:"tool_calls,omitempty"`
	PromptTokens  int               `json:"prompt_tokens"`
	OutputTokens  int               `json:"output_tokens"`
	DurationMs    int64             `json:"duration_ms"`
	StartedAt    time.Time          `json:"started_at"`
	EndedAt      time.Time          `json:"ended_at,omitempty"`
	SkillsLoaded []string           `json:"skills_loaded,omitempty"`
	ConfigSnapshot map[string]string `json:"config_snapshot,omitempty"`
	Error        string             `json:"error,omitempty"`
}

// ToolCallRecord is a single tool call within a run.
type ToolCallRecord struct {
	ToolName    string `json:"tool_name"`
	InputHash   string `json:"input_hash,omitempty"`  // SHA-256 of input (not full input)
	Success     bool   `json:"success"`
	DurationMs  int64  `json:"duration_ms"`
	Error       string `json:"error,omitempty"`
}

// RunStatus constants.
const (
	StatusCompleted = "completed"
	StatusFailed    = "failed"
	StatusTimedOut  = "timed_out"
)