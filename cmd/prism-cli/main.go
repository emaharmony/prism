// Package main implements the prism CLI — the human-facing entry point to the Prism
// agent platform. Every interaction starts here.
//
// Subcommands:
//   prism run           — Execute a full V1→V5 run (task → LLM → tools → approval → validation → review)
//   prism health        — Check if the NATS event bus is reachable
//   prism tool list      — List available tools and their policies
//   prism tool run       — Execute a single tool directly (for testing)
//   prism approval list  — List pending/approved/denied approvals
//   prism approval show  — Show details of a specific approval
//   prism approval approve — Approve a pending mutation (writes file to disk)
//   prism approval deny  — Deny a pending mutation (file is NOT written)
//   prism validation list — List available validation profiles
//   prism validation run — Run a validation profile (e.g., go_test_all)
//   prism workflow list — List registered workflows
//   prism workflow run  — Run a named workflow
//   prism workflow status — Show workflow run state
//
// Why raw os.Args instead of a CLI framework (cobra, etc.)? Prism's CLI surface is
// small and stable — 10 subcommands with simple flag sets. A framework would add
// a dependency for no real benefit. If the CLI grows significantly, cobra would be
// worth adopting.
//
// Output formatting: Uses Unicode symbols (✅ ❌ 💎 ═══) for visual structure.
// These are cosmetic — the actual data is always in JSON artifacts under runs/.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/emaharmony/prism/internal/approval"
	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/mutation"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/review"
	"github.com/emaharmony/prism/internal/run"
	"github.com/emaharmony/prism/internal/tool"
	"github.com/emaharmony/prism/internal/validation"
	"github.com/emaharmony/prism/internal/workflow"
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

	// LLM flags
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
	case "approval":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: approval subcommand required (list, show, approve, deny)")
			fmt.Fprintln(os.Stderr, "Usage: prism approval list [--run <run_id>]")
			fmt.Fprintln(os.Stderr, "       prism approval show <approval_id> [--run <run_id>]")
			fmt.Fprintln(os.Stderr, "       prism approval approve <approval_id> --by <name> [--run <run_id>] [--workspace <path>]")
			fmt.Fprintln(os.Stderr, "       prism approval deny <approval_id> --by <name> [--run <run_id>] [--reason <reason>]")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			approvalListCmd := flag.NewFlagSet("approval list", flag.ExitOnError)
			approvalListRun := approvalListCmd.String("run", "", "Run ID (optional, lists approvals for a specific run)")
			approvalListRunsDir := approvalListCmd.String("run-dir", "./runs", "Directory for run outputs")
			approvalListCmd.Parse(os.Args[3:])
			executeApprovalList(*approvalListRun, *approvalListRunsDir)

		case "show":
			approvalShowCmd := flag.NewFlagSet("approval show", flag.ExitOnError)
			approvalShowRun := approvalShowCmd.String("run", "", "Run ID (required)")
			approvalShowRunsDir := approvalShowCmd.String("run-dir", "./runs", "Directory for run outputs")
			approvalShowCmd.Parse(os.Args[3:])
			// The approval ID comes after show, e.g., `prism approval show appr_xxx --run run_yyy`
			// But since go flags parses differently with positional args...
			// Use os.Args[3] if it doesn't look like a flag
			// For simplicity, we'll pass empty string and expect user to put approval ID
			// Let me rework: approval ID is the first positional arg after 'show'
			args := os.Args[3:]
			approvalID := ""
			for i, arg := range args {
				if arg == "show" && i+1 < len(args) {
					approvalID = args[i+1]
					break
				}
			}
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: approval ID required")
				os.Exit(1)
			}
			// The approval ID is os.Args[3] (after prism approval show)
			if len(os.Args) > 3 && os.Args[3] != "--run" && os.Args[3] != "--run-dir" {
				approvalID = os.Args[3]
			}
			executeApprovalShow(approvalID, *approvalShowRun, *approvalShowRunsDir)

		case "approve":
			approvalApproveCmd := flag.NewFlagSet("approval approve", flag.ExitOnError)
			approvalApproveBy := approvalApproveCmd.String("by", "", "Name of the approver (required)")
			approvalApproveRun := approvalApproveCmd.String("run", "", "Run ID (required)")
			approvalApproveWorkspace := approvalApproveCmd.String("workspace", ".", "Workspace root directory")
			approvalApproveRunsDir := approvalApproveCmd.String("run-dir", "./runs", "Directory for run outputs")
			approvalValidate := approvalApproveCmd.Bool("validate", false, "Run validation and review after approving the mutation")
			approvalApproveCmd.Parse(os.Args[4:])

			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: approval ID required")
				os.Exit(1)
			}
			approveID := os.Args[3]
			if *approvalValidate {
				executeApprovalApproveWithValidation(approveID, *approvalApproveBy, *approvalApproveRun, *approvalApproveWorkspace, *approvalApproveRunsDir)
			} else {
				executeApprovalApprove(approveID, *approvalApproveBy, *approvalApproveRun, *approvalApproveWorkspace, *approvalApproveRunsDir)
			}

		case "deny":
			approvalDenyCmd := flag.NewFlagSet("approval deny", flag.ExitOnError)
			approvalDenyBy := approvalDenyCmd.String("by", "", "Name of the denier (required)")
			approvalDenyRun := approvalDenyCmd.String("run", "", "Run ID (required)")
			approvalDenyReason := approvalDenyCmd.String("reason", "", "Reason for denial")
			approvalDenyRunsDir := approvalDenyCmd.String("run-dir", "./runs", "Directory for run outputs")
			approvalDenyCmd.Parse(os.Args[4:])

			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: approval ID required")
				os.Exit(1)
			}
			denyID := os.Args[3]
			executeApprovalDeny(denyID, *approvalDenyBy, *approvalDenyReason, *approvalDenyRun, *approvalDenyRunsDir)

		default:
			fmt.Fprintf(os.Stderr, "Error: unknown approval subcommand '%s'\n", os.Args[2])
			os.Exit(1)
		}
	case "health":
		healthCmd.Parse(os.Args[2:])
		executeHealth(*healthBusURL)
	case "validation":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: validation subcommand required (list or run)")
			fmt.Fprintln(os.Stderr, "Usage: prism validation list")
			fmt.Fprintln(os.Stderr, "       prism validation run <profile_name> --project <project>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			executeValidationList()
		case "run":
			validationRunCmd := flag.NewFlagSet("validation run", flag.ExitOnError)
			validationProject := validationRunCmd.String("project", "prism", "Project name")
			validationRunDir := validationRunCmd.String("run-dir", "./runs", "Directory for run outputs")
			validationRunID := validationRunCmd.String("run-id", "", "Run ID (optional, generates one if not provided)")
			validationRunCmd.Parse(os.Args[4:])
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: profile name required")
				os.Exit(1)
			}
			profileName := os.Args[3]
			executeValidationRun(profileName, *validationProject, *validationRunDir, *validationRunID)
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown validation subcommand '%s'\n", os.Args[2])
			os.Exit(1)
		}
	case "workflow":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: workflow subcommand required (list, show, run, or status)")
			fmt.Fprintln(os.Stderr, "Usage: prism workflow list")
			fmt.Fprintln(os.Stderr, "       prism workflow show <workflow_name>")
			fmt.Fprintln(os.Stderr, "       prism workflow run <workflow_name> --input <file.json>")
			fmt.Fprintln(os.Stderr, "       prism workflow status <run_id>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			executeWorkflowList()
		case "show":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: workflow name required")
				os.Exit(1)
			}
			executeWorkflowShow(os.Args[3])
		case "run":
			workflowRunCmd := flag.NewFlagSet("workflow run", flag.ExitOnError)
			workflowInput := workflowRunCmd.String("input", "", "JSON input file for workflow (optional)")
			workflowRunDir := workflowRunCmd.String("run-dir", "./runs", "Directory for run outputs")
			workflowRunCmd.Parse(os.Args[4:])
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: workflow name required")
				fmt.Fprintln(os.Stderr, "Usage: prism workflow run <workflow_name> --input <file.json>")
				os.Exit(1)
			}
			workflowName := os.Args[3]
			executeWorkflowRun(workflowName, *workflowInput, *workflowRunDir)
		case "status":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: run ID required")
				os.Exit(1)
			}
			wsCmd := flag.NewFlagSet("workflow status", flag.ExitOnError)
			wsRunDir := wsCmd.String("run-dir", "./runs", "Directory for run outputs")
			wsCmd.Parse(os.Args[4:])
			executeWorkflowStatus(os.Args[3], *wsRunDir)
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown workflow subcommand '%s'\n", os.Args[2])
			os.Exit(1)
		}
	case "version":
		fmt.Println("prism v0.7.0")
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

	// LLM fields
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
		fmt.Fprintln(os.Stderr, "  ❌ Prism Run Failed")
		fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════")
		if result != nil {
			fmt.Fprintf(os.Stderr, "  Run ID:          %s\n", result.RunID)
			fmt.Fprintf(os.Stderr, "  Error:           %s\n", result.Error)
			if result.Provider != "" {
				fmt.Fprintf(os.Stderr, "  Provider:        %s\n", result.Provider)
			}
			if result.Model != "" {
				fmt.Fprintf(os.Stderr, "  Model:           %s\n", result.Model)
			}
			fmt.Fprintf(os.Stderr, "  Events:          %s\n", result.EventsPath)
		} else {
			fmt.Fprintf(os.Stderr, "  Error:           %s\n", err)
		}
		fmt.Fprintln(os.Stderr, "═══════════════════════════════════════════")
		fmt.Fprintln(os.Stderr)
		os.Exit(1)
	}

	// Print success summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")

	if result.DryRun {
		fmt.Println("  ✅ Prism Run Complete (dry-run)")
	} else {
		fmt.Println("  ✅ Prism Run Complete")
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
	tool.RegisterBuiltinsV4(registry, ".", 1024*1024)

	names := registry.List()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V4 Built-in Tools")
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
	tool.RegisterBuiltinsV4(registry, workspace, maxFileSize)
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

// ── V4: Approval Subcommand Functions ──────────────────────────────────────

func executeApprovalList(runID, runsDir string) {
	store := approval.NewStore(runsDir)

	var approvals []*approval.Approval
	var err error

	if runID != "" {
		approvals, err = store.List(runID)
	} else {
		// List all approvals across all runs
		entries, readErr := os.ReadDir(runsDir)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to read runs directory: %v\n", readErr)
			os.Exit(1)
		}
		for _, entry := range entries {
			if entry.IsDir() {
				runApprovals, listErr := store.List(entry.Name())
				if listErr == nil {
					approvals = append(approvals, runApprovals...)
				}
			}
		}
		err = nil
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	if len(approvals) == 0 {
		fmt.Println("No approvals found.")
		return
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V4 Approvals")
	fmt.Println("═══════════════════════════════════════════")
	for _, a := range approvals {
		fmt.Printf("  %s\n", a.ApprovalID)
		fmt.Printf("    Status:       %s\n", a.Status)
		fmt.Printf("    Type:         %s\n", a.MutationType)
		fmt.Printf("    Target:       %s\n", a.TargetPath)
		fmt.Printf("    Requested by: %s\n", a.RequestedBy)
		fmt.Printf("    Run:          %s\n", a.RunID)
		if a.Status == approval.StatusApproved {
			fmt.Printf("    Approved by:  %s at %v\n", a.ApprovedBy, a.ApprovedAt)
		} else if a.Status == approval.StatusDenied {
			fmt.Printf("    Denied by:    %s\n", a.DeniedBy)
			if a.DenialReason != "" {
				fmt.Printf("    Reason:       %s\n", a.DenialReason)
			}
		}
		fmt.Println()
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeApprovalShow(approvalID, runID, runsDir string) {
	if runID == "" {
		fmt.Fprintln(os.Stderr, "Error: --run flag is required for show")
		os.Exit(1)
	}
	if approvalID == "" || approvalID == "show" {
		fmt.Fprintln(os.Stderr, "Error: approval ID required")
		os.Exit(1)
	}

	store := approval.NewStore(runsDir)

	a, err := store.Load(runID, approvalID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Approval: %s\n", a.ApprovalID)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Status:       %s\n", a.Status)
	fmt.Printf("  Type:         %s\n", a.MutationType)
	fmt.Printf("  Target:       %s\n", a.TargetPath)
	fmt.Printf("  Run:          %s\n", a.RunID)
	fmt.Printf("  Correlation:  %s\n", a.CorrelationID)
	fmt.Printf("  Requested by: %s\n", a.RequestedBy)
	fmt.Printf("  Project:      %s\n", a.Project)
	fmt.Printf("  Policy:       %s (%s)\n", a.Policy.Decision, a.Policy.Reason)
	fmt.Printf("  Created:      %v\n", a.CreatedAt)
	if a.Preview != "" {
		fmt.Printf("  Preview:\n%s\n", a.Preview)
	}
	if a.Status == approval.StatusApproved {
		fmt.Printf("  Approved by:  %s at %v\n", a.ApprovedBy, a.ApprovedAt)
	}
	if a.Status == approval.StatusDenied {
		fmt.Printf("  Denied by:    %s at %v\n", a.DeniedBy, a.DeniedAt)
		if a.DenialReason != "" {
			fmt.Printf("  Reason:       %s\n", a.DenialReason)
		}
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeApprovalApprove(approvalID, approvedBy, runID, workspace, runsDir string) {
	if approvedBy == "" {
		fmt.Fprintln(os.Stderr, "Error: --by flag is required")
		os.Exit(1)
	}
	if runID == "" {
		fmt.Fprintln(os.Stderr, "Error: --run flag is required")
		os.Exit(1)
	}

	store := approval.NewStore(runsDir)
	executor := mutation.NewExecutor(workspace, store)

	// We'll emit events via print
	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		fmt.Printf("  💎 [%s] ", eventType)
		if id, ok := payload["approval_id"].(string); ok {
			fmt.Printf("approval=%s ", id)
		}
		if tp, ok := payload["target_path"].(string); ok {
			fmt.Printf("target=%s", tp)
		}
		fmt.Println()
	})

	result, err := executor.ApplyWithRun(context.Background(), runID, approvalID, approvedBy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	if result.Success {
		fmt.Println("  ✅ Mutation Applied Successfully")
	} else {
		fmt.Println("  ❌ Mutation Failed")
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Approval ID: %s\n", result.ApprovalID)
	fmt.Printf("  Target Path: %s\n", result.TargetPath)
	if result.Message != "" {
		fmt.Printf("  Message:     %s\n", result.Message)
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeApprovalDeny(approvalID, deniedBy, reason, runID, runsDir string) {
	if deniedBy == "" {
		fmt.Fprintln(os.Stderr, "Error: --by flag is required")
		os.Exit(1)
	}
	if runID == "" {
		fmt.Fprintln(os.Stderr, "Error: --run flag is required")
		os.Exit(1)
	}

	store := approval.NewStore(runsDir)
	executor := mutation.NewExecutor(".", store)

	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		fmt.Printf("  💎 [%s] ", eventType)
		if id, ok := payload["approval_id"].(string); ok {
			fmt.Printf("approval=%s", id)
		}
		fmt.Println()
	})

	err := executor.DenyApproval(runID, approvalID, deniedBy, reason)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  ❌ Approval Denied")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Approval ID: %s\n", approvalID)
	fmt.Printf("  Denied By:   %s\n", deniedBy)
	if reason != "" {
		fmt.Printf("  Reason:      %s\n", reason)
	}
	fmt.Println("  (No files were written)")
	fmt.Println("═══════════════════════════════════════════")
}


// ── V7: Workflow CLI Functions ──────────────────────────────────────────────────

func newWorkflowRegistry() *workflow.Registry {
	reg := workflow.NewRegistry()
	reg.LoadFromDir("examples/workflows") //nolint:errcheck // best-effort loading
	return reg
}

func executeWorkflowList() {
	registry := newWorkflowRegistry()
	names := registry.List()

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V7 Workflows")
	fmt.Println("═══════════════════════════════════════════")
	if len(names) == 0 {
		fmt.Println("  (no workflows registered)")
	}
	for _, name := range names {
		w, err := registry.Resolve(name)
		if err != nil {
			fmt.Printf("  %-30s (error: %v)\n", name, err)
			continue
		}
		fmt.Printf("  %-30s v%d — %s\n", name, w.Version, w.Description)
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeWorkflowShow(workflowName string) {
	registry := newWorkflowRegistry()
	w, err := registry.Resolve(workflowName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Workflow: %s\n", w.Name)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Description: %s\n", w.Description)
	fmt.Printf("  Version:     %d\n", w.Version)
	fmt.Printf("  Steps:       %d\n", len(w.Steps))
	for i, s := range w.Steps {
		fmt.Printf("    %d. [%s] %s", i+1, s.ID, s.Type)
		if s.When != "" {
			fmt.Printf(" (when: %s)", s.When)
		}
		fmt.Println()
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeWorkflowRun(workflowName, inputFile, runDir string) {
	registry := newWorkflowRegistry()
	w, err := registry.Resolve(workflowName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	var input map[string]any
	if inputFile != "" {
		data, readErr := os.ReadFile(inputFile)
		if readErr != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to read input file: %v\n", readErr)
			os.Exit(1)
		}
		if jsonErr := json.Unmarshal(data, &input); jsonErr != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid JSON input: %v\n", jsonErr)
			os.Exit(1)
		}
	}
	if input == nil {
		input = map[string]any{}
	}

	runID := event.NewRunID()
	artifactDir := filepath.Join(runDir, runID)
	if mkErr := os.MkdirAll(artifactDir, 0755); mkErr != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create run directory: %v\n", mkErr)
		os.Exit(1)
	}

	// Build handlers connecting to Prism primitives
	toolReg := tool.NewRegistry()
	tool.RegisterBuiltinsV4(toolReg, ".", 1024*1024)
	policyCfg := tool.PolicyConfig{WorkspaceRoot: ".", MaxFileSize: 1024 * 1024}
	toolExec := tool.NewExecutor(toolReg, policyCfg)

	handlers := workflow.StepHandlers{
		ToolExecute: func(ctx context.Context, toolName string, toolInput map[string]any) (map[string]any, error) {
			result, execErr := toolExec.ExecuteWithPolicy(ctx, toolName, "prism-workflow", "prism", runID, toolInput)
			if execErr != nil {
				return nil, execErr
			}
			if !result.Success {
				return nil, fmt.Errorf("tool %q failed: %s", toolName, result.Error)
			}
			return result.Output, nil
		},
	}

	runner := workflow.NewRunner(handlers)
	runner.SetRunDir(artifactDir)
	runner.SetEmitter(func(eventType, source string, payload map[string]any) {
		fmt.Printf("  \U0001F48E [%s]", eventType)
		if wn, ok := payload["workflow_name"]; ok {
			fmt.Printf(" workflow=%v", wn)
		}
		if sid, ok := payload["step_id"]; ok && sid != "" {
			fmt.Printf(" step=%v", sid)
		}
		if st, ok := payload["status"]; ok {
			fmt.Printf(" status=%v", st)
		}
		fmt.Println()
	})

	ctx := context.Background()
	result, runErr := runner.Run(ctx, w, input)
	if runErr != nil {
		fmt.Fprintf(os.Stderr, "Error: workflow failed: %v\n", runErr)
		os.Exit(1)
	}

	// Write artifacts
	_ = workflow.WriteWorkflowArtifacts(artifactDir, w, result.State, result)

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	switch result.Status {
	case "completed":
		fmt.Println("  \u2705 Workflow: COMPLETED")
	case "failed":
		fmt.Println("  \u274C Workflow: FAILED")
	case "paused":
		fmt.Println("  \u23F8\uFE0F  Workflow: PAUSED")
	default:
		fmt.Printf("  \u2753 Workflow: %s\n", result.Status)
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Workflow:     %s\n", w.Name)
	fmt.Printf("  Version:      %d\n", w.Version)
	fmt.Printf("  Status:       %s\n", result.Status)
	fmt.Printf("  Steps Run:    %d\n", len(result.State.StepStates))
	fmt.Printf("  Run ID:       %s\n", result.RunID)
	fmt.Printf("  Artifacts:    %s\n", artifactDir)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
}

func executeWorkflowStatus(runID, runDir string) {
	statePath := filepath.Join(runDir, runID, "workflow_state.json")
	data, err := os.ReadFile(statePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: workflow state not found for run %s: %v\n", runID, err)
		os.Exit(1)
	}

	var state workflow.WorkflowState
	if jsonErr := json.Unmarshal(data, &state); jsonErr != nil {
		fmt.Fprintf(os.Stderr, "Error: invalid workflow state: %v\n", jsonErr)
		os.Exit(1)
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Workflow: %s (v%d)\n", state.WorkflowName, state.Version)
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Status:        %s\n", state.Status)
	fmt.Printf("  Run ID:        %s\n", state.RunID)
	if state.CorrelationID != "" {
		fmt.Printf("  Correlation:   %s\n", state.CorrelationID)
	}
	if state.CurrentStep != nil {
		fmt.Printf("  Current Step:  %s\n", *state.CurrentStep)
	}
	fmt.Printf("  Steps:         %d\n", len(state.StepStates))
	for _, s := range state.StepStates {
		icon := "\u23F3"
		switch s.Status {
		case "completed":
			icon = "\u2705"
		case "failed":
			icon = "\u274C"
		case "skipped":
			icon = "\u23ED\uFE0F"
		}
		fmt.Printf("    %s [%s] %s", icon, s.ID, s.Type)
		if s.Status != "started" {
			fmt.Printf(" \u2014 %s", s.Status)
		}
		fmt.Println()
	}
	fmt.Println("═══════════════════════════════════════════")
}

func printUsage() {
	fmt.Println("Prism — Event-Native AI Agent Platform")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  prism run --task <description> [options]    Run a task lifecycle")
	fmt.Println("  prism tool list                               List available tools")
	fmt.Println("  prism tool run <name> --input '{...}'         Run a tool directly")
	fmt.Println("  prism validation list                         List validation profiles")
	fmt.Println("  prism validation run <name>                   Run a validation profile")
	fmt.Println("  prism approval list                           List pending approvals")
	fmt.Println("  prism approval show <id> --run <run_id>       Show approval details")
	fmt.Println("  prism approval approve <id> --by <name>       Approve a mutation")
	fmt.Println("  prism approval deny <id> --by <name>          Deny a mutation")
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
	fmt.Println("LLM provider options:")
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
	fmt.Println("  prism run --task \"Test event lifecycle\"")
	fmt.Println("  prism run --task \"Analyze code\" --provider ollama --model qwen2.5:7b")
	fmt.Println("  prism run --task \"Test dry run\" --dry-run-prompt")
	fmt.Println("  prism run --task \"Deploy service\" --project myapp --agent coder")
	fmt.Println("  prism tool list")
	fmt.Println("  prism tool run echo --input '{\"text\": \"hello\"}'")
	fmt.Println("  prism tool run read_file --input '{\"path\": \"README.md\"}' --workspace .")
	fmt.Println("  prism validation list")
	fmt.Println("  prism validation run go_test_all --project .")
	fmt.Println("  prism health")
	fmt.Println("  prism approval approve appr_xxx --by ema --run run_xxx --validate")
}

// ── V5: Validation CLI Functions ──────────────────────────────────────────

func executeValidationList() {
	registry := validation.NewRegistry()
	profiles := registry.List()

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V5 Validation Profiles")
	fmt.Println("═══════════════════════════════════════════")
	for _, name := range profiles {
		p, err := registry.Resolve(name)
		if err != nil {
			fmt.Printf("  %-20s (error: %v)\n", name, err)
			continue
		}
		fmt.Printf("  %-20s %s\n", p.Name, p.Description)
		fmt.Printf("    Command:  %s %s\n", p.Command, fmtArgs(p.Args))
		fmt.Printf("    Timeout:  %ds\n", p.TimeoutSeconds)
		fmt.Printf("    Allowed Exit Codes: %v\n", p.AllowedExitCodes)
	}
	fmt.Println("═══════════════════════════════════════════")
}

func fmtArgs(args []string) string {
	result := ""
	for i, a := range args {
		if i > 0 {
			result += " "
		}
		result += a
	}
	return result
}

func executeValidationRun(profileName, project, runDir, runID string) {
	registry := validation.NewRegistry()

	// Generate a run ID if not provided
	if runID == "" {
		runID = event.NewRunID()
	}

	artifactDir := filepath.Join(runDir, runID)
	if err := os.MkdirAll(artifactDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error: failed to create run directory: %v\n", err)
		os.Exit(1)
	}

	executor := validation.NewExecutor(registry, project, artifactDir)
	executor.SetEmitter(func(eventType, source string, payload map[string]any) {
		fmt.Printf("  💎 [%s] ", eventType)
		if pn, ok := payload["profile_name"].(string); ok {
			fmt.Printf("profile=%s ", pn)
		}
		fmt.Println()
	})

	ctx := context.Background()
	result, err := executor.Run(ctx, profileName, event.NewCorrelationID())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	if result.Status == "passed" {
		fmt.Println("  ✅ Validation Passed")
	} else {
		fmt.Printf("  ❌ Validation %s\n", result.Status)
	}
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Profile:    %s\n", result.Profile)
	fmt.Printf("  Status:     %s\n", result.Status)
	fmt.Printf("  Exit Code:  %d\n", result.ExitCode)
	fmt.Printf("  Duration:   %dms\n", result.DurationMs)
	if result.StdoutPath != "" {
		fmt.Printf("  Stdout:     %s\n", result.StdoutPath)
	}
	if result.StderrPath != "" {
		fmt.Printf("  Stderr:     %s\n", result.StderrPath)
	}
	if result.Error != "" {
		fmt.Printf("  Error:      %s\n", result.Error)
	}
	fmt.Println("═══════════════════════════════════════════")
}

// ── V5: Approve with Validation ───────────────────────────────────────────

func executeApprovalApproveWithValidation(approvalID, approvedBy, runID, workspace, runsDir string) {
	if approvedBy == "" {
		fmt.Fprintln(os.Stderr, "Error: --by flag is required")
		os.Exit(1)
	}
	if runID == "" {
		fmt.Fprintln(os.Stderr, "Error: --run flag is required")
		os.Exit(1)
	}

	log.SetFlags(log.Ltime | log.Lshortfile)

	runDir := filepath.Join(runsDir, runID)

	// Create a minimal runner for the V5 pipeline
	runner := run.NewRunner(run.RunConfig{
		Project:            "prism",
		RunDir:             runsDir,
		BusURL:             "nats://localhost:4222",
		ValidationRegistry: validation.NewRegistry(),
		Reviewer:           review.NewReviewer("lumi-deterministic"),
	})

	result, err := runner.ApproveWithValidation(runDir, approvalID, approvedBy, workspace)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println()
	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  ✅ Mutation Approved + Validated + Reviewed")
	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Run ID:       %s\n", runID)
	fmt.Printf("  Status:       %s\n", result.Status)

	if len(result.ValidationResults) > 0 {
		fmt.Println("  ── Validations ──")
		for _, vr := range result.ValidationResults {
			statusIcon := "✅"
			if vr.Status != "passed" {
				statusIcon = "❌"
			}
			fmt.Printf("    %s %s: %s (%dms)\n", statusIcon, vr.Profile, vr.Status, vr.DurationMs)
		}
	}

	if result.Review != nil {
		fmt.Println("  ── Review ──")
		fmt.Printf("    Reviewer:      %s\n", result.Review.Reviewer)
		fmt.Printf("    Recommendation: %s\n", result.Review.Recommendation)
		fmt.Printf("    Summary:        %s\n", result.Review.Summary)
		if result.ReviewArtifactPath != "" {
			fmt.Printf("    Artifact:       %s\n", result.ReviewArtifactPath)
		}
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println()
}