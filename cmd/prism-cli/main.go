// Package main implements the prism CLI with run, health, and tool commands for V3.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/run"
	"github.com/emaharmony/prism/internal/tool"
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

	// V2 LLM flags
	providerFlag := runCmd.String("provider", "mock", "LLM provider: mock or ollama")
	modelFlag := runCmd.String("model", "mock-model", "Model name")
	temperatureFlag := runCmd.Float64("temperature", 0.2, "LLM temperature")
	maxTokensFlag := runCmd.Int("max-tokens", 2048, "Max output tokens")
	timeoutFlag := runCmd.Duration("timeout", 60*time.Second, "LLM request timeout")
	dryRunPrompt := runCmd.Bool("dry-run-prompt", false, "Build prompt and artifacts but skip LLM call")
	ollamaURL := runCmd.String("ollama-url", "http://localhost:11434", "Ollama base URL")

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

		// Resolve provider
		var p provider.Provider
		var providerName string
		model := *modelFlag

		switch *providerFlag {
		case "mock":
			p = provider.NewMockProvider()
			providerName = "mock"
		case "ollama":
			p = provider.NewOllamaProvider(*ollamaURL)
			providerName = "ollama"
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown provider '%s' (expected mock or ollama)\n", *providerFlag)
			os.Exit(1)
		}

		executeRun(runConfig{
			Task:           *taskFlag,
			Project:        *projectFlag,
			Agent:          *agentFlag,
			BusURL:         *busURL,
			MemoryEnabled:  *memoryEnabled,
			RequireMemory:  *requireMemory,
			MemoryURL:      *memoryURL,
			RunDir:         *runDir,
			Provider:       p,
			ProviderName:   providerName,
			Model:          model,
			Temperature:    *temperatureFlag,
			MaxTokens:      *maxTokensFlag,
			Timeout:        *timeoutFlag,
			DryRunPrompt:   *dryRunPrompt,
		})
	case "tool":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: tool subcommand required (list or run)")
			fmt.Fprintln(os.Stderr, "Usage: prism-cli tool list")
			fmt.Fprintln(os.Stderr, "       prism-cli tool run <tool_name> --input '{...}' --project prism")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			executeToolList()
		case "run":
			toolRunCmd := flag.NewFlagSet("tool run", flag.ExitOnError)
			toolInput := toolRunCmd.String("input", "{}", "JSON input for the tool")
			toolProject := toolRunCmd.String("project", "prism", "Project name for the tool call")
			toolWorkspace := toolRunCmd.String("workspace", ".", "Workspace root directory")
			toolMaxSize := toolRunCmd.Int64("max-file-size", 1048576, "Max file size in bytes (default 1MB)")
			toolRunCmd.Parse(os.Args[4:])

			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: tool name required")
				fmt.Fprintln(os.Stderr, "Usage: prism-cli tool run <tool_name> --input '{...}' --project prism")
				os.Exit(1)
			}
			toolName := os.Args[3]
			executeToolRun(toolName, *toolInput, *toolProject, *toolWorkspace, *toolMaxSize)
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown tool subcommand '%s'\n", os.Args[2])
			os.Exit(1)
		}
	case "health":
		healthCmd.Parse(os.Args[2:])
		executeHealth(*healthBusURL)
	case "version":
		fmt.Println("prism v0.3.0")
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

	// V2 LLM fields
	Provider     provider.Provider
	ProviderName string
	Model        string
	Temperature  float64
	MaxTokens    int
	Timeout      time.Duration
	DryRunPrompt bool
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
		Provider:       cfg.Provider,
		ProviderName:   cfg.ProviderName,
		Model:          cfg.Model,
		Temperature:    cfg.Temperature,
		MaxTokens:      cfg.MaxTokens,
		Timeout:        cfg.Timeout,
		DryRunPrompt:   cfg.DryRunPrompt,
	})

	result, err := runner.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════")
		fmt.Fprintln(os.Stderr, "  ❌ Prism V2 Run Failed")
		fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════")
		fmt.Fprintf(os.Stderr, "  Run ID:          %s\n", result.RunID)
		fmt.Fprintf(os.Stderr, "  Error:           %s\n", result.Error)
		if result.Provider != "" {
			fmt.Fprintf(os.Stderr, "  Provider:        %s\n", result.Provider)
		}
		if result.Model != "" {
			fmt.Fprintf(os.Stderr, "  Model:           %s\n", result.Model)
		}
		fmt.Fprintf(os.Stderr, "  Events:          %s\n", result.EventsPath)
		fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════")
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

	// Print success summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")

	if result.DryRun {
		fmt.Println("  ✅ Prism V2 Run Complete (dry-run)")
	} else {
		fmt.Println("  ✅ Prism V2 Run Complete")
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Run ID:          %s\n", result.RunID)
	fmt.Printf("  Status:          %s\n", result.Status)
	if result.Provider != "" {
		fmt.Printf("  Provider:        %s\n", result.Provider)
	}
	if result.Model != "" {
		fmt.Printf("  Model:           %s\n", result.Model)
	}
	fmt.Printf("  Events emitted:  %d\n", result.EventCount)
	fmt.Printf("  Event log:       %s\n", result.EventsPath)
	if result.PromptPath != "" {
		fmt.Printf("  Prompt:          %s\n", result.PromptPath)
	}
	if result.OutputPath != "" {
		fmt.Printf("  Output:          %s\n", result.OutputPath)
	}
	fmt.Printf("  Summary:         %s\n", result.SummaryPath)

	if result.DryRun {
		fmt.Println("  (No LLM call — dry-run mode)")
	}

	if result.ToolCallResult != nil {
		fmt.Println("  ── Tool Call ──")
		fmt.Printf("  Success:         %v\n", result.ToolCallResult.Success)
		if result.ToolCallResult.Error != "" {
			fmt.Printf("  Error:           %s\n", result.ToolCallResult.Error)
		}
	}

	fmt.Printf("  Duration:        %dms\n", result.DurationMs)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
}

func executeToolList() {
	registry := tool.NewRegistry()
	tool.RegisterBuiltins(registry, ".", 1024*1024)

	names := registry.List()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V3 Built-in Tools")
	fmt.Println("═══════════════════════════════════════════")
	for _, name := range names {
		t, err := registry.Resolve(name)
		if err != nil {
			fmt.Printf("  %-20s (error: %v)\n", name, err)
			continue
		}
		fmt.Printf("  %-20s %s\n", name, t.Description())
		schema := t.Schema()
		if len(schema.Input) > 0 {
			fmt.Println("    Input:")
			for paramName, spec := range schema.Input {
				req := ""
				if spec.Required {
					req = " (required)"
				}
				fmt.Printf("      %s: %s%s — %s\n", paramName, spec.Type, req, spec.Description)
			}
		}
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeToolRun(toolName, inputJSON, project, workspace string, maxFileSize int64) {
	// Parse input JSON
	var input map[string]any
	if err := json.Unmarshal([]byte(inputJSON), &input); err != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid JSON input: %v\n", err)
		os.Exit(1)
	}

	// Set up registry and policy
	registry := tool.NewRegistry()
	tool.RegisterBuiltins(registry, workspace, maxFileSize)
	policyConfig := tool.PolicyConfig{
		WorkspaceRoot: workspace,
		MaxFileSize:   maxFileSize,
	}
	executor := tool.NewExecutor(registry, policyConfig)

	fmt.Printf("Running tool %q with input: %s\n", toolName, inputJSON)

	result, err := executor.ExecuteWithPolicy(context.Background(), toolName, "prism-cli", project, "tool-cli-run", input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error executing tool: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	if result.Success {
		fmt.Println("  ✅ Tool Execution Succeeded")
	} else {
		fmt.Println("  ❌ Tool Execution Failed")
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Tool:    %s\n", toolName)
	fmt.Printf("  Project: %s\n", project)
	if result.Error != "" {
		fmt.Printf("  Error:   %s\n", result.Error)
	}
	if len(result.Output) > 0 {
		fmt.Println("  Output:")
		outputData, _ := json.MarshalIndent(result.Output, "    ", "  ")
		fmt.Printf("    %s\n", string(outputData))
	}
	fmt.Println("═══════════════════════════════════════════")
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
	fmt.Println("  prism run --task <description> [options]    Run a V2 task lifecycle")
	fmt.Println("  prism tool list                               List available tools")
	fmt.Println("  prism tool run <name> --input '{...}'         Run a tool directly")
	fmt.Println("  prism health [options]                        Check bus health")
	fmt.Println("  prism version                                 Print version")
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
	fmt.Println("V2 LLM provider options:")
	fmt.Println("  --provider <string>    LLM provider: mock or ollama (default: mock)")
	fmt.Println("  --model <string>       Model name (default: mock-model)")
	fmt.Println("  --temperature <float>  LLM temperature (default: 0.2)")
	fmt.Println("  --max-tokens <int>     Max output tokens (default: 2048)")
	fmt.Println("  --timeout <duration>   LLM request timeout (default: 60s)")
	fmt.Println("  --dry-run-prompt       Build prompt and artifacts but skip LLM call")
	fmt.Println("  --ollama-url <string>  Ollama base URL (default: http://localhost:11434)")
	fmt.Println()
	fmt.Println("Tool options:")
	fmt.Println("  prism tool list                                   List all built-in tools")
	fmt.Println("  prism tool run <name> --input '{...}' [options]   Run a tool directly")
	fmt.Println("    --input <json>       JSON input for the tool (default: {})")
	fmt.Println("    --project <string>  Project name (default: prism)")
	fmt.Println("    --workspace <path>  Workspace root directory (default: .)")
	fmt.Println("    --max-file-size <int> Max file size in bytes (default: 1048576)")
	fmt.Println()
	fmt.Println("Health options:")
	fmt.Println("  --bus-url <string>     NATS bus URL (default: nats://localhost:4222)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  prism run --task \"Test V2 event lifecycle\"")
	fmt.Println("  prism run --task \"Analyze code\" --provider ollama --model qwen2.5:7b")
	fmt.Println("  prism run --task \"Test dry run\" --dry-run-prompt")
	fmt.Println("  prism run --task \"Deploy service\" --project myapp --agent coder")
	fmt.Println("  prism tool list")
	fmt.Println("  prism tool run echo --input '{\"text\": \"hello\"}'")
	fmt.Println("  prism tool run read_file --input '{\"path\": \"README.md\"}' --workspace .")
	fmt.Println("  prism health")
}