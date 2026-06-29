package v2

import (
	"context"
	"strings"
	"testing"
)

// delegConfigWithAgents builds a delegation config whose workflow knows its roster.
func delegConfigWithAgents(agents []AgentConfig) *WorkflowConfig {
	cfg := delegConfig()
	cfg.Agents = agents
	return cfg
}

func TestHandleDelegateRejectsUnknownAgent(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	engine := NewEngine(delegConfigWithAgents([]AgentConfig{{Name: "mango", Availability: "online"}}), nil, dm)
	engine.GetState().Plan = &PlanGraph{Tasks: []PlanTask{{ID: "T2", Agent: "ghost"}}}

	msg := engine.handleDelegate(context.Background(), &ToolRequest{Tool: "delegate", Input: map[string]any{"task_id": "T2"}})
	if !strings.Contains(msg, "not registered") || !strings.Contains(msg, "mango") {
		t.Fatalf("expected unknown-agent message listing known agents, got %q", msg)
	}
	if len(engine.GetState().Delegations) != 0 {
		t.Fatalf("no delegation should be recorded for an unknown agent")
	}
}

func TestHandleDelegateRejectsOfflineAgent(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	engine := NewEngine(delegConfigWithAgents([]AgentConfig{{Name: "mango", Availability: "offline"}}), nil, dm)
	engine.GetState().Plan = &PlanGraph{Tasks: []PlanTask{{ID: "T2", Agent: "mango"}}}

	msg := engine.handleDelegate(context.Background(), &ToolRequest{Tool: "delegate", Input: map[string]any{"task_id": "T2"}})
	if !strings.Contains(msg, "offline") {
		t.Fatalf("expected offline message, got %q", msg)
	}
	if len(engine.GetState().Delegations) != 0 {
		t.Fatalf("no delegation should be recorded for an offline agent")
	}
}

func TestHandleDelegateAllowsRegisteredOnlineAgent(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	engine := NewEngine(delegConfigWithAgents([]AgentConfig{{Name: "mango", Availability: "online"}}), nil, dm)
	engine.GetState().Plan = &PlanGraph{Tasks: []PlanTask{{ID: "T2", Agent: "mango"}}}

	msg := engine.handleDelegate(context.Background(), &ToolRequest{Tool: "delegate", Input: map[string]any{"task_id": "T2"}})
	if strings.Contains(msg, "not registered") || strings.Contains(msg, "offline") {
		t.Fatalf("registered online agent should be allowed, got %q", msg)
	}
	if len(engine.GetState().Delegations) != 1 {
		t.Fatalf("delegation should be recorded, got %d", len(engine.GetState().Delegations))
	}
}

// With an empty roster (agents resolved dynamically), the guard is skipped.
func TestHandleDelegateSkipsCheckWhenRosterEmpty(t *testing.T) {
	dm := NewDelegationManager("s", "s.complete")
	engine := NewEngine(delegConfig(), nil, dm) // no Agents → empty registry
	engine.GetState().Plan = &PlanGraph{Tasks: []PlanTask{{ID: "T2", Agent: "anyone"}}}

	msg := engine.handleDelegate(context.Background(), &ToolRequest{Tool: "delegate", Input: map[string]any{"task_id": "T2"}})
	if strings.Contains(msg, "not registered") {
		t.Fatalf("dynamic roster should not reject, got %q", msg)
	}
	if len(engine.GetState().Delegations) != 1 {
		t.Fatalf("delegation should proceed with empty roster, got %d", len(engine.GetState().Delegations))
	}
}
