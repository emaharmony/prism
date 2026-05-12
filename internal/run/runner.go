// Package run implements the lifecycle orchestrator for Prism V2.
// It accepts a task, emits the complete event lifecycle through NATS,
// optionally retrieves Remembrance context, calls an LLM provider,
// and persists the event log with artifacts.
package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/prompt"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/remembrance"
)

// RunConfig holds the configuration for a run.
type RunConfig struct {
	Task          string
	Project       string
	Agent         string
	BusURL        string
	MemoryEnabled bool
	RequireMemory bool
	MemoryURL     string
	RunDir        string // Base directory for run outputs (default: ./runs)

	// V2 LLM provider configuration
	Provider     provider.Provider
	ProviderName string        // Human-readable provider name for output/summaries
	Model        string
	Temperature  float64
	MaxTokens    int
	Timeout      time.Duration
	DryRunPrompt bool // If true, build prompt and artifacts but skip LLM call
}

// RunResult holds the result of a completed run.
type RunResult struct {
	RunID       string `json:"run_id"`
	Status      string `json:"status"`
	EventCount  int    `json:"event_count"`
	EventsPath  string `json:"events_path"`
	SummaryPath string `json:"summary_path"`
	Error       string `json:"error,omitempty"`

	// V2 fields for CLI output
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	PromptPath string `json:"prompt_path,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
	DryRun     bool   `json:"dry_run,omitempty"`
}

// Runner orchestrates the lifecycle.
type Runner struct {
	config        RunConfig
	runID         string
	correlationID string
	sessionID     string
	events        []event.Event
	nc            *nats.Conn
	js            nats.JetStreamContext
	memClient     *remembrance.Client
	startedAt     string
	taskStartedID string // ID of the task.started event for linking failure events
}

// NewRunner creates a new lifecycle runner.
func NewRunner(config RunConfig) *Runner {
	return &Runner{
		config: config,
		events: make([]event.Event, 0),
	}
}

// Run executes the complete lifecycle.
// It returns a RunResult and an error if the lifecycle failed.
func (r *Runner) Run() (*RunResult, error) {
	// Generate IDs
	r.runID = event.NewRunID()
	r.correlationID = event.NewCorrelationID()
	r.sessionID = event.NewSessionID()
	r.startedAt = time.Now().UTC().Format(time.RFC3339Nano)

	log.Printf("prism: starting run %s", r.runID)

	// Default provider for V1 backward compat
	if r.config.Provider == nil {
		r.config.Provider = provider.NewMockProvider()
	}
	if r.config.Model == "" {
		r.config.Model = "mock-model"
	}
	if r.config.Temperature == 0 {
		r.config.Temperature = 0.2
	}
	if r.config.MaxTokens == 0 {
		r.config.MaxTokens = 2048
	}
	if r.config.Timeout == 0 {
		r.config.Timeout = 60 * time.Second
	}

	// 1. Connect to NATS
	nc, err := nats.Connect(r.config.BusURL, nats.Name(fmt.Sprintf("prism-run-%s", r.runID)))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	r.nc = nc
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		return nil, fmt.Errorf("failed to init JetStream: %w", err)
	}
	r.js = js

	// Ensure the PRISM stream exists
	r.ensureStream()

	// 2. Emit task.created
	evt := r.emit(event.V1EventTypes.TaskCreated, "prism-cli", map[string]any{
		"task":    r.config.Task,
		"project": r.config.Project,
		"agent":   r.config.Agent,
	})

	// 3. Emit task.started
	evt = r.emitWithParent(event.V1EventTypes.TaskStarted, "prism-cli", map[string]any{
		"task":    r.config.Task,
		"project": r.config.Project,
		"agent":   r.config.Agent,
	}, evt.ID)
	r.taskStartedID = evt.ID

	// 4. Optional: Remembrance context hook
	var contextStr string
	memoryStatus := "none" // "none", "injected", "failed"

	if r.config.MemoryEnabled {
		r.memClient = remembrance.NewClient(r.config.MemoryURL)

		// Emit V1 memory.context_requested (backward compat)
		r.emitWithParent(event.V1EventTypes.MemoryContextRequested, "prism-cli", map[string]any{
			"task":    r.config.Task,
			"project": r.config.Project,
			"agent":   r.config.Agent,
		}, evt.ID)

		// Emit V2 context.requested
		r.emitWithParent(event.V2EventTypes.ContextRequested, "prism-cli", map[string]any{
			"task":    r.config.Task,
			"project": r.config.Project,
			"agent":   r.config.Agent,
		}, evt.ID)

		ctxResp, err := r.memClient.BuildContext(r.config.Task, r.config.Project, r.config.Agent)
		if err != nil {
			// Memory failed
			log.Printf("prism: remembrance context failed: %v", err)
			memoryStatus = "failed"

			// V1 backward compat
			r.emitWithParent(event.V1EventTypes.MemoryContextFailed, "prism-cli", map[string]any{
				"task":  r.config.Task,
				"error": err.Error(),
			}, evt.ID)

			// V2 context.failed
			r.emitWithParent(event.V2EventTypes.ContextFailed, "prism-cli", map[string]any{
				"task":  r.config.Task,
				"error": err.Error(),
			}, evt.ID)

			if r.config.RequireMemory {
				return r.fail(fmt.Sprintf("remembrance context required but failed: %v", err))
			}
			// Continue without context
			contextStr = ""
		} else if ctxResp != nil {
			// Memory succeeded
			contextStr = ctxResp.Context
			memoryStatus = "injected"
			log.Printf("prism: remembrance context built (%d sources)", len(ctxResp.Sources))

			// V1 backward compat
			r.emitWithParent(event.V1EventTypes.MemoryContextBuilt, "prism-cli", map[string]any{
				"task":          r.config.Task,
				"sources_count": len(ctxResp.Sources),
			}, evt.ID)

			// V2 context.injected
			r.emitWithParent(event.V2EventTypes.ContextInjected, "prism-cli", map[string]any{
				"task":          r.config.Task,
				"sources_count": len(ctxResp.Sources),
			}, evt.ID)
		} else {
			// No context available (404)
			log.Printf("prism: no remembrance context available")
			memoryStatus = "failed"

			r.emitWithParent(event.V1EventTypes.MemoryContextFailed, "prism-cli", map[string]any{
				"task":  r.config.Task,
				"error": "no context available",
			}, evt.ID)

			r.emitWithParent(event.V2EventTypes.ContextFailed, "prism-cli", map[string]any{
				"task":  r.config.Task,
				"error": "no context available",
			}, evt.ID)

			if r.config.RequireMemory {
				return r.fail("remembrance context required but none available")
			}
		}
	}

	// 5. Emit agent.started
	agentEvt := r.emitWithParent(event.V1EventTypes.AgentStarted, "prism-cli", map[string]any{
		"agent":   r.config.Agent,
		"task":    r.config.Task,
		"project": r.config.Project,
	}, evt.ID)

	// 6. Build and write prompt
	promptContent := prompt.BuildPrompt(r.config.Agent, r.config.Project, r.config.Task, contextStr)
	runDir := filepath.Join(r.config.RunDir, r.runID)
	if err := prompt.WritePrompt(runDir, promptContent); err != nil {
		log.Printf("prism: failed to write prompt.md: %v", err)
		return r.fail(fmt.Sprintf("failed to write prompt: %v", err))
	}

	// 7. Dry run prompt — skip LLM, complete task
	if r.config.DryRunPrompt {
		log.Printf("prism: dry-run mode — prompt written, skipping LLM call")
		r.emitWithParent(event.V1EventTypes.AgentCompleted, "prism-cli", map[string]any{
			"agent":     r.config.Agent,
			"dry_run":   true,
			"prompt.md": true,
		}, agentEvt.ID)

		// Must find the original task.started event to use as parent for task.completed
		var taskStartedID string
		for _, evt := range r.events {
			if evt.Type == event.V1EventTypes.TaskStarted {
				taskStartedID = evt.ID
				break
			}
		}
		r.emitWithParent(event.V1EventTypes.TaskCompleted, "prism-cli", map[string]any{
			"task":    r.config.Task,
			"project": r.config.Project,
			"agent":   r.config.Agent,
			"status":  "completed",
			"dry_run": true,
		}, taskStartedID)

		completedAt := time.Now().UTC().Format(time.RFC3339Nano)
		durationMs := r.calculateDuration()

		summary := event.Summary{
			RunID:         r.runID,
			CorrelationID: r.correlationID,
			Status:        "completed",
			EventCount:    len(r.events),
			StartedAt:     r.startedAt,
			CompletedAt:   completedAt,
			DurationMs:    durationMs,
			MemoryUsed:    memoryStatus == "injected",
			Agent:         r.config.Agent,
			Project:       r.config.Project,
			Task:          r.config.Task,
			Provider:      "dry-run",
			Model:         r.config.Model,
			PromptPath:    filepath.Join(runDir, "prompt.md"),
			MemoryStatus:  memoryStatus,
		}

		eventsPath, summaryPath, err := r.persist(summary)
		if err != nil {
			return nil, fmt.Errorf("failed to persist run data: %w", err)
		}

		log.Printf("prism: dry-run %s completed (%d events, %dms)", r.runID, len(r.events), durationMs)
		return &RunResult{
			RunID:       r.runID,
			Status:      "completed",
			EventCount:  len(r.events),
			EventsPath:  eventsPath,
			SummaryPath: summaryPath,
			Provider:    r.config.ProviderName,
			Model:       r.config.Model,
			PromptPath:  filepath.Join(runDir, "prompt.md"),
			DurationMs:  durationMs,
			DryRun:      true,
		}, nil
	}

	// 8. Emit llm.requested
	llmReqEvt := r.emitWithParent(event.V2EventTypes.LLMRequested, "prism-cli", map[string]any{
		"model":       r.config.Model,
		"temperature": r.config.Temperature,
		"max_tokens":  r.config.MaxTokens,
	}, agentEvt.ID)

	// 9. Call the provider with timeout
	ctx, cancel := context.WithTimeout(context.Background(), r.config.Timeout)
	defer cancel()

	genReq := provider.GenerateRequest{
		RunID:         r.runID,
		CorrelationID: r.correlationID,
		Agent:         r.config.Agent,
		Project:       r.config.Project,
		Task:          r.config.Task,
		Prompt:        promptContent,
		Model:         r.config.Model,
		Temperature:   r.config.Temperature,
		MaxTokens:     r.config.MaxTokens,
	}

	genResp, err := r.config.Provider.Generate(ctx, genReq)

	if err != nil {
		// LLM failed
		log.Printf("prism: LLM generate failed: %v", err)

		// llm.failed
		r.emitWithParent(event.V2EventTypes.LLMFailed, "prism-cli", map[string]any{
			"error":        err.Error(),
			"context_done": errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled),
		}, llmReqEvt.ID)

		// agent.failed (V2)
		r.emitWithParent(event.V2EventTypes.AgentFailed, "prism-cli", map[string]any{
			"agent": r.config.Agent,
			"error": err.Error(),
		}, llmReqEvt.ID)

		return r.failWithLLMError(fmt.Sprintf("LLM generation failed: %v", err), err.Error())
	}

	// 10. LLM succeeded — emit llm.completed
	llmCompletedEvt := r.emitWithParent(event.V2EventTypes.LLMCompleted, "prism-cli", map[string]any{
		"model":          genResp.Model,
		"provider":       genResp.Provider,
		"latency_ms":     genResp.LatencyMS,
		"prompt_tokens":  genResp.PromptTokens,
		"output_tokens":  genResp.OutputTokens,
		"text_length":    len(genResp.Text),
	}, llmReqEvt.ID)

	// 11. Emit agent.completed (V1 backward compat)
	// Parent is llm.completed (V2 causal chain: agent started → llm requested → llm completed → agent completed)
	agentCompletedEvt := r.emitWithParent(event.V1EventTypes.AgentCompleted, "prism-cli", map[string]any{
		"agent":   r.config.Agent,
		"status":  "completed",
		"summary": genResp.Text[:min(len(genResp.Text), 200)],
	}, llmCompletedEvt.ID)

	// 12. Write output.md
	outputPath := filepath.Join(runDir, "output.md")
	if err := prompt.WriteOutput(runDir, genResp.Text); err != nil {
		log.Printf("prism: failed to write output.md: %v", err)
		return r.fail(fmt.Sprintf("failed to write output: %v", err))
	}

	// Emit output.written
	r.emitWithParent(event.V2EventTypes.OutputWritten, "prism-cli", map[string]any{
		"path":         outputPath,
		"agent":        r.config.Agent,
		"output_bytes": len(genResp.Text),
	}, agentCompletedEvt.ID)

	// 13. Emit task.completed
	// 13. Emit task.completed
	r.emitWithParent(event.V1EventTypes.TaskCompleted, "prism-cli", map[string]any{
		"task":    r.config.Task,
		"project": r.config.Project,
		"agent":   r.config.Agent,
		"status":  "completed",
	}, agentCompletedEvt.ID)

	// 14. Persist event log and summary
	completedAt := time.Now().UTC().Format(time.RFC3339Nano)
	durationMs := r.calculateDuration()

	summary := event.Summary{
		RunID:         r.runID,
		CorrelationID: r.correlationID,
		Status:        "completed",
		EventCount:    len(r.events),
		StartedAt:     r.startedAt,
		CompletedAt:   completedAt,
		DurationMs:    durationMs,
		MemoryUsed:    memoryStatus == "injected",
		Agent:         r.config.Agent,
		Project:       r.config.Project,
		Task:          r.config.Task,
		// V2 fields
		Provider:     genResp.Provider,
		Model:        genResp.Model,
		OutputPath:   outputPath,
		PromptPath:   filepath.Join(runDir, "prompt.md"),
		MemoryStatus: memoryStatus,
		LLMLatencyMs: genResp.LatencyMS,
	}

	eventsPath, summaryPath, err := r.persist(summary)
	if err != nil {
		return nil, fmt.Errorf("failed to persist run data: %w", err)
	}

	log.Printf("prism: run %s completed (%d events, %dms)", r.runID, len(r.events), durationMs)

	return &RunResult{
		RunID:       r.runID,
		Status:      "completed",
		EventCount:  len(r.events),
		EventsPath:  eventsPath,
		SummaryPath: summaryPath,
		Provider:    r.config.ProviderName,
		Model:       r.config.Model,
		PromptPath:  filepath.Join(runDir, "prompt.md"),
		OutputPath:  outputPath,
		DurationMs:  durationMs,
		DryRun:      false,
	}, nil
}

// fail emits a task.failed event (linked to task.started) and returns an error result.
func (r *Runner) fail(msg string) (*RunResult, error) {
	return r.failWithLLMError(msg, "")
}

// failWithLLMError emits a task.failed event and returns an error result,
// optionally with an LLM error in the summary.
func (r *Runner) failWithLLMError(msg string, llmError string) (*RunResult, error) {
	// Use emitWithParent so task.failed links to task.started in both
	// the in-memory slice and the NATS-published event.
	parentID := r.taskStartedID
	if parentID == "" {
		// Fallback: if task.started was never emitted (extreme edge case),
		// use plain emit so we still log the failure.
		r.emit(event.V1EventTypes.TaskFailed, "prism-cli", map[string]any{
			"task":    r.config.Task,
			"project": r.config.Project,
			"error":   msg,
		})
	} else {
		r.emitWithParent(event.V1EventTypes.TaskFailed, "prism-cli", map[string]any{
			"task":    r.config.Task,
			"project": r.config.Project,
			"error":   msg,
		}, parentID)
	}

	completedAt := time.Now().UTC().Format(time.RFC3339Nano)
	durationMs := r.calculateDuration()

	summary := event.Summary{
		RunID:         r.runID,
		CorrelationID: r.correlationID,
		Status:        "failed",
		EventCount:    len(r.events),
		StartedAt:     r.startedAt,
		CompletedAt:   completedAt,
		DurationMs:    durationMs,
		MemoryUsed:    false,
		Agent:         r.config.Agent,
		Project:       r.config.Project,
		Task:          r.config.Task,
		ErrorMessage:  msg,
		Model:         r.config.Model,
		LLMError:      llmError,
	}

	eventsPath, summaryPath, _ := r.persist(summary)

	result := &RunResult{
		RunID:       r.runID,
		Status:      "failed",
		EventCount:  len(r.events),
		EventsPath:  eventsPath,
		SummaryPath: summaryPath,
		Error:       msg,
		Provider:    r.config.ProviderName,
		Model:       r.config.Model,
		DurationMs:  durationMs,
	}

	return result, fmt.Errorf("run failed: %s", msg)
}

// buildEvent creates an event, sets correlation_id and metadata, and appends
// it to the in-memory r.events slice. It does NOT publish to NATS — call
// publishEvent after building to publish.
func (r *Runner) buildEvent(eventType, source string, payload map[string]any) event.Event {
	evt := event.NewEvent(eventType, source, payload)
	evt.CorrelationID = r.correlationID
	evt.Metadata = event.EventMetadata{
		RunID:     r.runID,
		SessionID: r.sessionID,
		Project:   r.config.Project,
		Agent:     r.config.Agent,
	}
	r.events = append(r.events, evt)
	return evt
}

// publishEvent publishes a fully-built event to NATS (best effort for V1).
func (r *Runner) publishEvent(evt event.Event) {
	data, err := evt.ToJSON()
	if err != nil {
		log.Printf("prism: failed to marshal event %s: %v", evt.ID, err)
		return
	}
	if _, err := r.js.Publish(evt.Type, data); err != nil {
		log.Printf("prism: failed to publish event %s: %v", evt.ID, err)
	} else {
		log.Printf("  💎 [%s] id=%s", evt.Type, evt.ID[:24])
	}
}

// emit builds and publishes an event to NATS.
func (r *Runner) emit(eventType, source string, payload map[string]any) event.Event {
	evt := r.buildEvent(eventType, source, payload)
	r.publishEvent(evt)
	return evt
}

// emitWithParent builds an event with a parent_id, then publishes to NATS.
// Unlike the old implementation, parent_id is set BEFORE publishing, so both
// the in-memory slice and the NATS-published event include it.
func (r *Runner) emitWithParent(eventType, source string, payload map[string]any, parentID string) event.Event {
	evt := r.buildEvent(eventType, source, payload)
	evt.ParentID = parentID
	r.events[len(r.events)-1] = evt
	r.publishEvent(evt)
	return evt
}

// ensureStream creates the PRISM stream if it doesn't exist.
func (r *Runner) ensureStream() {
	_, err := r.js.AddStream(&nats.StreamConfig{
		Name:      "PRISM",
		Subjects:  []string{"prism.>"},
		Retention: nats.LimitsPolicy,
		MaxMsgs:   1000000,
		MaxBytes:  1024 * 1024 * 1024,
		MaxAge:    7 * 24 * time.Hour,
		Storage:   nats.FileStorage,
	})
	if err != nil {
		// "stream name already in use" is expected on re-creation; log at info level.
		// Any other error is unexpected and logged at warning level.
		if errors.Is(err, nats.ErrStreamNameAlreadyInUse) ||
			strings.Contains(err.Error(), "already in use") {
			log.Printf("prism: stream PRISM already exists (reusing)")
		} else {
			log.Printf("prism: WARNING: failed to ensure stream PRISM: %v", err)
		}
	}
}

// persist writes the event log and summary to disk.
func (r *Runner) persist(summary event.Summary) (eventsPath, summaryPath string, err error) {
	runDir := filepath.Join(r.config.RunDir, r.runID)
	if err = os.MkdirAll(runDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create run directory: %w", err)
	}

	// Write events.jsonl (one compact JSON object per line)
	eventsPath = filepath.Join(runDir, "events.jsonl")
	f, err := os.Create(eventsPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create events file: %w", err)
	}
	defer f.Close()

	for _, evt := range r.events {
		data, err := evt.ToJSON()
		if err != nil {
			log.Printf("prism: failed to marshal event %s: %v", evt.ID, err)
			continue
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			log.Printf("prism: failed to write event %s: %v", evt.ID, err)
		}
	}

	// Write summary.json
	summaryPath = filepath.Join(runDir, "summary.json")
	sf, err := os.Create(summaryPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to create summary file: %w", err)
	}
	defer sf.Close()

	summaryData, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return "", "", fmt.Errorf("failed to marshal summary: %w", err)
	}
	if _, err := sf.Write(append(summaryData, '\n')); err != nil {
		return "", "", fmt.Errorf("failed to write summary: %w", err)
	}

	log.Printf("prism: persisted %d events to %s", len(r.events), eventsPath)
	log.Printf("prism: summary written to %s", summaryPath)

	return eventsPath, summaryPath, nil
}

// calculateDuration computes milliseconds between startedAt and now.
func (r *Runner) calculateDuration() int64 {
	start, err := time.Parse(time.RFC3339Nano, r.startedAt)
	if err != nil {
		return 0
	}
	return time.Since(start).Milliseconds()
}
