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
// Other commands remain one-shot (prism run, prism health, etc.).
// `prism serve` is the only persistent command.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emaharmony/prism/internal/action"
	"github.com/emaharmony/prism/internal/adapter/builtin/discordbot"
	"github.com/emaharmony/prism/internal/agent"
	"github.com/emaharmony/prism/internal/bus"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/router"
	"github.com/emaharmony/prism/internal/session"
)

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

	// 3. Register agents
	agentReg := agent.NewRegistry()
	if err := cfg.RegisterAgents(agentReg); err != nil {
		fmt.Fprintf(os.Stderr, "Error registering agents: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  Registered %d agents\n", len(cfg.Agents))

	// 4. Start session manager
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

	// 5. Set up agent router
	rtr := router.New(agentReg, cfg)
	fmt.Println("  Router: ready")

	// 6. Set up action registry
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

	// 7. Connect channel adapters (Discord, etc.)
	var discordBots []*discordbot.BotAdapter
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, ch := range cfg.Channels {
		switch ch.Type {
		case "discord":
			bot := discordbot.NewBotAdapter(ch.Token)
			bot.OnMessage(func(msg *discordbot.InboundMessage) {
				handleDiscordMessage(msg, rtr, sessMgr, cfg, actionReg)
			})

			go func() {
				if err := bot.Start(ctx); err != nil {
					log.Printf("Discord bot error: %v", err)
				}
			}()

			discordBots = append(discordBots, bot)
			fmt.Printf("  Discord: connecting to %d channels\n", len(ch.Channels))

		default:
			fmt.Fprintf(os.Stderr, "Warning: unknown channel type %q\n", ch.Type)
		}
	}

	// 8. Start health check server
	go startHealthServer(*portFlag, cfg, agentReg, sessMgr, discordBots)
	fmt.Printf("  Health: http://localhost:%d/health\n", *portFlag)

	fmt.Println()
	fmt.Println("🔮 Prism is running. Press Ctrl+C to stop.")

	// 9. Wait for shutdown signal
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
// routing pipeline: find/create session → route to agent → process actions.
func handleDiscordMessage(
	msg *discordbot.InboundMessage,
	rtr *router.Router,
	sessMgr *session.Manager,
	cfg *orchestrator.Config,
	actionReg *action.Registry,
) {
	// Route the message to the appropriate agent
	result := rtr.Route(msg.Content)

	log.Printf("Discord message from %s in %s → agent %s (method: %s)",
		msg.UserName, msg.ChannelID, result.AgentID, result.Method)

	// Find or create a session for this user/channel
	sess, err := sessMgr.FindActive("discord", msg.ChannelID, msg.UserID)
	if err != nil {
		log.Printf("Error finding session: %v", err)
		return
	}
	if sess == nil {
		// Create a new session with the routed agent
		sess, err = sessMgr.Create(result.AgentID, "discord", msg.ChannelID, msg.UserID)
		if err != nil {
			log.Printf("Error creating session: %v", err)
			return
		}
	}

	// Add the user message to the session
	_, err = sessMgr.AddMessage(sess.ID, "user", msg.Content, "")
	if err != nil {
		log.Printf("Error adding user message: %v", err)
		return
	}

	// TODO (V21): Send to LLM, get response, send back through Discord
	// For V20, we log the routed message. The LLM integration comes in V21
	// when we wire the full conversation pipeline.
	log.Printf("Session %s: user → %s (agent: %s)", sess.ID, result.CleanedContent, result.AgentID)

	_ = actionReg // Will be used when events are published to the bus
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