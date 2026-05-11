// Package run implements the V1 lifecycle orchestrator.
// It accepts a task, emits the complete event lifecycle through NATS,
// optionally retrieves Remembrance context, runs the placeholder agent,
// and persists the event log.
package run

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/emaharmony/prism/internal/agent"
	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/remembrance"
)

// RunConfig holds the configuration for a V1 run.
type RunConfig struct {
	Task           string
	Project        string
	Agent          string
	BusURL         string
	MemoryEnabled  bool
	RequireMemory  bool
	MemoryURL      string
	RunDir         string // Base directory for run outputs (default: ./runs)
}

// RunResult holds the result of a completed V1 run.
type RunResult struct {
	RunID       string `json:"run_id"`
	Status      string `json:"status"`
	EventCount  int    `json:"event_count"`
	EventsPath  string `json:"events_path"`
	SummaryPath string `json:"summary_path"`
	Error       string `json:"error,omitempty"`
}

// Runner orchestrates the V1 lifecycle.
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
}

// NewRunner creates a new V1 lifecycle runner.
func NewRunner(config RunConfig) *Runner {
	return &Runner{
		config: config,
		events: make([]event.Event, 0),
	}
}

// Run executes the complete V1 lifecycle.
// It returns a RunResult and an error if the lifecycle failed.
func (r *Runner) Run() (*RunResult, error) {
	// Generate IDs
	r.runID = event.NewRunID()
	r.correlationID = event.NewCorrelationID()
	r.sessionID = event.NewSessionID()
	r.startedAt = time.Now().UTC().Format(time.RFC3339Nano)

	log.Printf("prism: starting V1 run %s", r.runID)

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

	// 4. Optional: Remembrance context hook
	var contextStr string
	memoryUsed := false

	if r.config.MemoryEnabled {
		r.memClient = remembrance.NewClient(r.config.MemoryURL)

		// Emit memory.context_requested
		r.emitWithParent(event.V1EventTypes.MemoryContextRequested, "prism-cli", map[string]any{
			"task":    r.config.Task,
			"project": r.config.Project,
			"agent":   r.config.Agent,
		}, evt.ID)

		ctxResp, err := r.memClient.BuildContext(r.config.Task, r.config.Project, r.config.Agent)
		if err != nil {
			// Memory failed
			log.Printf("prism: remembrance context failed: %v", err)
			r.emitWithParent(event.V1EventTypes.MemoryContextFailed, "prism-cli", map[string]any{
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
			memoryUsed = true
			contextStr = ctxResp.Context
			log.Printf("prism: remembrance context built (%d sources)", len(ctxResp.Sources))
			r.emitWithParent(event.V1EventTypes.MemoryContextBuilt, "prism-cli", map[string]any{
				"task":         r.config.Task,
				"sources_count": len(ctxResp.Sources),
			}, evt.ID)
		} else {
			// No context available (404)
			log.Printf("prism: no remembrance context available")
			r.emitWithParent(event.V1EventTypes.MemoryContextFailed, "prism-cli", map[string]any{
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

	// 6. Run deterministic placeholder agent
	input := agent.PlaceholderInput{
		Task:    r.config.Task,
		Project: r.config.Project,
		Agent:   r.config.Agent,
		Context: contextStr,
	}
	output := agent.RunPlaceholder(input)

	// 7. Emit agent.output
	r.emitWithParent(event.V1EventTypes.AgentOutput, "prism-cli", map[string]any{
		"agent":            r.config.Agent,
		"status":           output.Status,
		"summary":          output.Summary,
		"context_received": output.ContextReceived,
		"actions":          output.Actions,
	}, agentEvt.ID)

	// 8. Emit agent.completed
	r.emitWithParent(event.V1EventTypes.AgentCompleted, "prism-cli", map[string]any{
		"agent":   r.config.Agent,
		"status":  output.Status,
		"summary": output.Summary,
	}, agentEvt.ID)

	// 9. Emit task.completed
	r.emitWithParent(event.V1EventTypes.TaskCompleted, "prism-cli", map[string]any{
		"task":    r.config.Task,
		"project": r.config.Project,
		"agent":   r.config.Agent,
		"status":  "completed",
	}, evt.ID)

	// 10. Persist event log and summary
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
		MemoryUsed:    memoryUsed,
		Agent:         r.config.Agent,
		Project:       r.config.Project,
		Task:          r.config.Task,
	}

	eventsPath, summaryPath, err := r.persist(summary)
	if err != nil {
		return nil, fmt.Errorf("failed to persist run data: %w", err)
	}

	log.Printf("prism: run %s completed (%d events, %dms)", r.runID, len(r.events), durationMs)

	return &RunResult{
		RunID:       r.runID,
		Status:      "completed",
		EventCount:   len(r.events),
		EventsPath:  eventsPath,
		SummaryPath: summaryPath,
	}, nil
}

// fail emits a task.failed event and returns an error result.
func (r *Runner) fail(msg string) (*RunResult, error) {
	r.emit(event.V1EventTypes.TaskFailed, "prism-cli", map[string]any{
		"task":    r.config.Task,
		"project": r.config.Project,
		"error":   msg,
	})

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
	}

	eventsPath, summaryPath, _ := r.persist(summary)

	result := &RunResult{
		RunID:       r.runID,
		Status:      "failed",
		EventCount:   len(r.events),
		EventsPath:  eventsPath,
		SummaryPath: summaryPath,
		Error:       msg,
	}

	return result, fmt.Errorf("run failed: %s", msg)
}

// emit creates and records an event, publishes it to NATS.
func (r *Runner) emit(eventType, source string, payload map[string]any) event.Event {
	evt := event.NewEvent(eventType, source, payload)
	evt.CorrelationID = r.correlationID
	evt.Metadata = event.EventMetadata{
		RunID:     r.runID,
		SessionID: r.sessionID,
		Project:   r.config.Project,
		Agent:     r.config.Agent,
	}

	r.events = append(r.events, evt)

	// Publish to NATS (best effort for V1)
	data, err := evt.ToJSON()
	if err != nil {
		log.Printf("prism: failed to marshal event %s: %v", evt.ID, err)
		return evt
	}
	if _, err := r.js.Publish(eventType, data); err != nil {
		log.Printf("prism: failed to publish event %s: %v", evt.ID, err)
	} else {
		log.Printf("  💎 [%s] id=%s", evt.Type, evt.ID[:24])
	}

	return evt
}

// emitWithParent creates an event with a parent ID.
func (r *Runner) emitWithParent(eventType, source string, payload map[string]any, parentID string) event.Event {
	evt := r.emit(eventType, source, payload)
	evt.ParentID = parentID
	// Re-marshal with parent ID
	r.events[len(r.events)-1] = evt
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
		// Stream likely already exists
		log.Printf("prism: stream PRISM already exists or error: %v", err)
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