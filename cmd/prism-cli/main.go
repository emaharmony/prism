// Package main implements the prism CLI with the `prism run` command for V1.
package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/nats-io/nats.go"

	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/run"
)

func main() {
	// ── Subcommand: run ────────────────────────────────────────────
	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	taskFlag := runCmd.String("task", "", "Task description (required)")
	projectFlag := runCmd.String("project", "prism", "Project name")
	agentFlag := runCmd.String("agent", "lumi", "Agent name")
	busURL := runCmd.String("bus-url", "nats://localhost:4222", "NATS bus URL")
	memoryEnabled := runCmd.Bool("memory-enabled", false, "Enable Remembrance context hook")
	requireMemory := runCmd.Bool("require-memory", false, "Fail if Remembrance is unavailable")
	memoryURL := runCmd.String("memory-url", "http://localhost:18790", "Remembrance URL")
	runDir := runCmd.String("run-dir", "./runs", "Directory for run outputs")

	// ── Subcommand: health ──────────────────────────────────────────
	healthCmd := flag.NewFlagSet("health", flag.ExitOnError)
	healthBusURL := healthCmd.String("bus-url", "nats://localhost:4222", "NATS bus URL")

	// ── Parse ───────────────────────────────────────────────────────
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "run":
		runCmd.Parse(os.Args[2:])
		executeRun(runConfig{
			Task:           *taskFlag,
			Project:        *projectFlag,
			Agent:          *agentFlag,
			BusURL:         *busURL,
			MemoryEnabled:  *memoryEnabled,
			RequireMemory:  *requireMemory,
			MemoryURL:      *memoryURL,
			RunDir:         *runDir,
		})
	case "health":
		healthCmd.Parse(os.Args[2:])
		executeHealth(*healthBusURL)
	case "version":
		fmt.Println("prism v0.1.0")
	default:
		printUsage()
		os.Exit(1)
	}
}

type runConfig struct {
	Task           string
	Project        string
	Agent          string
	BusURL         string
	MemoryEnabled  bool
	RequireMemory  bool
	MemoryURL      string
	RunDir         string
}

func executeRun(cfg runConfig) {
	log.SetFlags(log.Ltime | log.Lshortfile)

	if cfg.Task == "" {
		fmt.Fprintln(os.Stderr, "Error: --task is required")
		os.Exit(1)
	}

	runner := run.NewRunner(run.RunConfig{
		Task:           cfg.Task,
		Project:        cfg.Project,
		Agent:          cfg.Agent,
		BusURL:         cfg.BusURL,
		MemoryEnabled:  cfg.MemoryEnabled,
		RequireMemory:  cfg.RequireMemory,
		MemoryURL:      cfg.MemoryURL,
		RunDir:         cfg.RunDir,
	})

	result, err := runner.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Run failed: %v\n", err)
		if result != nil {
			fmt.Fprintf(os.Stderr, "   Run ID: %s\n", result.RunID)
			fmt.Fprintf(os.Stderr, "   Events: %s\n", result.EventsPath)
		}
		os.Exit(1)
	}

	// Print summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  ✅ Prism V1 Run Complete")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Run ID:          %s\n", result.RunID)
	fmt.Printf("  Status:          %s\n", result.Status)
	fmt.Printf("  Events emitted:  %d\n", result.EventCount)
	fmt.Printf("  Event log:       %s\n", result.EventsPath)
	fmt.Printf("  Summary:         %s\n", result.SummaryPath)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
}

func executeHealth(busURL string) {
	nc, err := nats.Connect(busURL, nats.Name("prism-health-check"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ NATS bus unreachable: %v\n", err)
		os.Exit(1)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ JetStream unavailable: %v\n", err)
		os.Exit(1)
	}

	info, err := js.StreamInfo("PRISM")
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ PRISM stream not found: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  ✅ Prism Bus Health Check")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  NATS URL:    %s\n", busURL)
	fmt.Printf("  Stream:      PRISM\n")
	fmt.Printf("  Messages:    %d\n", info.State.Msgs)
	fmt.Printf("  Bytes:       %d\n", info.State.Bytes)
	fmt.Printf("  Subjects:    %v\n", info.Config.Subjects)
	fmt.Println("═══════════════════════════════════════════")

	// Emit health event
	evt := event.NewEvent(event.V1EventTypes.SystemHealth, "prism-cli", map[string]any{
		"nats_url":    busURL,
		"stream_msgs": info.State.Msgs,
		"status":      "healthy",
	})
	data, _ := evt.ToJSON()
	js.Publish(event.V1EventTypes.SystemHealth, data)
	fmt.Printf("  Health event emitted: %s\n", evt.ID)
}

func printUsage() {
	fmt.Println("Prism — Event-Native AI Agent Platform")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  prism run --task <description> [options]    Run a V1 task lifecycle")
	fmt.Println("  prism health [options]                      Check bus health")
	fmt.Println("  prism version                               Print version")
	fmt.Println()
	fmt.Println("Run options:")
	fmt.Println("  --task <string>        Task description (required)")
	fmt.Println("  --project <string>     Project name (default: prism)")
	fmt.Println("  --agent <string>       Agent name (default: lumi)")
	fmt.Println("  --bus-url <string>     NATS bus URL (default: nats://localhost:4222)")
	fmt.Println("  --memory-enabled       Enable Remembrance context hook")
	fmt.Println("  --require-memory       Fail if Remembrance is unavailable")
	fmt.Println("  --memory-url <string>  Remembrance URL (default: http://localhost:18790)")
	fmt.Println("  --run-dir <string>     Run output directory (default: ./runs)")
	fmt.Println()
	fmt.Println("Health options:")
	fmt.Println("  --bus-url <string>     NATS bus URL (default: nats://localhost:4222)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  prism run --task \"Test V1 event lifecycle\"")
	fmt.Println("  prism run --task \"Analyze code\" --memory-enabled --require-memory")
	fmt.Println("  prism run --task \"Deploy service\" --project myapp --agent coder")
	fmt.Println("  prism health")
}