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
	stdctx "context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/emaharmony/prism/internal/action"
	"github.com/emaharmony/prism/internal/adapter/builtin/discordbot"
	"github.com/emaharmony/prism/internal/agent"
	"github.com/emaharmony/prism/internal/api"
	"github.com/emaharmony/prism/internal/bus"
	"github.com/emaharmony/prism/internal/codexworker"
	"github.com/emaharmony/prism/internal/context"
	"github.com/emaharmony/prism/internal/crossprism"
	"github.com/emaharmony/prism/internal/debounce"
	"github.com/emaharmony/prism/internal/delegation"
	"github.com/emaharmony/prism/internal/factory"
	"github.com/emaharmony/prism/internal/guard"
	"github.com/emaharmony/prism/internal/improve"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/plan"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/provider/anthropic"
	"github.com/emaharmony/prism/internal/provider/gemini"
	"github.com/emaharmony/prism/internal/provider/ollama"
	"github.com/emaharmony/prism/internal/provider/openai"
	"github.com/emaharmony/prism/internal/remembrance"
	"github.com/emaharmony/prism/internal/router"
	"github.com/emaharmony/prism/internal/runtrack"
	"github.com/emaharmony/prism/internal/safety"
	"github.com/emaharmony/prism/internal/scheduler"
	"github.com/emaharmony/prism/internal/session"
	"github.com/emaharmony/prism/internal/stage"
	"github.com/emaharmony/prism/internal/state"
	"github.com/emaharmony/prism/internal/task"
	"github.com/emaharmony/prism/internal/tool"

	"github.com/nats-io/nats.go"
)

// natsPublisherAdapter wraps a nats.Conn to implement stage.NatsPublisher.
type natsPublisherAdapter struct {
	conn *nats.Conn
}

func (n *natsPublisherAdapter) Publish(subject string, data []byte) error {
	if n.conn == nil {
		return fmt.Errorf("NATS not connected")
	}
	return n.conn.Publish(subject, data)
}

// discordBotClient abstracts the Discord operations needed by the handler,
// making it testable without a real Discord connection.
type discordBotClient interface {
	Typing(channelID string) error
	Send(msg *discordbot.OutboundMessage) error
	SendPlaceholder(channelID, content string) (string, error)
	EditMessage(channelID, messageID, content string) error
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
	bot         discordBotClient
	debounce    *debounce.Tracker
	eventLog    *runtrack.EventLogger
	cancelReg   *runtrack.CancelRegistry
	ctxBuilder  *context.Builder         // V21: workspace context injection
	natsConn    *nats.Conn               // V21: NATS bus connection for event publishing
	natsURL     string                   // V21: NATS bus URL
	actionReg   *action.Registry         // V21: action registry for event-triggered actions
	remClient   *remembrance.Client      // V21: Remembrance client for memory auto-save
	remSem      chan struct{}            // V21: Semaphore limiting concurrent Remembrance goroutines (max 4)
	remCache    *remembranceCache        // V26: TTL cache for BuildContext results
	delegEngine *delegation.Engine       // V22: Delegation engine for agent-to-agent task delegation
	taskStore   *task.Store              // V22: Task store for delegation tracking
	crossCoord  *crossprism.Coordinator  // Cross-Prism NATS delegation coordinator
	toolExec    *tool.Executor           // V27: Tool executor for file system access
	stateMgr    *state.Manager           // V32: Working state manager for adaptive context
	planMgr     *plan.Manager            // V32: Plan manager for plan-first pipeline
	improveMgr  *improve.Manager         // V32: Self-improvement loop
	guardian    *guard.Guard             // V32: Guard rail for plan enforcement
	toolPolicy  tool.PolicyConfig        // V27: Tool policy configuration
	rateLimiter *safety.UserRateLimiter  // V28: Per-user rate limiting
	toolGate    *stage.ToolRelevanceGate // P-008: Tool relevance gate

	// Cached static system content — built once, reused every message.
	staticSystemText string // For text-based provider path
	staticSystemChat string // For ChatProvider path (includes toolUsageGuidance)
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

	// V22: Task store and delegation engine (hoisted for API server)
	var (
		taskStore   *task.Store
		delegEngine *delegation.Engine
		orch        *orchestrator.Orchestrator
		remClient   *remembrance.Client
		codexWorker *codexworker.Worker
	)
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

	taskStore, err = task.NewStore(filepath.Join(cfg.Prism.DataDir, "tasks.db"))
	if err != nil {
		fmt.Printf("  Warning: task store failed: %v\n", err)
	} else {
		delegEngine = delegation.NewEngine(taskStore, natsConn)
		fmt.Println("  Task store: ready")
	}

	if cfg.Codex.Enabled {
		codexCfg := codexConfigFromOrchestrator(cfg.Codex, cfg)
		codexWorker, err = codexworker.New(codexCfg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error configuring Codex worker: %v\n", err)
			os.Exit(1)
		}
		if delegEngine != nil {
			if err := delegEngine.Subscribe("codex", func(ctx ctxcontext.Context, t *task.Task) error {
				result, runErr := codexWorker.RunTask(ctx, t.ID, t.Description, t.Context)
				if runErr != nil {
					return runErr
				}
				return delegEngine.Complete(ctxcontext.Background(), t.ID, result)
			}); err != nil {
				fmt.Fprintf(os.Stderr, "Error subscribing Codex worker: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Println("  Codex worker: enabled")
	}

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

	var crossCoord *crossprism.Coordinator
	var crossSvc *crossprism.Service
	if cfg.Bridge.Enabled {
		secret := bridgeSecret(cfg)
		if secret == "" {
			fmt.Fprintf(os.Stderr, "Error: bridge enabled but %s is not set and bridge.secret is empty\n", cfg.Bridge.SecretEnv)
			os.Exit(1)
		}
		allowedSubjects := cfg.Bridge.AllowedSubjects
		if len(allowedSubjects) == 0 {
			allowedSubjects = crossprism.DefaultSubjects()
		}

		var factoryOp *factory.Operator
		if cfg.Bridge.Factory.Enabled {
			factoryOp, err = factory.NewOperator(factoryConfigFromBridge(cfg.Bridge.Factory))
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error configuring Roblox Factory bridge: %v\n", err)
				os.Exit(1)
			}
		}

		crossCoord = crossprism.NewCoordinator(crossprism.CoordinatorConfig{
			InstanceID:             cfg.Prism.InstanceID,
			LeaderInstance:         cfg.Bridge.LeaderInstance,
			Secret:                 secret,
			NATS:                   natsConn,
			Store:                  taskStore,
			FactoryAdapter:         factoryOp,
			CodexAdapter:           codexWorker,
			TargetProfiles:         crossProfilesFromBridge(cfg.Bridge.TargetProfiles),
			ConfidenceThreshold:    cfg.Bridge.ConfidenceThreshold,
			MaxClarificationRounds: cfg.Bridge.MaxClarificationRounds,
			ReportOnly:             true,
		})

		crossSvc, err = crossprism.NewService(natsConn, crossprism.ServiceConfig{
			InstanceID:      cfg.Prism.InstanceID,
			Secret:          secret,
			AllowedSubjects: allowedSubjects,
			MaxAge:          5 * time.Minute,
		}, func(ctx ctxcontext.Context, subject string, msg crossprism.Message) (*crossprism.Message, error) {
			return crossCoord.Handle(ctx, subject, msg)
		})
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating cross-Prism bridge: %v\n", err)
			os.Exit(1)
		}
		if err := crossSvc.Start(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Error starting cross-Prism bridge: %v\n", err)
			os.Exit(1)
		}
		defer crossSvc.Close()
		fmt.Printf("  Cross-Prism: %s listening on %d signed subject(s)\n", cfg.Prism.InstanceID, len(allowedSubjects))
	}

	var discordBots []*discordbot.BotAdapter

	// V32: State manager, context builder, and plan manager — shared across all channels
	var stateMgr *state.Manager
	var ctxBuildr *context.Builder
	var planMgr *plan.Manager
	var improveMgr *improve.Manager

	for _, ch := range cfg.Channels {
		switch ch.Type {
		case "discord":
			bot := discordbot.NewBotAdapter(ch.Token)

			// V21: Build workspace context injection
			ctxBuildr = nil
			if cfg.Prism.Workspace != "" {
				ctxBuildr = context.NewBuilder(cfg.Prism.Workspace)
			} else {
				// Default: use home directory + .openclaw/workspace
				ctxBuildr = context.NewBuilder(filepath.Join(os.Getenv("HOME"), ".openclaw", "workspace"))
			}

			// V21: Create Remembrance client if enabled
			if cfg.Remembrance.Enabled {
				remClient = remembrance.NewClientWithTimeout(
					cfg.Remembrance.URL,
					remembranceTimeout(cfg),
				)
				if remClient.IsAvailable() {
					fmt.Println("  Remembrance: connected")
				} else {
					log.Printf("[WARN] Remembrance enabled but not reachable at %s", cfg.Remembrance.URL)
					remClient = nil // Disable gracefully
				}
			}

			// V22: Register agent subscriptions against the shared task store.
			if delegEngine != nil {
				// Register agent subscriptions
				for i := range cfg.Agents {
					a := &cfg.Agents[i]
					for _, sub := range a.Subscriptions {
						agentID := a.ID
						// Subscribe this agent to its configured NATS subjects
						// The handler runs the agent's pipeline when a task.created event arrives
						handler := func(agentID string, sub string) func(ctxcontext.Context, *task.Task) error {
							return func(ctx ctxcontext.Context, t *task.Task) error {
								log.Printf("[DELEGATION] agent %s received task %s via %s (type: %s)", agentID, t.ID, sub, t.Type)
								// Task processing will be wired in M3.1d (DelegationStage)
								return nil
							}
						}(agentID, sub)
						if err := delegEngine.Subscribe(agentID, handler); err != nil {
							log.Printf("[WARN] failed to subscribe agent %s to %s: %v", agentID, sub, err)
						}
					}
				}
			}

			// V27/V28: Set up tool executor with full tool suite
			workspaceRoot := cfg.Prism.Workspace
			if workspaceRoot == "" {
				workspaceRoot = "."
			}

			// V30: Resolve allowed paths to absolute
			allowedPaths := make([]string, 0, len(cfg.Prism.AllowedPaths))
			for _, p := range cfg.Prism.AllowedPaths {
				abs, err := filepath.Abs(p)
				if err != nil {
					log.Printf("[WARN] invalid allowed_path %q: %v", p, err)
					continue
				}
				allowedPaths = append(allowedPaths, abs)
				log.Printf("[TOOL] allowed path: %s", abs)
			}

			toolReg := tool.NewRegistry()
			tool.RegisterBuiltins(toolReg, workspaceRoot, 10*1024*1024, allowedPaths...) // all read-only + project tools
			// V28: Git mutation tools (require approval)
			toolReg.Register(&tool.GitAddTool{ToolPaths: tool.ToolPaths{WorkspaceRoot: workspaceRoot, AllowedPaths: allowedPaths}})
			toolReg.Register(&tool.GitCommitTool{ToolPaths: tool.ToolPaths{WorkspaceRoot: workspaceRoot, AllowedPaths: allowedPaths}})
			toolReg.Register(&tool.GitPushTool{ToolPaths: tool.ToolPaths{WorkspaceRoot: workspaceRoot, AllowedPaths: allowedPaths}})
			// V32: State management tools
			stateMgr = state.NewManager(workspaceRoot)
			stateMgr.EnsureDir()
			tool.RegisterStateTools(toolReg, stateMgr)

			// V32: Plan-First Pipeline tools
			planMgr = plan.NewManager(workspaceRoot)
			planMgr.EnsureDir()
			tool.RegisterPlanTools(toolReg, planMgr)

			// V34: Cross-Prism bridge tool — send messages to remote Prism instances
			if crossSvc != nil {
				toolReg.Register(tool.NewSendCrossMessageTool(crossSvc))
			}

			// V32: Self-Improvement Loop
			improveMgr = improve.NewManager(workspaceRoot)
			improveMgr.EnsureDir()

			// V32: Guard rail (plan-first enforcement)
			guardian := guard.NewGuard(planMgr, ctxBuildr)

			toolPolicy := tool.DefaultPolicyConfig()
			// Mutation operations require approval
			toolPolicy.MaxFileSize = 10 * 1024 * 1024 // 10MB for serve mode
			toolPolicy.WorkspaceRoot = workspaceRoot
			toolPolicy.AllowedPaths = allowedPaths
			toolExec := tool.NewExecutor(toolReg, toolPolicy)

			convCtx := &conversationContext{
				router:      rtr,
				sessMgr:     sessMgr,
				cfg:         cfg,
				providers:   provReg,
				bot:         bot,
				debounce:    msgDebounce,
				eventLog:    eventLog,
				cancelReg:   cancelReg,
				ctxBuilder:  ctxBuildr,
				natsConn:    natsConn,
				natsURL:     natsURL,
				actionReg:   actionReg,
				remClient:   remClient,
				remSem:      make(chan struct{}, 4),
				remCache:    newRemembranceCache(60 * time.Second),
				delegEngine: delegEngine,
				taskStore:   taskStore,
				crossCoord:  crossCoord,
				toolExec:    toolExec,
				toolPolicy:  toolPolicy,
				rateLimiter: safety.NewUserRateLimiter(
					10, // max 10 messages per burst per user
					1,  // refill 1 token/sec per user
					60, // global max 60 concurrent requests
					10, // global refill 10 tokens/sec
				),
				toolGate:   stage.NewToolRelevanceGate(true), // P-008: enabled by default
				stateMgr:   stateMgr,                         // V32: shared state manager (same instance as tools)
				planMgr:    planMgr,                          // V32: plan manager
				improveMgr: improveMgr,                       // V32: improvement manager
				guardian:   guardian,                         // V32: guard rail
			}

			// Pre-build static system content for all agents
			for _, a := range cfg.Agents {
				convCtx.rebuildStaticSystemContent(&a)
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

	// 10. Start API server
	var approvalMgr *delegation.ApprovalManager
	var delegTracker *delegation.Tracker
	if taskStore != nil && delegEngine != nil {
		approvalMgr = delegation.NewApprovalManager(taskStore, delegEngine)
		delegTracker = delegation.NewTracker(taskStore, delegEngine, delegation.TrackerConfig{
			TaskTimeout:   10 * time.Minute,
			CheckInterval: 1 * time.Minute,
		})
		go delegTracker.Start(stdctx.Background())
	}

	apiPort := *portFlag + 1 // API on port+1 (default 8322)
	apiServer := api.NewServer(api.Config{
		Addr:     fmt.Sprintf(":%d", apiPort),
		Orch:     orch,
		Store:    taskStore,
		Sessions: sessMgr,
		Engine:   delegEngine,
		Approval: approvalMgr,
		Tracker:  delegTracker,
		NATS:     natsConn,
	})
	go func() {
		if err := apiServer.Start(); err != nil {
			log.Printf("[WARN] API server failed: %v", err)
		}
	}()
	fmt.Printf("  API:    http://localhost:%d/api/v1/status\n", apiPort)

	// 11. Start health check server
	go startHealthServer(*portFlag, cfg, agentReg, sessMgr, discordBots)
	fmt.Printf("  Health: http://localhost:%d/health\n", *portFlag)

	fmt.Println()
	fmt.Println("🔮 Prism is running. Press Ctrl+C to stop.")

	// 11. Start dream cycle scheduler (3AM nightly + event-triggered)
	if remClient != nil {
		go startDreamScheduler(remClient)
	}

	// V32: Start cron-style task scheduler if configured
	if cfg.Prism.Scheduler.Enabled {
		sched := scheduler.NewScheduler(&natsPublisherAdapter{conn: natsConn})
		for _, jobCfg := range cfg.Prism.Scheduler.Jobs {
			schedule, err := scheduler.ParseCron(jobCfg.Schedule)
			if err != nil {
				log.Printf("[SCHEDULER] ERROR parsing cron for job %q: %v", jobCfg.Name, err)
				continue
			}
			sched.AddJob(&scheduler.Job{
				Name:     jobCfg.Name,
				Schedule: schedule,
				Event:    jobCfg.Event,
				Payload:  jobCfg.Payload,
				Enabled:  jobCfg.Enabled,
			})
		}
		sched.StartInBackground()
		log.Printf("[SCHEDULER] started with %d job(s)", len(cfg.Prism.Scheduler.Jobs))
	}

	// V32: Start wake handler to process scheduler events
	// Uses the first Discord bot as the notification channel
	if cfg.Prism.Scheduler.Enabled && len(discordBots) > 0 && natsConn != nil {
		wakeHandler := NewWakeHandler(
			cfg,
			provReg,
			sessMgr,
			stateMgr,
			natsConn,
			discordBots[0], // use first bot for notifications
			ctxBuildr,
			planMgr,
			improveMgr,
		)
		if err := wakeHandler.Start(); err != nil {
			log.Printf("[WAKE] WARN failed to start wake handler: %v", err)
		} else {
			log.Printf("[WAKE] handler started, listening for scheduled events")
		}
	}

	// 12. Wait for shutdown signal
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
// full conversation pipeline.
//
// V21-5: The handler is now an adapter layer — it handles platform-specific
// concerns (debounce, typing, session management, Discord delivery) while
// delegating the domain-agnostic core to the stage pipeline:
//
//	Pipeline: LLMStage → PersistenceStage → EventPublishStage
//	Handler: debounce, session, typing, prompt build, StreamCallback, Remembrance
//
// The StreamCallback bridges LLMStage streaming to Discord:
// LLMStage calls callback(token) → callback sends to Discord via placeholder + edits.
func (cc *conversationContext) handleDiscordMessage(msg *discordbot.InboundMessage) {
	// Step 0: Skip empty or whitespace-only messages
	trimmed := strings.TrimSpace(msg.Content)
	if trimmed == "" {
		return
	}

	// Step 0a: Handle plan approval commands ("approve P-XXX" / "reject P-XXX")
	if cc.planMgr != nil {
		if strings.HasPrefix(trimmed, "approve ") || strings.HasPrefix(trimmed, "reject ") {
			if handled := cc.handlePlanApproval(msg); handled {
				return
			}
		}
	}

	if cc.crossCoord != nil && cc.handleCrossPrismCommand(msg) {
		return
	}

	if msg.IsBot {
		log.Printf("[AGENT] ignoring Discord bot message from %s (%s); cross-Prism agents communicate over NATS", msg.UserName, msg.UserID)
		return
	}

	// Step 1: Debounce — drop rapid-fire messages from the same user
	debounceKey := msg.UserID + ":" + msg.ChannelID
	if !cc.debounce.Allow(debounceKey) {
		return
	}

	// Step 1b: Rate limiting — prevent abuse from rapid-fire users
	if cc.rateLimiter != nil {
		if !cc.rateLimiter.Allow(msg.UserID) {
			log.Printf("[RATE] user %s rate limited in channel %s", msg.UserID, msg.ChannelID)
			cc.bot.Send(&discordbot.OutboundMessage{
				ChannelID: msg.ChannelID,
				Content:   "⚠️ Slow down! You're sending messages too fast. Please wait a moment.",
			})
			return
		}
	}

	// Step 1c: Prompt injection defense — scan and sanitize user input
	injectionCheck := safety.CheckPromptInjection(msg.Content)
	if injectionCheck.Severity == "critical" {
		log.Printf("[SECURITY] blocked critical injection attempt from user %s: flags=%v", msg.UserID, injectionCheck.Flags)
		cc.bot.Send(&discordbot.OutboundMessage{
			ChannelID: msg.ChannelID,
			Content:   "⚠️ That message contains potentially dangerous content and was blocked for safety.",
		})
		return
	}
	sanitizedContent := msg.Content
	if injectionCheck.Severity == "high" {
		sanitizedContent = safety.SanitizeInput(msg.Content)
		log.Printf("[SECURITY] sanitized high-severity input from user %s: flags=%v", msg.UserID, injectionCheck.Flags)
	}
	if injectionCheck.Severity == "medium" {
		log.Printf("[SECURITY] medium-severity flags in input from user %s: flags=%v", msg.UserID, injectionCheck.Flags)
	}

	// Emit channel received event
	cc.publishEvent("prism.channel.received", map[string]any{
		"user_id":    msg.UserID,
		"channel_id": msg.ChannelID,
	})

	// Step 2: Route the sanitized message to the appropriate agent
	result := cc.router.Route(sanitizedContent)

	// Step 2b: Response gate — skip low-signal messages that don't warrant a full LLM call
	// Manager-room and build-room always respond. Fun channel always responds.
	gateRole := cc.cfg.ResolveChannelRole(msg.ChannelID)
	gateDecision := stage.ShouldRespond(sanitizedContent, gateRole)
	if gateDecision == stage.Skip {
		log.Printf("[GATE] skipping low-signal message from %s in %s", msg.UserName, gateRole)
		return
	}
	if gateDecision == stage.RespondLightly {
		log.Printf("[GATE] light acknowledgment for message from %s in %s", msg.UserName, gateRole)
		cc.bot.Send(&discordbot.OutboundMessage{
			ChannelID: msg.ChannelID,
			Content:   "👍",
		})
		return
	}

	// Step 3: Find or create a session (handler-level)
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

	// Add the sanitized user message to the session
	_, err = cc.sessMgr.AddMessage(sess.ID, "user", sanitizedContent, "")
	if err != nil {
		log.Printf("[ERROR] add user message: %v", err)
		return
	}

	// Step 4: Look up the agent's provider and model (handler-level)
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
	run.Cancel = runCancel
	cc.cancelReg.Register(sess.ID, runCancel)
	defer func() {
		runCancel() // Release context timer immediately, not after 60s
		cc.cancelReg.Unregister(sess.ID)
	}()

	// Step 6: Send typing indicator (Discord-specific)
	if err := cc.bot.Typing(msg.ChannelID); err != nil {
		log.Printf("[WARN] typing indicator failed: %v", err)
	}

	// Step 7: Build the full prompt (session history + workspace context)
	// Resolve the channel role for state action injection
	stateActionKey := cc.cfg.ResolveChannelRole(msg.ChannelID)
	channelRole := cc.cfg.ResolveChannelRoleConfig(msg.ChannelID)
	prompt := cc.buildPrompt(sess, agentCfg, stateActionKey, channelRole)

	// Step 7b: Inject Remembrance context (if available, with 60s TTL cache)
	// NOTE: This uses the same remembrance.Client as RemembranceStage but applies
	// session-aware caching and prompt injection that the generic stage can't do.
	// RemembranceStage is the reusable pipeline component; this is the runtime integration.
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
			// Use context_markdown directly if available (Python ContextPack format)
			if remCtx.ContextMarkdown != "" {
				memoryBlock := "\n\n---\nRelevant context from memory:\n" + remCtx.ContextMarkdown + "\n---\n\n"
				prompt = memoryBlock + prompt
				log.Printf("[REMEMBRANCE] injected %d memory sources into prompt (markdown)", len(remCtx.SelectedMemories))
			} else if remCtx.ContextJSON != nil && len(remCtx.ContextJSON.Memories) > 0 {
				// Fallback: build from structured memories
				var memoryParts []string
				for _, mem := range remCtx.ContextJSON.Memories {
					if mem.Summary != "" {
						memoryParts = append(memoryParts, mem.Summary)
					}
				}
				if len(memoryParts) > 0 {
					memoryBlock := "\n\n---\nRelevant context from memory:\n" + strings.Join(memoryParts, "\n\n") + "\n---\n\n"
					prompt = memoryBlock + prompt
					log.Printf("[REMEMBRANCE] injected %d memory sources into prompt (structured)", len(remCtx.ContextJSON.Memories))
				}
			}
		}
	}

	// P-008: Evaluate tool relevance gate BEFORE building the prompt
	// This determines whether to include tools in the LLM request
	toolNames := cc.toolExec.Registry.List()

	// V33: Channel tool filtering — some channels restrict tools
	var gateResult *stage.GateResult
	channelRoleConfig := cc.cfg.ResolveChannelRoleConfig(msg.ChannelID)
	if channelRoleConfig != nil && channelRoleConfig.Tools == "none" {
		// Fun channel: no tools at all. The agent is purely conversational.
		log.Printf("[TOOL-CHANNEL] tools excluded by channel role %q", channelRoleConfig.Role)
		gateResult = &stage.GateResult{Decision: stage.ToolDecisionExclude, Reason: "channel role excludes all tools"}
	} else {
		evalResult := cc.toolGate.Evaluate(msg.Content, toolNames)
		gateResult = &evalResult
		log.Printf("[TOOL-GATE] decision=%d reason=%q tools=%v", gateResult.Decision, gateResult.Reason, gateResult.ToolFilter)
	}

	// Step 7c: Append tool instructions to prompt so the LLM knows it has tools
	// Skip tool instructions if the gate excluded tools for this message
	if cc.toolExec != nil && gateResult.Decision != stage.ToolDecisionExclude {
		toolInfos := cc.toolExec.Registry.ListWithDescriptions()
		// V33: Filter tools based on channel role (read-only channels get limited tools)
		toolInfos = filterToolInfosByChannelRole(toolInfos, channelRoleConfig)
		if len(toolInfos) > 0 {
			prompt += agent.BuildToolPromptSuffix(toolInfos, cc.ctxBuilder.WorkspaceRoot, cc.toolPolicy.AllowedPaths...)
		}
	}

	// Step 8: Set up streaming — create placeholder and StreamCallback
	// Send a typing indicator instead of a placeholder message.
	// Placeholders ("✧ ...") were visible to other bots and caused false responses.
	// Now we show only the Discord "typing..." indicator and send the complete
	// response as a new message when the LLM is done.
	var placeholderMsgID string // Always empty — SendPlaceholder now returns ""
	var streamMu sync.Mutex
	var accumulatedText string
	var lastTypingTime time.Time

	cc.bot.Typing(msg.ChannelID) // Initial typing indicator

	_, placeholderErr := cc.bot.SendPlaceholder(msg.ChannelID, "")
	if placeholderErr != nil {
		log.Printf("[STREAM] placeholder/typing failed: %v", placeholderErr)
	}

	streamCallback := func(token string, index int, finished bool) error {
		streamMu.Lock()
		defer streamMu.Unlock()

		accumulatedText += token
		now := time.Now()

		// Refresh typing indicator every 8 seconds (Discord shows it for ~10s)
		if now.Sub(lastTypingTime) >= 8*time.Second {
			lastTypingTime = now
			go func() {
				if err := cc.bot.Typing(msg.ChannelID); err != nil {
					log.Printf("[STREAM] typing refresh failed: %v", err)
				}
			}()
		}

		// No placeholder to edit — streaming tokens accumulate in memory
		// and are sent as a complete message after the LLM finishes
		return nil
	}

	// Step 9: Construct and run the stage pipeline
	natsAdapter := &natsPublisherAdapter{conn: cc.natsConn}

	pipeline := stage.NewPipeline(
		&stage.LLMStage{},
		&stage.DelegationStage{Engine: cc.delegEngine, StripMarkers: true, AgentConfigs: cc.buildAgentConfigMap()},
		&stage.PersistenceStage{BusURL: cc.natsURL},
		&stage.EventPublishStage{Publisher: natsAdapter, BusURL: cc.natsURL},
	)

	rc := &stage.RunContext{
		RunID:          run.ID,
		Task:           prompt, // Full formatted prompt (not raw user input)
		Agent:          result.AgentID,
		Provider:       llmProvider,
		ProviderName:   agentCfg.Provider,
		Model:          agentCfg.Model,
		SessionID:      sess.ID,
		CleanedContent: result.CleanedContent,
		RouteMethod:    result.Method,
		StreamCallback: streamCallback,
	}

	finalRC, err := pipeline.Run(runCtx, rc)
	if err != nil {
		log.Printf("[ERROR] pipeline failed (run %s): %v", run.ID, err)
		cc.sendError(msg.ChannelID, "I'm having trouble thinking right now. Please try again in a moment.")
		return
	}

	// Step 9b: Tool execution loop — branch on ChatProvider vs text-based
	if cc.toolExec != nil && gateResult.Decision != stage.ToolDecisionExclude {
		// Check if the provider supports native tool calling (ChatProvider)
		chatProv, chatErr := cc.providers.GetChatProvider(agentCfg.Model)
		supportsChat := chatErr == nil

		if supportsChat {
			_ = chatProv // used via cc.runToolLoopChat which calls cc.providers.GetChatProvider internally
			// Native tool calling path: use ChatProvider with structured messages
			// Build messages array and tools list
			messages := cc.buildMessages(sess, agentCfg)
			chatTools := cc.buildChatTools()

			// V33: Filter tools based on channel role (none, read-only, all)
			chatTools = filterChatToolsByChannelRole(chatTools, channelRoleConfig)

			// P-008: Filter tools based on gate result
			if gateResult.Decision == stage.ToolDecisionSubset && len(gateResult.ToolFilter) > 0 {
				chatTools = filterChatTools(chatTools, gateResult.ToolFilter)
				log.Printf("[TOOL-GATE] subset: %d tools after filtering", len(chatTools))
			}

			log.Printf("[TOOL-CHAT] entering native tool loop with %d tools", len(chatTools))

			finalResponse, toolSummaries, toolErr := cc.runToolLoopChat(
				runCtx,
				messages,
				chatTools,
				agentCfg,
				msg.ChannelID,
				placeholderMsgID,
			)
			if toolErr != nil {
				log.Printf("[TOOL-CHAT] tool loop failed: %v", toolErr)
				finalRC.LLMResponse = "I had trouble processing that — the AI service returned an error. Please try again in a moment."
			} else if finalResponse != "" {
				finalRC.LLMResponse = finalResponse
			}

			// Log tool call summaries
			for _, ts := range toolSummaries {
				log.Printf("[TOOL-CHAT] %s: %s (%s)", ts.Tool, ts.Status, truncateStr(ts.Result, 100))
			}

			// Save tool interactions to session
			for _, ts := range toolSummaries {
				toolMsg := fmt.Sprintf("[Tool: %s] %s", ts.Tool, ts.Status)
				if ts.Error != "" {
					toolMsg += " — " + ts.Error
				}
				cc.sessMgr.AddMessage(sess.ID, "tool", toolMsg, ts.Tool)
			}
		} else {
			// Text-based tool calling path (fallback for providers without ChatProvider)
			// P-008: Skip tool loop if gate excluded tools for this message
			parsed := agent.ParseAgentOutput(finalRC.LLMResponse)
			if parsed.Type == agent.ResponseToolRequest && gateResult.Decision != stage.ToolDecisionExclude {
				log.Printf("[TOOL] LLM requested tool %q, entering tool loop", parsed.ToolName)

				finalResponse, toolSummaries, toolErr := cc.runToolLoop(
					runCtx,
					prompt,
					agentCfg,
					msg.ChannelID,
					placeholderMsgID,
				)
				if toolErr != nil {
					log.Printf("[TOOL] tool loop failed: %v", toolErr)
					finalRC.LLMResponse = "I had trouble processing that — the AI service returned an error. Please try again in a moment."
				}

				for _, ts := range toolSummaries {
					log.Printf("[TOOL] %s: %s (%s)", ts.Tool, ts.Status, truncateStr(ts.Result, 100))
				}

				if finalResponse != "" {
					finalRC.LLMResponse = finalResponse
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
	}
	// Check pipeline results for stage failures
	for stageName, stageResult := range finalRC.Results {
		if stageResult != nil && !stageResult.Success {
			log.Printf("[ERROR] stage %s failed (run %s): %s", stageName, run.ID, stageResult.Error)
			switch stageName {
			case "routing":
				cc.sendError(msg.ChannelID, "I'm not configured properly. Please contact the administrator.")
			case "llm":
				cc.sendError(msg.ChannelID, "I'm having trouble thinking right now. Please try again in a moment.")
			default:
				cc.sendError(msg.ChannelID, "Something went wrong processing your message. Please try again.")
			}
			return
		}
	}

	// Step 10: Post-pipeline — handle response delivery and session save
	responseText := finalRC.LLMResponse

	// Use accumulated text from streaming if the final response is empty
	if responseText == "" {
		streamMu.Lock()
		if accumulatedText != "" {
			responseText = accumulatedText
		}
		streamMu.Unlock()
	}

	// Deliver the response to Discord as a new message
	// (typing-only approach: no placeholder message, just send the complete response)
	if responseText != "" {
		err := cc.bot.Send(&discordbot.OutboundMessage{
			ChannelID: msg.ChannelID,
			Content:   responseText,
		})
		if err != nil {
			log.Printf("[ERROR] failed to send Discord response: %v", err)
		}
	}

	// Save agent response to session
	if responseText != "" {
		_, err := cc.sessMgr.AddMessage(sess.ID, "agent", responseText, result.AgentID)
		if err != nil {
			log.Printf("[WARN] failed to save agent response to session %s: %v", sess.ID, err)
		}
	}

	// Log and publish completion events
	llmResult := finalRC.Results["llm"]
	streamed := false
	if llmResult != nil && llmResult.Data != nil {
		if s, ok := llmResult.Data["streamed"].(bool); ok {
			streamed = s
		}
	}

	cc.eventLog.Log("run.completed", run.ID, finalRC.Agent, map[string]any{
		"latency_ms": run.Elapsed().Milliseconds(),
	})
	cc.publishEvent(finalRC.Agent+".run.completed", map[string]any{
		"run_id":        run.ID,
		"latency_ms":    run.Elapsed().Milliseconds(),
		"output_length": len(responseText),
		"streamed":      streamed,
	})
	cc.publishEvent(finalRC.Agent+".channel.sent", map[string]any{
		"run_id":     run.ID,
		"channel_id": msg.ChannelID,
		"user_id":    msg.UserID,
	})

	// Step 11: Auto-save to Remembrance (async fire-and-forget)
	if cc.remClient != nil && responseText != "" {
		captureAgentID := finalRC.Agent
		captureRunID := run.ID
		captureText := responseText
		captureSource := fmt.Sprintf("prism:%s", captureAgentID)

		// Acquire semaphore slot (non-blocking — skip if at capacity)
		select {
		case cc.remSem <- struct{}{}:
			go func() {
				defer func() { <-cc.remSem }() // Release semaphore slot

				result, err := cc.remClient.Capture(
					captureText,
					captureSource,
					"conversation",
					"",
				)
				if err != nil {
					log.Printf("[REMEMBRANCE] capture failed (run %s): %v", captureRunID, err)
					return
				}
				if decision, ok := result["decision"]; ok {
					log.Printf("[REMEMBRANCE] run %s: decision=%v, captured", captureRunID, decision)
					// Track PERSIST captures for dream cycle event trigger
					if decision == "PERSIST" || decision == "persist" {
						atomic.AddInt64(&dreamPersistCount, 1)
					}
				} else {
					log.Printf("[REMEMBRANCE] captured output from run %s", captureRunID)
				}

				// Invalidate context cache for this session so next turn gets fresh context
				if cc.remCache != nil {
					cacheKey := fmt.Sprintf("%s:%s", captureAgentID, sess.ID)
					cc.remCache.Invalidate(cacheKey)
				}
			}()
		default:
			// At capacity — skip this capture
			log.Printf("[REMEMBRANCE] skipped capture (run %s): concurrency limit reached", run.ID)
		}
	}

	log.Printf("[RUN] %s completed in %s", run, run.Elapsed().Round(time.Millisecond))
} // sendError sends a user-friendly error message to a Discord channel.
func (cc *conversationContext) sendError(channelID, message string) {
	err := cc.bot.Send(&discordbot.OutboundMessage{
		ChannelID: channelID,
		Content:   "⚠️ " + message,
	})
	if err != nil {
		log.Printf("[ERROR] failed to send error message to Discord: %v", err)
	}
}

// V21: Events use per-agent namespace prefixes (<agent-id>.*).
// System events use the prism.* namespace.
// buildAgentConfigMap creates a map of agent ID → AgentConfig for the
// DelegationStage capability checks.
func (cc *conversationContext) buildAgentConfigMap() map[string]*orchestrator.AgentConfig {
	configs := make(map[string]*orchestrator.AgentConfig, len(cc.cfg.Agents)+1)
	for i := range cc.cfg.Agents {
		a := &cc.cfg.Agents[i]
		configs[a.ID] = a
	}
	if cc.cfg.Codex.Enabled {
		configs["codex"] = &orchestrator.AgentConfig{
			ID:           "codex",
			Role:         "coder",
			Provider:     "codex_cli",
			Model:        cc.cfg.Codex.Model,
			Capabilities: []string{"code", "test", "review", "report"},
		}
	}
	return configs
}

// If NATS is not connected, the event is logged but not published.
// All events include a schema version field for forward compatibility.
func (cc *conversationContext) publishEvent(subject string, payload map[string]any) {
	// Add schema version to all events (don't mutate caller's map)
	eventPayload := make(map[string]any, len(payload)+1)
	for k, v := range payload {
		eventPayload[k] = v
	}
	eventPayload["v"] = 1
	if cc.natsConn == nil || !cc.natsConn.IsConnected() {
		log.Printf("[EVENT] %s (NATS not connected, skipped)", subject)
		return
	}

	data, err := json.Marshal(eventPayload)
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

// rebuildStaticSystemContent pre-builds the static portion of the system prompt
// for the given agent. V33: Assembles in explicit layers with priority labels.
// Layers: Identity → Context Files → Postfix → Tools
// Called once at startup and cached for reuse.
func (cc *conversationContext) rebuildStaticSystemContent(agentCfg *orchestrator.AgentConfig) {
	var sb strings.Builder

	// --- Layer 1: IDENTITY ---
	// V33: Derive identity from workspace files (SOUL.md, IDENTITY.md) instead of
	// generic "You are {id}, a {role} assistant". If workspace files have identity
	// content, use it. Fall back to config id/role only if no identity files exist.
	identityContent := ""
	contextIdentity := ""
	if cc.ctxBuilder != nil {
		builder := context.NewBuilder(cc.ctxBuilder.WorkspaceRoot).WithNamedContexts([]string{"soul", "identity"})
		injected, err := builder.Build()
		if err == nil {
			for _, f := range injected.Files {
				if f.Name == "soul" && f.Content != "" {
					contextIdentity = f.Content
				}
			}
		}
	}

	if contextIdentity != "" {
		// Use workspace identity content — it's the real source of truth
		identityContent = contextIdentity
	} else {
		// Fall back to config id/role — better than nothing
		identityContent = fmt.Sprintf("You are %s, a %s assistant.", agentCfg.ID, agentCfg.Role)
	}

	sb.WriteString("## Who You Are\n")
	sb.WriteString(identityContent + "\n\n")

	// --- Layer 2: WORKSPACE CONTEXT ---
	// Full context files (AGENTS.md, USER.md, MEMORY.md, etc.)
	// Excluding soul and identity which are already in Layer 1
	if len(agentCfg.Context) > 0 && cc.ctxBuilder != nil {
		budget := cc.cfg.Prism.ContextTokenBudget
		if budget <= 0 {
			budget = 4000
		}

		// Build context without soul/identity (already in Layer 1)
		otherContexts := make([]string, 0, len(agentCfg.Context))
		for _, c := range agentCfg.Context {
			if c != "soul" && c != "identity" {
				otherContexts = append(otherContexts, c)
			}
		}

		if len(otherContexts) > 0 {
			builder := context.NewBuilder(cc.ctxBuilder.WorkspaceRoot).
				WithNamedContexts(otherContexts).
				WithTokenBudget(budget)
			injected, err := builder.BuildCached()
			if err == nil && injected.FormattedString != "" {
				sb.WriteString("## Context\n")
				sb.WriteString(injected.FormattedString + "\n")
			}
		}
	}

	// --- Layer 3: BEHAVIOR ---
	// Conversation postfix (can be overridden by channel context at runtime)
	postfix := agentCfg.ConversationPostfix
	if postfix == "" {
		postfix = "Stay present in the conversation. Ask follow-up questions when appropriate. " +
			"Don't wrap things up in a bow unless the topic is genuinely resolved. " +
			"Be warm, curious, and engaged — not a transactional Q&A machine."
	}
	sb.WriteString("## How You Respond\n")
	sb.WriteString(postfix + "\n\n")

	// --- Layer 4: TOOLS (text-based path) ---
	sb.WriteString("## Tool Usage\n" + toolUsageGuidance + "\n\n")

	cc.staticSystemText = sb.String()

	// --- ChatProvider path: same layered format ---
	var sbChat strings.Builder
	sbChat.WriteString("## Who You Are\n")
	sbChat.WriteString(identityContent + "\n\n")

	if len(agentCfg.Context) > 0 && cc.ctxBuilder != nil {
		budget := cc.cfg.Prism.ContextTokenBudget
		if budget <= 0 {
			budget = 4000
		}

		otherContexts := make([]string, 0, len(agentCfg.Context))
		for _, c := range agentCfg.Context {
			if c != "soul" && c != "identity" {
				otherContexts = append(otherContexts, c)
			}
		}

		if len(otherContexts) > 0 {
			builder := context.NewBuilder(cc.ctxBuilder.WorkspaceRoot).
				WithNamedContexts(otherContexts).
				WithTokenBudget(budget)
			injected, err := builder.BuildCached()
			if err == nil && injected.FormattedString != "" {
				sbChat.WriteString("## Context\n")
				sbChat.WriteString(injected.FormattedString + "\n")
			}
		}
	}

	sbChat.WriteString("\n## How You Respond\n")
	sbChat.WriteString(postfix + "\n")
	sbChat.WriteString("\n## Tool Usage\n" + toolUsageGuidance + "\n")

	cc.staticSystemChat = sbChat.String()
}

// buildPrompt constructs the LLM prompt from session history and injected context.
// V33: Layered prompt assembly with explicit priority labels.
//  1. Who You Are (identity from SOUL.md/IDENTITY.md)
//  2. Context (workspace files, excluding identity)
//  3. How You Respond (conversation postfix)
//  4. Tools (tool usage guidance)
//  5. Working State (active task, decisions, blocked items)
//  6. Active Plan (plan-first pipeline)
//  7. Channel Context (where, who, what project, how to behave)
//  8. Session Awareness (message count, age)
//  9. Conversation History
func (cc *conversationContext) buildPrompt(sess *session.Session, agentCfg *orchestrator.AgentConfig, stateActionKey string, channelRole *orchestrator.ChannelRole) string {
	var sb strings.Builder

	// Static system content (cached, built once at startup)
	// Layers 1-4: Identity, Context, Behavior, Tools
	sb.WriteString(cc.staticSystemText)

	// --- Layer 5: Working state injection ---
	if cc.stateMgr != nil {
		if statePrompt := cc.stateMgr.FormatStateForPrompt(); statePrompt != "" {
			sb.WriteString("\n" + statePrompt + "\n")
			log.Printf("[STATE] injected working state into prompt")
		}
	}

	// --- Layer 6: Active plan injection ---
	if cc.planMgr != nil {
		if plans, err := cc.planMgr.LoadPlans(); err == nil {
			activePlan := plan.ActivePlan(plans)
			if activePlan != nil {
				sb.WriteString("\n" + plan.FormatPlanForPrompt(activePlan) + "\n")
				log.Printf("[PLAN] injected active plan %s into prompt", activePlan.ID)
			}
		}
	}

	// --- Layer 7: Channel context injection ---
	// V33: Structured channel context replaces raw state_actions.inject.
	// When a ChannelRole has a Context field, use that.
	// When it doesn't, fall back to state_actions.inject for backward compatibility.
	if channelRole != nil && channelRole.Context != "" {
		sb.WriteString("\n## Channel: #" + channelRole.Role + "\n")
		sb.WriteString(channelRole.Context + "\n\n")
		log.Printf("[CHANNEL] injected channel context for %q (tools=%s, personality=%s)",
			channelRole.Role, channelRole.Tools, channelRole.Personality)
	} else {
		// Backward compatibility: fall back to state_actions.inject
		if sa := cc.cfg.ResolveStateAction(agentCfg.ID, stateActionKey); sa != nil && sa.Inject != "" {
			sb.WriteString("\n## Context\n")
			sb.WriteString(sa.Inject)
			sb.WriteString("\n\n")
			log.Printf("[STATE] injected state action %q for agent %s", stateActionKey, agentCfg.ID)
		}
	}

	// --- Session awareness (V29) ---
	sessionAge := time.Since(sess.StartedAt).Round(time.Second)
	sessionMsgCount := len(sess.Messages)
	sb.WriteString(fmt.Sprintf("[Session: %d messages, started %v ago]\n", sessionMsgCount, sessionAge))

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
// V34 adds openai_responses as an additive OpenAI Responses API option.
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

	case "openai_responses":
		p, err := createOpenAIResponsesProvider(agentCfg.Model)
		if err != nil {
			return nil, info, fmt.Errorf("openai responses provider: %w", err)
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
		return nil, info, fmt.Errorf("unsupported provider: %s (supported: ollama, openai, openai_responses, anthropic, gemini)", agentCfg.Provider)
	}
}

func factoryConfigFromBridge(cfg orchestrator.FactoryBridgeConfig) factory.Config {
	return factory.Config{
		Enabled:            cfg.Enabled,
		Root:               cfg.Root,
		Project:            cfg.Project,
		ProjectPath:        cfg.ProjectPath,
		ApprovalMode:       cfg.ApprovalMode,
		RunCodex:           cfg.RunCodex,
		VisionReview:       cfg.VisionReview,
		PlaytestMode:       cfg.PlaytestMode,
		EnableUIGeneration: cfg.EnableUIGeneration,
		UIGenerationDryRun: cfg.UIGenerationDryRun,
	}
}

func crossProfilesFromBridge(profiles []orchestrator.BridgeTargetProfile) []crossprism.TargetProfile {
	out := make([]crossprism.TargetProfile, 0, len(profiles))
	for _, profile := range profiles {
		out = append(out, crossprism.TargetProfile{
			Name:         profile.Name,
			InstanceID:   profile.InstanceID,
			Adapter:      profile.Adapter,
			Capabilities: append([]string(nil), profile.Capabilities...),
		})
	}
	return out
}

func codexConfigFromOrchestrator(cfg orchestrator.CodexConfig, root *orchestrator.Config) codexworker.Config {
	workspace := cfg.Workspace
	if workspace == "" && root != nil {
		workspace = root.Prism.Workspace
	}
	if workspace == "" {
		workspace = "."
	}
	dataDir := filepath.Join(".", ".prism", "data", "codex")
	if root != nil && root.Prism.DataDir != "" {
		dataDir = filepath.Join(root.Prism.DataDir, "codex")
	}
	return codexworker.Config{
		Enabled:        cfg.Enabled,
		Executable:     cfg.Executable,
		Model:          cfg.Model,
		Profile:        cfg.Profile,
		Workspace:      workspace,
		Sandbox:        cfg.Sandbox,
		ApprovalPolicy: cfg.ApprovalPolicy,
		TimeoutMinutes: cfg.TimeoutMinutes,
		MaxConcurrency: cfg.MaxConcurrency,
		CaptureDiff:    cfg.CaptureDiff,
		ExtraArgs:      append([]string(nil), cfg.ExtraArgs...),
		DataDir:        dataDir,
	}
}

// createOllamaProvider creates an Ollama provider instance.
// Uses localhost:11434 by default. V21: configurable base URL.
// V30: Returns both Provider and ChatProvider — Ollama supports both interfaces.
func createOllamaProvider(model string) (provider.Provider, error) {
	p := ollama.New("") // empty = default localhost:11434
	// The Ollama Provider also implements ChatProvider via ollama.ChatProvider.
	// It is registered under the same model ID and the runtime uses interface
	// assertions to detect chat capability: prov.(provider.ChatProvider)
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

// createOpenAIResponsesProvider creates an OpenAI Responses API provider.
func createOpenAIResponsesProvider(model string) (provider.Provider, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY environment variable not set")
	}
	return openai.NewResponses(apiKey), nil
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

// filterToolInfosByChannelRole filters ToolInfo slices based on channel role configuration.
func filterToolInfosByChannelRole(toolInfos []tool.ToolInfo, channelRole *orchestrator.ChannelRole) []tool.ToolInfo {
	mode := resolveToolMode(channelRole)
	switch mode {
	case ToolModeNone:
		return nil
	case ToolModeReadOnly:
		var filtered []tool.ToolInfo
		for _, t := range toolInfos {
			if readOnlyTools[t.Name] {
				filtered = append(filtered, t)
			}
		}
		return filtered
	default:
		return toolInfos
	}
}

// remembranceTimeout returns the configured Remembrance timeout duration,
// falling back to the default if not set.
func remembranceTimeout(cfg *orchestrator.Config) time.Duration {
	if cfg.Remembrance.TimeoutSeconds > 0 {
		return time.Duration(cfg.Remembrance.TimeoutSeconds) * time.Second
	}
	return remembrance.DefaultTimeout
}

func bridgeSecret(cfg *orchestrator.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.Bridge.SecretEnv != "" {
		if secret := os.Getenv(cfg.Bridge.SecretEnv); secret != "" {
			return secret
		}
	}
	return cfg.Bridge.Secret
}

// filterChatTools filters a ChatTool slice to only include tools whose names
// are in the filter list. Used by P-008 ToolRelevanceGate for subset decisions.
func filterChatTools(tools []provider.ChatTool, filter []string) []provider.ChatTool {
	filterSet := make(map[string]bool, len(filter))
	for _, f := range filter {
		filterSet[f] = true
	}
	var filtered []provider.ChatTool
	for _, t := range tools {
		if filterSet[t.Function.Name] {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

// readOnlyTools is the set of tool names that are safe for read-only channels.
var readOnlyTools = map[string]bool{
	"read_project":     true,
	"search_files":     true,
	"project_overview": true,
	"git_status":       true,
	"git_log":          true,
	"git_diff":         true,
	"git_branch_list":  true,
	"plan_list":        true,
	"state_get":        true,
}

// ToolMode represents the tool access level for a channel.
type ToolMode int

const (
	ToolModeAll      ToolMode = iota // All tools available
	ToolModeReadOnly                 // Only read-only tools
	ToolModeNone                     // No tools at all
)

// resolveToolMode determines the tool access level from a channel role config.
func resolveToolMode(channelRole *orchestrator.ChannelRole) ToolMode {
	if channelRole == nil {
		return ToolModeAll
	}
	switch channelRole.Tools {
	case "none":
		return ToolModeNone
	case "read-only":
		return ToolModeReadOnly
	default:
		return ToolModeAll
	}
}

// filterChatToolsByChannelRole filters tools based on channel role configuration.
// "none" → empty list (no tools), "read-only" → only read tools, "" or "all" → all tools.
func filterChatToolsByChannelRole(tools []provider.ChatTool, channelRole *orchestrator.ChannelRole) []provider.ChatTool {
	mode := resolveToolMode(channelRole)
	switch mode {
	case ToolModeNone:
		return nil
	case ToolModeReadOnly:
		var filtered []provider.ChatTool
		for _, t := range tools {
			if readOnlyTools[t.Function.Name] {
				filtered = append(filtered, t)
			}
		}
		return filtered
	default:
		return tools
	}
}
