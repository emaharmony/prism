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
	"sync"
	"time"

	"github.com/emaharmony/prism/internal/adapter/builtin/discordbot"
	"github.com/emaharmony/prism/internal/agent"
	contextpkg "github.com/emaharmony/prism/internal/context"
	"github.com/emaharmony/prism/internal/factorymonitor"
	"github.com/emaharmony/prism/internal/improve"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/plan"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/remembrance"
	"github.com/emaharmony/prism/internal/session"
	"github.com/emaharmony/prism/internal/state"
	"github.com/emaharmony/prism/internal/tool"
	"github.com/nats-io/nats.go"
)

// WakeHandler subscribes to scheduler events and triggers LLM inference.
// It also listens for inter-agent messages on NATS (e.g. from OpenClaw Lumi).
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
	remClient  *remembrance.Client
	toolExec   *tool.Executor   // V35: Tool executor for project_work action
	toolReg    *tool.Registry    // V35: Tool registry for listing tools in prompt

	// agentMessages stores recent inter-agent messages received via NATS.
	// Accessed under agentMu for concurrent safety.
	agentMu      sync.Mutex
	agentMessages []agentMessage
}

// agentMessage is a message from another agent (e.g. OpenClaw Lumi) received via NATS.
type agentMessage struct {
	From      string    `json:"from"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
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
		Prompt: `You are the auto-patch agent. You are an active developer, not just a reporter. Your job is to find and fix bugs, improve user experience, and increase effectiveness across the codebase.

## Your Capabilities
You can: create git branches, write code, commit, push, and open PRs. You have git_status, git_log, git_diff, file_read, file_write, and file_list tools. Use them.

## Workflow
1. Check improvement proposals in workspace state
2. For each auto-PR-eligible proposal (bug fixes, error patterns, test coverage, doc updates):
   a. Create a plan using plan_create if one doesn't exist
   b. Set the plan status to auto_proceed
   c. Create a git branch with a descriptive name
   d. Write the fix
   e. Commit and push
   f. Open a pull request with a clear description
3. For proposals that need approval (architecture, process), notify Ema with a clear summary
4. After creating any PR, update the improvement status to in_progress
5. If a tool fails or doesn't exist, mention <@1512994928769237002> (OpenClaw Lumi) with the error so she can verify and respond

## Active Bug Hunting
Don't just wait for proposals. Actively look for:
- **Bugs**: Run git_status and git_diff to find uncommitted issues. Check recent test runs for failures.
- **UX issues**: Read through error messages, user-facing text, and logs. If something is confusing or unclear, fix it.
- **Effectiveness**: Look for redundant code, missing error handling, incomplete implementations. Fix them.
- **Test gaps**: If a function has no test coverage, add one.

## Quality Standards
- Every fix should include or update a test if applicable
- Commit messages should be clear and descriptive
- PR descriptions should explain what was fixed and why
- If you're unsure about a change, propose it as an improvement rather than auto-PR

Be thorough, proactive, and fast. You are not just a reporter — you are a developer.`,
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
	"project_work": {
		Prompt: `You are the project work agent. Your job is to pick up project assignments from OpenClaw Lumi (received via NATS messages or Remembrance memory) and actively work on them.

## How To Find Your Assignment
1. Read the "Direct Messages from OpenClaw Lumi (via NATS)" section in your context — these contain your current assignment
2. Search Remembrance for "BassBook" or the project name to find the creative brief and requirements
3. The creative brief is your instruction set. Follow it.

## How To Work — TOOL CALLING
You have real tools available. USE THEM. Do not just describe what you would do — actually DO it.
1. Use read_file to read project repo files at /Users/ema/projects/repos/BassBook/
2. Assess current state — what exists, what's missing, what needs improvement
3. Use git_add to stage changes, git_commit to commit, git_push to push
4. Use write_file to write code changes directly (auto-approved in autonomous mode)

## CRITICAL: Tool Calling Format
You MUST respond with JSON in one of these shapes:
- Tool request: {"type": "tool_request", "tool": "read_file", "input": {"path": "/Users/ema/projects/repos/BassBook/apps/web/src/app/globals.css"}}
- Final response: {"type": "final", "content": "your summary here"}

You may make ONE tool request per response. The system will execute it and give you the result. Then make your next request.
DO NOT say "Let me read the file" and stop. Actually emit the tool_request JSON.

## When You Need Direction
If you have a UX question, visual decision, or creative direction question:
- Tag <@1512994928769237002> (OpenClaw Lumi) in Discord with your question
- Wait for a response — do NOT guess on creative decisions
- OpenClaw Lumi is your creative director. Ema is the client. Do not bother Ema.

## Current Assignment
Check your NATS messages for the latest assignment from OpenClaw Lumi. If there are project assignments, start working on the first task immediately. Do not report that you found assignments — actually work on them by calling tools.

## Output Format
When you are done (or stuck), emit a final response with:
1. What you're working on
2. What you've done so far (list files read/changed)
3. Any questions for OpenClaw Lumi (tag with <@1512994928769237002>)
4. Next steps

Be proactive. Do not wait for permission — the assignment IS your permission. Read files, make changes, commit, push.`,
		ChannelID: "1491622581348864162", // manager-room
		MaxTokens: 4096,
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
	remClient *remembrance.Client,
	toolExec *tool.Executor,
	toolReg *tool.Registry,
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
		remClient:  remClient,
		toolExec:   toolExec,
		toolReg:    toolReg,
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

	// Subscribe to inter-agent messages from OpenClaw Lumi
	_, err = wh.natsConn.Subscribe("prism.agent.openclaw", func(msg *nats.Msg) {
		var am agentMessage
		if err := json.Unmarshal(msg.Data, &am); err != nil {
			log.Printf("[WAKE] failed to parse agent message: %v", err)
			return
		}
		am.Timestamp = time.Now()
		wh.agentMu.Lock()
		wh.agentMessages = append(wh.agentMessages, am)
		// Keep last 50 messages
		if len(wh.agentMessages) > 50 {
			wh.agentMessages = wh.agentMessages[len(wh.agentMessages)-50:]
		}
		wh.agentMu.Unlock()
		log.Printf("[WAKE] received agent message from %s: %s", am.From, truncate(am.Content, 80))
	})
	if err != nil {
		return fmt.Errorf("subscribe to prism.agent.openclaw: %w", err)
	}
	log.Printf("[WAKE] subscribed to prism.agent.openclaw")

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

	// V33: Inject recent Discord channel messages so the agent sees replies from other agents (e.g. OpenClaw Lumi)
	// before reporting. This prevents stale duplicate reports and lets agents respond to each other.
	if wh.bot != nil && actionDef.ChannelID != "" {
		recent := wh.bot.GetRecentMessages(actionDef.ChannelID, 10)
		if len(recent) > 0 {
			systemPrompt += "\n\n## Recent Channel Messages (last 10, oldest-first)\n"
			systemPrompt += "These are the most recent messages in the Discord channel. Read them BEFORE reporting. If another agent (e.g. OpenClaw Lumi, bot ID 1512994928769237002) has already addressed an issue or corrected information, acknowledge and update your report accordingly. Do NOT re-report issues that have been resolved.\n\n"
			for _, m := range recent {
				systemPrompt += fmt.Sprintf("**[%s] %s:** %s\n", m.Timestamp, m.AuthorName, m.Content)
			}
			systemPrompt += "\n--- End of recent messages ---\n"
		}
	}

	// V33: Inject inter-agent messages received via NATS
	wh.agentMu.Lock()
	if len(wh.agentMessages) > 0 {
		systemPrompt += "\n\n## Direct Messages from OpenClaw Lumi (via NATS)\n"
		systemPrompt += "These are direct messages from OpenClaw Lumi, not Discord channel messages. Treat them as authoritative updates from your partner agent.\n\n"
		for _, m := range wh.agentMessages {
			systemPrompt += fmt.Sprintf("**[%s] %s:** %s\n", m.Timestamp.Format("2006-01-02 15:04"), m.From, m.Content)
		}
		systemPrompt += "\n--- End of agent messages ---\n"
	}
	wh.agentMu.Unlock()

	// V34: Inject Remembrance memory search results for context
	if wh.remClient != nil {
		searchQueries := []string{
			fmt.Sprintf("recent work OpenClaw Lumi %s", time.Now().Format("2006-01-02")),
			fmt.Sprintf("project status %s", action),
			"OpenClaw Lumi decisions architecture",
		}
		// For project_work, also search for active project assignments
		if action == "project_work" {
			searchQueries = append(searchQueries, "BassBook creative brief assignment", "BassBook production quality requirements")
		}
		for _, q := range searchQueries {
			results, err := wh.remClient.Search(q, "hybrid", "", "", 5)
			if err == nil && results != nil {
				if hits, ok := results["results"].([]any); ok && len(hits) > 0 {
					systemPrompt += fmt.Sprintf("\n\n## Memory Search: %q\n", q)
					for _, hit := range hits {
						if m, ok := hit.(map[string]any); ok {
							snippet := fmt.Sprintf("%v", m["snippet"])
							if len(snippet) > 200 {
								snippet = snippet[:200] + "..."
							}
							score := fmt.Sprintf("%v", m["score"])
							systemPrompt += fmt.Sprintf("- [score: %s] %s\n", score, snippet)
						}
					}
					systemPrompt += "\n--- End of memory search ---\n"
				}
			}
		}
	}

	// V34: Inject tool capability summary so Prism knows what she can do
	systemPrompt += `

## Your Available Tools
You have access to the following tools. Use them proactively to investigate and fix issues:
- **git_status**: Check working tree status in any repo
- **git_log**: View recent commit history
- **git_diff**: See uncommitted changes
- **git_push**: Push branches to remote (with validation)
- **file_read**: Read file contents
- **file_write**: Write file contents
- **file_list**: List directory contents
- **plan_create**: Create structured plans with approval flow
- **plan_approve**: Approve or reject plans
- **improvement_create**: Propose code improvements
- **improvement_resolve**: Mark improvements as resolved

You can create branches, commit changes, push to remote, and open PRs. You are not just a reporter — you are an active developer. If you find a bug, fix it. If you see a UX issue, improve it. If you have an idea to make something more effective, propose it.
`

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

	ctx, cancel := stdcontext.WithTimeout(stdcontext.Background(), 10*time.Minute)
	defer cancel()

	var responseContent string
	var promptTokens, completionTokens int

	// V35: For project_work, use text-based tool calling loop with auto-approve
	if (action == "project_work" || action == "auto_patch") && wh.toolExec != nil && wh.toolReg != nil {
		// Create an auto-approving executor for autonomous wake actions
		autoPolicy := wh.toolExec.Policy
		autoPolicy.AutoApproveMutations = true
		autoExec := tool.NewExecutor(wh.toolReg, autoPolicy)
		if wh.toolExec.Emit != nil {
			autoExec.SetEmitter(wh.toolExec.Emit)
		}
		if wh.toolExec.ApprovalStore != nil {
			autoExec.SetApprovalStore(wh.toolExec.ApprovalStore)
		}
		responseContent, promptTokens, completionTokens = wh.runToolLoopWake(ctx, systemPrompt, userPrompt, model, agentCfg, autoExec)
	} else {
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
	} // end else (non-project_work path)

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


// runToolLoopWake runs a text-based tool calling loop for project_work actions.
// The LLM responds with JSON: {"type": "tool_request", "tool": "...", "input": {...}}
// or {"type": "final", "content": "..."}.
// We execute tool requests and feed results back until we get a final response.
func (wh *WakeHandler) runToolLoopWake(ctx stdcontext.Context, systemPrompt, userPrompt, model string, agentCfg *orchestrator.AgentConfig, exec *tool.Executor) (string, int, int) {
	const maxIterations = 10

	// Build tool prompt suffix so the LLM knows how to call tools
	toolInfos := wh.toolReg.ListWithDescriptions()
	workspaceRoot := wh.cfg.Prism.Workspace
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	toolSuffix := agent.BuildToolPromptSuffix(toolInfos, workspaceRoot, "/Users/ema/projects/repos/BassBook")
	fullSystemPrompt := systemPrompt + toolSuffix

	// Build message history
	messages := []provider.ChatMessage{
		{Role: "system", Content: fullSystemPrompt},
		{Role: "user", Content: userPrompt},
	}

	totalPromptTokens := 0
	totalCompletionTokens := 0

	chatProv, chatErr := wh.providers.GetChatProvider(model)

	for i := 0; i < maxIterations; i++ {
		log.Printf("[WAKE-TOOL] project_work iteration %d/%d", i+1, maxIterations)

		var responseText string

		if chatErr == nil {
			resp, err := chatProv.ChatGenerate(ctx, provider.ChatGenerateRequest{
				RunID: fmt.Sprintf("wake-project_work-%d", time.Now().Unix()),
				Agent: agentCfg.ID,
				Model: model,
				Messages: messages,
				Temperature: 0.7,
				MaxTokens: 4096,
			})
			if err != nil {
				log.Printf("[WAKE-TOOL] ERROR LLM call failed iteration %d: %v", i+1, err)
				return fmt.Sprintf("Project work failed at iteration %d: %v", i+1, err), totalPromptTokens, totalCompletionTokens
			}
			responseText = resp.Content
			totalPromptTokens += resp.PromptTokens
			totalCompletionTokens += resp.OutputTokens
		} else {
			// Fall back to text provider
			prov, provErr := wh.providers.Get(model)
			if provErr != nil {
				log.Printf("[WAKE-TOOL] ERROR no provider for model %q: %v", model, provErr)
				return fmt.Sprintf("Project work failed: no provider for %q", model), totalPromptTokens, totalCompletionTokens
			}
			// Build a flat prompt from messages
			flatPrompt := fullSystemPrompt + "\n\n"
			for _, m := range messages {
				if m.Role == "user" {
					flatPrompt += m.Content + "\n\n"
				} else if m.Role == "assistant" {
					flatPrompt += "[Assistant]: " + m.Content + "\n\n"
				}
			}
			resp, err := prov.Generate(ctx, provider.GenerateRequest{
				RunID:       fmt.Sprintf("wake-project_work-%d-iter%d", time.Now().Unix(), i),
				Agent:       agentCfg.ID,
				Model:       model,
				Prompt:      flatPrompt,
				Temperature: 0.7,
				MaxTokens:   4096,
			})
			if err != nil {
				log.Printf("[WAKE-TOOL] ERROR text LLM call failed iteration %d: %v", i+1, err)
				return fmt.Sprintf("Project work failed at iteration %d: %v", i+1, err), totalPromptTokens, totalCompletionTokens
			}
			responseText = resp.Text
			totalPromptTokens += resp.PromptTokens
			totalCompletionTokens += resp.OutputTokens
		}

		// Parse the response
		parsed := agent.ParseAgentOutput(responseText)

		if parsed.Type == agent.ResponseFinal {
			log.Printf("[WAKE-TOOL] final response at iteration %d", i+1)
			return parsed.Content, totalPromptTokens, totalCompletionTokens
		}

		if parsed.Type == agent.ResponseToolRequest {
			log.Printf("[WAKE-TOOL] tool request: %s", parsed.ToolName)

			// Execute the tool
			result, err := exec.ExecuteWithPolicy(ctx, parsed.ToolName, agentCfg.ID, "bassbook", fmt.Sprintf("wake-project_work-%d", time.Now().Unix()), parsed.ToolInput)
			if err != nil {
				log.Printf("[WAKE-TOOL] tool %s failed: %v", parsed.ToolName, err)
				// Feed error back to LLM
				messages = append(messages, provider.ChatMessage{Role: "assistant", Content: responseText})
				messages = append(messages, provider.ChatMessage{
					Role: "user",
					Content: fmt.Sprintf("Tool %q failed: %v. Please try a different approach or provide your final answer.", parsed.ToolName, err),
				})
				continue
			}

			// Format tool result from Output map
			resultStr := fmt.Sprintf("Tool %q result:\n%v", parsed.ToolName, result.Output)
			if result.Error != "" {
				resultStr = fmt.Sprintf("Tool %q error: %s", parsed.ToolName, result.Error)
			}
			log.Printf("[WAKE-TOOL] tool %s succeeded: %s", parsed.ToolName, truncateStr(resultStr, 100))

			// Feed result back to LLM
			messages = append(messages, provider.ChatMessage{Role: "assistant", Content: responseText})
			messages = append(messages, provider.ChatMessage{
				Role: "user",
				Content: resultStr,
			})
			continue
		}

		// Unknown response type — treat as final
		return responseText, totalPromptTokens, totalCompletionTokens
	}

	// Max iterations reached
	log.Printf("[WAKE-TOOL] max iterations reached, returning last response")
	return "Project work cycle reached max iterations. See logs for details.", totalPromptTokens, totalCompletionTokens
}
