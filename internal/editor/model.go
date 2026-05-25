// Package editor provides the visual workflow editor model and config round-trip.
//
// V25: Visual Workflow Editor
//
// The editor model represents Prism's agent configuration as a directed graph
// of nodes (agents, approvals, processes) and edges (delegation, review,
// approval, event flows). Changes to the graph write back to prism.yaml.
package editor

import (
	"fmt"
	"os"
	"strings"

	"github.com/emaharmony/prism/internal/orchestrator"
	"gopkg.in/yaml.v3"
)

// Position represents a node's position on the canvas.
type Position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// EditorNode represents a node in the workflow editor.
type EditorNode struct {
	ID            string   `json:"id"`
	Type          string   `json:"type"`           // "agent", "approval", "process"
	Label         string   `json:"label"`
	Role          string   `json:"role"`           // lead, coder, researcher, orchestrator
	Model         string   `json:"model"`
	Provider      string   `json:"provider"`
	Primary       bool     `json:"primary"`
	Capabilities  []string `json:"capabilities"`
	Subscriptions []string `json:"subscriptions"`
	Context       []string `json:"context"`
	Position      Position `json:"position"`
}

// EditorEdge represents a connection between two nodes.
type EditorEdge struct {
	ID     string `json:"id"`
	From   string `json:"from"`   // source node ID
	To     string `json:"to"`      // target node ID
	Type   string `json:"type"`    // "delegation", "review", "approval", "event"
	Label  string `json:"label"`   // optional display label
	Style  string `json:"style"`  // "solid", "dashed", "dotted", "bold"
}

// EditorState is the full state of the workflow editor.
type EditorState struct {
	Nodes []EditorNode `json:"nodes"`
	Edges []EditorEdge  `json:"edges"`
}

// ConfigToEditorState converts Prism agent config into an EditorState.
// It creates nodes for each agent and edges based on subscriptions.
func ConfigToEditorState(config *orchestrator.Config) *EditorState {
	state := &EditorState{}

	// Auto-layout positions
	cols := 3
	if len(config.Agents) > 6 {
		cols = 4
	}

	for i, agent := range config.Agents {
		col := i % cols
		row := i / cols
		x := 100 + col*250
		y := 100 + row*180

		node := EditorNode{
			ID:            agent.ID,
			Type:          "agent",
			Label:         strings.ToUpper(agent.ID),
			Role:          agent.Role,
			Model:         agent.Model,
			Provider:      agent.Provider,
			Primary:       agent.Primary,
			Capabilities:  agent.Capabilities,
			Subscriptions: agent.Subscriptions,
			Context:       agent.Context,
			Position:      Position{X: x, Y: y},
		}
		state.Nodes = append(state.Nodes, node)

		// Create delegation edges from subscriptions
		for _, sub := range agent.Subscriptions {
			// Parse subscription: "mango.task.completed" → from=mango
			parts := strings.SplitN(sub, ".", 2)
			fromAgent := parts[0]
			edgeType := "event"
			style := "solid"
			label := ""

			// Classify edge type based on subscription pattern
			if strings.Contains(sub, ".task.") || strings.Contains(sub, ".approval.") {
				edgeType = "delegation"
				style = "solid"
				label = sub
			} else if strings.Contains(sub, ".agent.output") {
				edgeType = "review"
				style = "dashed"
				label = "review"
			}

			state.Edges = append(state.Edges, EditorEdge{
				ID:    fmt.Sprintf("%s-%s-%s", fromAgent, agent.ID, edgeType),
				From:  fromAgent,
				To:    agent.ID,
				Type:  edgeType,
				Label: label,
				Style: style,
			})
		}

		// Primary agent gets delegation edges to all non-primary agents
		if agent.Primary {
			for j, other := range config.Agents {
				if other.ID == agent.ID {
					continue
				}
				// Check if delegation edge already exists
				edgeID := fmt.Sprintf("%s-%s-delegation", agent.ID, other.ID)
				exists := false
				for _, e := range state.Edges {
					if e.ID == edgeID {
						exists = true
						break
					}
				}
				if !exists {
					state.Edges = append(state.Edges, EditorEdge{
						ID:    edgeID,
						From:  agent.ID,
						To:    other.ID,
						Type:  "delegation",
						Label: "delegate",
						Style: "solid",
					})
				}
				_ = j
			}
		}
	}

	return state
}

// EditorStateToConfig converts an EditorState back into a Prism config.
// Only agent-type nodes are written to the config.
func EditorStateToConfig(state *EditorState) *orchestrator.Config {
	config := &orchestrator.Config{}

	for _, node := range state.Nodes {
		if node.Type != "agent" {
			continue
		}

		agent := orchestrator.AgentConfig{
			ID:            node.ID,
			Role:          node.Role,
			Provider:      node.Provider,
			Model:         node.Model,
			Primary:       node.Primary,
			Capabilities:  node.Capabilities,
			Subscriptions: node.Subscriptions,
			Context:       node.Context,
		}

		// If subscriptions are empty, derive from edges
		if len(agent.Subscriptions) == 0 {
			for _, edge := range state.Edges {
				if edge.To == node.ID && edge.Type == "delegation" {
					agent.Subscriptions = append(agent.Subscriptions,
						fmt.Sprintf("%s.task.completed", edge.From))
				}
			}
		}

		config.Agents = append(config.Agents, agent)
	}

	return config
}

// WriteConfigYAML converts an EditorState to YAML string.
func WriteConfigYAML(state *EditorState) (string, error) {
	config := EditorStateToConfig(state)

	// Build a YAML-friendly structure
	type agentYAML struct {
		ID            string   `yaml:"id"`
		Role          string   `yaml:"role"`
		Provider      string   `yaml:"provider,omitempty"`
		Model         string   `yaml:"model"`
		Primary       bool     `yaml:"primary,omitempty"`
		Capabilities  []string `yaml:"capabilities,omitempty"`
		Subscriptions []string `yaml:"subscriptions,omitempty"`
		Context       []string `yaml:"context,omitempty"`
	}

	type configYAML struct {
		Agents []agentYAML `yaml:"agents"`
	}

	var agents []agentYAML
	for _, a := range config.Agents {
		agents = append(agents, agentYAML{
			ID:            a.ID,
			Role:          a.Role,
			Provider:      a.Provider,
			Model:         a.Model,
			Primary:       a.Primary,
			Capabilities:  a.Capabilities,
			Subscriptions: a.Subscriptions,
			Context:       a.Context,
		})
	}

	data := configYAML{Agents: agents}

	out, err := yaml.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal config: %w", err)
	}

	return string(out), nil
}

// ValidateEditorState checks an EditorState for common errors.
func ValidateEditorState(state *EditorState) []string {
	var errors []string

	// Check for duplicate node IDs
	seen := make(map[string]bool)
	for _, node := range state.Nodes {
		if seen[node.ID] {
			errors = append(errors, fmt.Sprintf("duplicate node ID: %s", node.ID))
		}
		seen[node.ID] = true
	}

	// Check that edges reference valid nodes
	for _, edge := range state.Edges {
		if !seen[edge.From] {
			errors = append(errors, fmt.Sprintf("edge %s references unknown source: %s", edge.ID, edge.From))
		}
		if !seen[edge.To] {
			errors = append(errors, fmt.Sprintf("edge %s references unknown target: %s", edge.ID, edge.To))
		}
	}

	// Check for exactly one primary agent
	primaryCount := 0
	for _, node := range state.Nodes {
		if node.Type == "agent" && node.Primary {
			primaryCount++
		}
	}
	if primaryCount == 0 && len(state.Nodes) > 0 {
		errors = append(errors, "no primary agent designated")
	} else if primaryCount > 1 {
		errors = append(errors, fmt.Sprintf("multiple primary agents (%d)", primaryCount))
	}

	// Check agent IDs match regex
	for _, node := range state.Nodes {
		if node.Type == "agent" {
			if !isValidAgentID(node.ID) {
				errors = append(errors, fmt.Sprintf("invalid agent ID: %s (must match [a-z0-9][a-z0-9-]*)", node.ID))
			}
		}
	}

	// Check valid edge types
	validEdgeTypes := map[string]bool{
		"delegation": true,
		"review":     true,
		"approval":   true,
		"event":      true,
	}
	for _, edge := range state.Edges {
		if !validEdgeTypes[edge.Type] {
			errors = append(errors, fmt.Sprintf("invalid edge type: %s (edge %s)", edge.Type, edge.ID))
		}
	}

	return errors
}

// isValidAgentID checks if an agent ID matches the config validation regex.
func isValidAgentID(id string) bool {
	if len(id) == 0 {
		return false
	}
	if id[0] < 'a' || id[0] > 'z' {
		if id[0] < '0' || id[0] > '9' {
			return false
		}
	}
	for _, c := range id {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return true
}

// AddNode adds a node to the editor state with validation.
func (s *EditorState) AddNode(node EditorNode) error {
	for _, existing := range s.Nodes {
		if existing.ID == node.ID {
			return fmt.Errorf("node with ID %q already exists", node.ID)
		}
	}
	if node.Type == "" {
		node.Type = "agent"
	}
	if node.Label == "" {
		node.Label = strings.ToUpper(node.ID)
	}
	s.Nodes = append(s.Nodes, node)
	return nil
}

// RemoveNode removes a node and all edges connected to it.
func (s *EditorState) RemoveNode(id string) error {
	found := false
	newNodes := make([]EditorNode, 0, len(s.Nodes))
	for _, n := range s.Nodes {
		if n.ID == id {
			found = true
			continue
		}
		newNodes = append(newNodes, n)
	}
	if !found {
		return fmt.Errorf("node %q not found", id)
	}
	s.Nodes = newNodes

	// Remove connected edges
	newEdges := make([]EditorEdge, 0, len(s.Edges))
	for _, e := range s.Edges {
		if e.From != id && e.To != id {
			newEdges = append(newEdges, e)
		}
	}
	s.Edges = newEdges
	return nil
}

// AddEdge adds an edge to the editor state with validation.
func (s *EditorState) AddEdge(edge EditorEdge) error {
	// Check source and target exist
	sourceFound := false
	targetFound := false
	for _, n := range s.Nodes {
		if n.ID == edge.From {
			sourceFound = true
		}
		if n.ID == edge.To {
			targetFound = true
		}
	}
	if !sourceFound {
		return fmt.Errorf("source node %q not found", edge.From)
	}
	if !targetFound {
		return fmt.Errorf("target node %q not found", edge.To)
	}

	// Check for duplicate edge
	for _, existing := range s.Edges {
		if existing.ID == edge.ID {
			return fmt.Errorf("edge with ID %q already exists", edge.ID)
		}
	}

	if edge.Type == "" {
		edge.Type = "delegation"
	}
	if edge.Style == "" {
		switch edge.Type {
		case "delegation":
			edge.Style = "solid"
		case "review":
			edge.Style = "dashed"
		case "approval":
			edge.Style = "bold"
		case "event":
			edge.Style = "dotted"
		}
	}

	s.Edges = append(s.Edges, edge)
	return nil
}

// RemoveEdge removes an edge by ID.
func (s *EditorState) RemoveEdge(id string) error {
	found := false
	newEdges := make([]EditorEdge, 0, len(s.Edges))
	for _, e := range s.Edges {
		if e.ID == id {
			found = true
			continue
		}
		newEdges = append(newEdges, e)
	}
	if !found {
		return fmt.Errorf("edge %q not found", id)
	}
	s.Edges = newEdges
	return nil
}

// UpdateNode updates a node's properties by ID.
func (s *EditorState) UpdateNode(id string, updates EditorNode) error {
	for i, n := range s.Nodes {
		if n.ID == id {
			if updates.Label != "" {
				s.Nodes[i].Label = updates.Label
			}
			if updates.Role != "" {
				s.Nodes[i].Role = updates.Role
			}
			if updates.Model != "" {
				s.Nodes[i].Model = updates.Model
			}
			if updates.Provider != "" {
				s.Nodes[i].Provider = updates.Provider
			}
			if updates.Capabilities != nil {
				s.Nodes[i].Capabilities = updates.Capabilities
			}
			if updates.Subscriptions != nil {
				s.Nodes[i].Subscriptions = updates.Subscriptions
			}
			if updates.Context != nil {
				s.Nodes[i].Context = updates.Context
			}
			s.Nodes[i].Primary = updates.Primary
			s.Nodes[i].Position = updates.Position
			return nil
		}
	}
	return fmt.Errorf("node %q not found", id)
}

// WriteConfigToFile writes the editor state as YAML to a file on disk.
// This requires explicit path and should only be called with user confirmation.
func WriteConfigToFile(state *EditorState, path string) error {
	// Validate first
	errors := ValidateEditorState(state)
	if len(errors) > 0 {
		return fmt.Errorf("validation errors: %v", errors)
	}

	yaml, err := WriteConfigYAML(state)
	if err != nil {
		return fmt.Errorf("yaml generation error: %w", err)
	}

	// Write atomically: write to temp file then rename
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(yaml), 0644); err != nil {
		return fmt.Errorf("write temp file error: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath) // cleanup
		return fmt.Errorf("rename error: %w", err)
	}

	return nil
}