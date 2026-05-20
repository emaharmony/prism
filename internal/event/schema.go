// Package event provides lightweight payload validation for Prism events.
// Schema validation is opt-in: call Validate(evt) before persisting in development
// to catch missing required fields early. In production, it can be disabled.
package event

// Schema defines the expected fields for an event type.
type Schema struct {
	Required []string // Fields that must be present in Payload
	Optional []string // Fields that are allowed but not required
}

// Schemas maps event types to their expected payload schemas.
// Unknown event types are allowed (no schema enforcement).
var Schemas = map[string]Schema{
	// Task lifecycle
	"prism.task.created":   {Required: []string{"task"}, Optional: []string{"provider", "model", "agent"}},
	"prism.task.started":  {Required: []string{"task"}, Optional: []string{"run_id"}},
	"prism.task.completed": {Required: []string{"status"}, Optional: []string{"duration_ms", "output_path"}},
	"prism.task.failed":    {Required: []string{"error"}, Optional: []string{"status"}},

	// Agent lifecycle
	"prism.agent.started":   {Required: []string{"agent"}, Optional: []string{"model"}},
	"prism.agent.output":    {Required: []string{"content"}, Optional: []string{"tokens"}},
	"prism.agent.completed": {Required: []string{"status"}, Optional: []string{"duration_ms"}},
	"prism.agent.failed":    {Required: []string{"error"}, Optional: []string{}},

	// LLM
	"prism.llm.requested":  {Required: []string{"provider", "model"}, Optional: []string{"prompt_tokens"}},
	"prism.llm.completed":   {Required: []string{"provider", "model"}, Optional: []string{"completion_tokens", "total_tokens", "latency_ms", "duration_ms"}},
	"prism.llm.failed":      {Required: []string{"error"}, Optional: []string{"provider"}},

	// Tool execution (V3)
	"prism.tool.requested": {Required: []string{"tool_name"}, Optional: []string{"args", "policy_decision"}},
	"prism.tool.approved":  {Required: []string{"tool_name"}, Optional: []string{"policy"}},
	"prism.tool.denied":    {Required: []string{"tool_name"}, Optional: []string{"reason"}},
	"prism.tool.started":   {Required: []string{"tool_name"}, Optional: []string{}},
	"prism.tool.completed": {Required: []string{"tool_name"}, Optional: []string{"result"}},
	"prism.tool.failed":    {Required: []string{"tool_name"}, Optional: []string{"error"}},

	// Approval gates (V4)
	"prism.approval.requested": {Required: []string{"approval_id", "mutation_type"}, Optional: []string{"target_path"}},
	"prism.approval.granted":   {Required: []string{"approval_id"}, Optional: []string{"approved_by"}},
	"prism.approval.denied":    {Required: []string{"approval_id"}, Optional: []string{"reason"}},
	"prism.approval.expired":   {Required: []string{"approval_id"}, Optional: []string{}},

	// Mutations (V4)
	"prism.mutation.proposed":  {Required: []string{"mutation_type", "target_path"}, Optional: []string{"diff", "approval_id"}},
	"prism.mutation.validated": {Required: []string{"approval_id"}, Optional: []string{"validation_status"}},
	"prism.mutation.applied":   {Required: []string{"target_path"}, Optional: []string{"success", "approval_id"}},
	"prism.mutation.failed":     {Required: []string{"target_path"}, Optional: []string{"error"}},

	// Validation (V5)
	"prism.validation.requested": {Required: []string{"profile"}, Optional: []string{"command"}},
	"prism.validation.started":   {Required: []string{"profile"}, Optional: []string{"working_dir"}},
	"prism.validation.completed": {Required: []string{"profile"}, Optional: []string{"exit_code", "duration_ms"}},
	"prism.validation.failed":    {Required: []string{"profile"}, Optional: []string{"error"}},
	"prism.validation.skipped":   {Required: []string{"profile"}, Optional: []string{"reason"}},
	"prism.validation.timeout":   {Required: []string{"profile"}, Optional: []string{"timeout_seconds"}},

	// Review (V5)
	"prism.review.requested": {Required: []string{"run_id"}, Optional: []string{"mutation_id"}},
	"prism.review.started":   {Required: []string{"reviewer"}, Optional: []string{}},
	"prism.review.completed": {Required: []string{"recommendation"}, Optional: []string{"artifact_path"}},
	"prism.review.failed":    {Required: []string{"error"}, Optional: []string{}},

	// Policy (V8)
	"prism.policy.requested":        {Required: []string{"action", "resource"}, Optional: []string{}},
	"prism.policy.evaluated":        {Required: []string{"decision"}, Optional: []string{"rules_matched"}},
	"prism.policy.allowed":          {Required: []string{"action", "resource"}, Optional: []string{"rule"}},
	"prism.policy.denied":           {Required: []string{"action", "resource"}, Optional: []string{"reason"}},
	"prism.policy.approval_required": {Required: []string{"action", "resource"}, Optional: []string{"rule"}},
	"prism.policy.failed":           {Required: []string{"error"}, Optional: []string{}},

	// Context (V2)
	"prism.context.requested": {Required: []string{"task"}, Optional: []string{"project"}},
	"prism.context.injected":  {Required: []string{"context_size"}, Optional: []string{"source"}},
	"prism.context.failed":    {Required: []string{"error"}, Optional: []string{}},

	// Memory (V1)
	"prism.memory.context_requested": {Required: []string{"task"}, Optional: []string{"project", "agent"}},
	"prism.memory.context_built":     {Required: []string{}, Optional: []string{"memories_count", "entities_count"}},
	"prism.memory.context_failed":   {Required: []string{"error"}, Optional: []string{}},

	// Adapter (V9)
	"prism.adapter.registered": {Required: []string{"adapter_id"}, Optional: []string{"capabilities"}},
	"prism.adapter.health":     {Required: []string{"adapter_id"}, Optional: []string{"status"}},
	"prism.adapter.execute":   {Required: []string{"adapter_id", "action"}, Optional: []string{"payload"}},
	"prism.adapter.success":   {Required: []string{"adapter_id"}, Optional: []string{"result"}},
	"prism.adapter.failed":    {Required: []string{"adapter_id"}, Optional: []string{"error"}},

	// Projection (V10)
	"prism.projection.started":   {Required: []string{"projection_id"}, Optional: []string{"event_type"}},
	"prism.projection.completed": {Required: []string{"projection_id"}, Optional: []string{"state"}},
	"prism.projection.failed":    {Required: []string{"projection_id"}, Optional: []string{"error"}},

	// Workflow (V7)
	"prism.workflow.started":       {Required: []string{"workflow_id", "name"}, Optional: []string{"steps"}},
	"prism.workflow.step.started":  {Required: []string{"step_id"}, Optional: []string{"step_name"}},
	"prism.workflow.step.completed": {Required: []string{"step_id"}, Optional: []string{"result"}},
	"prism.workflow.step.failed":   {Required: []string{"step_id"}, Optional: []string{"error"}},
	"prism.workflow.step.skipped":  {Required: []string{"step_id"}, Optional: []string{"reason"}},
	"prism.workflow.paused":        {Required: []string{"workflow_id"}, Optional: []string{"pending_approval_id"}},
	"prism.workflow.resumed":       {Required: []string{"workflow_id"}, Optional: []string{"approval_id"}},
	"prism.workflow.completed":     {Required: []string{"workflow_id"}, Optional: []string{"status"}},
	"prism.workflow.failed":        {Required: []string{"workflow_id"}, Optional: []string{"error"}},

	// Cost (V16)
	"prism.cost.tracked":  {Required: []string{"provider", "model"}, Optional: []string{"prompt_tokens", "completion_tokens", "estimated_cost_usd"}},
	"prism.cost.reported": {Required: []string{"run_id"}, Optional: []string{"total_tokens", "estimated_cost_usd"}},

	// System
	"prism.system.health":      {Required: []string{"status"}, Optional: []string{"version", "uptime"}},
	"prism.persistence.completed": {Required: []string{"run_id"}, Optional: []string{"stage", "checkpoint_id"}},
	"prism.output.written":       {Required: []string{"path"}, Optional: []string{"size_bytes"}},
}

// ValidationError represents a schema validation failure.
type ValidationError struct {
	EventType string
	Missing   []string // Required fields that are missing
}

func (e *ValidationError) Error() string {
	return "event " + e.EventType + ": missing required fields: " + joinFields(e.Missing)
}

// Validate checks that an event's payload contains all required fields for its type.
// Returns nil if valid, a ValidationError if required fields are missing.
// Unknown event types always pass (no schema enforcement).
func Validate(evt Event) error {
	schema, ok := Schemas[evt.Type]
	if !ok {
		return nil // Unknown event type, allow it
	}

	var missing []string
	for _, field := range schema.Required {
		if _, exists := evt.Payload[field]; !exists {
			missing = append(missing, field)
		}
	}

	if len(missing) > 0 {
		return &ValidationError{
			EventType: evt.Type,
			Missing:   missing,
		}
	}

	return nil
}

// joinFields joins field names with commas for error messages.
func joinFields(fields []string) string {
	result := ""
	for i, f := range fields {
		if i > 0 {
			result += ", "
		}
		result += f
	}
	return result
}