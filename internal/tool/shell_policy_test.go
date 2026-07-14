package tool

import (
	"testing"
)

func TestEvaluateShellPolicy_HardBlocklist(t *testing.T) {
	policy := ShellPolicy{
		Tier:      "tier_3",
		Allowlist: []string{"*"},
		Blocklist: []string{},
	}

	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{"block rm -rf /", "rm -rf / --no-preserve-root", false},
		{"block rm -rf ~", "rm -rf ~/Documents", false},
		{"block rm -rf *", "rm -rf *", false},
		{"block mkfs", "mkfs.ext4 /dev/sda1", false},
		{"block dd to /dev", "dd if=/dev/zero of=/dev/sda", false},
		{"block redirect to /dev/sd", "echo foo > /dev/sda", false},
		{"block chmod 777", "chmod 777 /etc/passwd", false},
		{"block fork bomb", ":(){ :|:& };:", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateShellPolicy(policy, tt.command)
			if result.Allowed != tt.expected {
				t.Errorf("command %q: expected allowed=%v, got allowed=%v (reason: %s)", tt.command, tt.expected, result.Allowed, result.Reason)
			}
		})
	}
}

func TestEvaluateShellPolicy_TierNone(t *testing.T) {
	policy := ShellPolicy{
		Tier:      "none",
		Allowlist: []string{},
		Blocklist: []string{},
	}

	result := EvaluateShellPolicy(policy, "echo hello")
	if result.Allowed {
		t.Errorf("tier none should block all commands, got allowed (reason: %s)", result.Reason)
	}
}

func TestEvaluateShellPolicy_Tier1_Allowlist(t *testing.T) {
	policy := ShellPolicy{
		Tier: "tier_1",
		Allowlist: []string{
			"go build*",
			"go test*",
			"go vet*",
			"git status*",
			"git diff*",
			"git log*",
			"cat *",
			"ls *",
			"pwd",
			"head *",
			"tail *",
			"wc *",
		},
		Blocklist: []string{},
	}

	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{"allow go build", "go build ./...", true},
		{"allow go build simple", "go build", true},
		{"allow go test", "go test ./...", true},
		{"allow go vet", "go vet ./...", true},
		{"allow git status", "git status", true},
		{"allow git status --short", "git status --short", true},
		{"allow git diff", "git diff", true},
		{"allow git log", "git log --oneline", true},
		{"allow cat file", "cat main.go", true},
		{"allow ls", "ls -la", true},
		{"allow pwd", "pwd", true},
		{"allow head", "head -n 10 main.go", true},
		{"allow tail", "tail -n 10 main.go", true},
		{"allow wc", "wc -l main.go", true},
		{"block npm install", "npm install", false},
		{"block curl", "curl https://example.com", false},
		{"block rm file", "rm file.txt", false},
		{"block python3", "python3 script.py", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateShellPolicy(policy, tt.command)
			if result.Allowed != tt.expected {
				t.Errorf("command %q: expected allowed=%v, got allowed=%v (reason: %s)", tt.command, tt.expected, result.Allowed, result.Reason)
			}
		})
	}
}

func TestEvaluateShellPolicy_Tier2_Allowlist(t *testing.T) {
	policy := ShellPolicy{
		Tier: "tier_2",
		Allowlist: []string{
			"npm *",
			"node *",
			"python3 *",
			"pip *",
			"curl *",
			"docker *",
			"make *",
			"find *",
			"grep *",
			"sed *",
			"go *",
			"git *",
		},
		Blocklist: []string{},
	}

	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{"allow npm install", "npm install", true},
		{"allow npm test", "npm test", true},
		{"allow node script", "node index.js", true},
		{"allow python3", "python3 script.py", true},
		{"allow pip install", "pip install requests", true},
		{"allow curl", "curl https://example.com", true},
		{"allow docker ps", "docker ps", true},
		{"allow make", "make build", true},
		{"allow find", "find . -name '*.go'", true},
		{"allow grep", "grep -r 'pattern' .", true},
		{"allow sed", "sed 's/foo/bar/g' file.txt", true},
		{"allow go build", "go build ./...", true},
		{"allow git push", "git push origin main", true},
		{"block rm file", "rm file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateShellPolicy(policy, tt.command)
			if result.Allowed != tt.expected {
				t.Errorf("command %q: expected allowed=%v, got allowed=%v (reason: %s)", tt.command, tt.expected, result.Allowed, result.Reason)
			}
		})
	}
}

func TestEvaluateShellPolicy_Tier3_FullAccess(t *testing.T) {
	policy := ShellPolicy{
		Tier:      "tier_3",
		Allowlist: []string{"*"},
		Blocklist: []string{},
	}

	tests := []struct {
		name     string
		command  string
		expected bool
	}{
		{"allow any command", "echo hello", true},
		{"allow complex command", "find . -name '*.go' | xargs grep -l 'TODO'", true},
		{"allow npm", "npm install", true},
		{"allow python3", "python3 -c 'print(1+1)'", true},
		{"block hard blocklist still applies", "rm -rf /", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EvaluateShellPolicy(policy, tt.command)
			if result.Allowed != tt.expected {
				t.Errorf("command %q: expected allowed=%v, got allowed=%v (reason: %s)", tt.command, tt.expected, result.Allowed, result.Reason)
			}
		})
	}
}

func TestEvaluateShellPolicy_ConfigBlocklist(t *testing.T) {
	policy := ShellPolicy{
		Tier:      "tier_3",
		Allowlist: []string{"*"},
		Blocklist: []string{"shutdown*", "reboot*"},
	}

	result := EvaluateShellPolicy(policy, "shutdown -h now")
	if result.Allowed {
		t.Errorf("config blocklist should block shutdown, got allowed (reason: %s)", result.Reason)
	}

	result = EvaluateShellPolicy(policy, "reboot")
	if result.Allowed {
		t.Errorf("config blocklist should block reboot, got allowed (reason: %s)", result.Reason)
	}

	result = EvaluateShellPolicy(policy, "echo hello")
	if !result.Allowed {
		t.Errorf("echo should be allowed, got blocked (reason: %s)", result.Reason)
	}
}

func TestEvaluateShellPolicy_EmptyCommand(t *testing.T) {
	policy := ShellPolicy{
		Tier:      "tier_3",
		Allowlist: []string{"*"},
		Blocklist: []string{},
	}

	result := EvaluateShellPolicy(policy, "")
	if result.Allowed {
		t.Errorf("empty command should be blocked")
	}

	result = EvaluateShellPolicy(policy, "   ")
	if result.Allowed {
		t.Errorf("whitespace-only command should be blocked")
	}
}

func TestMatchShellPattern(t *testing.T) {
	tests := []struct {
		name     string
		pattern  string
		command  string
		expected bool
	}{
		{"exact match", "go build", "go build", true},
		{"wildcard matches everything", "*", "anything at all", true},
		{"prefix glob matches", "go build*", "go build ./...", true},
		{"prefix glob matches simple", "go build*", "go build", true},
		{"prefix glob no match", "go build*", "go test ./...", false},
		{"git status glob", "git status*", "git status --short", true},
		{"npm glob", "npm *", "npm install", true},
		{"npm glob no match", "npm *", "node index.js", false},
		{"no match different command", "ls *", "cat file.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := matchShellPattern(tt.pattern, tt.command)
			if result != tt.expected {
				t.Errorf("matchShellPattern(%q, %q): expected %v, got %v", tt.pattern, tt.command, tt.expected, result)
			}
		})
	}
}

func TestBuildShellPolicyFromConfig(t *testing.T) {
	allowlists := map[string][]string{
		"tier_1": {"go build*", "go test*"},
		"tier_2": {"npm *", "node *"},
	}
	blocked := []string{"shutdown*"}

	tests := []struct {
		name          string
		tier          string
		expectTier    string
		expectAllowed bool
		testCmd       string
	}{
		{"tier_1 with allowlist", "tier_1", "tier_1", true, "go build ./..."},
		{"tier_1 blocks non-allowlisted", "tier_1", "tier_1", false, "npm install"},
		{"tier_2 with allowlist", "tier_2", "tier_2", true, "npm install"},
		{"tier_3 full access", "tier_3", "tier_3", true, "echo hello"},
		{"none blocks all", "none", "none", false, "echo hello"},
		{"empty defaults to none", "", "none", false, "echo hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := BuildShellPolicyFromConfig(tt.tier, allowlists, blocked)
			if policy.Tier != tt.expectTier {
				t.Errorf("expected tier %q, got %q", tt.expectTier, policy.Tier)
			}
			result := EvaluateShellPolicy(policy, tt.testCmd)
			if result.Allowed != tt.expectAllowed {
				t.Errorf("command %q: expected allowed=%v, got allowed=%v (reason: %s)", tt.testCmd, tt.expectAllowed, result.Allowed, result.Reason)
			}
		})
	}
}
