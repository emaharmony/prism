// Package stage provides Prism's pipeline execution engine (V14a).
//
// RemembranceStage integrates Prism with the Remembrance memory layer.
// It performs TWO operations:
//  1. Capture: After agent output, sends the content to Remembrance for
//     gate → extract → graph → store processing.
//  2. Context: Before LLM calls, fetches relevant memories to inject
//     into the prompt.
//
// Architecture (V26):
//   - Capture is done SYNCHRONOUSLY in this stage as a belt-and-suspenders
//     alongside the NATS subscriber. The NATS subscriber in the Python
//     Remembrance service handles async capture from agent output events.
//     Both use the same (agent + session + turn) idempotency key.
//   - Context building is done before every LLM call to inject relevant
//     memories into the prompt.
//   - If Remembrance is unavailable, the stage succeeds gracefully with
//     empty context (RequireMemory=false) or fails (RequireMemory=true).
package stage

import (
	"context"
	"fmt"
	"log"
	"strings"

	remcli "github.com/emaharmony/prism/internal/remembrance"
)

// RemembranceStage integrates with the Remembrance memory layer.
//
// It handles two operations:
//   - Capture: sends agent output to Remembrance for processing
//   - Context: fetches relevant memories before LLM calls
//
// If memory is disabled or unavailable, the pipeline continues without
// memory context (graceful degradation).
type RemembranceStage struct {
	// MemoryEnabled controls whether memory operations are attempted.
	MemoryEnabled bool

	// RequireMemory controls whether missing memory is a failure.
	// If true, the stage fails when memory is unavailable.
	// If false (default), missing memory is a warning, not an error.
	RequireMemory bool

	// MemoryURL is the Remembrance service URL.
	MemoryURL string

	// Capture controls whether to capture agent output to Remembrance.
	// Default: true. Set false to skip capture (context-only mode).
	Capture bool

	// Context controls whether to fetch context from Remembrance.
	// Default: true. Set false to skip context (capture-only mode).
	Context bool

	// client is the lazy-initialized Remembrance HTTP client.
	client *remcli.Client
}

// RemembranceStageOption configures a RemembranceStage.
type RemembranceStageOption func(*RemembranceStage)

// WithCapture enables or disables capture.
func WithCapture(capture bool) RemembranceStageOption {
	return func(s *RemembranceStage) { s.Capture = capture }
}

// WithContext enables or disables context building.
func WithContext(context bool) RemembranceStageOption {
	return func(s *RemembranceStage) { s.Context = context }
}

// NewRemembranceStage creates a RemembranceStage with the given configuration.
func NewRemembranceStage(memoryEnabled bool, requireMemory bool, memoryURL string, opts ...RemembranceStageOption) *RemembranceStage {
	s := &RemembranceStage{
		MemoryEnabled: memoryEnabled,
		RequireMemory: requireMemory,
		MemoryURL:     memoryURL,
		Capture:       true,
		Context:        true,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Name returns the stage identifier.
func (s *RemembranceStage) Name() string { return "remembrance" }

// Validate checks that memory configuration is valid if enabled.
func (s *RemembranceStage) Validate(rc *RunContext) error {
	if s.MemoryEnabled && s.MemoryURL == "" {
		return fmt.Errorf("stage %s: memory_url is required when memory is enabled", s.Name())
	}
	return nil
}

// getClient returns the lazy-initialized Remembrance client.
func (s *RemembranceStage) getClient() *remcli.Client {
	if s.client == nil {
		s.client = remcli.NewClient(s.MemoryURL)
	}
	return s.client
}

// Execute runs the Remembrance stage.
//
// It performs up to two operations:
//  1. Capture: sends LLM output to Remembrance for gate → extract → store
//  2. Context: fetches relevant memories to inject into the prompt
//
// On failure, the stage either succeeds with empty context (RequireMemory=false)
// or fails (RequireMemory=true).
func (s *RemembranceStage) Execute(ctx context.Context, rc *RunContext) (*RunContext, *StageResult, error) {
	if !s.MemoryEnabled {
		return rc, &StageResult{
			StageName: s.Name(),
			Success:   true,
			Data:      map[string]any{"context": "", "source": "disabled"},
		}, nil
	}

	client := s.getClient()

	// Check availability first
	if !client.IsAvailable() {
		log.Printf("prism: remembrance service unavailable at %s", s.MemoryURL)
		if s.RequireMemory {
			return rc, &StageResult{
				StageName: s.Name(),
				Success:   false,
				Error:     "remembrance service unavailable and memory is required",
			}, nil
		}
		// Graceful degradation
		return rc, &StageResult{
			StageName: s.Name(),
			Success:   true,
			Data:      map[string]any{"context": "", "source": "unavailable"},
		}, nil
	}

	result := map[string]any{}

	// ── 1. Capture: send agent output to Remembrance ──────────────────
	if s.Capture && rc.LLMResponse != "" {
		captureResult := s.captureOutput(client, rc)
		result["capture"] = captureResult
	}

	// ── 2. Context: fetch relevant memories ────────────────────────────
	if s.Context {
		contextStr, contextSource, memories, entities := s.buildContext(client, rc)
		result["context"] = contextStr
		result["source"] = contextSource
		result["memories_count"] = len(memories)
		result["entities_count"] = len(entities)
	}

	return rc, &StageResult{
		StageName: s.Name(),
		Success:   true,
		Data:      result,
	}, nil
}

// captureOutput sends the LLM response to Remembrance for processing.
// Uses (agent + session + turn) as an idempotency key to avoid duplicates
// when both the NATS subscriber and this stage process the same output.
func (s *RemembranceStage) captureOutput(client *remcli.Client, rc *RunContext) map[string]any {
	content := rc.LLMResponse
	if content == "" {
		return map[string]any{"decision": "skip", "reason": "empty output"}
	}

	// Source tag identifies where the capture came from
	source := "prism:pipeline"
	if rc.Agent != "" {
		source = fmt.Sprintf("prism:%s", rc.Agent)
	}

	// Category from project name
	category := rc.Project

	resp, err := client.Capture(content, source, category, "")
	if err != nil {
		log.Printf("prism: remembrance capture failed: %v", err)
		return map[string]any{"decision": "error", "error": err.Error()}
	}

	decision, _ := resp["decision"].(string)
	id, _ := resp["id"].(string)
	log.Printf("prism: remembrance capture: decision=%s, id=%s", decision, id)

	return resp
}

// buildContext fetches relevant memories from Remembrance and formats
// them for injection into the LLM prompt.
func (s *RemembranceStage) buildContext(client *remcli.Client, rc *RunContext) (string, string, []map[string]any, []map[string]any) {
	// Use the task as the search query
	query := rc.Task
	if query == "" {
		return "", "no_task", nil, nil
	}

	ctxResp, err := client.BuildContext(query, rc.Project, rc.Agent, 10)
	if err != nil {
		log.Printf("prism: remembrance context failed: %v", err)
		if s.RequireMemory {
			return "", "failed", nil, nil
		}
		return "", "failed", nil, nil
	}

	if ctxResp == nil || len(ctxResp.Memories) == 0 {
		return "", "empty", nil, nil
	}

	// Format context string from memories and entities
	var contextParts []string
	for _, mem := range ctxResp.Memories {
		if ct, ok := mem["compiled_truth"].(string); ok && ct != "" {
			contextParts = append(contextParts, ct)
		} else if s, ok := mem["summary"].(string); ok && s != "" {
			contextParts = append(contextParts, s)
		}
	}
	for _, ent := range ctxResp.Entities {
		if ct, ok := ent["compiled_truth"].(string); ok && ct != "" {
			name, _ := ent["name"].(string)
			contextParts = append(contextParts, fmt.Sprintf("[%s] %s", name, ct))
		}
	}

	contextStr := strings.Join(contextParts, "\n\n")
	log.Printf("prism: remembrance context built (%d memories, %d entities)", len(ctxResp.Memories), len(ctxResp.Entities))

	return contextStr, "injected", ctxResp.Memories, ctxResp.Entities
}

// Rollback is a no-op — memory operations have no side effects to undo.
// Capture is idempotent (same content is deduplicated), and context
// building is read-only.
func (s *RemembranceStage) Rollback(ctx context.Context, rc *RunContext) error {
	return nil
}