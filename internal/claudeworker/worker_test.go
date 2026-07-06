package claudeworker

import (
	"context"
	"strings"
	"testing"
)

func TestParseVerdict(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"approved", "Looks good.\nVERDICT: approved", "approved"},
		{"changes", "Issues found.\nVERDICT: changes_requested\nNOTES: missing tests", "changes_requested"},
		{"rejected", "VERDICT: rejected", "rejected"},
		{"approve word variant", "VERDICT: Approve", "approved"},
		{"no verdict defaults safe", "I think it's fine overall.", "changes_requested"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			v := ParseVerdict(c.in)
			if v.Decision != c.want {
				t.Fatalf("got %q want %q", v.Decision, c.want)
			}
		})
	}

	// NOTES line is captured.
	v := ParseVerdict("VERDICT: changes_requested\nNOTES: add a test for the error path")
	if v.Notes != "add a test for the error path" {
		t.Fatalf("notes not parsed: %q", v.Notes)
	}
}

// fakeRunner records the invocation and returns canned output.
type fakeRunner struct {
	gotExe   string
	gotArgs  []string
	gotStdin string
	gotCwd   string
	out      string
}

func (f *fakeRunner) Run(_ context.Context, exe string, args []string, stdin, cwd string) (RunResult, error) {
	f.gotExe, f.gotArgs, f.gotStdin, f.gotCwd = exe, args, stdin, cwd
	return RunResult{Stdout: f.out}, nil
}

func TestReviewInvokesPrintMode(t *testing.T) {
	fr := &fakeRunner{out: "VERDICT: approved"}
	w := NewWithRunner(Config{Model: "claude-opus-4-8", ReviewerName: "claude"}, fr)

	out, err := w.Review(context.Background(), "/repo/x", "review this plan")
	if err != nil {
		t.Fatalf("review error: %v", err)
	}
	if out != "VERDICT: approved" {
		t.Fatalf("unexpected output: %q", out)
	}
	if fr.gotCwd != "/repo/x" {
		t.Fatalf("cwd not passed: %q", fr.gotCwd)
	}
	if fr.gotStdin != "review this plan" {
		t.Fatalf("prompt not piped via stdin: %q", fr.gotStdin)
	}
	joined := strings.Join(fr.gotArgs, " ")
	if !strings.Contains(joined, "-p") {
		t.Fatalf("expected print mode -p, got args: %v", fr.gotArgs)
	}
	if !strings.Contains(joined, "--allowedTools") {
		t.Fatalf("expected read-only allowedTools, got args: %v", fr.gotArgs)
	}
	if !strings.Contains(joined, "claude-opus-4-8") {
		t.Fatalf("expected model flag, got args: %v", fr.gotArgs)
	}
	if w.ReviewerName() != "claude" {
		t.Fatalf("reviewer name: %q", w.ReviewerName())
	}
}

func TestReviewDefaultsReadOnly(t *testing.T) {
	cfg := Normalize(Config{})
	if cfg.Executable != "claude" {
		t.Fatalf("default executable = %q, want claude", cfg.Executable)
	}
	if !strings.Contains(cfg.AllowedTools, "Read") || strings.Contains(cfg.AllowedTools, "Edit") || strings.Contains(cfg.AllowedTools, "Write") {
		t.Fatalf("default allowed tools should be read-only: %q", cfg.AllowedTools)
	}
}
