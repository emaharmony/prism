package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/adapter/builtin/discordbot"
	"github.com/emaharmony/prism/internal/agent"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/provider"
	ctxcontext "context"
)

// handleAgentMessage processes an incoming message from another bot agent.
// Agent messages bypass injection defense and rate limiting (trusted peers).
//
// Key difference from handleDiscordMessage: NO placeholder messages.
// Placeholders ("✧ ...") are visible to other bots and trigger false responses.
// Instead, we use only the typing indicator, then send the complete response.
func (cc *conversationContext) handleAgentMessage(msg *discordbot.InboundMessage) {
	// Frame the message as coming from a peer agent
	framedContent := fmt.Sprintf("[Message from agent %s]: %s", msg.UserName, msg.Content)

	// Emit agent-to-agent event
	cc.publishEvent("prism.agent.message_received", map[string]any{
		"from_agent": msg.UserName,
		"from_id":    msg.UserID,
		"channel_id": msg.ChannelID,
	})

	log.Printf("[AGENT] processing agent message from %s (%s) in channel %s", msg.UserName, msg.UserID, msg.ChannelID)

	// Route the message
	result := cc.router.Route(framedContent)

	// Find or create a session keyed by channel + bot user
	sess, err := cc.sessMgr.FindActive("discord", msg.ChannelID, "agent:"+msg.UserID)
	if err != nil {
		log.Printf("[ERROR] find agent session: %v", err)
		return
	}
	if sess == nil {
		sess, err = cc.sessMgr.Create(result.AgentID, "discord", msg.ChannelID, "agent:"+msg.UserID)
		if err != nil {
			log.Printf("[ERROR] create agent session: %v", err)
			return
		}
	}

	// Add the framed agent message to the session
	_, err = cc.sessMgr.AddMessage(sess.ID, "user", framedContent, "")
	if err != nil {
		log.Printf("[ERROR] add agent message: %v", err)
		return
	}

	// Look up agent config and provider
	agentCfg := cc.findAgentConfig(result.AgentID)
	if agentCfg == nil {
		log.Printf("[ERROR] no config for agent %s", result.AgentID)
		return
	}

	// Create run context with longer timeout for agent-to-agent
	runCtx, runCancel := ctxcontext.WithTimeout(ctxcontext.Background(), 120*time.Second)
	defer runCancel()

	// Typing indicator only — NO placeholder message
	if err := cc.bot.Typing(msg.ChannelID); err != nil {
		log.Printf("[WARN] typing indicator failed: %v", err)
	}

	// Build the full prompt
	prompt := cc.buildPrompt(sess, agentCfg)

	// Inject Remembrance context
	if cc.remClient != nil {
		cacheKey := fmt.Sprintf("%s:%s", agentCfg.ID, sess.ID)
		remCtx := cc.remCache.Get(cacheKey)
		if remCtx == nil {
			var remCtxErr error
			remCtx, remCtxErr = cc.remClient.BuildContext(prompt, "", agentCfg.ID, 5)
			if remCtxErr != nil {
				log.Printf("[REMEMBRANCE] context build failed: %v", remCtxErr)
			} else if remCtx != nil {
				cc.remCache.Set(cacheKey, remCtx)
			}
		}
		if remCtx != nil {
			if remCtx.ContextMarkdown != "" {
				memoryBlock := "\n\n---\nRelevant context from memory:\n" + remCtx.ContextMarkdown + "\n---\n\n"
				prompt = memoryBlock + prompt
				log.Printf("[REMEMBRANCE] injected %d memory sources into agent prompt (markdown)", len(remCtx.SelectedMemories))
			} else if remCtx.ContextJSON != nil && len(remCtx.ContextJSON.Memories) > 0 {
				var memoryParts []string
				for _, mem := range remCtx.ContextJSON.Memories {
					if mem.Summary != "" {
						memoryParts = append(memoryParts, mem.Summary)
					}
				}
				if len(memoryParts) > 0 {
					memoryBlock := "\n\n---\nRelevant context from memory:\n" + strings.Join(memoryParts, "\n\n") + "\n---\n\n"
					prompt = memoryBlock + prompt
				}
			}
		}
	}

	// Append tool instructions
	if cc.toolExec != nil {
		toolInfos := cc.toolExec.Registry.ListWithDescriptions()
		if len(toolInfos) > 0 {
			prompt += agent.BuildToolPromptSuffix(toolInfos, cc.ctxBuilder.WorkspaceRoot, cc.toolPolicy.AllowedPaths...)
		}
	}

	// Execute LLM with tool loop support
	var response string

	if cc.toolExec != nil {
		// Check if the provider supports native tool calling (ChatProvider)
		_, chatErr := cc.providers.GetChatProvider(agentCfg.Model)
		supportsChat := chatErr == nil

		if supportsChat {
			// Native tool calling path
			messages := cc.buildMessages(sess, agentCfg)
			chatTools := cc.buildChatTools()
			log.Printf("[AGENT-TOOL-CHAT] entering native tool loop with %d tools", len(chatTools))

			finalResponse, toolSummaries, toolErr := cc.runToolLoopChat(
				runCtx,
				messages,
				chatTools,
				agentCfg,
				msg.ChannelID,
				"", // NO placeholder — critical for agent-to-agent
			)
			if toolErr != nil {
				log.Printf("[AGENT-TOOL-CHAT] tool loop failed: %v", toolErr)
				finalResponse = "I tried to use a tool to help with that, but something went wrong."
			}
			response = finalResponse

			for _, ts := range toolSummaries {
				log.Printf("[AGENT-TOOL-CHAT] %s: %s", ts.Tool, ts.Status)
				toolMsg := fmt.Sprintf("[Tool: %s] %s", ts.Tool, ts.Status)
				if ts.Error != "" {
					toolMsg += " — " + ts.Error
				}
				cc.sessMgr.AddMessage(sess.ID, "tool", toolMsg, ts.Tool)
			}
		} else {
			// Text-based tool calling path — first do a direct LLM call, then check for tool intent
			llmProvider, provErr := cc.providers.Get(agentCfg.Model)
			if provErr != nil {
				log.Printf("[ERROR] no provider for model %s: %v", agentCfg.Model, provErr)
				return
			}
			resp, genErr := llmProvider.Generate(runCtx, provider.GenerateRequest{
				Prompt:  prompt,
				Model:   agentCfg.Model,
				Agent:   agentCfg.ID,
				RunID:   sess.ID,
			})
			if genErr != nil {
				log.Printf("[ERROR] agent Generate failed: %v", genErr)
				return
			}
			response = resp.Text

			// Check if LLM requested a tool
			parsed := agent.ParseAgentOutput(response)
			if parsed.Type == agent.ResponseToolRequest {
				log.Printf("[AGENT-TOOL] LLM requested tool %q", parsed.ToolName)
				finalResponse, toolSummaries, toolErr := cc.runToolLoop(
					runCtx,
					prompt,
					agentCfg,
					msg.ChannelID,
					"", // NO placeholder
				)
				if toolErr != nil {
					log.Printf("[AGENT-TOOL] tool loop failed: %v", toolErr)
				} else if finalResponse != "" {
					response = finalResponse
				}
				for _, ts := range toolSummaries {
					toolMsg := fmt.Sprintf("[Tool: %s] %s", ts.Tool, ts.Status)
					if ts.Error != "" {
						toolMsg += " — " + ts.Error
					}
					cc.sessMgr.AddMessage(sess.ID, "tool", toolMsg, ts.Tool)
				}
			}
		}
	} else {
		// No tools — direct LLM call
		llmProvider, provErr := cc.providers.Get(agentCfg.Model)
		if provErr != nil {
			log.Printf("[ERROR] no provider for model %s: %v", agentCfg.Model, provErr)
			return
		}

		// Try ChatProvider first
		if chatProv, isChat := llmProvider.(provider.ChatProvider); isChat {
			messages := cc.buildMessages(sess, agentCfg)
			chatResp, chatErr := chatProv.ChatGenerate(runCtx, provider.ChatGenerateRequest{
				Messages: messages,
				Model:    agentCfg.Model,
				Agent:    agentCfg.ID,
				RunID:    sess.ID,
			})
			if chatErr != nil {
				log.Printf("[ERROR] agent ChatGenerate failed: %v", chatErr)
				return
			}
			response = chatResp.Content
		} else {
			resp, genErr := llmProvider.Generate(runCtx, provider.GenerateRequest{
				Prompt:  prompt,
				Model:   agentCfg.Model,
				Agent:   agentCfg.ID,
				RunID:   sess.ID,
			})
			if genErr != nil {
				log.Printf("[ERROR] agent Generate failed: %v", genErr)
				return
			}
			response = resp.Text
		}
	}

	if response == "" {
		log.Printf("[WARN] agent produced empty response")
		return
	}

	// Save assistant response to session
	cc.sessMgr.AddMessage(sess.ID, "assistant", response, "")

	// Send the response — split if needed, NO placeholder editing
	chunks := discordbot.SplitMessage(response, discordbot.MessageLimit)
	for _, chunk := range chunks {
		if err := cc.bot.Send(&discordbot.OutboundMessage{ChannelID: msg.ChannelID, Content: chunk}); err != nil {
			log.Printf("[ERROR] failed to send agent response chunk: %v", err)
		}
	}

	// Publish agent response event
	cc.publishEvent("prism.agent.message_sent", map[string]any{
		"from_agent": agentCfg.ID,
		"to_channel": msg.ChannelID,
	})

	log.Printf("[AGENT] sent response to agent %s in channel %s (%d chars)", msg.UserName, msg.ChannelID, len(response))

	// Auto-save to Remembrance (non-blocking)
	if cc.remClient != nil {
		cc.remSem <- struct{}{}
		go func(agentID, content string) {
			defer func() { <-cc.remSem }()
			if _, capErr := cc.remClient.Capture(content, "agent_conversation", agentID, "low"); capErr != nil {
				log.Printf("[REMEMBRANCE] agent capture failed: %v", capErr)
			}
		}(agentCfg.ID, response)
	}
}

// findPrimaryAgent returns the ID of the primary agent, or the first agent if none is primary.
func (cc *conversationContext) findPrimaryAgent() string {
	for i := range cc.cfg.Agents {
		if cc.cfg.Agents[i].Primary {
			return cc.cfg.Agents[i].ID
		}
	}
	if len(cc.cfg.Agents) > 0 {
		return cc.cfg.Agents[0].ID
	}
	return "prism1"
}

// isAgentBot checks if a Discord user ID is in any agent's listen_to_agents list.
func isAgentBot(agents []orchestrator.AgentConfig, userID string) bool {
	for i := range agents {
		for _, botID := range agents[i].ListenToAgents {
			if botID == userID {
				return true
			}
		}
	}
	return false
}