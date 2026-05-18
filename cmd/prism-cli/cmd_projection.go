package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/emaharmony/prism/internal/projection"
	approvalproj "github.com/emaharmony/prism/internal/projection/builtin/approval"
	"github.com/emaharmony/prism/internal/projection/builtin/runstatus"
	"github.com/emaharmony/prism/internal/projection/builtin/toolhistory"
)

func newProjectionRunner() *projection.Runner {
	return projection.NewRunner(
		runstatus.New(),
		approvalproj.New(),
		toolhistory.New(),
	)
}

func executeProjectionList() {
	runner := newProjectionRunner()
	names := runner.List()

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V10 Projections")
	fmt.Println("═══════════════════════════════════════════")
	for _, name := range names {
		fmt.Printf("  • %s\n", name)
	}
	fmt.Println("═══════════════════════════════════════════")
}

func executeProjectionRebuild(runID string, all bool, runsDir string) {
	runner := newProjectionRunner()

	if all {
		// Rebuild all runs
		entries, err := os.ReadDir(runsDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot read runs directory: %v\n", err)
			os.Exit(1)
		}

		count := 0
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if entry.Name() == "policy" {
				continue // skip policy dir
			}

			eventsFile := filepath.Join(runsDir, entry.Name(), "events.jsonl")
			if _, err := os.Stat(eventsFile); os.IsNotExist(err) {
				continue // skip runs without events
			}

			runPath := filepath.Join(runsDir, entry.Name())
			if err := runner.Run(eventsFile, runPath); err != nil {
				fmt.Fprintf(os.Stderr, "  Warning: %s: %v\n", entry.Name(), err)
				continue
			}
			count++
		}

		fmt.Println("═══════════════════════════════════════════")
		fmt.Printf("  Rebuilt projections for %d runs\n", count)
		fmt.Println("═══════════════════════════════════════════")
	} else {
		// Rebuild a single run
		eventsFile := filepath.Join(runsDir, runID, "events.jsonl")
		runPath := filepath.Join(runsDir, runID)

		if _, err := os.Stat(eventsFile); os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "Error: events file not found: %s\n", eventsFile)
			os.Exit(1)
		}

		if err := runner.Run(eventsFile, runPath); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("═══════════════════════════════════════════")
		fmt.Printf("  Rebuilt projections for run %s\n", runID)
		fmt.Println("═══════════════════════════════════════════")

		// Show which projections were written
		projDir := filepath.Join(runPath, "projections")
		entries, _ := os.ReadDir(projDir)
		for _, entry := range entries {
			fmt.Printf("  • %s\n", entry.Name())
		}
	}
}

func executeProjectionQuery(name, runID, runsDir string) {
	projFile := filepath.Join(runsDir, runID, "projections", name+".json")

	data, err := os.ReadFile(projFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: projection %q not found for run %s\n", name, runID)
		fmt.Fprintf(os.Stderr, "  Run 'prism projection rebuild --run %s' first\n", runID)
		os.Exit(1)
	}

	fmt.Println(string(data))
}
// ── V11: Dashboard CLI Function ─────────────────────────────────────────────

