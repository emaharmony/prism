// cmd_adapter.go implements the `prism adapter` subcommands.
//
// Adapters are Prism's way of talking to the outside world (V9). Each adapter
// implements a contract: Name, Version, Capabilities, Execute, Health. The
// CLI commands here let you inspect which adapters are registered, see their
// capabilities, and check their health.
//
// Built-in adapters (like echo) are registered programmatically. Future
// adapters can be loaded from YAML manifests (see adapters/ directory).
//
// Commands:
//   prism adapter list          — Show all registered adapters
//   prism adapter show <name>   — Show adapter details and capabilities
//   prism adapter health <name> — Check if an adapter is ready
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/emaharmony/prism/internal/adapter"
	"github.com/emaharmony/prism/internal/adapter/builtin/echo"
)

// newAdapterRegistry creates a registry with built-in adapters.
// Currently only the echo adapter is registered. As new adapters are added
// (e.g., a trading adapter, a database adapter), they get registered here.
func newAdapterRegistry() *adapter.Registry {
	reg := adapter.NewRegistry()
	echoA := &echo.EchoAdapter{}
	reg.Register(echoA)
	return reg
}

// executeAdapterList shows all registered adapters with version and
// capability count.
func executeAdapterList() {
	reg := newAdapterRegistry()
	names := reg.List()

	fmt.Println("═══════════════════════════════════════════")
	fmt.Println("  Prism V9 Adapters")
	fmt.Println("═══════════════════════════════════════════")
	if len(names) == 0 {
		fmt.Println("  (no adapters registered)")
	}
	for _, name := range names {
		a, _ := reg.Resolve(name)
		caps := a.Capabilities()
		fmt.Printf("  %-20s v%-5s  %d capabilit%s\n", name, a.Version(), len(caps), plural(len(caps)))
	}
	fmt.Println("═══════════════════════════════════════════")
}

// executeAdapterShow displays detailed info about one adapter: version,
// capabilities, and whether each capability requires approval.
func executeAdapterShow(name string) {
	reg := newAdapterRegistry()
	a, err := reg.Resolve(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Adapter: %s\n", a.Name())
	fmt.Printf("  Version:  %s\n", a.Version())
	fmt.Println("═══════════════════════════════════════════")

	caps := a.Capabilities()
	if len(caps) > 0 {
		fmt.Println("  Capabilities:")
		for _, c := range caps {
			approval := ""
			if c.RequiresApproval {
				approval = " (requires approval)"
			}
			fmt.Printf("    %-15s %s%s\n", c.Action, c.Description, approval)
		}
	} else {
		fmt.Println("  (no capabilities)")
	}
	fmt.Println("═══════════════════════════════════════════")
}

// executeAdapterHealth checks if an adapter is ready to process requests.
// This is useful for verifying that external dependencies (APIs, databases)
// are reachable before running a workflow that depends on them.
func executeAdapterHealth(name string) {
	reg := newAdapterRegistry()
	a, err := reg.Resolve(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()
	health, err := a.Health(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking health: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("═══════════════════════════════════════════")
	fmt.Printf("  Adapter: %s\n", name)
	if health.Ready {
		fmt.Println("  Status:  ✅ Ready")
	} else {
		fmt.Println("  Status:  ❌ Not Ready")
	}
	if health.Message != "" {
		fmt.Printf("  Message: %s\n", health.Message)
	}
	if len(health.Details) > 0 {
		fmt.Println("  Details:")
		for k, v := range health.Details {
			fmt.Printf("    %s: %v\n", k, v)
		}
	}
	fmt.Println("═══════════════════════════════════════════")
}

// plural returns the correct suffix for "capability" (y/ies).
func plural(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}