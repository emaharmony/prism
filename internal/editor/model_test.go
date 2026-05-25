package editor

import (
	"strings"
	"testing"

	"github.com/emaharmony/prism/internal/orchestrator"
)

func TestConfigToEditorState(t *testing.T) {
	config := &orchestrator.Config{
		Agents: []orchestrator.AgentConfig{
			{ID: "lumi", Role: "lead", Provider: "ollama", Model: "glm-5.1:cloud", Primary: true, Capabilities: []string{"plan", "delegate", "review"}},
			{ID: "mango", Role: "coder", Provider: "ollama", Model: "deepseek-v4-pro:cloud", Capabilities: []string{"code", "test"}, Subscriptions: []string{"lumi.task.created"}},
		},
	}

	state := ConfigToEditorState(config)

	if len(state.Nodes) != 2 {
		t.Errorf("expected 2 nodes, got %d", len(state.Nodes))
	}

	// Check lumi node
	lumi := state.Nodes[0]
	if lumi.ID != "lumi" {
		t.Errorf("expected lumi, got %s", lumi.ID)
	}
	if lumi.Role != "lead" {
		t.Errorf("expected lead, got %s", lumi.Role)
	}
	if !lumi.Primary {
		t.Error("expected lumi to be primary")
	}
	if lumi.Position.X == 0 && lumi.Position.Y == 0 {
		t.Error("expected non-zero position for lumi")
	}

	// Check mango node
	mango := state.Nodes[1]
	if mango.ID != "mango" {
		t.Errorf("expected mango, got %s", mango.ID)
	}

	// Check edges exist (at least delegation edge from primary)
	if len(state.Edges) == 0 {
		t.Error("expected at least one edge")
	}

	// Check delegation edge from lumi to mango
	foundDelegation := false
	for _, e := range state.Edges {
		if e.From == "lumi" && e.To == "mango" && e.Type == "delegation" {
			foundDelegation = true
		}
	}
	if !foundDelegation {
		t.Error("expected delegation edge from lumi to mango")
	}
}

func TestConfigToEditorState_SingleAgent(t *testing.T) {
	config := &orchestrator.Config{
		Agents: []orchestrator.AgentConfig{
			{ID: "solo", Role: "lead", Model: "gpt-4o", Primary: true},
		},
	}

	state := ConfigToEditorState(config)

	if len(state.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(state.Nodes))
	}
	if len(state.Edges) != 0 {
		t.Errorf("expected 0 edges for single agent, got %d", len(state.Edges))
	}
}

func TestEditorStateToConfig(t *testing.T) {
	state := &EditorState{
		Nodes: []EditorNode{
			{ID: "lumi", Type: "agent", Role: "lead", Model: "glm-5.1:cloud", Primary: true, Capabilities: []string{"plan", "delegate"}},
			{ID: "mango", Type: "agent", Role: "coder", Model: "deepseek-v4-pro:cloud", Capabilities: []string{"code", "test"}},
		},
		Edges: []EditorEdge{
			{ID: "lumi-mango-delegation", From: "lumi", To: "mango", Type: "delegation"},
		},
	}

	config := EditorStateToConfig(state)

	if len(config.Agents) != 2 {
		t.Errorf("expected 2 agents, got %d", len(config.Agents))
	}

	// Check lumi
	if config.Agents[0].ID != "lumi" {
		t.Errorf("expected lumi, got %s", config.Agents[0].ID)
	}
	if config.Agents[0].Role != "lead" {
		t.Errorf("expected lead, got %s", config.Agents[0].Role)
	}

	// Check mango has subscription from edge
	if len(config.Agents[1].Subscriptions) == 0 {
		t.Error("expected mango to have subscriptions derived from edges")
	}

	found := false
	for _, sub := range config.Agents[1].Subscriptions {
		if sub == "lumi.task.completed" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected mango to subscribe to lumi.task.completed, got %v", config.Agents[1].Subscriptions)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	original := &orchestrator.Config{
		Agents: []orchestrator.AgentConfig{
			{ID: "lumi", Role: "lead", Provider: "ollama", Model: "glm-5.1:cloud", Primary: true, Capabilities: []string{"plan", "delegate", "review"}},
			{ID: "mango", Role: "coder", Provider: "ollama", Model: "deepseek-v4-pro:cloud", Capabilities: []string{"code", "test"}, Subscriptions: []string{"lumi.task.created"}},
		},
	}

	// Config → EditorState
	state := ConfigToEditorState(original)

	// EditorState → Config
	roundTripped := EditorStateToConfig(state)

	if len(roundTripped.Agents) != 2 {
		t.Errorf("expected 2 agents after round trip, got %d", len(roundTripped.Agents))
	}

	// Check lumi round-trips correctly
	if roundTripped.Agents[0].ID != "lumi" {
		t.Errorf("expected lumi, got %s", roundTripped.Agents[0].ID)
	}
	if roundTripped.Agents[0].Role != "lead" {
		t.Errorf("expected lead, got %s", roundTripped.Agents[0].Role)
	}
	if roundTripped.Agents[0].Model != "glm-5.1:cloud" {
		t.Errorf("expected glm-5.1:cloud, got %s", roundTripped.Agents[0].Model)
	}
	if !roundTripped.Agents[0].Primary {
		t.Error("expected primary to be true")
	}

	// Check mango round-trips
	if roundTripped.Agents[1].ID != "mango" {
		t.Errorf("expected mango, got %s", roundTripped.Agents[1].ID)
	}
}

func TestWriteConfigYAML(t *testing.T) {
	state := &EditorState{
		Nodes: []EditorNode{
			{ID: "lumi", Type: "agent", Role: "lead", Model: "glm-5.1:cloud", Primary: true},
			{ID: "mango", Type: "agent", Role: "coder", Model: "deepseek-v4-pro:cloud"},
		},
		Edges: []EditorEdge{},
	}

	yaml, err := WriteConfigYAML(state)
	if err != nil {
		t.Fatalf("WriteConfigYAML error: %v", err)
	}

	if !strings.Contains(yaml, "lumi") {
		t.Error("expected yaml to contain lumi")
	}
	if !strings.Contains(yaml, "mango") {
		t.Error("expected yaml to contain mango")
	}
	if !strings.Contains(yaml, "lead") {
		t.Error("expected yaml to contain lead role")
	}
	if !strings.Contains(yaml, "glm-5.1:cloud") {
		t.Error("expected yaml to contain model")
	}
}

func TestValidateEditorState(t *testing.T) {
	tests := []struct {
		name    string
		state   *EditorState
		wantErr int
	}{
		{
			name: "valid state",
			state: &EditorState{
				Nodes: []EditorNode{
					{ID: "lumi", Type: "agent", Role: "lead", Primary: true},
					{ID: "mango", Type: "agent", Role: "coder"},
				},
				Edges: []EditorEdge{
					{ID: "e1", From: "lumi", To: "mango", Type: "delegation"},
				},
			},
			wantErr: 0,
		},
		{
			name: "duplicate node ID",
			state: &EditorState{
				Nodes: []EditorNode{
					{ID: "lumi", Type: "agent", Primary: true},
					{ID: "lumi", Type: "agent"},
				},
			},
			wantErr: 1,
		},
		{
			name: "invalid edge source",
			state: &EditorState{
				Nodes: []EditorNode{
					{ID: "lumi", Type: "agent", Primary: true},
				},
				Edges: []EditorEdge{
					{ID: "e1", From: "ghost", To: "lumi", Type: "delegation"},
				},
			},
			wantErr: 1,
		},
		{
			name: "no primary agent",
			state: &EditorState{
				Nodes: []EditorNode{
					{ID: "lumi", Type: "agent"},
					{ID: "mango", Type: "agent"},
				},
			},
			wantErr: 1,
		},
		{
			name: "multiple primary agents",
			state: &EditorState{
				Nodes: []EditorNode{
					{ID: "lumi", Type: "agent", Primary: true},
					{ID: "mango", Type: "agent", Primary: true},
				},
			},
			wantErr: 1,
		},
		{
			name: "invalid agent ID",
			state: &EditorState{
				Nodes: []EditorNode{
					{ID: "My Agent", Type: "agent", Primary: true},
				},
			},
			wantErr: 1,
		},
		{
			name: "invalid edge type",
			state: &EditorState{
				Nodes: []EditorNode{
					{ID: "lumi", Type: "agent", Primary: true},
					{ID: "mango", Type: "agent"},
				},
				Edges: []EditorEdge{
					{ID: "e1", From: "lumi", To: "mango", Type: "teleport"},
				},
			},
			wantErr: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errors := ValidateEditorState(tt.state)
			if len(errors) != tt.wantErr {
				t.Errorf("expected %d errors, got %d: %v", tt.wantErr, len(errors), errors)
			}
		})
	}
}

func TestAddNode(t *testing.T) {
	state := &EditorState{}

	err := state.AddNode(EditorNode{ID: "lumi", Role: "lead", Primary: true})
	if err != nil {
		t.Fatalf("AddNode error: %v", err)
	}
	if len(state.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(state.Nodes))
	}
	if state.Nodes[0].Label != "LUMI" {
		t.Errorf("expected LUMI label, got %s", state.Nodes[0].Label)
	}

	// Duplicate ID should fail
	err = state.AddNode(EditorNode{ID: "lumi"})
	if err == nil {
		t.Error("expected error for duplicate node ID")
	}
}

func TestRemoveNode(t *testing.T) {
	state := &EditorState{
		Nodes: []EditorNode{
			{ID: "lumi", Primary: true},
			{ID: "mango"},
		},
		Edges: []EditorEdge{
			{ID: "e1", From: "lumi", To: "mango"},
		},
	}

	err := state.RemoveNode("mango")
	if err != nil {
		t.Fatalf("RemoveNode error: %v", err)
	}
	if len(state.Nodes) != 1 {
		t.Errorf("expected 1 node after removal, got %d", len(state.Nodes))
	}
	if len(state.Edges) != 0 {
		t.Errorf("expected 0 edges after removing connected node, got %d", len(state.Edges))
	}

	// Non-existent node
	err = state.RemoveNode("ghost")
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestAddEdge(t *testing.T) {
	state := &EditorState{
		Nodes: []EditorNode{
			{ID: "lumi"},
			{ID: "mango"},
		},
	}

	err := state.AddEdge(EditorEdge{ID: "e1", From: "lumi", To: "mango", Type: "delegation"})
	if err != nil {
		t.Fatalf("AddEdge error: %v", err)
	}
	if len(state.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(state.Edges))
	}
	if state.Edges[0].Style != "solid" {
		t.Errorf("delegation edges should default to solid style, got %s", state.Edges[0].Style)
	}

	// Edge with unknown source
	err = state.AddEdge(EditorEdge{ID: "e2", From: "ghost", To: "mango"})
	if err == nil {
		t.Error("expected error for unknown source")
	}

	// Duplicate edge ID
	err = state.AddEdge(EditorEdge{ID: "e1", From: "lumi", To: "mango"})
	if err == nil {
		t.Error("expected error for duplicate edge ID")
	}
}

func TestRemoveEdge(t *testing.T) {
	state := &EditorState{
		Nodes: []EditorNode{{ID: "lumi"}, {ID: "mango"}},
		Edges: []EditorEdge{{ID: "e1", From: "lumi", To: "mango"}},
	}

	err := state.RemoveEdge("e1")
	if err != nil {
		t.Fatalf("RemoveEdge error: %v", err)
	}
	if len(state.Edges) != 0 {
		t.Errorf("expected 0 edges after removal, got %d", len(state.Edges))
	}

	// Non-existent edge
	err = state.RemoveEdge("ghost")
	if err == nil {
		t.Error("expected error for non-existent edge")
	}
}

func TestUpdateNode(t *testing.T) {
	state := &EditorState{
		Nodes: []EditorNode{
			{ID: "lumi", Role: "lead", Model: "gpt-4o"},
		},
	}

	err := state.UpdateNode("lumi", EditorNode{Model: "glm-5.1:cloud", Position: Position{X: 200, Y: 300}})
	if err != nil {
		t.Fatalf("UpdateNode error: %v", err)
	}

	if state.Nodes[0].Model != "glm-5.1:cloud" {
		t.Errorf("expected model update, got %s", state.Nodes[0].Model)
	}
	if state.Nodes[0].Position.X != 200 {
		t.Errorf("expected position update X=200, got %d", state.Nodes[0].Position.X)
	}
	// Role should not change (empty string is not applied)
	if state.Nodes[0].Role != "lead" {
		t.Errorf("expected role unchanged, got %s", state.Nodes[0].Role)
	}

	// Non-existent node
	err = state.UpdateNode("ghost", EditorNode{})
	if err == nil {
		t.Error("expected error for non-existent node")
	}
}

func TestIsValidAgentID(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"lumi", true},
		{"mango", true},
		{"agent-1", true},
		{"a1", true},
		{"My Agent", false},
		{"", false},
		{"1agent", true},
		{"agent!", false},
		{"agent name", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := isValidAgentID(tt.id)
			if got != tt.want {
				t.Errorf("isValidAgentID(%q) = %v, want %v", tt.id, got, tt.want)
			}
		})
	}
}

func TestEdgeStyles(t *testing.T) {
	state := &EditorState{
		Nodes: []EditorNode{{ID: "a"}, {ID: "b"}},
	}

	styles := map[string]string{
		"delegation": "solid",
		"review":     "dashed",
		"approval":   "bold",
		"event":      "dotted",
	}

	for edgeType := range styles {
		err := state.AddEdge(EditorEdge{ID: "e-" + edgeType, From: "a", To: "b", Type: edgeType})
		if err != nil {
			t.Fatalf("AddEdge(%s) error: %v", edgeType, err)
		}
	}

	for _, edge := range state.Edges {
		want, ok := styles[edge.Type]
		if !ok {
			continue
		}
		if edge.Style != want {
			t.Errorf("edge type %s: expected style %s, got %s", edge.Type, want, edge.Style)
		}
	}
}