package main

import (
	"fmt"
	"os"
	"github.com/nats-io/nats.go"
	"github.com/emaharmony/prism/internal/event"
)

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

