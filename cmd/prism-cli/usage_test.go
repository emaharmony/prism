package main

import (
	"strings"
	"testing"
)

func TestCommandUsageHasGroups(t *testing.T) {
	out := commandUsage()
	for _, group := range []string{
		"Run & interact", "Inspect & validate", "Observe runs",
		"Self-patching", "MCP tool servers", "Approvals & mutations", "Advanced",
	} {
		if !strings.Contains(out, group+":") {
			t.Fatalf("usage missing group %q:\n%s", group, out)
		}
	}
}

// The grouped usage must not hardcode a specific persona (de-hardcoding: command
// help should be roster-neutral so it reads correctly for any configured agents).
func TestCommandUsageNoHardcodedPersona(t *testing.T) {
	out := strings.ToLower(commandUsage())
	for _, persona := range []string{"lumi", "mango", "junie"} {
		if strings.Contains(out, persona) {
			t.Fatalf("usage hardcodes persona %q — keep command help roster-neutral:\n%s", persona, commandUsage())
		}
	}
}

// Every user-facing subcommand should be discoverable in the grouped usage.
func TestCommandUsageCoversCommands(t *testing.T) {
	out := commandUsage()
	for _, cmd := range []string{
		"prism run", "prism chat", "prism serve",
		"prism config", "prism doctor", "prism preview", "prism agent", "prism tool", "prism validation", "prism context",
		"prism watch", "prism panel", "prism runs", "prism cost", "prism trace", "prism dashboard",
		"prism scan", "prism mcp", "prism skills", "prism approval",
		"prism health", "prism search", "prism projection", "prism adapter", "prism remembrance", "prism version",
	} {
		if !strings.Contains(out, cmd) {
			t.Fatalf("usage does not mention %q:\n%s", cmd, out)
		}
	}
}
