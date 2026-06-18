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
	"os/exec"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/adapter/builtin/discordbot"
	contextpkg "github.com/emaharmony/prism/internal/context"
	"github.com/emaharmony/prism/internal/factorymonitor"
	"github.com/emaharmony/prism/internal/improve"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/plan"
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
	planMgr    *plan.Manager
	improveMgr *improve.Manager
	factoryMon *factorymonitor.Monitor
}

// wakeAction defines a scheduled action prompt.
type wakeAction struct {
	Prompt    string // System prompt for the LLM
	ChannelID string // Discord channel to send results to
	MaxTokens int    // Max response tokens
	SkipLLM   bool   // If true, run the action directly without LLM (e.g., gh pr list)
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
		Prompt: `You are summarizing PR status for a Discord channel. Take the raw PR data below and format it concisely:
1. List each PR with number, title, author, review status, and CI status
2. Flag any PRs that need attention (stale reviews, failed CI, ready to merge)
3. If no PRs are open, say "No open PRs" and nothing else

Be concise — this is a status report, not a novel.`,
		ChannelID: "1491622581348864162", // manager-room
		MaxTokens: 1024,
		SkipLLM:   true, // Run gh pr list directly, then optionally summarize
	},
	"factory_status_digest": {
		Prompt:    `Report the current Roblox Factory queue status.`,
		ChannelID: "1496591599940010055",
		MaxTokens: 512,
		SkipLLM:   true,
	},
	"auto_patch": {
		Prompt: `You are the auto-patch agent. Your job is to review improvement proposals and create fixes.

1. Check the improvement proposals in your workspace state
2. For each auto-PR-eligible proposal (bug fixes, error patterns, test coverage, doc updates):
   a. Create a plan using plan_create if one doesn't exist
   b. Set the plan status to auto_proceed
   c. Create a git branch with a descriptive name
   d. Write the fix
   e. Commit and push
   f. Open a pull request with a clear description
3. For proposals that need approval (architecture, process), notify Ema with a clear summary
4. After creating any PR, update the improvement status to in_progress

Be thorough but fast. Focus on correctness.`,
		ChannelID: "1491622581348864162", // manager-room
		MaxTokens: 2048,
	},
	"review_improvements": {
		Prompt: `You are reviewing active improvement proposals.

1. Check the improvement proposals in your workspace state
2. For each proposed improvement:
   a. Assess whether it's still relevant
   b. Check if the underlying error pattern has resolved
   c. Determine the correct approval level
   d. If auto-PR eligible, flag it for the next auto_patch cycle
   e. If it needs Ema's approval, send a clear notification with the details
3. Dismiss duplicates or stale proposals
4. Be concise — list proposals with status and recommended action`,
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
	planMgr *plan.Manager,
	improveMgr *improve.Manager,
	factoryMon *factorymonitor.Monitor,
) *WakeHandler {
	return &WakeHandler{
		cfg:        cfg,
		providers:  providers,
		sessMgr:    sessMgr,
		stateMgr:   stateMgr,
		natsConn:   natsConn,
		bot:        bot,
		ctxBuilder: ctxBuilder,
		planMgr:    planMgr,
		improveMgr: improveMgr,
		factoryMon: factoryMon,
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

	// Handle direct-execution actions (no LLM needed)
	if actionDef.SkipLLM {
		wh.handleDirectAction(action, actionDef, firedAt)
		return
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

	// V32: Inject improvement proposals for auto_patch and review_improvements actions
	if wh.improveMgr != nil && (action == "auto_patch" || action == "review_improvements") {
		activeImpr := wh.improveMgr.GetActiveImprovements()
		if len(activeImpr) > 0 {
			systemPrompt += "\n\n" + improve.FormatImprovementsForPrompt(activeImpr)
		}
		errorPatterns := wh.improveMgr.GetErrorPatterns()
		if len(errorPatterns) > 0 {
			systemPrompt += "\n\n" + improve.FormatErrorPatternsForPrompt(errorPatterns)
		}
	}

	// V32: Inject active plans for auto_patch action
	if wh.planMgr != nil && action == "auto_patch" {
		if plans, err := wh.planMgr.LoadPlans(); err == nil && len(plans) > 0 {
			if activePlan := plan.ActivePlan(plans); activePlan != nil {
				systemPrompt += "\n\n" + plan.FormatPlanForPrompt(activePlan)
			}
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
			RunID: fmt.Sprintf("wake-%s-%d", action, time.Now().Unix()),
			Agent: agentCfg.ID,
			Model: model,
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
	// Send result to Discord (the bot adapter handles splitting long messages)
		content := fmt.Sprintf("🔔 **%s**\n\n%s", formatActionName(action), responseContent)
		wh.bot.Send(&discordbot.OutboundMessage{
			ChannelID: actionDef.ChannelID,
			Content:   content,
		})
		log.Printf("[WAKE] sent %s result to Discord channel %s", action, actionDef.ChannelID)
	}

	// V32: After sending result, check for plans that need approval and notify Ema
	// Only check after improvement-related actions (auto_patch, review_improvements, daily_review)
	// to avoid unnecessary disk reads on every wake event.
	if wh.bot != nil && wh.planMgr != nil && (action == "auto_patch" || action == "review_improvements" || action == "daily_review") {
		wh.notifyPendingApprovals(actionDef.ChannelID)
	}

	log.Printf("[WAKE] completed action %q (tokens: prompt=%d, completion=%d)", action, promptTokens, completionTokens)
}

// handleDirectAction processes actions that don't need LLM inference.
// Currently only supports check_prs, which runs gh pr list directly.
func (wh *WakeHandler) handleDirectAction(action string, actionDef wakeAction, firedAt string) {
	var resultContent string

	switch action {
	case "check_prs":
		resultContent = wh.checkPRStatus()
	case "factory_status_digest":
		resultContent = wh.factoryStatusDigest()
	default:
		log.Printf("[WAKE] WARN unknown direct action %q", action)
		return
	}

	if resultContent == "" {
		log.Printf("[WAKE] direct action %q produced no output", action)
		return
	}

	// Send result to Discord (the bot adapter handles splitting long messages)
	if wh.bot != nil && actionDef.ChannelID != "" {
		content := fmt.Sprintf("\U0001F514 **%s**\n\n%s", formatActionName(action), resultContent)
		wh.bot.Send(&discordbot.OutboundMessage{
			ChannelID: actionDef.ChannelID,
			Content:   content,
		})
	}
	log.Printf("[WAKE] completed direct action %q", action)
}

func (wh *WakeHandler) factoryStatusDigest() string {
	if wh.factoryMon == nil {
		return "Factory monitor is not enabled."
	}
	wh.factoryMon.PublishDigest()
	return ""
}

// checkPRStatus runs gh pr list and formats the results.
func (wh *WakeHandler) checkPRStatus() string {
	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "gh", "pr", "list",
		"--json", "number,title,author,updatedAt,reviewDecision,statusCheckRollup,labels",
		"--state", "open",
		"--limit", "20",
	)
	// Run in the Prism repo directory so gh pr list checks Prism's PRs specifically
	cmd.Dir = "/Users/ema/projects/repos/prism"

	output, err := cmd.Output()
	if err != nil {
		// gh might not be installed or no PRs exist
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			// No open PRs — return empty so handleDirectAction skips sending
			return ""
		}
		return fmt.Sprintf("Could not check PRs: %v", err)
	}

	if len(strings.TrimSpace(string(output))) == 0 {
		// No open PRs — return empty so handleDirectAction skips sending
		return ""
	}

	return wh.formatPRList(string(output))
}

// formatPRList formats gh pr list JSON output into a readable Discord message.
func (wh *WakeHandler) formatPRList(jsonOutput string) string {
	type PRAuthor struct {
		Login string `json:"login"`
	}
	type PRLabel struct {
		Name string `json:"name"`
	}
	type PRStatusCheck struct {
		State string `json:"state"`
		Name  string `json:"name"`
	}
	type PR struct {
		Number            int             `json:"number"`
		Title             string          `json:"title"`
		Author            PRAuthor        `json:"author"`
		UpdatedAt         string          `json:"updatedAt"`
		ReviewDecision    string          `json:"reviewDecision"`
		StatusCheckRollup []PRStatusCheck `json:"statusCheckRollup"`
		Labels            []PRLabel       `json:"labels"`
	}

	var prs []PR
	if err := json.Unmarshal([]byte(jsonOutput), &prs); err != nil {
		// If parsing fails, return the raw output
		return "PR Status:\n" + jsonOutput
	}

	if len(prs) == 0 {
		// No open PRs — return empty so handleDirectAction skips sending
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("PR Status Report — %d open PRs\n\n", len(prs)))

	for _, pr := range prs {
		// Format review decision
		review := pr.ReviewDecision
		if review == "" {
			review = "PENDING"
		}
		switch review {
		case "APPROVED":
			review = "\u2705 Approved"
		case "CHANGES_REQUESTED":
			review = "\u26a0\ufe0f Changes Requested"
		case "REVIEW_REQUIRED":
			review = "\U0001f50d Review Required"
		}

		// Format CI status
		ciStatus := "\u2753 Unknown"
		if len(pr.StatusCheckRollup) > 0 {
			allPass := true
			for _, check := range pr.StatusCheckRollup {
				if check.State != "success" {
					allPass = false
					break
				}
			}
			if allPass {
				ciStatus = "\u2705 All passing"
			} else {
				ciStatus = "\u274c Failing"
			}
		}

		// Format labels
		var labelStr string
		if len(pr.Labels) > 0 {
			labels := make([]string, len(pr.Labels))
			for i, l := range pr.Labels {
				labels[i] = l.Name
			}
			labelStr = " [" + strings.Join(labels, ", ") + "]"
		}

		sb.WriteString(fmt.Sprintf("**#%d** %s\n", pr.Number, pr.Title))
		sb.WriteString(fmt.Sprintf("   by @%s • %s • CI: %s%s\n\n", pr.Author.Login, review, ciStatus, labelStr))
	}

	return sb.String()
}
func formatActionName(action string) string {
	parts := strings.Split(action, "_")
	for i, p := range parts {
		if len(p) > 0 {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// notifyPendingApprovals checks for plans that need Ema's approval and sends a Discord notification.
func (wh *WakeHandler) notifyPendingApprovals(channelID string) {
	plans, err := wh.planMgr.LoadPlans()
	if err != nil {
		log.Printf("[WAKE] WARN could not load plans for approval check: %v", err)
		return
	}

	var pendingApproval []*plan.Plan
	for i := range plans {
		if plans[i].Status == plan.StatusPendingApproval {
			pendingApproval = append(pendingApproval, &plans[i])
		}
	}

	if len(pendingApproval) == 0 {
		return
	}

	var msg strings.Builder
	msg.WriteString("\u26a0\ufe0f **Plans Needing Approval**\n\n")
	for _, p := range pendingApproval {
		msg.WriteString(fmt.Sprintf("**%s** (%s)\n", p.Title, p.ID))
		msg.WriteString(fmt.Sprintf("Approval level: %s\n", p.ApprovalLevel))
		if p.Description != "" {
			msg.WriteString(fmt.Sprintf("Description: %s\n", p.Description))
		}
		if p.Reasoning != "" {
			msg.WriteString(fmt.Sprintf("Reasoning: %s\n", p.Reasoning))
		}
		msg.WriteString(fmt.Sprintf("Created: %s\n\n", p.CreatedAt.Format("Jan 02 15:04")))
	}
	msg.WriteString("Reply with `approve P-XXX` or `reject P-XXX` to act on these plans.")

	if wh.bot != nil {
		wh.bot.Send(&discordbot.OutboundMessage{
			ChannelID: channelID,
			Content:   msg.String(),
		})
	}
	log.Printf("[WAKE] notified %d pending approval plans", len(pendingApproval))
}
