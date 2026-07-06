// Package claudecli contains shared Claude Code CLI executable resolution.
package claudecli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

const DefaultExecutable = "claude"

// LookPathFunc matches exec.LookPath and is injectable for tests.
type LookPathFunc func(string) (string, error)

// NormalizeExecutable applies the runtime default without probing the system.
func NormalizeExecutable(executable string) string {
	executable = strings.TrimSpace(executable)
	if executable != "" {
		return executable
	}
	return DefaultExecutable
}

// ResolveExecutable returns the executable path that should be used for Claude
// Code, failing early when no configured or default executable is available.
func ResolveExecutable(executable string, lookPath LookPathFunc) (string, error) {
	return resolveExecutable(executable, runtime.GOOS, lookPath)
}

func resolveExecutable(executable, goos string, lookPath LookPathFunc) (string, error) {
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	executable = strings.TrimSpace(executable)
	if executable != "" {
		path, err := lookPath(executable)
		if err != nil {
			return "", fmt.Errorf("Claude Code CLI executable %q not found: %w", executable, err)
		}
		return path, nil
	}

	candidates := defaultCandidates(goos)
	for _, candidate := range candidates {
		path, err := lookPath(candidate)
		if err == nil {
			return path, nil
		}
	}

	return "", fmt.Errorf("Claude Code CLI not found (tried %s); install Claude Code or set claude_code.executable", strings.Join(candidates, ", "))
}

func defaultCandidates(goos string) []string {
	candidates := []string{DefaultExecutable}
	if goos == "windows" {
		candidates = append(candidates, "claude.exe", "claude.cmd")
	}
	return candidates
}
