// Package agent provides the V1 deterministic placeholder agent.
// It does NOT make LLM calls — it returns a structured result to prove
// the event lifecycle works end-to-end. Real LLM calls are V2.
package agent

import (
	"encoding/json"
	"time"
)

// PlaceholderInput is the input to the V1 placeholder agent.
type PlaceholderInput struct {
	Task    string `json:"task"`
	Project string `json:"project"`
	Agent   string `json:"agent"`
	Context string `json:"context,omitempty"`
}

// PlaceholderOutput is the structured result from the V1 placeholder agent.
type PlaceholderOutput struct {
	Status          string   `json:"status"`
	Summary         string   `json:"summary"`
	ContextReceived bool     `json:"context_received"`
	Actions         []string `json:"actions"`
}

// RunPlaceholder executes the deterministic V1 agent placeholder.
// It always succeeds and returns a predictable structured result.
func RunPlaceholder(input PlaceholderInput) PlaceholderOutput {
	hasContext := input.Context != ""
	actions := []string{
		"validated event lifecycle",
		"emitted agent output",
		"persisted event log",
	}
	if hasContext {
		actions = append(actions, "incorporated remembrance context")
	}

	return PlaceholderOutput{
		Status:          "completed",
		Summary:         "V1 lifecycle task executed successfully.",
		ContextReceived: hasContext,
		Actions:         actions,
	}
}

// RunPlaceholderWithDelay simulates agent processing time.
func RunPlaceholderWithDelay(input PlaceholderInput, delay time.Duration) PlaceholderOutput {
	time.Sleep(delay)
	return RunPlaceholder(input)
}

// ToJSON serializes placeholder output.
func (o PlaceholderOutput) ToJSON() ([]byte, error) {
	return json.Marshal(o)
}
