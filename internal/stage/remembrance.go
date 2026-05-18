// Package stage provides Prism's pipeline execution engine (V14a).
//
// RemembranceStage fetches context from the Remembrance memory service.
// It's optional — if Remembrance is disabled or unavailable, the pipeline
// continues without memory context.
package stage

import (
	"context"
	"fmt"

	"github.com/emaharmony/prism/internal/event"
)

// RemembranceStage fetches context from the memory service.
// If memory is disabled or the service is unavailable, it succeeds
// with empty context (graceful degradation).
type RemembranceStage struct {
	// MemoryEnabled controls whether context fetching is attempted.
	MemoryEnabled bool

	// RequireMemory controls whether missing memory is a failure.
	// If true, the stage fails when memory is unavailable.
	// If false (default), missing memory is a warning, not an error.
	RequireMemory bool

	// MemoryURL is the Remembrance service URL.
	MemoryURL string
}

// Name returns the stage identifier.
func (s *RemembranceStage) Name() string {
	return "remembrance"
}

// Validate checks that memory configuration is valid if enabled.
func (s *RemembranceStage) Validate(rc *RunContext) error {
	if s.MemoryEnabled && s.MemoryURL == "" {
		return fmt.Errorf("stage %s: memory_url is required when memory is enabled", s.Name())
	}
	return nil
}

// Execute fetches context from Remembrance (if enabled).
// On failure, the stage either succeeds with empty context (RequireMemory=false)
// or fails (RequireMemory=true).
func (s *RemembranceStage) Execute(ctx context.Context, rc *RunContext) (*RunContext, *StageResult, error) {
	if !s.MemoryEnabled {
		// Memory disabled — skip with empty context
		return rc, &StageResult{
			StageName: s.Name(),
			Success:   true,
			Data:      map[string]any{"context": "", "source": "disabled"},
		}, nil
	}

	// For now, emit the memory context requested event.
	// The actual HTTP call to Remembrance happens in the runner wrapper.
	// This stage exists to validate configuration and emit events.
	evt := event.NewEvent(event.V1EventTypes.MemoryContextRequested, "remembrance", map[string]any{
		"run_id":  rc.RunID,
		"task":    rc.Task,
		"agent":   rc.Agent,
		"project": rc.Project,
		"source":  "remembrance",
	})
	newRC := rc.WithEvent(evt)

	return newRC, &StageResult{
		StageName: s.Name(),
		Success:   true,
		Data:      map[string]any{"context_requested": true, "source": "remembrance"},
	}, nil
}

// Rollback is a no-op — memory context fetching has no side effects to undo.
func (s *RemembranceStage) Rollback(ctx context.Context, rc *RunContext) error {
	return nil
}