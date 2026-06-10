package autopatch

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emaharmony/prism/internal/validation"
)

type fakeWorker struct {
	name string
	run  func(req WorkerRequest) error
}

func (f fakeWorker) Name() string { return f.name }
func (f fakeWorker) Run(ctx context.Context, req WorkerRequest) (WorkerResult, error) {
	if f.run != nil {
		if err := f.run(req); err != nil {
			return WorkerResult{Worker: f.name}, err
		}
	}
	return WorkerResult{Worker: f.name, Diagnosis: "fake diagnosis"}, nil
}

func TestRequestMatches(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"auto patch the validation failure", true},
		{"tests are failing, fix this bug", true},
		{"diagnose the software bug and solve it", true},
		{"summarize the codebase", false},
		{"hello", false},
	}
	for _, tt := range tests {
		t.Run(tt.msg, func(t *testing.T) {
			if got := RequestMatches(tt.msg); got != tt.want {
				t.Fatalf("RequestMatches() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRunProposesPatch(t *testing.T) {
	root := newGitRepo(t)
	svc := NewService(Config{
		Enabled:              true,
		RequireCleanWorktree: true,
		MaxAttempts:          1,
		Root:                 root,
		WorktreeRoot:         filepath.Join(t.TempDir(), "worktrees"),
		ArtifactRoot:         filepath.Join(t.TempDir(), "artifacts"),
		ValidationProfiles:   []string{"echo_test"},
		Workers: map[string]PatchWorker{
			"fake": fakeWorker{name: "fake", run: func(req WorkerRequest) error {
				return os.WriteFile(filepath.Join(req.Worktree, "hello.txt"), []byte("patched\n"), 0644)
			}},
		},
		WorkerOrder: []string{"fake"},
	})

	result, err := svc.Run(context.Background(), "autopatch-test", Request{Description: "fix this bug"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Status != StatusProposed {
		t.Fatalf("status = %q, want proposed: %+v", result.Status, result)
	}
	if result.PatchPath == "" || result.ReviewPath == "" {
		t.Fatalf("missing artifacts: %+v", result)
	}
	diff, err := os.ReadFile(result.PatchPath)
	if err != nil {
		t.Fatalf("read patch: %v", err)
	}
	if !strings.Contains(string(diff), "patched") {
		t.Fatalf("patch missing change:\n%s", string(diff))
	}
}

func TestRunRejectsDirtyWorktree(t *testing.T) {
	root := newGitRepo(t)
	if err := os.WriteFile(filepath.Join(root, "dirty.txt"), []byte("dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{
		RequireCleanWorktree: true,
		Root:                 root,
		WorktreeRoot:         filepath.Join(t.TempDir(), "worktrees"),
		ArtifactRoot:         filepath.Join(t.TempDir(), "artifacts"),
		Workers:              map[string]PatchWorker{"fake": fakeWorker{name: "fake"}},
		WorkerOrder:          []string{"fake"},
	})
	_, err := svc.Run(context.Background(), "autopatch-dirty", Request{Description: "fix this bug"})
	if err == nil || !strings.Contains(err.Error(), ErrDirtyWorktree.Error()) {
		t.Fatalf("expected dirty worktree error, got %v", err)
	}
}

func TestRunRetriesAfterValidationFailure(t *testing.T) {
	root := newGoRepo(t)
	attempts := 0
	registry := validation.NewEmptyRegistry()
	if err := registry.Register(validation.Profile{
		Name:             "grep_fixed",
		Description:      "Passes when main.go contains the fixed return value",
		Command:          "git",
		Args:             []string{"grep", "return 1", "--", "main.go"},
		WorkingDir:       ".",
		TimeoutSeconds:   10,
		AllowedExitCodes: []int{0},
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewService(Config{
		RequireCleanWorktree: true,
		MaxAttempts:          2,
		Root:                 root,
		WorktreeRoot:         filepath.Join(t.TempDir(), "worktrees"),
		ArtifactRoot:         filepath.Join(t.TempDir(), "artifacts"),
		ValidationProfiles:   []string{"grep_fixed"},
		Registry:             registry,
		Workers: map[string]PatchWorker{
			"fake": fakeWorker{name: "fake", run: func(req WorkerRequest) error {
				attempts++
				content := "package main\nfunc Value() int { return 1 }\n"
				if attempts == 1 {
					content = "package main\nfunc Value() int { return }\n"
				}
				return os.WriteFile(filepath.Join(req.Worktree, "main.go"), []byte(content), 0644)
			}},
		},
		WorkerOrder: []string{"fake"},
	})

	result, err := svc.Run(context.Background(), "autopatch-retry", Request{Description: "fix compile bug"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
	if result.Status != StatusProposed {
		for _, vr := range result.ValidationResults {
			if vr.StdoutPath != "" {
				if data, err := os.ReadFile(vr.StdoutPath); err == nil {
					t.Logf("%s stdout:\n%s", vr.Profile, string(data))
				}
			}
			if vr.StderrPath != "" {
				if data, err := os.ReadFile(vr.StderrPath); err == nil {
					t.Logf("%s stderr:\n%s", vr.Profile, string(data))
				}
			}
		}
		t.Fatalf("status = %q, want proposed: %+v", result.Status, result.ValidationResults)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(dir, "hello.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial")
	return dir
}

func newGoRepo(t *testing.T) string {
	t.Helper()
	dir := newGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/autopatchtest\n\ngo 1.22\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\nfunc Value() int { return 0 }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main_test.go"), []byte("package main\nimport \"testing\"\nfunc TestValue(t *testing.T) { if Value() != 1 { t.Fatal(Value()) } }\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "go files")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, string(out))
	}
}
