package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/run"
)

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

