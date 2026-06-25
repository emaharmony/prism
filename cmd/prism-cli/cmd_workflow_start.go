// Package main implements the `prism workflow start` subcommand — the
// interactive trigger for the gated loop. It publishes a start request to a
// running Prism serve instance over NATS, which runs the full 7-phase loop.
//
// Usage:
//
//	prism workflow start --project <id> --prompt "build feature X"
//	prism workflow start --prompt "..." --nats nats://127.0.0.1:4222
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/nats-io/nats.go"
)

func executeWorkflowStart(args []string) {
	cmd := flag.NewFlagSet("workflow start", flag.ExitOnError)
	configPath := cmd.String("config", "prism.yaml", "Path to prism.yaml configuration file")
	project := cmd.String("project", "", "Project ID to work on (defaults to the configured default project)")
	prompt := cmd.String("prompt", "", "The task prompt that seeds the gated loop")
	channel := cmd.String("channel", "", "Channel ID to post results to (defaults to the project channel)")
	natsURL := cmd.String("nats", "", "NATS URL (defaults to prism.nats_url or nats://127.0.0.1:4222)")
	cmd.Parse(args)

	if *prompt == "" {
		fmt.Fprintln(os.Stderr, "Error: --prompt is required")
		fmt.Fprintln(os.Stderr, `Usage: prism workflow start --project <id> --prompt "build feature X"`)
		os.Exit(1)
	}

	// Resolve the NATS URL from config when not given explicitly.
	url := *natsURL
	if url == "" {
		if cfg, err := orchestrator.LoadConfig(*configPath); err == nil && cfg.Prism.NATSURL != "" {
			url = cfg.Prism.NATSURL
		}
	}
	if url == "" {
		url = "nats://127.0.0.1:4222"
	}

	nc, err := nats.Connect(url, nats.Name("prism-workflow-start"), nats.Timeout(5*time.Second))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: could not connect to NATS at %s: %v\n", url, err)
		fmt.Fprintln(os.Stderr, "Is `prism serve` running with an external NATS (prism.nats_url)?")
		os.Exit(1)
	}
	defer nc.Close()

	payload, _ := json.Marshal(map[string]string{
		"project": *project,
		"prompt":  *prompt,
		"channel": *channel,
	})
	if err := nc.Publish("prism.workflow.start", payload); err != nil {
		fmt.Fprintf(os.Stderr, "Error publishing start request: %v\n", err)
		os.Exit(1)
	}
	if err := nc.FlushTimeout(5 * time.Second); err != nil {
		fmt.Fprintf(os.Stderr, "Error flushing to NATS: %v\n", err)
		os.Exit(1)
	}

	target := *project
	if target == "" {
		target = "(default project)"
	}
	fmt.Printf("🔁 Gated loop start requested for %s\n", target)
	fmt.Printf("   prompt: %s\n", *prompt)
	fmt.Println("   The running Prism instance will execute the loop and post results.")
}
