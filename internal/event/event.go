// Package event defines the canonical Prism event schema and utilities.
// All Prism components (bus, agent, CLI) share this package to ensure
// consistent event structure across the system.
package event

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"time"

	"github.com/oklog/ulid/v2"
)

// V1EventTypes defines the minimal set of event types for V1.
var V1EventTypes = struct {
	// Task lifecycle
	TaskCreated   string
	TaskStarted   string
	TaskCompleted string
	TaskFailed    string

	// Memory context hooks
	MemoryContextRequested string
	MemoryContextBuilt     string
	MemoryContextFailed    string

	// Agent lifecycle
	AgentStarted   string
	AgentOutput    string
	AgentCompleted string
	AgentFailed    string

	// Tool invocations
	ToolCalled  string
	ToolResult  string
	ToolFailed  string

	// System
	SystemHealth string
}{
	TaskCreated:             "prism.task.created",
	TaskStarted:            "prism.task.started",
	TaskCompleted:          "prism.task.completed",
	TaskFailed:             "prism.task.failed",
	MemoryContextRequested: "prism.memory.context_requested",
	MemoryContextBuilt:     "prism.memory.context_built",
	MemoryContextFailed:    "prism.memory.context_failed",
	AgentStarted:           "prism.agent.started",
	AgentOutput:            "prism.agent.output",
	AgentCompleted:        "prism.agent.completed",
	AgentFailed:            "prism.agent.failed",
	ToolCalled:             "prism.tool.called",
	ToolResult:             "prism.tool.result",
	ToolFailed:             "prism.tool.failed",
	SystemHealth:           "prism.system.health",
}

// Event is the canonical Prism event schema.
// Every action, decision, and state change flows as one of these.
type Event struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Source        string         `json:"source"`
	Timestamp     string         `json:"timestamp"`
	CorrelationID string         `json:"correlation_id"`
	ParentID      string         `json:"parent_id,omitempty"`
	Payload       map[string]any `json:"payload"`
	Metadata      EventMetadata  `json:"metadata"`
}

// EventMetadata tracks runtime context, LLM provenance, and cost.
type EventMetadata struct {
	RunID     string `json:"run_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	Project   string `json:"project,omitempty"`
	Agent     string `json:"agent,omitempty"`
	Model     string `json:"model,omitempty"`
	TokenCost int    `json:"token_cost,omitempty"`
	LatencyMs int    `json:"latency_ms,omitempty"`
}

// NewID generates a unique, sortable event ID using ULID.
// Format: evt_<ulid>
func NewID() string {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	return fmt.Sprintf("evt_%s", id.String())
}

// NewCorrelationID generates a correlation ID.
// Format: corr_<ulid>
func NewCorrelationID() string {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	return fmt.Sprintf("corr_%s", id.String())
}

// NewRunID generates a run ID.
// Format: run_<ulid>
func NewRunID() string {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	return fmt.Sprintf("run_%s", id.String())
}

// NewSessionID generates a session ID.
// Format: sess_<ulid>
func NewSessionID() string {
	id := ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader)
	return fmt.Sprintf("sess_%s", id.String())
}

// NewEvent creates a new Event with generated ID and timestamp.
func NewEvent(eventType, source string, payload map[string]any) Event {
	return Event{
		ID:        NewID(),
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Payload:   payload,
	}
}

// WithCorrelationID sets the correlation ID on the event.
func (e Event) WithCorrelationID(id string) Event {
	e.CorrelationID = id
	return e
}

// WithParentID sets the parent ID on the event.
func (e Event) WithParentID(id string) Event {
	e.ParentID = id
	return e
}

// WithMetadata sets the metadata on the event.
func (e Event) WithMetadata(m EventMetadata) Event {
	e.Metadata = m
	return e
}

// ToJSON serializes the event to JSON bytes.
func (e Event) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// EventFromBytes deserializes an event from JSON bytes.
func EventFromBytes(data []byte) (Event, error) {
	var evt Event
	err := json.Unmarshal(data, &evt)
	return evt, err
}

// Summary represents a V1 run summary written to summary.json.
type Summary struct {
	RunID          string `json:"run_id"`
	CorrelationID  string `json:"correlation_id"`
	Status         string `json:"status"`
	EventCount     int    `json:"event_count"`
	StartedAt      string `json:"started_at"`
	CompletedAt    string `json:"completed_at"`
	DurationMs     int64  `json:"duration_ms"`
	MemoryUsed     bool   `json:"memory_used"`
	Agent          string `json:"agent"`
	Project        string `json:"project"`
	Task           string `json:"task"`
	ErrorMessage   string `json:"error_message,omitempty"`
}