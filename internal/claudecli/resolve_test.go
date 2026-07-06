package claudecli

import (
	"errors"
	"strings"
	"testing"
)

var errMissingExecutable = errors.New("not found")

func fakeLookPath(found map[string]string) LookPathFunc {
	return func(name string) (string, error) {
		if path, ok := found[name]; ok {
			return path, nil
		}
		return "", errMissingExecutable
	}
}

func TestNormalizeExecutableDefaultsToClaude(t *testing.T) {
	if got := NormalizeExecutable(""); got != "claude" {
		t.Fatalf("default executable = %q, want claude", got)
	}
	if got := NormalizeExecutable(" claude.exe "); got != "claude.exe" {
		t.Fatalf("explicit executable = %q, want claude.exe", got)
	}
}

func TestResolveExecutableHonorsExplicitExecutable(t *testing.T) {
	path, err := resolveExecutable("C:/Tools/claude.exe", "windows", fakeLookPath(map[string]string{
		"C:/Tools/claude.exe": "C:/Tools/claude.exe",
		"claude":              "ignored",
	}))
	if err != nil {
		t.Fatalf("ResolveExecutable: %v", err)
	}
	if path != "C:/Tools/claude.exe" {
		t.Fatalf("path = %q", path)
	}
}

func TestResolveExecutableDoesNotFallbackWhenExplicitMissing(t *testing.T) {
	_, err := resolveExecutable("missing-claude", "windows", fakeLookPath(map[string]string{
		"claude": "C:/Users/me/.local/bin/claude.exe",
	}))
	if err == nil {
		t.Fatal("expected explicit missing executable to fail")
	}
	if !strings.Contains(err.Error(), "missing-claude") {
		t.Fatalf("error should name explicit executable, got %v", err)
	}
}

func TestResolveExecutableUsesWindowsFallbackCandidates(t *testing.T) {
	path, err := resolveExecutable("", "windows", fakeLookPath(map[string]string{
		"claude.exe": "C:/Users/me/.local/bin/claude.exe",
	}))
	if err != nil {
		t.Fatalf("ResolveExecutable: %v", err)
	}
	if path != "C:/Users/me/.local/bin/claude.exe" {
		t.Fatalf("path = %q", path)
	}
}

func TestResolveExecutableFailsWithCandidateList(t *testing.T) {
	_, err := resolveExecutable("", "windows", fakeLookPath(nil))
	if err == nil {
		t.Fatal("expected missing executable to fail")
	}
	for _, want := range []string{"claude", "claude.exe", "claude.cmd"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q should include %q", err, want)
		}
	}
}
