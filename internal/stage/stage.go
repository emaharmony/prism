// Package stage provides Prism's pipeline execution engine (V14a).
//
// Prism is React for AI. When state changes (events), actions fire automatically.
// The event bus IS the render loop. The pipeline IS the event-driven architecture.
//
// Each stage is an event producer. The pipeline sequences stages and passes
// immutable RunContext between them. No shared mutable state = no data races.
//
// Stage lifecycle:
//  1. Validate(ctx) — check prerequisites
//  2. Execute(ctx) — run the stage, return new context + result
//  3. Rollback(ctx) — undo side effects on failure (optional)
//
// The pipeline aborts on infrastructure errors and records stage failures
// without aborting (so later stages can inspect what went wrong).
package stage

import (
	"context"
	"fmt"

	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/provider"
)

// RunContext carries immutable state through the pipeline.
// Copy-on-write: each stage receives a copy and returns a new copy.
// No stage can mutate another stage's state. This eliminates ALL data races
// because there IS no shared mutable state.
//
// The RunContext accumulates events as the pipeline runs. Each stage appends
// its events and returns a new RunContext with the updated events slice.
// The PersistenceStage writes all accumulated events at the end.
type RunContext struct {
	// Run identification
	RunID         string
	CorrelationID string

	// Task configuration
	Task    string
	Project string
	Agent   string

	// LLM configuration
	Provider     provider.Provider
	ProviderName string
	Model        string
	Temperature  float64
	MaxTokens    int

	// Run directory for artifacts
	RunDir string

	// Accumulated events (copy-on-write)
	Events []event.Event

	// Stage results accumulated as the pipeline runs
	Results map[string]*StageResult

	// LLM response (populated by LLMStage)
	LLMResponse string

	// Tool results (populated by ToolStage)
	ToolResults []map[string]any

	// Dry run mode (skip LLM call, just build prompt)
	DryRunPrompt bool
}

// StageResult is the output of a single stage execution.
type StageResult struct {
	// StageName identifies which stage produced this result.
	StageName string

	// Success indicates whether the stage completed successfully.
	// A stage can fail gracefully (Success=false) without crashing the pipeline.
	// Infrastructure errors are returned as Go errors, not StageResult failures.
	Success bool

	// Data contains stage-specific output.
	// Each stage defines its own data keys.
	Data map[string]any

	// Error contains a human-readable error message if Success=false.
	// This is NOT a Go error — it's a stage failure that the pipeline
	// can inspect and decide what to do with.
	Error string
}

// Stage is a single step in the Prism pipeline.
//
// Each stage is independently testable, mockable, and replaceable.
// Stages emit events by returning new RunContexts with appended events.
// The pipeline handles persistence and NATS publishing.
//
// Example stages:
//   - ConnectionStage: connect to NATS, validate config, create run directory
//   - RemembranceStage: fetch context from Remembrance (optional)
//   - LLMStage: build prompt, call provider, parse response
//   - ToolStage: execute tool calls with policy enforcement
//   - ApprovalStage: create approval requests for mutations
//   - PersistenceStage: write artifacts (events, summary, output, prompt)
type Stage interface {
	// Name returns the stage identifier for logging, events, and WAL entries.
	Name() string

	// Validate checks that the RunContext has everything this stage needs.
	// Returns an error if prerequisites are missing (e.g., LLMStage needs
	// a provider). The pipeline calls this before Execute.
	Validate(rc *RunContext) error

	// Execute runs the stage. Returns a new RunContext (copy-on-write)
	// and a StageResult. Infrastructure errors (NATS down, disk full) are
	// returned as Go errors. Stage failures (LLM returned an error, tool
	// failed) are returned in StageResult with Success=false.
	//
	// The pipeline decides what to do with failures — it can abort, skip,
	// or continue depending on the stage and configuration.
	Execute(ctx context.Context, rc *RunContext) (*RunContext, *StageResult, error)

	// Rollback undoes side effects if a later stage fails.
	// Not all stages need rollback (e.g., NATS publish can't be undone).
	// Return nil if rollback is not applicable.
	Rollback(ctx context.Context, rc *RunContext) error
}

// Pipeline sequences stages and executes them in order.
// Each stage receives a copy of the RunContext and returns a new copy.
// If a stage fails, the pipeline records the failure and returns.
// Infrastructure errors abort the pipeline immediately.
type Pipeline struct {
	Stages []Stage
}

// NewPipeline creates a pipeline with the given stages.
func NewPipeline(stages ...Stage) *Pipeline {
	return &Pipeline{Stages: stages}
}

// Run executes all stages in sequence. Returns the final RunContext
// (with all accumulated events and results) and any error.
//
// If a stage returns a Go error (infrastructure failure), the pipeline
// aborts immediately and returns the error.
//
// If a stage returns Success=false (stage failure), the pipeline
// records the failure in Results and returns the context as-is.
// The caller can inspect Results to determine which stage failed.
func (p *Pipeline) Run(ctx context.Context, initial *RunContext) (*RunContext, error) {
	current := initial

	for i, stage := range p.Stages {
		// Validate prerequisites
		if err := stage.Validate(current); err != nil {
			return current, fmt.Errorf("stage %s: validation failed: %w", stage.Name(), err)
		}

		// Execute the stage
		next, result, err := stage.Execute(ctx, current)
		if err != nil {
			// Infrastructure error — abort pipeline
			return current, fmt.Errorf("stage %s: infrastructure error: %w", stage.Name(), err)
		}

		// Merge results: copy previous results + new result into a fresh map
		// This preserves copy-on-write — no shared mutable references
		mergedResults := make(map[string]*StageResult, len(current.Results)+1)
		for k, v := range current.Results {
			mergedResults[k] = v
		}
		for k, v := range next.Results {
			mergedResults[k] = v
		}
		mergedResults[stage.Name()] = result

		// Replace next.Results with the merged copy
		next.Results = mergedResults

		// Check for stage failure
		if !result.Success {
			return next, nil
		}

		// Advance to next stage
		current = next
		_ = i // stage index available for logging
	}

	return current, nil
}

// WithEvent returns a new RunContext with an event appended.
// This is the copy-on-write mechanism: stages append events by calling
// rc.WithEvent(evt) and returning the new context.
// Results is deep-copied to prevent shared mutable references.
func (rc *RunContext) WithEvent(evt event.Event) *RunContext {
	newEvents := make([]event.Event, len(rc.Events), len(rc.Events)+1)
	copy(newEvents, rc.Events)
	newEvents = append(newEvents, evt)

	// Deep-copy Results to prevent shared mutable references
	newResults := make(map[string]*StageResult, len(rc.Results))
	for k, v := range rc.Results {
		newResults[k] = v
	}

	return &RunContext{
		RunID:         rc.RunID,
		CorrelationID: rc.CorrelationID,
		Task:          rc.Task,
		Project:       rc.Project,
		Agent:         rc.Agent,
		Provider:      rc.Provider,
		ProviderName:  rc.ProviderName,
		Model:         rc.Model,
		Temperature:   rc.Temperature,
		MaxTokens:     rc.MaxTokens,
		RunDir:        rc.RunDir,
		Events:        newEvents,
		Results:       newResults,
		LLMResponse:   rc.LLMResponse,
		ToolResults:   rc.ToolResults,
		DryRunPrompt:  rc.DryRunPrompt,
	}
}

// WithResults returns a new RunContext with updated results.
// Results is deep-copied to prevent shared mutable references.
func (rc *RunContext) WithResults(results map[string]*StageResult) *RunContext {
	newResults := make(map[string]*StageResult, len(results))
	for k, v := range results {
		newResults[k] = v
	}

	return &RunContext{
		RunID:         rc.RunID,
		CorrelationID: rc.CorrelationID,
		Task:          rc.Task,
		Project:       rc.Project,
		Agent:         rc.Agent,
		Provider:      rc.Provider,
		ProviderName:  rc.ProviderName,
		Model:         rc.Model,
		Temperature:   rc.Temperature,
		MaxTokens:     rc.MaxTokens,
		RunDir:        rc.RunDir,
		Events:        rc.Events,
		Results:       newResults,
		LLMResponse:   rc.LLMResponse,
		ToolResults:   rc.ToolResults,
		DryRunPrompt:  rc.DryRunPrompt,
	}
}

// WithLLMResponse returns a new RunContext with the LLM response set.
// This prevents stages from manually constructing RunContexts with field drift.
func (rc *RunContext) WithLLMResponse(response string) *RunContext {
	return &RunContext{
		RunID:         rc.RunID,
		CorrelationID: rc.CorrelationID,
		Task:          rc.Task,
		Project:       rc.Project,
		Agent:         rc.Agent,
		Provider:      rc.Provider,
		ProviderName:  rc.ProviderName,
		Model:         rc.Model,
		Temperature:   rc.Temperature,
		MaxTokens:     rc.MaxTokens,
		RunDir:        rc.RunDir,
		Events:        rc.Events,
		Results:       rc.Results,
		LLMResponse:   response,
		ToolResults:   rc.ToolResults,
		DryRunPrompt:  rc.DryRunPrompt,
	}
}

// WithRunDir returns a new RunContext with the run directory set.
// Used by ConnectionStage to set the resolved run path.
func (rc *RunContext) WithRunDir(runDir string) *RunContext {
	return &RunContext{
		RunID:         rc.RunID,
		CorrelationID: rc.CorrelationID,
		Task:          rc.Task,
		Project:       rc.Project,
		Agent:         rc.Agent,
		Provider:      rc.Provider,
		ProviderName:  rc.ProviderName,
		Model:         rc.Model,
		Temperature:   rc.Temperature,
		MaxTokens:     rc.MaxTokens,
		RunDir:        runDir,
		Events:        rc.Events,
		Results:       rc.Results,
		LLMResponse:   rc.LLMResponse,
		ToolResults:   rc.ToolResults,
		DryRunPrompt:  rc.DryRunPrompt,
	}
}