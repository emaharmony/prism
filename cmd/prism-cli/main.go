// Package main implements the prism CLI — the human-facing entry point to the Prism
// agent platform. Every interaction starts here.
//
// As of V12, the CLI is split into multiple files for maintainability:
//
//	main.go          — This file: subcommand dispatch and flag definitions
//	cmd_run.go       — prism run
//	cmd_tool.go      — prism tool list/run
//	cmd_approval.go  — prism approval list/show/approve/deny
//	cmd_validation.go — prism validation list/run
//	cmd_policy.go    — prism policy list/evaluate
//	cmd_workflow.go  — prism workflow list/show/run/status
//	cmd_adapter.go   — prism adapter list/show/health
//	cmd_projection.go — prism projection list/rebuild/query
//	cmd_dashboard.go — prism dashboard
//	cmd_health.go    — prism health
//
// Subcommands:
//
//	prism run           — Execute a full V1→V5 run (task → LLM → tools → approval → validation → review)
//	prism health        — Check if the NATS event bus is reachable
//	prism tool list      — List available tools and their policies
//	prism tool run       — Execute a single tool directly (for testing)
//	prism approval list  — List pending/approved/denied approvals
//	prism approval show  — Show details of a specific approval
//	prism approval approve — Approve a pending mutation (writes file to disk)
//	prism approval deny  — Deny a pending mutation (file is NOT written)
//	prism validation list — List available validation profiles
//	prism validation run — Run a validation profile (e.g., go_test_all)
//	prism policy list    — List registered policy rules
//	prism policy evaluate — Evaluate a policy request
//	prism adapter list   — List registered adapters
//	prism adapter show   — Show details of a specific adapter
//	prism adapter health — Check health of a specific adapter
//	prism agent list     — List registered agents
//	prism agent show    — Show details of a specific agent
//	prism projection list — List available projections
//	prism projection rebuild — Rebuild projection snapshots from events
//	prism projection query — Query a projection snapshot
//	prism workflow list  — List registered workflows
//	prism workflow run   — Run a named workflow
//	prism workflow status — Show workflow run state
//
// Why raw os.Args instead of a CLI framework (cobra, etc.)? Prism's CLI grew to
// ~25 subcommands with simple flag sets; dispatch is a flat switch and help is
// grouped by commandUsage(). A framework would add a dependency for modest benefit;
// if argument parsing gets more complex, cobra would be worth adopting.
//
// Output formatting: Uses Unicode symbols (✅ ❌ 💎 ═══) for visual structure.
// These are cosmetic — the actual data is always in JSON artifacts under runs/.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/config"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/provider/anthropic"
	"github.com/emaharmony/prism/internal/provider/gemini"
	"github.com/emaharmony/prism/internal/provider/mock"
	"github.com/emaharmony/prism/internal/provider/ollama"
	"github.com/emaharmony/prism/internal/provider/openai"
)

func main() {
	// ── Subcommand: run ────────────────────────────────────────────
	runCmd := flag.NewFlagSet("run", flag.ExitOnError)
	taskFlag := runCmd.String("task", "", "Task description (required)")
	projectFlag := runCmd.String("project", "prism", "Project name")
	agentFlag := runCmd.String("agent", "prism", "Agent name (label for the run; defaults to the neutral instance name)")
	busURL := runCmd.String("bus-url", "nats://localhost:4222", "NATS bus URL")
	memoryEnabled := runCmd.Bool("memory-enabled", false, "Enable Remembrance context hook")
	requireMemory := runCmd.Bool("require-memory", false, "Fail if Remembrance is unavailable")
	memoryURL := runCmd.String("memory-url", "http://localhost:18790", "Remembrance URL")
	runDir := runCmd.String("run-dir", "./runs", "Directory for run outputs")

	// LLM flags
	providerFlag := runCmd.String("provider", "mock", "LLM provider: mock, ollama, openai, anthropic, gemini")
	modelFlag := runCmd.String("model", "mock-model", "Model name")
	temperatureFlag := runCmd.Float64("temperature", 0.2, "LLM temperature")
	maxTokensFlag := runCmd.Int("max-tokens", 2048, "Max output tokens")
	timeoutFlag := runCmd.Duration("timeout", 20*time.Minute, "LLM request timeout")
	dryRunPrompt := runCmd.Bool("dry-run-prompt", false, "Build prompt and artifacts but skip LLM call")
	ollamaURL := runCmd.String("ollama-url", "", "Ollama base URL (default: OLLAMA_BASE_URL env, then http://localhost:11434)")

	// OpenClaw config flags (V18)
	fromConfig := runCmd.Bool("from-config", false, "Load provider configuration from OpenClaw config")
	configPath := runCmd.String("config", "", "Path to openclaw.json (default: ~/.openclaw/openclaw.json)")

	// Context injection flags (V19)
	runContextFlag := runCmd.String("context", "", "Named contexts to inject (comma-separated: soul,agents,user,heartbeat,memory)")
	runContextAuto := runCmd.Bool("context-auto", false, "Auto-discover docs matching the task description")
	runContextFile := runCmd.String("context-file", "", "Additional context file to inject")
	runWorkspaceRoot := runCmd.String("workspace-root", "", "Workspace root directory (default: ~/.openclaw/workspace)")

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

		// Resolve provider — either from OpenClaw config or manual flags
		var p provider.Provider
		var providerName string
		model := *modelFlag
		var registry *provider.ProviderRegistry

		if *fromConfig {
			cfgFile := config.FindOpenClawConfig(*configPath)
			var err error
			registry, err = config.LoadFromOpenClaw(cfgFile)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error loading OpenClaw config: %v\n", err)
				os.Exit(1)
			}

			if registry.HasModel(model) {
				p, err = registry.Get(model)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: %v\n", err)
					os.Exit(1)
				}
				info, _ := registry.ModelInfo(model)
				providerName = info.ProviderName
			} else {
				fmt.Fprintf(os.Stderr, "Error: model %q not found in OpenClaw config. Available: %v\n", model, registry.ListModels())
				os.Exit(1)
			}
		} else {
			switch *providerFlag {
			case "mock":
				p = mock.New()
				providerName = "mock"
			case "ollama":
				p = ollama.New(resolveOllamaURL(*ollamaURL, ""))
				providerName = "ollama"
			case "openai":
				apiKey := os.Getenv("OPENAI_API_KEY")
				if apiKey == "" {
					fmt.Fprintln(os.Stderr, "Error: OPENAI_API_KEY environment variable required")
					os.Exit(1)
				}
				p = openai.New(apiKey)
				providerName = "openai"
			case "anthropic":
				apiKey := os.Getenv("ANTHROPIC_API_KEY")
				if apiKey == "" {
					fmt.Fprintln(os.Stderr, "Error: ANTHROPIC_API_KEY environment variable required")
					os.Exit(1)
				}
				p = anthropic.New(apiKey)
				providerName = "anthropic"
			case "gemini":
				apiKey := os.Getenv("GEMINI_API_KEY")
				if apiKey == "" {
					fmt.Fprintln(os.Stderr, "Error: GEMINI_API_KEY environment variable required")
					os.Exit(1)
				}
				p = gemini.New(apiKey)
				providerName = "gemini"
			default:
				fmt.Fprintf(os.Stderr, "Error: unknown provider '%s' (expected mock, ollama, openai, anthropic, or gemini)\n", *providerFlag)
				os.Exit(1)
			}
		}

		// V19 context flags — build workspace context with the same builder
		// as `prism context show` and inject it into the run prompt.
		var workspaceContext string
		if *runContextFlag != "" || *runContextAuto || *runContextFile != "" {
			workspaceRoot := *runWorkspaceRoot
			if workspaceRoot == "" {
				home, err := os.UserHomeDir()
				if err != nil {
					fmt.Fprintf(os.Stderr, "Error: cannot determine home directory for workspace root: %v\n", err)
					os.Exit(1)
				}
				workspaceRoot = filepath.Join(home, ".openclaw", "workspace")
			}
			// Same check as `prism context show`: a missing workspace is a
			// loud error, not silently-empty context.
			if _, err := os.Stat(workspaceRoot); os.IsNotExist(err) {
				fmt.Fprintf(os.Stderr, "Error: workspace directory not found: %s\n", workspaceRoot)
				os.Exit(1)
			}
			var namedContexts []string
			if *runContextFlag != "" {
				namedContexts = strings.Split(*runContextFlag, ",")
			}
			autoTask := ""
			if *runContextAuto {
				autoTask = *taskFlag
			}
			var explicitFiles []string
			if *runContextFile != "" {
				explicitFiles = []string{*runContextFile}
			}
			injected, err := buildContextForRun(workspaceRoot, namedContexts, autoTask, explicitFiles, 0)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error building context: %v\n", err)
				os.Exit(1)
			}
			workspaceContext = injected.FormattedString
		}

		executeRun(runConfig{
			Task:             *taskFlag,
			WorkspaceContext: workspaceContext,
			Project:          *projectFlag,
			Agent:            *agentFlag,
			BusURL:        *busURL,
			MemoryEnabled: *memoryEnabled,
			RequireMemory: *requireMemory,
			MemoryURL:     *memoryURL,
			RunDir:        *runDir,
			Provider:      p,
			ProviderName:  providerName,
			Model:         model,
			Temperature:   *temperatureFlag,
			MaxTokens:     *maxTokensFlag,
			Timeout:       *timeoutFlag,
			DryRunPrompt:  *dryRunPrompt,
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
			approvalApproveConfig := approvalApproveCmd.String("config", "prism.yaml", "Path to prism.yaml for write_roots")
			approvalValidate := approvalApproveCmd.Bool("validate", false, "Run validation and review after approving the mutation")
			approvalApproveCmd.Parse(os.Args[4:])

			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: approval ID required")
				os.Exit(1)
			}
			approveID := os.Args[3]
			if *approvalValidate {
				executeApprovalApproveWithValidation(approveID, *approvalApproveBy, *approvalApproveRun, *approvalApproveWorkspace, *approvalApproveRunsDir, *approvalApproveConfig)
			} else {
				executeApprovalApprove(approveID, *approvalApproveBy, *approvalApproveRun, *approvalApproveWorkspace, *approvalApproveRunsDir, *approvalApproveConfig)
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
	case "policy":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: policy subcommand required (list or evaluate)")
			fmt.Fprintln(os.Stderr, "Usage: prism policy list")
			fmt.Fprintln(os.Stderr, "       prism policy evaluate --input <request.json>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			executePolicyList()
		case "evaluate":
			policyEvalCmd := flag.NewFlagSet("policy evaluate", flag.ExitOnError)
			policyInput := policyEvalCmd.String("input", "", "JSON policy request file")
			policyDir := policyEvalCmd.String("policy-dir", "policies", "Directory containing policy YAML files")
			policyEvalCmd.Parse(os.Args[3:])
			if *policyInput == "" {
				fmt.Fprintln(os.Stderr, "Error: --input required")
				fmt.Fprintln(os.Stderr, "Usage: prism policy evaluate --input <request.json> --policy-dir <dir>")
				os.Exit(1)
			}
			executePolicyEvaluate(*policyInput, *policyDir)
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown policy subcommand '%s'\n", os.Args[2])
			os.Exit(1)
		}
	case "agent":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: agent subcommand required (list or show)")
			fmt.Fprintln(os.Stderr, "Usage: prism agent list")
			fmt.Fprintln(os.Stderr, "       prism agent show <agent_name>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			executeAgentList()
		case "show":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: agent name required")
				os.Exit(1)
			}
			executeAgentShow(os.Args[3])
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown agent subcommand '%s'\n", os.Args[2])
			os.Exit(1)
		}
	case "adapter":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: adapter subcommand required (list, show, or health)")
			fmt.Fprintln(os.Stderr, "Usage: prism adapter list")
			fmt.Fprintln(os.Stderr, "       prism adapter show <adapter_name>")
			fmt.Fprintln(os.Stderr, "       prism adapter health <adapter_name>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			executeAdapterList()
		case "show":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: adapter name required")
				os.Exit(1)
			}
			executeAdapterShow(os.Args[3])
		case "health":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: adapter name required")
				os.Exit(1)
			}
			executeAdapterHealth(os.Args[3])
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown adapter subcommand '%s'\n", os.Args[2])
			os.Exit(1)
		}
	case "projection":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: projection subcommand required (list, rebuild, or query)")
			fmt.Fprintln(os.Stderr, "Usage: prism projection list")
			fmt.Fprintln(os.Stderr, "       prism projection rebuild --run <run_id>")
			fmt.Fprintln(os.Stderr, "       prism projection rebuild --all")
			fmt.Fprintln(os.Stderr, "       prism projection query <name> --run <run_id>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "list":
			executeProjectionList()
		case "rebuild":
			projRebuildCmd := flag.NewFlagSet("projection rebuild", flag.ExitOnError)
			projRunID := projRebuildCmd.String("run", "", "Run ID to rebuild projections for")
			projAll := projRebuildCmd.Bool("all", false, "Rebuild projections for all runs")
			projDir := projRebuildCmd.String("run-dir", "./runs", "Directory for run outputs")
			projRebuildCmd.Parse(os.Args[3:])
			if *projRunID == "" && !*projAll {
				fmt.Fprintln(os.Stderr, "Error: --run or --all required")
				fmt.Fprintln(os.Stderr, "Usage: prism projection rebuild --run <run_id>")
				fmt.Fprintln(os.Stderr, "       prism projection rebuild --all")
				os.Exit(1)
			}
			executeProjectionRebuild(*projRunID, *projAll, *projDir)
		case "query":
			if len(os.Args) < 4 {
				fmt.Fprintln(os.Stderr, "Error: projection name required")
				fmt.Fprintln(os.Stderr, "Usage: prism projection query <name> --run <run_id>")
				os.Exit(1)
			}
			projName := os.Args[3]
			projQueryCmd := flag.NewFlagSet("projection query", flag.ExitOnError)
			projQueryRun := projQueryCmd.String("run", "", "Run ID")
			projQueryDir := projQueryCmd.String("run-dir", "./runs", "Directory for run outputs")
			projQueryCmd.Parse(os.Args[4:])
			if *projQueryRun == "" {
				fmt.Fprintln(os.Stderr, "Error: --run required")
				fmt.Fprintln(os.Stderr, "Usage: prism projection query <name> --run <run_id>")
				os.Exit(1)
			}
			executeProjectionQuery(projName, *projQueryRun, *projQueryDir)
		default:
			fmt.Fprintf(os.Stderr, "Error: unknown projection subcommand '%s'\n", os.Args[2])
			os.Exit(1)
		}
	case "dashboard":
		dashCmd := flag.NewFlagSet("dashboard", flag.ExitOnError)
		dashPort := dashCmd.String("port", "8080", "Dashboard listen port")
		dashRunDir := dashCmd.String("run-dir", "./runs", "Directory for run outputs")
		dashPolicyDir := dashCmd.String("policy-dir", "policies", "Directory containing policy YAML files")
		dashCmd.Parse(os.Args[2:])
		executeDashboard(*dashPort, *dashRunDir, *dashPolicyDir)
	case "workflow":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: workflow subcommand required (start, list, show, run, or status)")
			fmt.Fprintln(os.Stderr, "Usage: prism workflow start --project <id> --prompt \"...\"")
			fmt.Fprintln(os.Stderr, "       prism workflow list")
			fmt.Fprintln(os.Stderr, "       prism workflow show <workflow_name>")
			fmt.Fprintln(os.Stderr, "       prism workflow run <workflow_name> --input <file.json>")
			fmt.Fprintln(os.Stderr, "       prism workflow status <run_id>")
			os.Exit(1)
		}
		switch os.Args[2] {
		case "start":
			executeWorkflowStart(os.Args[3:])
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
	case "context":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "Error: context subcommand required (show)")
			fmt.Fprintln(os.Stderr, "Usage: prism context show [--context soul,agents] [--auto] [--task <desc>]")
			os.Exit(1)
		}
		if err := runContextCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "cost":
		costCmd.Parse(os.Args[2:])
		if costCmd.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Error: run ID required")
			fmt.Fprintln(os.Stderr, "Usage: prism cost <run_id>")
			os.Exit(1)
		}
		if err := runCostCommand(costCmd.Args()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "trace":
		traceCmd.Parse(os.Args[2:])
		if traceCmd.NArg() < 1 {
			fmt.Fprintln(os.Stderr, "Error: run ID required")
			fmt.Fprintln(os.Stderr, "Usage: prism trace <run_id>")
			os.Exit(1)
		}
		if err := runTraceCommand(traceCmd.Args()); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "search":
		searchCmd(os.Args[2:])
	case "chat":
		executeChat(os.Args[2:])
	case "serve":
		executeServe(os.Args[2:])
	case "status":
		executeStatus(os.Args[2:])
	case "watch":
		executeWatch(os.Args[2:])
	case "panel":
		executePanel(os.Args[2:])
	case "doctor":
		executeDoctor(os.Args[2:])
	case "preview":
		executePreview(os.Args[2:])
	case "scan":
		executeScan(os.Args[2:])
	case "mcp":
		executeMCP(os.Args[2:])
	case "skills":
		executeSkills(os.Args[2:])
	case "config":
		executeConfig(os.Args[2:])
	case "runs":
		executeRuns(os.Args[2:])
	case "remembrance":
		executeRemembrance(os.Args[2:])
	case "version":
		fmt.Println("prism v0.24.0")
	default:
		printUsage()
		os.Exit(1)
	}
}

// commandUsage returns the grouped command listing. Pure (no I/O) so the grouping
// and command coverage are unit-testable.
func commandUsage() string {
	groups := []struct {
		title string
		lines []string
	}{
		{"Run & interact", []string{
			"prism run --task <description> [options]      Run a one-shot task lifecycle",
			"prism chat [--config prism.yaml] [--agent <name>]  Interactive terminal chat",
			"prism serve [--config prism.yaml]             Start the persistent daemon (API, dashboard, bot)",
		}},
		{"Set up config", []string{
			"prism config wizard [--out prism.yaml]        Interactive setup — generate a prism.yaml",
			"prism config import <openclaw.json> [--out]   Convert an OpenClaw JSON to prism.yaml",
		}},
		{"Inspect & validate", []string{
			"prism config [--config prism.yaml] [--json]   Validate prism.yaml and summarize it",
			"prism doctor [--config prism.yaml] [--json]   Preflight check of run dependencies",
			"prism preview [--workflow <file>]             Static preview of the gated-loop workflow",
			"prism agent list | show <name>                List / show registered agents",
			"prism tool list | run <name> --input '{...}'  List / run built-in tools",
			"prism validation list | run <name>            List / run validation profiles",
			"prism context show [--context soul,agents]    Show context that would be injected",
		}},
		{"Observe runs", []string{
			"prism watch [--config prism.yaml]             Live view of a running gated-loop workflow",
			"prism panel [--port 8322]                     Launch the desktop pet panel (needs prism-panel build)",
			"prism runs [<run-id>|latest] [--json]         List runs / show a run's report (or state JSON)",
			"prism cost <run_id>                           Show token usage and cost report",
			"prism trace <run_id>                          Show event trace (causal DAG)",
			"prism dashboard [--port 8080]                 Start the local read-only dashboard",
		}},
		{"Self-patching", []string{
			"prism scan [--severity ...] [--json] [--start] Scan for issues (--start fixes the top one)",
		}},
		{"MCP tool servers", []string{
			"prism mcp [--json] | mcp probe <name>         List MCP servers / live-probe one's tools",
		}},
		{"Skills", []string{
			"prism skills [--json] | skills show <name>    List SKILL.md skills / show one's instructions",
		}},
		{"Approvals & mutations", []string{
			"prism approval list | show <id> --run <run_id> List / show approvals",
			"prism approval approve|deny <id> --by <name>  Approve or deny a mutation",
		}},
		{"Advanced", []string{
			"prism health [options]                        Check bus health",
			"prism search --query <text> [options]         Search the vector store",
			"prism projection list|rebuild|query           Manage CQRS projection snapshots",
			"prism adapter list|show|health [<name>]       Inspect adapters",
			"prism remembrance health|status|serve         Manage the Remembrance memory service",
			"prism version                                 Print version",
		}},
	}
	var b strings.Builder
	b.WriteString("Prism — Event-Native AI Agent Platform\n\nUsage: prism <command> [options]\n")
	for _, g := range groups {
		b.WriteString("\n" + g.title + ":\n")
		for _, l := range g.lines {
			b.WriteString("  " + l + "\n")
		}
	}
	return b.String()
}

func printUsage() {
	fmt.Print(commandUsage())
	fmt.Println()
	fmt.Println("Run options:")
	fmt.Println("  --task <string>        Task description (required)")
	fmt.Println("  --project <string>     Project name (default: prism)")
	fmt.Println("  --agent <string>       Agent name (default: prism)")
	fmt.Println("  --bus-url <string>     NATS bus URL (default: nats://localhost:4222)")
	fmt.Println("  --memory-enabled       Enable Remembrance context hook")
	fmt.Println("  --require-memory       Fail if Remembrance is unavailable")
	fmt.Println("  --memory-url <string>  Remembrance URL (default: http://localhost:18790)")
	fmt.Println("  --run-dir <string>     Run output directory (default: ./runs)")
	fmt.Println()
	fmt.Println("LLM provider options:")
	fmt.Println("  --provider <string>    LLM provider: mock, ollama, openai, anthropic, gemini (default: mock)")
	fmt.Println("  --model <string>       Model name (default: mock-model)")
	fmt.Println("  --temperature <float>  LLM temperature (default: 0.2)")
	fmt.Println("  --max-tokens <int>     Max output tokens (default: 2048)")
	fmt.Println("  --timeout <duration>   LLM request timeout (default: 20m)")
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
