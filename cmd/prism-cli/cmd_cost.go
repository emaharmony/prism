package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/emaharmony/prism/internal/cost"
	"github.com/emaharmony/prism/internal/event"
)

// costCmdFlags and cost command implementation.
var costCmd = flag.NewFlagSet("cost", flag.ExitOnError)
var costDataDir = costCmd.String("data", "./runs", "Data directory containing run outputs")

func init() {
	// Registered in main.go switch statement
}

func runCostCommand(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: prism cost <run_id>")
	}
	runID := args[0]
	dataDir := *costDataDir

	report, err := buildCostReport(runID, dataDir)
	if err != nil {
		return fmt.Errorf("cost: %w", err)
	}

	fmt.Printf("Run: %s\n", report.RunID)
	fmt.Printf("Total tokens: %d (prompt: %d, completion: %d)\n",
		report.TotalTokens, report.PromptTokens, report.CompletionTokens)
	fmt.Printf("Estimated cost: $%.6f\n", report.EstimatedCostUsd)
	fmt.Printf("Events: %d\n", report.EventCount)

	if len(report.ByProvider) > 0 {
		fmt.Println("\nBy provider:")
		for provider, costVal := range report.ByProvider {
			fmt.Printf("  %s: $%.6f\n", provider, costVal)
		}
	}

	if len(report.ByModel) > 0 {
		fmt.Println("\nBy model:")
		for model, costVal := range report.ByModel {
			fmt.Printf("  %s: $%.6f\n", model, costVal)
		}
	}

	if len(report.ByAgent) > 0 {
		fmt.Println("\nBy agent:")
		for agent, tokens := range report.ByAgent {
			fmt.Printf("  %s: %d tokens\n", agent, tokens)
		}
	}

	return nil
}

func buildCostReport(runID, dataDir string) (*cost.CostReport, error) {
	eventsFile := filepath.Join(dataDir, runID, "events.jsonl")
	f, err := os.Open(eventsFile)
	if err != nil {
		return nil, fmt.Errorf("open events: %w", err)
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	tracker := cost.NewCostTracker(runID)

	for decoder.More() {
		var evt event.Event
		if err := decoder.Decode(&evt); err != nil {
			continue
		}

		if evt.Type == "prism.llm.completed" || evt.Type == "prism.llm.failed" {
			provider, _ := evt.Payload["provider"].(string)
			model, _ := evt.Payload["model"].(string)
			agent := evt.Metadata.Agent

			var usage *cost.TokenUsage
			if evt.Metadata.TokenUsage != nil {
				tu := evt.Metadata.TokenUsage
				usage = &cost.TokenUsage{
					PromptTokens:     tu.PromptTokens,
					CompletionTokens: tu.CompletionTokens,
					TotalTokens:      tu.TotalTokens,
					EstimatedCostUsd:  tu.EstimatedCostUsd,
				}
			} else if evt.Metadata.TokenCost > 0 {
				usage = &cost.TokenUsage{
					TotalTokens:      evt.Metadata.TokenCost,
					EstimatedCostUsd: cost.EstimateCost(model, evt.Metadata.TokenCost),
				}
			}

			tracker.Track(provider, model, agent, usage)
		}
	}

	return tracker.Report(), nil
}

// Trace command implementation.
var traceCmd = flag.NewFlagSet("trace", flag.ExitOnError)
var traceDataDir = traceCmd.String("data", "./runs", "Data directory containing run outputs")

func runTraceCommand(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: prism trace <run_id>")
	}
	runID := args[0]
	dataDir := *traceDataDir

	events, err := loadTraceEvents(runID, dataDir)
	if err != nil {
		return fmt.Errorf("trace: %w", err)
	}

	printTrace(events)
	return nil
}

func loadTraceEvents(runID, dataDir string) ([]event.Event, error) {
	eventsFile := filepath.Join(dataDir, runID, "events.jsonl")
	f, err := os.Open(eventsFile)
	if err != nil {
		return nil, fmt.Errorf("open events: %w", err)
	}
	defer f.Close()

	var events []event.Event
	decoder := json.NewDecoder(f)
	for decoder.More() {
		var evt event.Event
		if err := decoder.Decode(&evt); err != nil {
			continue
		}
		events = append(events, evt)
	}
	return events, nil
}

func printTrace(events []event.Event) {
	children := make(map[string][]*event.Event)
	var roots []*event.Event

	for i := range events {
		evt := &events[i]
		if evt.ParentID == "" {
			roots = append(roots, evt)
		} else {
			children[evt.ParentID] = append(children[evt.ParentID], evt)
		}
	}

	var totalTokens int
	var totalCost float64

	for _, root := range roots {
		printEventNode(root, "", children, &totalTokens, &totalCost)
	}

	fmt.Printf("\nTotal: %d tokens | $%.4f\n", totalTokens, totalCost)
}

func printEventNode(evt *event.Event, indent string, children map[string][]*event.Event, totalTokens *int, totalCost *float64) {
	duration := ""
	if evt.Metadata.DurationMs > 0 {
		duration = fmt.Sprintf(" [%dms]", evt.Metadata.DurationMs)
	} else if evt.Metadata.LatencyMs > 0 {
		duration = fmt.Sprintf(" [%dms]", evt.Metadata.LatencyMs)
	}

	tokens := ""
	if evt.Metadata.TokenUsage != nil && evt.Metadata.TokenUsage.TotalTokens > 0 {
		tokens = fmt.Sprintf(" %d tokens", evt.Metadata.TokenUsage.TotalTokens)
		*totalTokens += evt.Metadata.TokenUsage.TotalTokens
		*totalCost += evt.Metadata.TokenUsage.EstimatedCostUsd
	} else if evt.Metadata.TokenCost > 0 {
		tokens = fmt.Sprintf(" %d tokens", evt.Metadata.TokenCost)
		*totalTokens += evt.Metadata.TokenCost
		*totalCost += cost.EstimateCost(evt.Metadata.Model, evt.Metadata.TokenCost)
	}

	outcome := ""
	if evt.Metadata.Outcome != "" {
		outcome = fmt.Sprintf(" (%s)", evt.Metadata.Outcome)
	}

	eventType := strings.TrimPrefix(evt.Type, "prism.")

	source := ""
	if evt.Metadata.Agent != "" {
		source = " " + evt.Metadata.Agent
	} else if evt.Metadata.Model != "" {
		source = " " + evt.Metadata.Model
	}

	payload := ""
	switch evt.Type {
	case "prism.task.created":
		if task, ok := evt.Payload["task"].(string); ok {
			payload = " " + traceTruncate(task, 40)
		}
	case "prism.tool.completed", "prism.tool.requested":
		if tool, ok := evt.Payload["tool_name"].(string); ok {
			payload = " " + tool
		}
	case "prism.mutation.proposed", "prism.mutation.applied":
		if path, ok := evt.Payload["target_path"].(string); ok {
			payload = " " + filepath.Base(path)
		}
	case "prism.approval.requested":
		if id, ok := evt.Payload["approval_id"].(string); ok {
			payload = " " + traceTruncate(id, 16)
		}
	}

	fmt.Printf("%s└─ %s%s%s%s%s%s\n",
		indent, eventType, source, payload, tokens, duration, outcome)

	childIndent := indent + "   "
	for _, child := range children[evt.ID] {
		printEventNode(child, childIndent, children, totalTokens, totalCost)
	}
}

func traceTruncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}