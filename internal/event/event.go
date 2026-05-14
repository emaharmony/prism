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

// V3EventTypes defines the event types introduced in V3 (controlled tool execution).
var V3EventTypes = struct {
	// Tool lifecycle
	ToolRequested      string
	ToolApproved       string
	ToolDenied          string
	ToolStarted         string
	ToolCompleted       string
	ToolFailed          string
}{
	ToolRequested: "prism.tool.requested",
	ToolApproved:  "prism.tool.approved",
	ToolDenied:    "prism.tool.denied",
	ToolStarted:   "prism.tool.started",
	ToolCompleted: "prism.tool.completed",
	ToolFailed:    "prism.tool.failed",
}

// V2EventTypes defines the event types introduced in V2 (real LLM agent execution).
var V2EventTypes = struct {
	// LLM request lifecycle
	LLMRequested string
	LLMCompleted string
	LLMFailed    string

	// Context injection (additional to V1 context events)
	ContextRequested string
	ContextInjected  string
	ContextFailed    string

	// Agent lifecycle extensions
	AgentFailed string

	// Output artifacts
	OutputWritten string
}{
	LLMRequested:     "prism.llm.requested",
	LLMCompleted:     "prism.llm.completed",
	LLMFailed:        "prism.llm.failed",
	ContextRequested: "prism.context.requested",
	ContextInjected:  "prism.context.injected",
	ContextFailed:    "prism.context.failed",
	AgentFailed:      "prism.agent.failed",
	OutputWritten:    "prism.output.written",
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

// V5EventTypes defines the event types introduced in V5 (validation + review pipeline).
var V5EventTypes = struct {
	// Validation lifecycle
	ValidationRequested string
	ValidationStarted   string
	ValidationCompleted string
	ValidationFailed    string
	ValidationSkipped   string
	ValidationTimeout   string

	// Review lifecycle
	ReviewRequested string
	ReviewStarted   string
	ReviewCompleted string
	ReviewFailed    string
}{
	ValidationRequested: "prism.validation.requested",
	ValidationStarted:   "prism.validation.started",
	ValidationCompleted: "prism.validation.completed",
	ValidationFailed:    "prism.validation.failed",
	ValidationSkipped:   "prism.validation.skipped",
	ValidationTimeout:   "prism.validation.timeout",
	ReviewRequested:     "prism.review.requested",
	ReviewStarted:       "prism.review.started",
	ReviewCompleted:     "prism.review.completed",
	ReviewFailed:        "prism.review.failed",
}

// V4EventTypes defines the event types introduced in V4 (approval-gated mutations).
var V4EventTypes = struct {
	// Approval lifecycle
	ApprovalRequested string
	ApprovalGranted   string
	ApprovalDenied    string
	ApprovalExpired   string

	// Mutation lifecycle
	MutationProposed  string
	MutationValidated string
	MutationApplied   string
	MutationFailed    string
}{
	ApprovalRequested: "prism.approval.requested",
	ApprovalGranted:   "prism.approval.granted",
	ApprovalDenied:    "prism.approval.denied",
	ApprovalExpired:   "prism.approval.expired",
	MutationProposed:  "prism.mutation.proposed",
	MutationValidated: "prism.mutation.validated",
	MutationApplied:   "prism.mutation.applied",
	MutationFailed:    "prism.mutation.failed",
}

// Summary represents a V1 run summary written to summary.json.
type Summary struct {
	RunID         string `json:"run_id"`
	CorrelationID string `json:"correlation_id"`
	Status        string `json:"status"`
	EventCount    int    `json:"event_count"`
	StartedAt     string `json:"started_at"`
	CompletedAt   string `json:"completed_at"`
	DurationMs    int64  `json:"duration_ms"`
	MemoryUsed    bool   `json:"memory_used"`
	Agent         string `json:"agent"`
	Project       string `json:"project"`
	Task          string `json:"task"`
	ErrorMessage  string `json:"error_message,omitempty"`
	Provider      string `json:"provider,omitempty"`
	Model         string `json:"model,omitempty"`
	OutputPath    string `json:"output_path,omitempty"`
	PromptPath    string `json:"prompt_path,omitempty"`
	MemoryStatus  string `json:"memory_status,omitempty"` // "none", "injected", "failed"
	LLMLatencyMs  int64  `json:"llm_latency_ms,omitempty"`
	LLMError      string    `json:"llm_error,omitempty"`

	// V3 tool execution fields
	ToolCalls []ToolCallSummary `json:"tool_calls,omitempty"`

	// V4 approval and mutation fields
	Approvals []ApprovalSummary `json:"approvals,omitempty"`
	Mutations []MutationSummary `json:"mutations,omitempty"`

	// V5 validation and review fields
	Validations []ValidationSummary `json:"validations,omitempty"`
	Reviews     []ReviewSummary     `json:"reviews,omitempty"`
}

// ValidationSummary records a validation run in the summary.
type ValidationSummary struct {
	Profile    string `json:"profile"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	ResultPath string `json:"result_path,omitempty"`
}

// ReviewSummary records a review in the summary.
type ReviewSummary struct {
	Reviewer       string `json:"reviewer"`
	Status         string `json:"status"`
	Recommendation string `json:"recommendation"`
	ArtifactPath   string `json:"artifact_path,omitempty"`
}

// ToolCallSummary records a single tool call in the run summary.
type ToolCallSummary struct {
	ToolName       string `json:"tool_name"`
	PolicyDecision string `json:"policy_decision"`
	Status         string `json:"status"` // "approved", "denied", "completed", "failed"
	EventID        string `json:"event_id,omitempty"`
}

// ApprovalSummary records an approval in the run summary.
type ApprovalSummary struct {
	ApprovalID    string `json:"approval_id"`
	MutationType  string `json:"mutation_type"`
	TargetPath    string `json:"target_path"`
	Status        string `json:"status"`
	RequestedBy   string `json:"requested_by"`
	PolicyDecision string `json:"policy_decision"`
}

// MutationSummary records a mutation result in the run summary.
type MutationSummary struct {
	ApprovalID   string `json:"approval_id"`
	MutationType string `json:"mutation_type"`
	TargetPath   string `json:"target_path"`
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
}