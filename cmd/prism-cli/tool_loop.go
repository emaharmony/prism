package main

import (
	"context"
	"fmt"
	"log"

	"github.com/emaharmony/prism/internal/agent"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/tool"
)

const maxToolIterations = 5

// runToolLoop executes a multi-turn tool execution loop.
//
// After each LLM response, it checks whether the agent requested a tool call.
// If so, it executes the tool, feeds the result back as a new prompt, and
// calls the LLM again. This repeats until:
//   - The LLM gives a final text response (no tool request)
//   - The maximum iteration count is reached
//   - A tool requires approval (pauses for human review)
//   - An error occurs
//
// The final text response is returned. Tool call summaries are logged
// and stored in the session for context continuity.
func (cc *conversationContext) runToolLoop(
	runCtx context.Context,
	prompt string,
	agentCfg *orchestrator.AgentConfig,
	llmProvider interface{},
	channelID string,
	placeholderMsgID string,
) (string, []toolCallSummary, error) {
	var summaries []toolCallSummary
	currentPrompt := prompt

	for i := 0; i < maxToolIterations; i++ {
		log.Printf("[TOOL] iteration %d/%d", i+1, maxToolIterations)

		// Call the LLM with the current prompt
		responseText, err := cc.callLLMForToolLoop(runCtx, currentPrompt, agentCfg)
		if err != nil {
			return "", summaries, err
		}

		// Parse the response — is it a tool request or a final response?
		parsed := agent.ParseAgentOutput(responseText)

		if parsed.Type == agent.ResponseFinal {
			// Final text response — we're done
			log.Printf("[TOOL] iteration %d: final response (%d chars)", i+1, len(parsed.Content))
			return parsed.Content, summaries, nil
		}

		if parsed.Type == agent.ResponseToolRequest {
			log.Printf("[TOOL] iteration %d: agent requests tool %q", i+1, parsed.ToolName)

			// Execute the tool
			toolResult, approvalNeeded, err := cc.executeTool(runCtx, parsed, agentCfg)
			if err != nil {
				log.Printf("[TOOL] tool %q failed: %v", parsed.ToolName, err)
				// Feed the error back to the LLM so it can try a different approach
				currentPrompt = formatToolErrorPrompt(parsed.ToolName, err)
				summaries = append(summaries, toolCallSummary{
					Tool:   parsed.ToolName,
					Input:  parsed.ToolInput,
					Status: "error",
					Error:  err.Error(),
				})
				continue
			}

			summary := toolCallSummary{
				Tool:  parsed.ToolName,
				Input: parsed.ToolInput,
			}

			if approvalNeeded {
				log.Printf("[TOOL] tool %q requires approval — pausing for human review", parsed.ToolName)
				summary.Status = "pending_approval"
				summaries = append(summaries, summary)
				return formatApprovalMessage(parsed), summaries, nil
			}

			resultStr := formatToolResult(parsed.ToolName, toolResult)
			summary.Status = "success"
			summary.Result = resultStr
			summaries = append(summaries, summary)

			log.Printf("[TOOL] tool %q succeeded (%d chars result)", parsed.ToolName, len(resultStr))

			// Feed the tool result back to the LLM
			currentPrompt = formatToolResultPrompt(parsed.ToolName, resultStr)
			continue
		}

		// Unknown response type — treat as final
		return responseText, summaries, nil
	}

	// Max iterations reached — return whatever we have
	log.Printf("[TOOL] max iterations (%d) reached, returning last response", maxToolIterations)
	return "", summaries, nil
}

// callLLMForToolLoop makes an LLM call within the tool loop context.
func (cc *conversationContext) callLLMForToolLoop(ctx context.Context, prompt string, agentCfg *orchestrator.AgentConfig) (string, error) {
	// Add tool instructions to the prompt
	availableTools := cc.toolExec.Registry.List()
	if len(availableTools) > 0 {
		prompt += agent.BuildToolPromptSuffix(availableTools)
	}

	// Use the provider from the registry
	prov, err := cc.providers.Get(agentCfg.Model)
	if err != nil {
		return "", fmt.Errorf("no provider for model %s: %w", agentCfg.Model, err)
	}

	// Call Generate directly
	resp, err := prov.Generate(ctx, provider.GenerateRequest{
		Model:       agentCfg.Model,
		Prompt:      prompt,
		Temperature: 0.7,
		MaxTokens:   4096,
	})
	if err != nil {
		return "", fmt.Errorf("LLM call failed: %w", err)
	}

	return resp.Text, nil
}

// executeTool runs a tool with policy checks and returns the result.
func (cc *conversationContext) executeTool(ctx context.Context, parsed agent.AgentResponse, agentCfg *orchestrator.AgentConfig) (map[string]any, bool, error) {
	if cc.toolExec == nil {
		return nil, false, fmt.Errorf("tool execution not available")
	}

	// Check policy
	policyResult := tool.EvaluatePolicy(cc.toolPolicy, parsed.ToolName, parsed.ToolInput)

	if policyResult.Decision == tool.PolicyDenied {
		return nil, false, fmt.Errorf("tool %q denied by policy: %s", parsed.ToolName, policyResult.Reason)
	}

	if policyResult.Decision == tool.PolicyRequiresApproval {
		return nil, true, nil // approval needed
	}

	// Execute the tool
	result, err := cc.toolExec.ExecuteWithPolicy(ctx, parsed.ToolName, agentCfg.ID, "", "", parsed.ToolInput)
	if err != nil {
		return nil, false, err
	}

	return map[string]any{
		"output":  result.Output,
		"success": result.Success,
		"error":   result.Error,
	}, false, nil
}

// toolCallSummary records a single tool call in the loop.
type toolCallSummary struct {
	Tool   string         `json:"tool"`
	Input  map[string]any `json:"input,omitempty"`
	Result string         `json:"result,omitempty"`
	Status string         `json:"status"` // success, error, pending_approval
	Error  string         `json:"error,omitempty"`
}

// formatToolResult formats a tool result for inclusion in the LLM prompt.
func formatToolResult(toolName string, result map[string]any) string {
	output, _ := result["output"].(string)
	success, _ := result["success"].(bool)
	errStr, _ := result["error"].(string)

	if !success && errStr != "" {
		return fmt.Sprintf("Error: %s", errStr)
	}
	if output == "" {
		return "(tool returned no output)"
	}
	if len(output) > 8000 {
		return output[:8000] + "\n... (truncated)"
	}
	return output
}

// formatToolResultPrompt creates the prompt to feed back to the LLM after a tool succeeds.
func formatToolResultPrompt(toolName, result string) string {
	return fmt.Sprintf("Tool result for %q:\n%s\n\nBased on this tool result, please respond to the user. If you need another tool, request it. Otherwise, provide a final answer.", toolName, result)
}

// formatToolErrorPrompt creates the prompt to feed back to the LLM after a tool fails.
func formatToolErrorPrompt(toolName string, err error) string {
	return fmt.Sprintf("Tool %q failed with error: %v\n\nPlease respond to the user based on this information.", toolName, err)
}

// formatApprovalMessage creates a human-readable message for Discord when a tool needs approval.
func formatApprovalMessage(parsed agent.AgentResponse) string {
	action := formatToolAction(parsed)
	return fmt.Sprintf("I need your approval to %s. React with ✅ to approve or ❌ to deny.", action)
}

// formatToolAction creates a human-readable description of what a tool wants to do.
func formatToolAction(parsed agent.AgentResponse) string {
	switch parsed.ToolName {
	case "read_file":
		if path, ok := parsed.ToolInput["path"].(string); ok {
			return fmt.Sprintf("read file %s", path)
		}
		return "read a file"
	case "list_dir":
		if path, ok := parsed.ToolInput["path"].(string); ok {
			return fmt.Sprintf("list directory %s", path)
		}
		return "list a directory"
	case "write_file_proposal":
		if path, ok := parsed.ToolInput["path"].(string); ok {
			return fmt.Sprintf("write to file %s", path)
		}
		return "write to a file"
	default:
		return fmt.Sprintf("use tool %s", parsed.ToolName)
	}
}

// truncateStr truncates a string to maxLen characters.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}