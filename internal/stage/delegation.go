package stage

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/delegation"
	"github.com/emaharmony/prism/internal/event"
)

// delegationPattern matches delegation intent markers in LLM output.
// Matches patterns like: [DELEGATE: mango] or [DELEGATE: mango | code_implementation]
var delegationPattern = regexp.MustCompile(`\[DELEGATE:\s*(\w+)\s*(?:\|([^]]+))?\]`)

// DelegationStage detects delegation intent in LLM output and delegates
// tasks to other agents via the delegation engine.
//
// The stage scans the LLM response for delegation markers like:
//
//	[DELEGATE: mango] Implement the X feature
//	[DELEGATE: mango | code_implementation] Fix the bug in Y
//
// When a marker is found, the stage:
// 1. Creates a task via the delegation engine
// 2. Publishes a <agent>.task.created event to NATS
// 3. Strips the marker from the response (user doesn't see it)
//
// If no delegation marker is found, the stage is a no-op (pass-through).
type DelegationStage struct {
	// Engine is the delegation engine for creating tasks.
	Engine *delegation.Engine

	// StripMarkers controls whether delegation markers are removed from
	// the LLM response before passing to the next stage.
	StripMarkers bool
}

// Name returns the stage identifier.
func (s *DelegationStage) Name() string {
	return "delegation"
}

// Rollback is a no-op for delegation — tasks are already published.
func (s *DelegationStage) Rollback(ctx context.Context, rc *RunContext) error {
	return nil
}

// Validate checks that the delegation engine is available.
// If no engine is set, the stage skips (no-op).
func (s *DelegationStage) Validate(rc *RunContext) error {
	return nil // Engine is optional — no-op if nil
}

// Execute scans the LLM response for delegation markers and delegates tasks.
func (s *DelegationStage) Execute(ctx context.Context, rc *RunContext) (*RunContext, *StageResult, error) {
	if s.Engine == nil || rc.LLMResponse == "" {
		// No engine or no response — pass through
		return rc, &StageResult{
			StageName: s.Name(),
			Success:   true,
			Data:      map[string]any{"skipped": true, "reason": "no_engine"},
		}, nil
	}

	// Scan for delegation markers
	matches := delegationPattern.FindAllStringSubmatch(rc.LLMResponse, -1)
	if len(matches) == 0 {
		// No delegation intent — pass through
		return rc, &StageResult{
			StageName: s.Name(),
			Success:   true,
			Data:      map[string]any{"delegations": 0},
		}, nil
	}

	// Process each delegation marker
	delegatedTasks := make([]string, 0, len(matches))
	cleanedResponse := rc.LLMResponse

	for _, match := range matches {
		fullMatch := match[0]    // e.g., "[DELEGATE: mango | code_implementation]"
		agentID := match[1]     // e.g., "mango"
		taskType := match[2]    // e.g., "code_implementation" (optional)

		if taskType == "" {
			taskType = "general"
		}
		taskType = strings.TrimSpace(taskType)
		agentID = strings.TrimSpace(agentID)

		// Extract description from remaining context
		description := extractDelegationDescription(rc.LLMResponse, fullMatch)

		// Delegate the task
		contextData := map[string]any{
			"source_run_id": rc.RunID,
			"source_agent":  rc.Agent,
			"session_id":    rc.SessionID,
		}

		delegateTask, err := s.Engine.Delegate(ctx, rc.Agent, agentID, taskType, description, contextData)
		if err != nil {
			log.Printf("[DELEGATION] failed to delegate to %s: %v", agentID, err)
			continue
		}

		delegatedTasks = append(delegatedTasks, delegateTask.ID)

		// Add event to RunContext
		rc.Events = append(rc.Events, event.Event{
			ID:        fmt.Sprintf("delegation-%s", delegateTask.ID),
			Type:      agentID + ".task.created",
			Source:    rc.Agent,
			Timestamp: time.Now().Format(time.RFC3339),
			Payload: map[string]any{
				"task_id":       delegateTask.ID,
				"delegated_by":  rc.Agent,
				"delegated_to":  agentID,
				"task_type":     taskType,
				"description":   description,
			},
		})

		log.Printf("[DELEGATION] %s delegated task %s to %s (type: %s)", rc.Agent, delegateTask.ID, agentID, taskType)
	}

	// Strip delegation markers from the response
	if s.StripMarkers {
		cleanedResponse = delegationPattern.ReplaceAllString(cleanedResponse, "")
		rc.LLMResponse = cleanedResponse
	}

	return rc, &StageResult{
		StageName: s.Name(),
		Success:   true,
		Data: map[string]any{
			"delegations":        len(delegatedTasks),
			"delegated_task_ids": delegatedTasks,
		},
	}, nil
}

// extractDelegationDescription tries to extract a task description from
// the text following the delegation marker.
func extractDelegationDescription(text, marker string) string {
	// Find the marker position
	markerEnd := -1
	for i := 0; i <= len(text)-len(marker); i++ {
		if text[i:i+len(marker)] == marker {
			markerEnd = i + len(marker)
			break
		}
	}

	if markerEnd == -1 || markerEnd >= len(text) {
		return "Delegated task"
	}

	// Get remaining text on the same line
	remaining := text[markerEnd:]
	end := len(remaining)
	for i, c := range remaining {
		if c == '\n' {
			end = i
			break
		}
	}

	desc := remaining[:end]
	if len(desc) > 200 {
		desc = desc[:200] + "..."
	}

	if desc == "" {
		return "Delegated task"
	}

	// Clean up leading space
	if len(desc) > 0 && desc[0] == ' ' {
		desc = desc[1:]
	}

	return desc
}