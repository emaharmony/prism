package tool

import (
	"fmt"
	"strings"
)

// ShellPolicy defines the shell access tier and allow/block lists for a channel.
type ShellPolicy struct {
	Tier      string   // "none", "tier_1", "tier_2", "tier_3"
	Allowlist []string // glob-style patterns for allowed commands
	Blocklist []string // hard blocklist patterns (always enforced)
}

// ShellPolicyResult captures the decision and reason for a shell policy evaluation.
type ShellPolicyResult struct {
	Allowed bool
	Reason  string
}

// HardBlocklistPatterns are patterns that are ALWAYS blocked regardless of tier.
// These are checked first, before any tier-based allowlist evaluation.
// Patterns use glob-style matching: "chmod 777*" matches "chmod 777 /etc/passwd".
// Patterns starting with "*" match anywhere in the command (contains check).
var HardBlocklistPatterns = []string{
	"rm -rf /*",
	"rm -rf ~*",
	"rm -rf *",
	"mkfs*",
	"*dd if=*of=/dev/*",
	"*> /dev/sd*",
	"chmod 777*",
	":(){ :|:& };:", // fork bomb
}

// EvaluateShellPolicy checks whether a shell command is allowed under the given policy.
// The hard blocklist is ALWAYS enforced first. Then tier-based allowlist matching.
func EvaluateShellPolicy(policy ShellPolicy, command string) ShellPolicyResult {
	// Normalize command: trim whitespace
	cmd := strings.TrimSpace(command)
	if cmd == "" {
		return ShellPolicyResult{Allowed: false, Reason: "empty command"}
	}

	// 1. Hard blocklist — ALWAYS enforced, checked first
	for _, pattern := range HardBlocklistPatterns {
		if matchShellPattern(pattern, cmd) {
			return ShellPolicyResult{Allowed: false, Reason: fmt.Sprintf("command matches hard blocklist pattern %q", pattern)}
		}
	}

	// Also check config-defined blocked patterns
	for _, pattern := range policy.Blocklist {
		if matchShellPattern(pattern, cmd) {
			return ShellPolicyResult{Allowed: false, Reason: fmt.Sprintf("command matches blocked pattern %q", pattern)}
		}
	}

	// 2. Tier-based allowlist
	switch policy.Tier {
	case "none":
		return ShellPolicyResult{Allowed: false, Reason: "shell access is disabled (tier: none)"}

	case "tier_1", "tier_2":
		// Check against allowlist
		for _, pattern := range policy.Allowlist {
			if matchShellPattern(pattern, cmd) {
				return ShellPolicyResult{Allowed: true, Reason: fmt.Sprintf("command matches allowlist pattern %q (tier: %s)", pattern, policy.Tier)}
			}
		}
		return ShellPolicyResult{Allowed: false, Reason: fmt.Sprintf("command %q not in allowlist (tier: %s)", cmd, policy.Tier)}

	case "tier_3":
		// Full access — any command allowed (hard blocklist already checked)
		return ShellPolicyResult{Allowed: true, Reason: "full shell access (tier: tier_3)"}

	default:
		return ShellPolicyResult{Allowed: false, Reason: fmt.Sprintf("unknown shell policy tier: %q", policy.Tier)}
	}
}

// matchShellPattern performs glob-style pattern matching for shell commands.
// Patterns like "go build*" match "go build ./..." and "go build -o bin ."
// Patterns like "git status*" match "git status" and "git status --short"
// The "*" pattern matches everything.
// Patterns with embedded wildcards like "dd if=*of=/dev/*" use custom matching
// because filepath.Match treats "/" as a separator and won't match across it.
func matchShellPattern(pattern, command string) bool {
	// Exact match
	if pattern == command {
		return true
	}

	// Wildcard "*" matches everything
	if pattern == "*" {
		return true
	}

	// Glob-style suffix matching: "go build*" matches "go build ./..."
	// Only for patterns with a single trailing wildcard (no embedded wildcards).
	if strings.HasSuffix(pattern, "*") && !strings.Contains(pattern[:len(pattern)-1], "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(command, prefix)
	}

	// Glob-style prefix matching: "*> /dev/sd*" matches "echo foo > /dev/sda"
	// Only for patterns with a single leading wildcard (no other wildcards).
	if strings.HasPrefix(pattern, "*") && !strings.Contains(pattern[1:], "*") {
		suffix := strings.TrimPrefix(pattern, "*")
		return strings.Contains(command, suffix)
	}

	// For patterns with embedded wildcards (e.g., "dd if=*of=/dev/*"),
	// split on "*" and check that each segment appears in order.
	if strings.Contains(pattern, "*") {
		return matchGlobSegments(pattern, command)
	}

	return false
}

// matchGlobSegments splits a pattern on "*" and checks that each segment
// appears in the command in order. This handles patterns like
// "dd if=*of=/dev/*" matching "dd if=/dev/zero of=/dev/sda".
// A leading "*" means the pattern can match anywhere in the command (contains check).
func matchGlobSegments(pattern, command string) bool {
	segments := strings.Split(pattern, "*")
	remaining := command
	startsWith := !strings.HasPrefix(pattern, "*")

	for i, seg := range segments {
		if seg == "" {
			// Leading or trailing wildcard — skip
			if i == 0 {
				continue
			}
			// Trailing wildcard — everything after the last segment is fine
			if i == len(segments)-1 {
				return true
			}
			continue
		}

		idx := strings.Index(remaining, seg)
		if idx < 0 {
			return false
		}

		// For the first non-wildcard segment, if the pattern starts with it,
		// it must appear at the start of the command.
		if i == 0 && startsWith && idx != 0 {
			return false
		}

		// Move past this segment
		remaining = remaining[idx+len(seg):]
	}

	return true
}

// BuildShellPolicyFromConfig constructs a ShellPolicy from the orchestrator config.
func BuildShellPolicyFromConfig(tier string, allowlists map[string][]string, blockedPatterns []string) ShellPolicy {
	policy := ShellPolicy{
		Tier:      tier,
		Blocklist: blockedPatterns,
	}

	if tier == "none" || tier == "" {
		policy.Tier = "none"
		return policy
	}

	if tier == "tier_3" {
		policy.Tier = "tier_3"
		return policy
	}

	// For tier_1 and tier_2, populate the allowlist from config
	if allowlist, ok := allowlists[tier]; ok {
		policy.Allowlist = allowlist
	}

	return policy
}
