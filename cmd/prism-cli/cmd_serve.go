// Package main implements the `prism serve` subcommand — the persistent daemon
// that runs Prism as a live service.
//
// Usage:
//
//	prism serve [--config prism.yaml] [--port 8321]
//
// The serve command:
//  1. Loads prism.yaml configuration
//  2. Starts the embedded NATS server
//  3. Registers agents from config
//  4. Starts the session manager
//  5. Sets up the agent router
//  6. Connects channel adapters (Discord, etc.)
//  7. Registers action triggers
//  8. Starts the health check server
//  9. Blocks until SIGINT/SIGTERM
//
// V20-respond: The serve command now wires the full conversation pipeline:
// Discord message → debounce → route → session → LLM → response → Discord.
package main

import (
	ctxcontext "context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/emaharmony/prism/internal/action"
	"github.com/emaharmony/prism/internal/convstage"
	"github.com/emaharmony/prism/internal/context"
	"github.com/emaharmony/prism/internal/adapter/builtin/discordbot"
	"github.com/emaharmony/prism/internal/agent"
	"github.com/emaharmony/prism/internal/bus"
	"github.com/emaharmony/prism/internal/debounce"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/provider/anthropic"
	"github.com/emaharmony/prism/internal/provider/gemini"
	"github.com/emaharmony/prism/internal/provider/ollama"
	"github.com/emaharmony/prism/internal/provider/openai"
	"github.com/emaharmony/prism/internal/remembrance"
	"github.com/emaharmony/prism/internal/router"
	"github.com/emaharmony/prism/internal/runtrack"
	"github.com/emaharmony/prism/internal/session"

	"github.com/nats-io/nats.go"
)

// botMessageSender adapts discordbot.BotAdapter to implement convstage.MessageSender.
// This allows the ConversationStage to send/edit Discord messages without
// depending on the concrete BotAdapter type.
type botMessageSender struct {
	bot *discordbot.BotAdapter
}

func (b *botMessageSender) Send(channelID, content string) error {
	return b.bot.Send(&discordbot.OutboundMessage{
		ChannelID: channelID,
		Content:   content,
	})
}

func (b *botMessageSender) SendPlaceholder(channelID, content string) (string, error) {
	return b.bot.SendPlaceholder(channelID, content)
}

func (b *botMessageSender) EditMessage(channelID, messageID, content string) error {
	return b.bot.EditMessage(channelID, messageID, content)
}

// conversationContext holds all the dependencies needed to process a
// Discord message through the full pipeline. It's closed over by the
// OnMessage handler so each message has access to routing, sessions,
// LLM providers, and the Discord bot for responses.
type conversationContext struct {
	router      *router.Router
	sessMgr     *session.Manager
	cfg         *orchestrator.Config
	providers   *provider.ProviderRegistry
	bot         *discordbot.BotAdapter
	debounce    *debounce.Tracker
	eventLog    *runtrack.EventLogger
	cancelReg   *runtrack.CancelRegistry
	ctxBuilder  *context.Builder    // V21: workspace context injection
	natsConn    *nats.Conn           // V21: NATS bus connection for event publishing
	actionReg   *action.Registry     // V21: action registry for event-triggered actions
	remClient   *remembrance.Client   // V21: Remembrance client for memory auto-save
}

func executeServe(args []string) {
	serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := serveCmd.String("config", "prism.yaml", "Path to prism.yaml configuration file")
	portFlag := serveCmd.Int("port", 8321, "Health check server port")
	busURL := serveCmd.String("bus-url", "", "NATS bus URL (empty = embedded)")

	serveCmd.Parse(args)

	fmt.Println("🔮 Starting Prism...")

	// 1. Load configuration
	cfg, err := orchestrator.LoadConfig(*configPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: config file %q not found. Create one with 'prism init' or specify --config\n", *configPath)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Override NATS URL if provided
	if *busURL != "" {
		cfg.Prism.NATSURL = *busURL
	}

	fmt.Printf("  Agents: %v\n", agentNames(cfg))
	fmt.Printf("  Primary: %s\n", primaryName(cfg))
	fmt.Printf("  Sessions: max=%d, idle=%dm, compaction=%s\n",
		cfg.Sessions.MaxContextMessages,
		cfg.Sessions.IdleTimeoutMinutes,
		cfg.Sessions.CompactionStrategy,
	)

	// 2. Start embedded NATS
	natsURL := cfg.Prism.NATSURL
	var natsCleanup func()
	if natsURL == "" {
		url, cleanup, err := bus.StartEmbeddedBus(0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error starting embedded bus: %v\n", err)
			os.Exit(1)
		}
		natsURL = url
		natsCleanup = cleanup
		fmt.Printf("  NATS: embedded at %s\n", natsURL)
	} else {
		fmt.Printf("  NATS: external at %s\n", natsURL)
	}

	// 2b. Connect to NATS
	natsConn, err := bus.ConnectToBus(natsURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error connecting to NATS: %v\n", err)
		os.Exit(1)
	}
	defer natsConn.Close()
	fmt.Println("  NATS: connected")

	// 3. Register agents
	agentReg := agent.NewRegistry()
	if err := cfg.RegisterAgents(agentReg); err != nil {
		fmt.Fprintf(os.Stderr, "Error registering agents: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Registered %d agents\n", len(cfg.Agents))

	// 4. Set up LLM providers from agent configs
	provReg := provider.NewProviderRegistry()
	if err := registerProviders(cfg, provReg); err != nil {
		fmt.Fprintf(os.Stderr, "Error registering providers: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Providers: %d models registered\n", len(providerModelIDs(provReg)))

	// 5. Start session manager
	if err := os.MkdirAll(cfg.Prism.DataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating data directory: %v\n", err)
		os.Exit(1)
	}
	dbPath := cfg.Prism.DataDir + "/sessions.db"
	sessMgr, err := session.NewManager(
		dbPath,
		cfg.Sessions.MaxContextMessages,
		time.Duration(cfg.Sessions.IdleTimeoutMinutes)*time.Minute,
		cfg.Sessions.DailyResetHour,
		cfg.Sessions.CompactionStrategy,
	)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting session manager: %v\n", err)
		os.Exit(1)
	}
	defer sessMgr.Close()
	fmt.Println("  Session manager: ready")

	// 6. Set up agent router
	rtr := router.New(agentReg, cfg)
	fmt.Println("  Router: ready")

	// 7. Set up action registry
	actionReg := action.NewRegistry()
	for _, a := range cfg.Actions {
		act := action.Action{
			Trigger: a.Trigger,
			Action:  a.Action,
			Enabled: a.Enabled,
		}
		if err := actionReg.RegisterAction(act); err != nil {
			fmt.Fprintf(os.Stderr, "Error registering action: %v\n", err)
			os.Exit(1)
		}
	}
	fmt.Printf("  Registered %d actions\n", len(cfg.Actions))

	// 8. Set up debounce, event logger, and cancel registry
	msgDebounce := debounce.New(
		debounce.WithInterval(500*time.Millisecond),
		debounce.WithOnDrop(func(key string) {
			log.Printf("[DEBOUNCE] dropped message from %s", key)
		}),
	)
	eventLog := &runtrack.EventLogger{}
	cancelReg := runtrack.NewCancelRegistry()

	// 9. Connect channel adapters (Discord, etc.)
	ctx, cancel := ctxcontext.WithCancel(ctxcontext.Background())
	defer cancel()

	var discordBots []*discordbot.BotAdapter

	for _, ch := range cfg.Channels {
		switch ch.Type {
		case "discord":
			bot := discordbot.NewBotAdapter(ch.Token)

			// V21: Build workspace context injection
			var ctxBuilder *context.Builder
			if cfg.Prism.Workspace != "" {
				ctxBuilder = context.NewBuilder(cfg.Prism.Workspace)
			} else {
				// Default: use home directory + .openclaw/workspace
				ctxBuilder = context.NewBuilder(filepath.Join(os.Getenv("HOME"), ".openclaw", "workspace"))
			}

			// V21: Create Remembrance client if enabled
			var remClient *remembrance.Client
			if cfg.Remembrance.Enabled {
				remClient = remembrance.NewClient(cfg.Remembrance.URL)
				if remClient.IsAvailable() {
					fmt.Println("  Remembrance: connected")
				} else {
					log.Printf("[WARN] Remembrance enabled but not reachable at %s", cfg.Remembrance.URL)
					remClient = nil // Disable gracefully
				}
			}

			// Build conversation context for this bot
			convCtx := &conversationContext{
				router:    rtr,
				sessMgr:   sessMgr,
				cfg:       cfg,
				providers: provReg,
				bot:       bot,
				debounce:  msgDebounce,
				eventLog:  eventLog,
				cancelReg: cancelReg,
				ctxBuilder: ctxBuilder,
				natsConn:  natsConn,
				actionReg: actionReg,
				remClient: remClient,
			}

			bot.OnMessage(func(msg *discordbot.InboundMessage) {
				convCtx.handleDiscordMessage(msg)
			})

			go func() {
				if err := bot.Start(ctx); err != nil {
					log.Printf("Discord bot error: %v", err)
				}
			}()

			discordBots = append(discordBots, bot)
			fmt.Printf("  Discord: connecting\n")

		default:
			fmt.Fprintf(os.Stderr, "Warning: unknown channel type %q\n", ch.Type)
		}
	}

	// 10. Start health check server
	go startHealthServer(*portFlag, cfg, agentReg, sessMgr, discordBots)
	fmt.Printf("  Health: http://localhost:%d/health\n", *portFlag)

	fmt.Println()
	fmt.Println("🔮 Prism is running. Press Ctrl+C to stop.")

	// 11. Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\n🛑 Shutting down Prism...")

	// Cleanup
	for _, bot := range discordBots {
		bot.Stop()
	}
	if natsCleanup != nil {
		natsCleanup()
	}

	fmt.Println("✅ Prism stopped.")
}

// handleDiscordMessage processes an incoming Discord message through the
// full conversation pipeline:
//  1. Debounce (drop rapid-fire messages)
//  2. Route to the appropriate agent
//  3. Find or create a session
//  4. Send typing indicator
//  5. Build prompt from session history
//  6. Call the LLM provider
//  7. Save agent response to session
//  8. Send response to Discord
//  9. Emit structured events for observability
func (cc *conversationContext) handleDiscordMessage(msg *discordbot.InboundMessage) {
	// Step 1: Debounce — drop rapid-fire messages from the same user
	debounceKey := msg.UserID + ":" + msg.ChannelID
	if !cc.debounce.Allow(debounceKey) {
		return
	}

	// Step 2: Route the message to the appropriate agent
	result := cc.router.Route(msg.Content)

	cc.eventLog.Log("message.routed", "", result.AgentID, map[string]any{
		"user_id":    msg.UserID,
		"channel_id": msg.ChannelID,
		"method":     result.Method,
		"clean_text": result.CleanedContent,
	})
	cc.publishEvent("prism.channel.received", map[string]any{
		"user_id":    msg.UserID,
		"channel_id": msg.ChannelID,
		"agent_id":  result.AgentID,
		"method":     result.Method,
	})

	// Step 3: Find or create a session
	sess, err := cc.sessMgr.FindActive("discord", msg.ChannelID, msg.UserID)
	if err != nil {
		log.Printf("[ERROR] find session: %v", err)
		return
	}
	if sess == nil {
		sess, err = cc.sessMgr.Create(result.AgentID, "discord", msg.ChannelID, msg.UserID)
		if err != nil {
			log.Printf("[ERROR] create session: %v", err)
			return
		}
	}

	// Add the user message to the session
	_, err = cc.sessMgr.AddMessage(sess.ID, "user", msg.Content, "")
	if err != nil {
		log.Printf("[ERROR] add user message: %v", err)
		return
	}

	// Step 4: Look up the agent's provider and model
	agentCfg := cc.findAgentConfig(result.AgentID)
	if agentCfg == nil {
		log.Printf("[ERROR] no config for agent %s", result.AgentID)
		cc.sendError(msg.ChannelID, "I'm not configured properly. Please contact the administrator.")
		return
	}

	llmProvider, err := cc.providers.Get(agentCfg.Model)
	if err != nil {
		log.Printf("[ERROR] no provider for model %s: %v", agentCfg.Model, err)
		cc.sendError(msg.ChannelID, "I can't reach my language model right now. Please try again in a moment.")
		return
	}

	// Step 5: Create a run with proper context for cancellation and timeout
	runCtx, runCancel := ctxcontext.WithTimeout(ctxcontext.Background(), 60*time.Second)
	run := runtrack.NewRun(result.AgentID, sess.ID, agentCfg.Model, agentCfg.Provider)
	run.Cancel = runCancel // Wire the actual cancel func
	cc.cancelReg.Register(sess.ID, runCancel)
	defer cc.cancelReg.Unregister(sess.ID)

	cc.eventLog.Log("run.started", run.ID, result.AgentID, map[string]any{
		"model":      agentCfg.Model,
		"provider":   agentCfg.Provider,
		"session_id": sess.ID,
	})
	cc.publishEvent(result.AgentID+".run.started", map[string]any{
		"run_id":     run.ID,
		"model":      agentCfg.Model,
		"provider":   agentCfg.Provider,
		"session_id": sess.ID,
	})

	// Step 6: Send typing indicator
	if err := cc.bot.Typing(msg.ChannelID); err != nil {
		log.Printf("[WARN] typing indicator failed: %v", err)
	}

	// Step 7: Build prompt and execute via ConversationStage
	prompt := cc.buildPrompt(sess, agentCfg)

	cc.eventLog.Log("agent.llm.calling", run.ID, result.AgentID, map[string]any{
		"model":        agentCfg.Model,
		"prompt_length": len(prompt),
	})
	cc.publishEvent(result.AgentID+".llm.calling", map[string]any{
		"run_id":        run.ID,
		"model":          agentCfg.Model,
		"prompt_length": len(prompt),
	})

	// Use ConversationStage for LLM call + response delivery
	sender := &botMessageSender{bot: cc.bot}
	stage := convstage.NewConversationStage(sender, llmProvider, agentCfg.Model, result.AgentID)

	stageResult, err := stage.ExecuteStreaming(runCtx, prompt, msg.ChannelID)
	if err != nil {
		log.Printf("[ERROR] LLM call failed (run %s): %v", run.ID, err)
		cc.eventLog.Log("agent.llm.error", run.ID, result.AgentID, map[string]any{
			"error": err.Error(),
		})
		cc.sendError(msg.ChannelID, "I'm having trouble thinking right now. Please try again in a moment.")
		return
	}

	cc.eventLog.Log("agent.llm.completed", run.ID, result.AgentID, map[string]any{
		"latency_ms":     stageResult.LatencyMS,
		"output_tokens":  stageResult.OutputTokens,
		"streamed":       stageResult.Streamed,
	})
	cc.publishEvent(result.AgentID+".llm.completed", map[string]any{
		"run_id":          run.ID,
		"latency_ms":      stageResult.LatencyMS,
		"output_tokens":   stageResult.OutputTokens,
		"streamed":        stageResult.Streamed,
	})

	// Step 8: Save agent response to session
	_, err = cc.sessMgr.AddMessage(sess.ID, "agent", stageResult.Text, result.AgentID)
	if err != nil {
		log.Printf("[WARN] failed to save agent response to session %s: %v", sess.ID, err)
	}

	// Step 9 is now handled by ConversationStage (streaming or sync delivery)

	// Remembrance auto-save uses stageResult.Text

	cc.eventLog.Log("run.completed", run.ID, result.AgentID, map[string]any{
		"latency_ms": run.Elapsed().Milliseconds(),
	})

	cc.publishEvent(result.AgentID+".run.completed", map[string]any{
		"run_id":       run.ID,
		"latency_ms":   run.Elapsed().Milliseconds(),
		"output_length": len(stageResult.Text),
		"output_tokens": stageResult.OutputTokens,
	})
	cc.publishEvent(result.AgentID+".channel.sent", map[string]any{
		"run_id":     run.ID,
		"channel_id": msg.ChannelID,
		"user_id":    msg.UserID,
	})

	// Step 10: Auto-save to Remembrance (V21)
	// Asynchronously capture the agent output for long-term memory.
	// Uses a 30s timeout context to bound the goroutine lifetime.
	// Fire-and-forget: memory save failure never blocks the response.
	if cc.remClient != nil {
		captureAgentID := result.AgentID
		captureRunID := run.ID
		captureText := stageResult.Text
		captureCtx, captureCancel := ctxcontext.WithTimeout(ctxcontext.Background(), 30*time.Second)

		go func() {
			defer captureCancel()

			result, err := cc.remClient.Capture(
				captureText,
				captureAgentID,
				"conversation",
				"working",
			)
			if err != nil {
				log.Printf("[REMEMBRANCE] capture failed (run %s): %v", captureRunID, err)
				return
			}
			// Log capture metadata for observability
			if decision, ok := result["decision"]; ok {
				log.Printf("[REMEMBRANCE] run %s: decision=%v, captured", captureRunID, decision)
			} else {
				log.Printf("[REMEMBRANCE] captured output from run %s", captureRunID)
			}

			_ = captureCtx // Use context for timeout bounding
		}()
	}

	log.Printf("[RUN] %s completed in %s", run, run.Elapsed().Round(time.Millisecond))
}

// sendError sends a user-friendly error message to a Discord channel.
func (cc *conversationContext) sendError(channelID, message string) {
	err := cc.bot.Send(&discordbot.OutboundMessage{
		ChannelID: channelID,
		Content:   "⚠️ " + message,
	})
	if err != nil {
		log.Printf("[ERROR] failed to send error message to Discord: %v", err)
	}
}

// publishEvent publishes an event to the NATS bus.
// V21: Events use per-agent namespace prefixes (<agent-id>.*).
// System events use the prism.* namespace.
// If NATS is not connected, the event is logged but not published.
// All events include a schema version field for forward compatibility.
func (cc *conversationContext) publishEvent(subject string, payload map[string]any) {
	// Add schema version to all events
	if payload == nil {
		payload = map[string]any{}
	}
	payload["v"] = 1
	if cc.natsConn == nil || !cc.natsConn.IsConnected() {
		log.Printf("[EVENT] %s (NATS not connected, skipped)", subject)
		return
	}

	data, err := json.Marshal(payload)
	if err != nil {
		log.Printf("[EVENT] marshal error for %s: %v", subject, err)
		return
	}

	if err := cc.natsConn.Publish(subject, data); err != nil {
		log.Printf("[EVENT] publish error for %s: %v", subject, err)
		return
	}

	log.Printf("[EVENT] → %s", subject)
}

// findAgentConfig looks up the AgentConfig for a given agent ID.
func (cc *conversationContext) findAgentConfig(agentID string) *orchestrator.AgentConfig {
	for i := range cc.cfg.Agents {
		if cc.cfg.Agents[i].ID == agentID {
			return &cc.cfg.Agents[i]
		}
	}
	return nil
}

// buildPrompt constructs the LLM prompt from session history and injected context.
// V21: System prompt is built from:
//   1. Agent identity (role + name)
//   2. Workspace context (SOUL.md, AGENTS.md, USER.md, etc.) based on agent's `context` config
//   3. Conversation history
func (cc *conversationContext) buildPrompt(sess *session.Session, agentCfg *orchestrator.AgentConfig) string {
	var sb strings.Builder

	// --- System prompt: Agent identity ---
	sb.WriteString(fmt.Sprintf("You are %s, a %s assistant.\n", agentCfg.ID, agentCfg.Role))

	// --- System prompt: Workspace context injection (V21) ---
	// The agent's `context` field in prism.yaml controls which sources to inject.
	// Example: context: [soul, agents, user] → inject SOUL.md, AGENTS.md, USER.md
	if len(agentCfg.Context) > 0 && cc.ctxBuilder != nil {
		// Use configurable token budget from PrismConfig (default: 4000)
		budget := cc.cfg.Prism.ContextTokenBudget
		if budget <= 0 {
			budget = 4000
		}

		builder := context.NewBuilder(cc.ctxBuilder.WorkspaceRoot).
			WithNamedContexts(agentCfg.Context).
			WithTokenBudget(budget)

		injected, err := builder.Build()
		if err == nil && injected.FormattedString != "" {
			sb.WriteString("\n")
			sb.WriteString(injected.FormattedString)
			log.Printf("[CONTEXT] Injected %d tokens from %d sources (hash: %s)",
				injected.TotalTokens, len(injected.Files), injected.ContentHash[:12])
		}
	}

	sb.WriteString("\nRespond helpfully and with the personality and principles described above.\n\n")

	// --- Conversation history ---
	for _, msg := range sess.Messages {
		switch msg.Role {
		case "user":
			sb.WriteString(fmt.Sprintf("User: %s\n", msg.Content))
		case "agent":
			sb.WriteString(fmt.Sprintf("%s: %s\n", msg.AgentID, msg.Content))
		case "system":
			sb.WriteString(fmt.Sprintf("System: %s\n", msg.Content))
		}
	}

	sb.WriteString(fmt.Sprintf("%s:", agentCfg.ID))
	return sb.String()
}

// registerProviders creates LLM providers from agent configs and registers them.
func registerProviders(cfg *orchestrator.Config, reg *provider.ProviderRegistry) error {
	// V20: We register providers based on agent configs.
	// Each agent specifies its provider and model.
	// For now, we support Ollama (local/cloud), OpenAI, Anthropic, and Gemini.
	//
	// V21 will add provider chains, fallbacks, and cost tiers.
	for _, agentCfg := range cfg.Agents {
		p, info, err := createProvider(agentCfg)
		if err != nil {
			return fmt.Errorf("agent %s: %w", agentCfg.ID, err)
		}
		reg.Register(agentCfg.Model, p, info)
	}
	return nil
}

// createProvider creates a provider instance for an agent config.
// V20 supports: ollama, openai, anthropic, gemini.
func createProvider(agentCfg orchestrator.AgentConfig) (provider.Provider, provider.ModelInfo, error) {
	info := provider.ModelInfo{
		ID:           agentCfg.Model,
		ProviderName: agentCfg.Provider,
	}

	switch agentCfg.Provider {
	case "ollama":
		// V20: Use the Ollama provider with default localhost endpoint.
		// V21: Read base URL from config.
		p, err := createOllamaProvider(agentCfg.Model)
		if err != nil {
			return nil, info, fmt.Errorf("ollama provider: %w", err)
		}
		return p, info, nil

	case "openai":
		p, err := createOpenAIProvider(agentCfg.Model)
		if err != nil {
			return nil, info, fmt.Errorf("openai provider: %w", err)
		}
		return p, info, nil

	case "anthropic":
		p, err := createAnthropicProvider(agentCfg.Model)
		if err != nil {
			return nil, info, fmt.Errorf("anthropic provider: %w", err)
		}
		return p, info, nil

	case "gemini":
		p, err := createGeminiProvider(agentCfg.Model)
		if err != nil {
			return nil, info, fmt.Errorf("gemini provider: %w", err)
		}
		return p, info, nil

	default:
		return nil, info, fmt.Errorf("unsupported provider: %s (supported: ollama, openai, anthropic, gemini)", agentCfg.Provider)
	}
}

// createOllamaProvider creates an Ollama provider instance.
// Uses localhost:11434 by default. V21: configurable base URL.
func createOllamaProvider(model string) (provider.Provider, error) {
	p := ollama.New("") // empty = default localhost:11434
	return p, nil
}

// createOpenAIProvider creates an OpenAI provider instance.
func createOpenAIProvider(model string) (provider.Provider, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}
	return openai.New(apiKey), nil
}

// createAnthropicProvider creates an Anthropic provider instance.
func createAnthropicProvider(model string) (provider.Provider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}
	return anthropic.New(apiKey), nil
}

// createGeminiProvider creates a Gemini provider instance.
func createGeminiProvider(model string) (provider.Provider, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}
	return gemini.New(apiKey), nil
}

// providerModelIDs returns all registered model IDs (for logging).
func providerModelIDs(reg *provider.ProviderRegistry) []string {
	return reg.ListModels()
}

// startHealthServer starts a simple HTTP server for health checks.
func startHealthServer(port int, cfg *orchestrator.Config, agentReg *agent.Registry, sessMgr *session.Manager, discordBots []*discordbot.BotAdapter) {
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		discordReady := false
		for _, bot := range discordBots {
			if bot.IsReady() {
				discordReady = true
				break
			}
		}
		fmt.Fprintf(w, `{"status":"ok","agents":%d,"discord_ready":%v}`, len(cfg.Agents), discordReady)
	})

	if err := http.ListenAndServe(fmt.Sprintf(":%d", port), nil); err != nil {
		log.Printf("Health server error: %v", err)
	}
}

func agentNames(cfg *orchestrator.Config) []string {
	names := make([]string, len(cfg.Agents))
	for i, a := range cfg.Agents {
		names[i] = a.ID
	}
	return names
}

func primaryName(cfg *orchestrator.Config) string {
	p := cfg.PrimaryAgent()
	if p == nil {
		return "(none)"
	}
	return p.ID
}