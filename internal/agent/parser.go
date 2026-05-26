// Package agent provides tool request parsing for V3 controlled tool execution.
// It parses LLM output into either a final response or a structured tool request.
package agent

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/emaharmony/prism/internal/tool"
)

// AgentResponseType distinguishes between a final text response and a tool request.
type AgentResponseType string

const (
	// ResponseFinal means the agent is done and returning text output.
	ResponseFinal AgentResponseType = "final"
	// ResponseToolRequest means the agent wants to call a tool before completing.
	ResponseToolRequest AgentResponseType = "tool_request"
)

// AgentResponse represents the parsed output from the LLM. It can be either
// a final text response or a request to execute a tool.
type AgentResponse struct {
	Type       AgentResponseType `json:"type"`
	Content    string            `json:"content,omitempty"`
	ToolName   string            `json:"tool,omitempty"`
	ToolInput  map[string]any   `json:"input,omitempty"`
}

// ParseAgentOutput attempts to parse LLM output as JSON. It recognizes two shapes:
//
//	{"type": "final", "content": "..."}     → final response
//	{"type": "tool_request", "tool": "read_file", "input": {"path": "README.md"}}
//	                                      → tool request
//
// If the output is not valid JSON or doesn't match either shape, it is treated
// as a final response with the raw text as content (fallback behavior).
func ParseAgentOutput(raw string) AgentResponse {
	// Trim whitespace and markdown code fences
	text := strings.TrimSpace(raw)

	// Try to extract JSON from markdown code fences if present
	text = extractJSON(text)

	var parsed AgentResponse
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// Not valid JSON — treat as final text response
		return AgentResponse{
			Type:    ResponseFinal,
			Content: raw,
		}
	}

	switch parsed.Type {
	case "final":
		return AgentResponse{
			Type:    ResponseFinal,
			Content: parsed.Content,
		}
	case "tool_request":
		if parsed.ToolName == "" {
			// Missing tool name — fall back to final response
			return AgentResponse{
				Type:    ResponseFinal,
				Content: raw,
			}
		}
		return parsed
	default:
		// Unknown type — fall back to final response
		return AgentResponse{
			Type:    ResponseFinal,
			Content: raw,
		}
	}
}

// extractJSON tries to pull JSON content out of markdown code fences.
// If no fences are found, it returns the text unchanged.
func extractJSON(text string) string {
	// Check for ```json ... ``` blocks
	if idx := strings.Index(text, "```json"); idx != -1 {
		start := idx + 7 // skip ```json
		if end := strings.Index(text[start:], "```"); end != -1 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	// Check for ``` ... ``` blocks
	if idx := strings.Index(text, "```"); idx != -1 {
		start := idx + 3
		// Skip the language tag line if present
		if newlineIdx := strings.Index(text[start:], "\n"); newlineIdx != -1 && newlineIdx < 30 {
			start = start + newlineIdx + 1
		}
		if end := strings.Index(text[start:], "```"); end != -1 {
			return strings.TrimSpace(text[start : start+end])
		}
	}
	return text
}

// BuildToolPromptSuffix appends tool instructions to a prompt, telling the
// model to return JSON with either a "final" or "tool_request" shape.
// V4: Includes approval-gated mutation instructions.
// V28: Includes tool descriptions and parameter schemas so the model knows how to call them.
func BuildToolPromptSuffix(toolInfos []tool.ToolInfo) string {
	var sb strings.Builder
	sb.WriteString("\n\n## Tool Instructions\n")
	sb.WriteString("You have access to the following tools:\n\n")
	for _, ti := range toolInfos {
		sb.WriteString(fmt.Sprintf("- **%s**: %s", ti.Name, ti.Description))
		if len(ti.Schema.Input) > 0 {
			sb.WriteString(" Parameters: ")
			first := true
			for pname, spec := range ti.Schema.Input {
				if !first {
					sb.WriteString(", ")
				}
				first = false
				req := ""
				if spec.Required {
					req = " (required)"
				}
				sb.WriteString(fmt.Sprintf("%s: %s%s", pname, spec.Type, req))
			}
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\nRespond with JSON in one of these shapes:\n")
	sb.WriteString("- Final response: {\"type\": \"final\", \"content\": \"your text here\"}\n")
	sb.WriteString("- Tool request: {\"type\": \"tool_request\", \"tool\": \"tool_name\", \"input\": {\"key\": \"value\"}}\n")
	sb.WriteString("\nYou may make at most ONE tool request per response. If you need multiple tool calls, respond with the first one and the system will provide results.\n")
	sb.WriteString("\n### V4: File Mutation Approval\n")
	sb.WriteString("File mutations require approval. You may propose a file change using write_file_proposal.\n")
	sb.WriteString("Do not claim that a file was changed unless Prism returns a successful mutation.applied event.\n")
	sb.WriteString("If your proposal is approved, the file will be written by Prism and you will receive a mutation.applied confirmation.\n")
	return sb.String()
}