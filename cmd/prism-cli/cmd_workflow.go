package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/tool"
	"github.com/emaharmony/prism/internal/workflow"
)

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

// ── V8: Policy CLI Functions ─────────────────────────────────────────────────────

