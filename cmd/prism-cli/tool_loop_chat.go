package main

import (
	stdctx "context"
	"fmt"
	"log"
	"time"

	"github.com/emaharmony/prism/internal/context"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/session"
	"github.com/emaharmony/prism/internal/tool"
)

const maxChatToolIterations = 5
const chatToolLoopTimeout = 2 * time.Minute

// runToolLoopChat handles native tool calling via ChatProvider.
// The LLM returns structured tool_calls, which are executed and fed back
// as tool role messages. No text parsing needed.
func (cc *conversationContext) runToolLoopChat(
	parentCtx stdctx.Context,
	messages []provider.ChatMessage,
	chatTools []provider.ChatTool,
	agentCfg *orchestrator.AgentConfig,
	channelID string,
	placeholderMsgID string,
) (string, []toolCallSummary, error) {
	ctx, cancel := stdctx.WithTimeout(stdctx.Background(), chatToolLoopTimeout)
	defer cancel()

	var summaries []toolCallSummary
	currentMessages := make([]provider.ChatMessage, len(messages))
	copy(currentMessages, messages)

	for i := 0; i < maxChatToolIterations; i++ {
		log.Printf("[TOOL-CHAT] iteration %d/%d", i+1, maxChatToolIterations)

		// Call the LLM with current messages
		response, err := cc.callChatLLM(ctx, currentMessages, chatTools, agentCfg)
		if err != nil {
			return "", summaries, fmt.Errorf("LLM call failed in chat tool loop iteration %d: %w", i+1, err)
		}

		// No tool calls — this is the final response
		if !response.HasToolCalls() {
			log.Printf("[TOOL-CHAT] iteration %d: final response (%d chars)", i+1, len(response.Content))
			return response.Content, summaries, nil
		}

		// Process tool calls
		log.Printf("[TOOL-CHAT] iteration %d: model requests %d tool calls", i+1, len(response.ToolCalls))

		// Add the assistant's message (with tool_calls) to the conversation
		currentMessages = append(currentMessages, provider.ChatMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})

		// Execute each tool call and feed results back
		for _, tc := range response.ToolCalls {
			toolResult, summary := cc.executeChatTool(ctx, tc, agentCfg)

			// Build the tool result message
			currentMessages = append(currentMessages, provider.ChatMessage{
				Role:   "tool",
				Content: toolResult,
				ToolID:  tc.ID,
			})

			summaries = append(summaries, summary)
		}
	}

	// Max iterations reached — return whatever we have
	return "", summaries, fmt.Errorf("chat tool loop exceeded max iterations (%d)", maxChatToolIterations)
}

// callChatLLM makes a single LLM call in the chat tool loop context.
func (cc *conversationContext) callChatLLM(
	ctx stdctx.Context,
	messages []provider.ChatMessage,
	chatTools []provider.ChatTool,
	agentCfg *orchestrator.AgentConfig,
) (provider.ChatGenerateResponse, error) {
	chatProv, err := cc.providers.GetChatProvider(agentCfg.Model)
	if err != nil {
		return provider.ChatGenerateResponse{}, fmt.Errorf("get chat provider: %w", err)
	}

	req := provider.ChatGenerateRequest{
		RunID:       "chat-tool-loop",
		Agent:       agentCfg.ID,
		Model:       agentCfg.Model,
		Messages:    messages,
		Tools:       chatTools,
		Temperature: 0.7,
		MaxTokens:   4096,
	}

	return chatProv.ChatGenerate(ctx, req)
}

// executeChatTool executes a single tool call from the chat-based tool loop.
func (cc *conversationContext) executeChatTool(
	ctx stdctx.Context,
	tc provider.ToolCall,
	agentCfg *orchestrator.AgentConfig,
) (string, toolCallSummary) {
	// Execute the tool via the registry
	result, execErr := cc.toolExec.Registry.Execute(ctx, tc.Function.Name, tc.Function.Arguments)

	// Check policy for mutation tools
	policyResult := tool.EvaluatePolicy(cc.toolExec.Policy, tc.Function.Name, tc.Function.Arguments)

	status := "success"
	resultStr := ""
	if execErr != nil {
		status = "error"
		resultStr = execErr.Error()
	} else {
		// Convert ToolResult.Output to string
		if result.Success {
			for _, v := range result.Output {
				if s, ok := v.(string); ok {
					resultStr = s
					break
				}
			}
			if resultStr == "" {
				resultStr = fmt.Sprintf("%v", result.Output)
			}
		} else {
			status = "error"
			resultStr = result.Error
		}
	}

	if policyResult.Decision == tool.PolicyRequiresApproval {
		status = "approval_needed"
		resultStr = "This action requires human approval. An approval request has been created."
	}

	summary := toolCallSummary{
		Tool:   tc.Function.Name,
		Input:  tc.Function.Arguments,
		Status: status,
		Result: truncateStr(resultStr, 200),
	}
	if execErr != nil {
		summary.Error = execErr.Error()
	}

	return resultStr, summary
}

// buildMessages constructs a ChatMessage array from session history and workspace context.
// This replaces buildPrompt for ChatProvider — the conversation is structured messages
// instead of a flat string.
func (cc *conversationContext) buildMessages(sess *session.Session, agentCfg *orchestrator.AgentConfig) []provider.ChatMessage {
	var messages []provider.ChatMessage

	// --- System message: Agent identity + workspace context + conversation postfix ---
	var systemContent string
	systemContent += fmt.Sprintf("You are %s, a %s assistant.\n", agentCfg.ID, agentCfg.Role)

	// Inject workspace context
	if len(agentCfg.Context) > 0 && cc.ctxBuilder != nil {
		budget := cc.cfg.Prism.ContextTokenBudget
		if budget <= 0 {
			budget = 4000
		}

		builder := context.NewBuilder(cc.ctxBuilder.WorkspaceRoot).
			WithNamedContexts(agentCfg.Context).
			WithTokenBudget(budget)

		injected, err := builder.Build()
		if err == nil && injected.FormattedString != "" {
			systemContent += "\n" + injected.FormattedString
			log.Printf("[CONTEXT] Injected %d tokens from %d sources (hash: %s)",
				injected.TotalTokens, len(injected.Files), injected.ContentHash[:12])
		}
	}

	// Session awareness
	sessionAge := time.Since(sess.StartedAt).Round(time.Second)
	sessionMsgCount := len(sess.Messages)
	systemContent += fmt.Sprintf("\n[Session: %d messages, started %v ago]\n", sessionMsgCount, sessionAge)

	// Conversation postfix
	postfix := agentCfg.ConversationPostfix
	if postfix == "" {
		postfix = "Stay present in the conversation. Ask follow-up questions when appropriate. " +
			"Don't wrap things up unless the topic is genuinely resolved. " +
			"Be warm, curious, and engaged — not a transactional Q&A machine."
	}
	systemContent += "\n" + postfix + "\n"

	messages = append(messages, provider.ChatMessage{
		Role:    "system",
		Content: systemContent,
	})

	// --- Conversation history ---
	for _, msg := range sess.Messages {
		switch msg.Role {
		case "user":
			messages = append(messages, provider.ChatMessage{
				Role:    "user",
				Content: msg.Content,
			})
		case "agent":
			messages = append(messages, provider.ChatMessage{
				Role:    "assistant",
				Content: msg.Content,
			})
		case "system":
			messages = append(messages, provider.ChatMessage{
				Role:    "system",
				Content: msg.Content,
			})
		}
	}

	return messages
}

// buildChatTools converts the tool registry into ChatTool format for ChatProvider.
func (cc *conversationContext) buildChatTools() []provider.ChatTool {
	toolInfos := cc.toolExec.Registry.ListWithDescriptions()
	chatTools := make([]provider.ChatTool, 0, len(toolInfos))

	for _, ti := range toolInfos {
		params := map[string]any{
			"type":       "object",
			"properties": make(map[string]any),
		}
		required := make([]string, 0)

		for pname, spec := range ti.Schema.Input {
			props := map[string]any{
				"type":        spec.Type,
				"description": spec.Description,
			}
			params["properties"].(map[string]any)[pname] = props
			if spec.Required {
				required = append(required, pname)
			}
		}

		if len(required) > 0 {
			params["required"] = required
		}

		chatTools = append(chatTools, provider.ChatTool{
			Type: "function",
			Function: provider.FunctionDef{
				Name:        ti.Name,
				Description: ti.Description,
				Parameters:  params,
			},
		})
	}

	return chatTools
}