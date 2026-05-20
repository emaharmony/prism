// Package event defines the canonical Prism event schema — the shared vocabulary
// that every component (CLI, runner, tool executor, validation engine) uses to
// communicate. Events are Prism's universal language.
//
// Why events? Instead of components calling each other directly (tight coupling),
// everything emits events to NATS. This means:
//   - Any consumer can listen to any event without the producer knowing about it
//   - The event log (events.jsonl) IS the audit trail — replay it to rebuild state
//   - New features (like a web dashboard) can plug in by just subscribing
//
// The parent chain concept: every event has an optional ParentID that points to
// the event that caused it. This creates a causal DAG you can trace from task
// creation all the way down to the final review artifact. For example:
//   task.created → task.started → agent.started → llm.requested → llm.completed
// If llm.requested fails, the failure event still links back so you know what broke.
//
// Versioned event types (V1-V5) represent the platform's evolution, not API
// breaking changes. V1 is the foundation; each version adds more specific events.
// New consumers should use the latest version's event types; old consumers still
// work because V1 events are always emitted alongside newer ones for backward compat.
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

// Event is the canonical Prism event — the atomic unit of communication.
// Every action, decision, and state change flows as one of these. An event
// has a unique ULID-based ID, belongs to a type namespace (e.g., prism.task.started),
// optionally links to a parent event (causal trace), and carries arbitrary
// payload data. The CorrelationID groups all events from the same logical request.
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
	RunID       string    `json:"run_id,omitempty"`
	SessionID   string    `json:"session_id,omitempty"`
	Project     string    `json:"project,omitempty"`
	Agent       string    `json:"agent,omitempty"`
	Model       string    `json:"model,omitempty"`
	TokenCost   int       `json:"token_cost,omitempty"`     // V1: total tokens (deprecated, use TokenUsage)
	LatencyMs   int       `json:"latency_ms,omitempty"`    // V1: LLM latency

	// V16: Enriched metadata
	DurationMs  int64      `json:"duration_ms,omitempty"`   // Wall-clock duration
	Outcome     string     `json:"outcome,omitempty"`        // "success" | "failure" | "timeout" | "skipped"
	TokenUsage  *TokenUsage `json:"token_usage,omitempty"` // Detailed token breakdown
}

// TokenUsage provides a detailed breakdown of LLM token consumption.
// Replaces the flat TokenCost (int) field with structured data.
type TokenUsage struct {
	PromptTokens     int     `json:"prompt_tokens,omitempty"`
	CompletionTokens int     `json:"completion_tokens,omitempty"`
	TotalTokens      int     `json:"total_tokens,omitempty"`
	EstimatedCostUsd float64 `json:"estimated_cost_usd,omitempty"` // in microdollars
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

// V16EventTypes defines the event types introduced in V16 (Intelligence Arc).
// Cost tracking and enriched metadata events.
var V16EventTypes = struct {
	// Cost tracking
	CostTracked  string
	CostReported string
}{
	CostTracked:  "prism.cost.tracked",
	CostReported: "prism.cost.reported",
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
// It's the human-readable digest of an entire run — status, timing, what
// happened, and links to all artifacts (prompt, output, review). This is
// what you'd check first when reviewing a run's outcome.
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

// ValidationSummary records a single validation run in the summary.
// Each validation profile (e.g., "go_test_all", "go_lint") produces
// one of these. The exit code and status tell the reviewer whether
// the code passes automated checks.
type ValidationSummary struct {
	Profile    string `json:"profile"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	DurationMs int64  `json:"duration_ms"`
	ResultPath string `json:"result_path,omitempty"`
}

// ReviewSummary records a deterministic review in the summary.
// Unlike an LLM-generated review, this is rule-based: "all validations
// passed? → approve for review" / "validations failed? → needs fix".
// The recommendation field is the reviewer's verdict.
type ReviewSummary struct {
	Reviewer       string `json:"reviewer"`
	Status         string `json:"status"`
	Recommendation string `json:"recommendation"`
	ArtifactPath   string `json:"artifact_path,omitempty"`
}

// ToolCallSummary records a single tool call in the run summary.
// Each time the agent invokes a tool (echo, read_file, write_file_dry_run,
// write_file_proposal), we record which tool was called, what policy decided,
// and whether the call succeeded.
type ToolCallSummary struct {
	ToolName       string `json:"tool_name"`
	PolicyDecision string `json:"policy_decision"`
	Status         string `json:"status"` // "approved", "denied", "completed", "failed"
	EventID        string `json:"event_id,omitempty"`
}

// ApprovalSummary records an approval request in the run summary.
// When the agent proposes a mutation that requires human approval
// (e.g., writing a file), we record the approval ID, what's being changed,
// and its current status (pending/approved/denied).
type ApprovalSummary struct {
	ApprovalID    string `json:"approval_id"`
	MutationType  string `json:"mutation_type"`
	TargetPath    string `json:"target_path"`
	Status        string `json:"status"`
	RequestedBy   string `json:"requested_by"`
	PolicyDecision string `json:"policy_decision"`
}

// MutationSummary records an applied mutation result in the summary.
// Once a human approves a mutation and it's applied, this records
// what file was changed and whether the write succeeded.
type MutationSummary struct {
	ApprovalID   string `json:"approval_id"`
	MutationType string `json:"mutation_type"`
	TargetPath   string `json:"target_path"`
	Success      bool   `json:"success"`
	Message      string `json:"message,omitempty"`
}