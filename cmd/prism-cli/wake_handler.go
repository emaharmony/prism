// Package main provides the wake handler for V32 event-driven wake.
//
// The wake handler subscribes to NATS events from the scheduler and triggers
// LLM inference with context-appropriate prompts. This is how Prism "wakes up"
// on schedule instead of relying on heartbeat babysitting.
//
// Supported actions:
//   - daily_review: Review state, check for stale tasks, summarize progress
//   - memory_consolidation: Trigger Remembrance dream cycle
//   - check_prs: Check for open PRs and notify
package main

import (
	stdcontext "context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	contextpkg "github.com/emaharmony/prism/internal/context"
	"github.com/emaharmony/prism/internal/adapter/builtin/discordbot"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/session"
	"github.com/emaharmony/prism/internal/state"
	"github.com/nats-io/nats.go"
)

// WakeHandler subscribes to scheduler events and triggers LLM inference.
type WakeHandler struct {
	cfg        *orchestrator.Config
	providers  *provider.ProviderRegistry
	sessMgr    *session.Manager
	stateMgr   *state.Manager
	natsConn   *nats.Conn
	bot        discordBotClient
	ctxBuilder *contextpkg.Builder
}

// wakeAction defines a scheduled action prompt.
type wakeAction struct {
	Prompt    string // System prompt for the LLM
	ChannelID string // Discord channel to send results to
	MaxTokens int    // Max response tokens
}

// knownActions maps action names to their prompts and target channels.
var knownActions = map[string]wakeAction{
	"daily_review": {
		Prompt: `You are performing a daily review of your working state. Read your state files and:
1. Check if any tasks are stale (status hasn't changed in >24 hours)
2. Review blocked items — are any unblocked now?
3. Summarize what you accomplished yesterday and what's next
4. Clean up state files if needed (clear completed tasks, update context)
5. Be concise — this is a status report, not a novel

If everything looks good, say so briefly. If something needs attention, flag it clearly.`,
		ChannelID: "1491622581348864162", // manager-room
		MaxTokens: 2048,
	},
	"memory_consolidation": {
		Prompt: `You are performing weekly memory consolidation. Review recent conversations and:
1. Identify recurring themes, decisions, and patterns
2. Flag anything that should be persisted to long-term memory
3. Note any contradictions or outdated information
4. Be concise — this is a consolidation report

Focus on what matters. Skip trivia.`,
		ChannelID: "1491622581348864162", // manager-room
		MaxTokens: 1024,
	},
	"check_prs": {
		Prompt: `You are checking for open pull requests. Use the git tools to:
1. List open PRs on the current repository
2. For each PR, summarize the status (open, review comments, CI status)
3. Flag any PRs that need attention (stale reviews, failed CI)
4. Be concise — this is a PR status report

If no PRs are open, say "No open PRs" and nothing else.`,
		ChannelID: "1491622581348864162", // manager-room
		MaxTokens: 1024,
	},
}

// NewWakeHandler creates a wake handler with the given dependencies.
func NewWakeHandler(
	cfg *orchestrator.Config,
	providers *provider.ProviderRegistry,
	sessMgr *session.Manager,
	stateMgr *state.Manager,
	natsConn *nats.Conn,
	bot discordBotClient,
	ctxBuilder *contextpkg.Builder,
) *WakeHandler {
	return &WakeHandler{
		cfg:        cfg,
		providers:  providers,
		sessMgr:    sessMgr,
		stateMgr:   stateMgr,
		natsConn:   natsConn,
		bot:        bot,
		ctxBuilder: ctxBuilder,
	}
}

// Start subscribes to scheduler events and begins processing.
func (wh *WakeHandler) Start() error {
	if wh.natsConn == nil {
		return fmt.Errorf("NATS not connected")
	}

	_, err := wh.natsConn.Subscribe("prism.task.scheduled", func(msg *nats.Msg) {
		wh.handleScheduledEvent(msg)
	})
	if err != nil {
		return fmt.Errorf("subscribe to prism.task.scheduled: %w", err)
	}

	log.Printf("[WAKE] subscribed to prism.task.scheduled")
	return nil
}

// handleScheduledEvent processes a scheduler event.
func (wh *WakeHandler) handleScheduledEvent(msg *nats.Msg) {
	var payload map[string]any
	if err := json.Unmarshal(msg.Data, &payload); err != nil {
		log.Printf("[WAKE] ERROR unmarshaling event: %v", err)
		return
	}

	action, _ := payload["action"].(string)
	jobName, _ := payload["job_name"].(string)
	firedAt, _ := payload["fired_at"].(string)

	if action == "" {
		log.Printf("[WAKE] ERROR event missing action field: %v", payload)
		return
	}

	log.Printf("[WAKE] processing scheduled event: job=%q action=%q fired_at=%s", jobName, action, firedAt)

	actionDef, ok := knownActions[action]
	if !ok {
		log.Printf("[WAKE] WARN unknown action %q, using generic prompt", action)
		actionDef = wakeAction{
			Prompt:    fmt.Sprintf("You received a scheduled event with action %q. Check your state and report anything that needs attention.", action),
			ChannelID: "1491622581348864162",
			MaxTokens: 1024,
		}
	}

	// Build the system prompt with state context
	systemPrompt := actionDef.Prompt

	// Inject working state if available
	if wh.stateMgr != nil {
		if statePrompt := wh.stateMgr.FormatStateForPrompt(); statePrompt != "" {
			systemPrompt += "\n\n## Current Working State\n" + statePrompt
		}
	}

	// Inject workspace context if available
	if wh.ctxBuilder != nil {
		if injected, err := wh.ctxBuilder.BuildCached(); err == nil && injected != nil && injected.FormattedString != "" {
			systemPrompt += "\n\n## Workspace Context\n" + injected.FormattedString
		}
	}

	// Get the primary agent config
	var agentCfg *orchestrator.AgentConfig
	for i := range wh.cfg.Agents {
		if wh.cfg.Agents[i].Primary {
			agentCfg = &wh.cfg.Agents[i]
			break
		}
	}
	if agentCfg == nil && len(wh.cfg.Agents) > 0 {
		agentCfg = &wh.cfg.Agents[0]
	}
	if agentCfg == nil {
		log.Printf("[WAKE] ERROR no agent configured")
		return
	}

	model := agentCfg.Model
	if model == "" {
		model = "glm-5.1:cloud"
	}

	// Create or reuse a session for this action
	sessionKey := fmt.Sprintf("wake:%s:%s", action, time.Now().Format("2006-01-02"))
	sess, err := wh.sessMgr.FindActive("wake", sessionKey, "scheduler")
	if err != nil || sess == nil {
		sess, err = wh.sessMgr.Create(agentCfg.ID, "wake", sessionKey, "scheduler")
		if err != nil {
			log.Printf("[WAKE] ERROR creating session: %v", err)
			return
		}
	}

	// Build the prompt
	userPrompt := fmt.Sprintf("Perform the %s action now. This was triggered by the scheduler at %s.", action, firedAt)

	// Call the LLM — try ChatProvider first (supports tool calling), fall back to text
	log.Printf("[WAKE] calling LLM for action %q with model %q", action, model)

	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 5*time.Minute)
	defer cancel()

	var responseContent string
	var promptTokens, completionTokens int

	chatProv, chatErr := wh.providers.GetChatProvider(model)
	if chatErr == nil {
		// Use ChatProvider (preferred path — same as Discord serve)
		resp, err := chatProv.ChatGenerate(ctx, provider.ChatGenerateRequest{
			RunID:       fmt.Sprintf("wake-%s-%d", action, time.Now().Unix()),
			Agent:       agentCfg.ID,
			Model:       model,
			Messages: []provider.ChatMessage{
				{Role: "system", Content: systemPrompt},
				{Role: "user", Content: userPrompt},
			},
			Temperature: 0.7,
			MaxTokens:   actionDef.MaxTokens,
		})
		if err != nil {
			log.Printf("[WAKE] ERROR chat LLM call failed for action %q: %v", action, err)
			if wh.bot != nil {
				wh.bot.Send(&discordbot.OutboundMessage{
					ChannelID: actionDef.ChannelID,
					Content:   fmt.Sprintf("⚠️ Scheduled task **%s** failed: %v", formatActionName(action), err),
				})
			}
			return
		}
		responseContent = resp.Content
		promptTokens = resp.PromptTokens
		completionTokens = resp.OutputTokens
	} else {
		// Fall back to text provider
		log.Printf("[WAKE] WARN no chat provider for %q: %v, falling back to text provider", model, chatErr)
		prov, err := wh.providers.Get(model)
		if err != nil {
			log.Printf("[WAKE] ERROR no provider for model %q: %v", model, err)
			return
		}

		resp, err := prov.Generate(ctx, provider.GenerateRequest{
			RunID:       fmt.Sprintf("wake-%s-%d", action, time.Now().Unix()),
			Agent:       agentCfg.ID,
			Model:       model,
			Prompt:      systemPrompt + "\n\n" + userPrompt,
			Temperature: 0.7,
			MaxTokens:   actionDef.MaxTokens,
		})
		if err != nil {
			log.Printf("[WAKE] ERROR text LLM call failed for action %q: %v", action, err)
			if wh.bot != nil {
				wh.bot.Send(&discordbot.OutboundMessage{
					ChannelID: actionDef.ChannelID,
					Content:   fmt.Sprintf("⚠️ Scheduled task **%s** failed: %v", formatActionName(action), err),
				})
			}
			return
		}
		responseContent = resp.Text
		promptTokens = resp.PromptTokens
		completionTokens = resp.OutputTokens
	}

	// Save to session
	wh.sessMgr.AddMessage(sess.ID, "user", userPrompt, "scheduler")
	wh.sessMgr.AddMessage(sess.ID, "agent", responseContent, agentCfg.ID)

	// Send result to Discord
	if wh.bot != nil && actionDef.ChannelID != "" {
		content := responseContent
		// Discord has a 2000 character limit
		if len(content) > 1950 {
			content = content[:1950] + "\n\n...(truncated)"
		}

		// Prefix with action name
		content = fmt.Sprintf("🔔 **%s**\n\n%s", formatActionName(action), content)

		wh.bot.Send(&discordbot.OutboundMessage{
			ChannelID: actionDef.ChannelID,
			Content:   content,
		})
		log.Printf("[WAKE] sent %s result to Discord channel %s", action, actionDef.ChannelID)
	}

	log.Printf("[WAKE] completed action %q (tokens: prompt=%d, completion=%d)", action, promptTokens, completionTokens)
}

// formatActionName converts snake_case to Title Case for display.
func formatActionName(action string) string {
	parts := strings.Split(action, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}