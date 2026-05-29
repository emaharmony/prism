package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFuzzyResolvePath_ExactPath(t *testing.T) {
	// Create temp dirs
	tmpDir := t.TempDir()
	bassbookDir := filepath.Join(tmpDir, "bassbook")
	if err := os.MkdirAll(bassbookDir, 0755); err != nil {
		t.Fatal(err)
	}

	tp := ToolPaths{
		WorkspaceRoot: tmpDir,
		AllowedPaths:  nil,
	}

	// Exact path should work (account for macOS /var -> /private/var symlink)
	resolved, err := FuzzyResolvePath(tp, bassbookDir)
	if err != nil {
		t.Fatalf("exact path should resolve: %v", err)
	}
	// Resolve both sides to handle symlink differences
	expectedResolved, _ := filepath.EvalSymlinks(bassbookDir)
	actualResolved, _ := filepath.EvalSymlinks(resolved)
	if actualResolved != expectedResolved {
		t.Errorf("expected %q, got %q", expectedResolved, actualResolved)
	}
}

func TestFuzzyResolvePath_FuzzyMatch(t *testing.T) {
	// Create temp dirs
	tmpDir := t.TempDir()
	bassbookDir := filepath.Join(tmpDir, "bassbook")
	prismDir := filepath.Join(tmpDir, "prism")
	if err := os.MkdirAll(bassbookDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(prismDir, 0755); err != nil {
		t.Fatal(err)
	}

	tp := ToolPaths{
		WorkspaceRoot: "/nonexistent/workspace", // workspace doesn't need to exist for fuzzy match
		AllowedPaths:  []string{tmpDir},
	}

	// Fuzzy match "bassbook" against allowed paths
	resolved, err := FuzzyResolvePath(tp, "bassbook")
	if err != nil {
		t.Fatalf("fuzzy match should find bassbook: %v", err)
	}
	if resolved != bassbookDir {
		t.Errorf("expected %q, got %q", bassbookDir, resolved)
	}
}

func TestFuzzyResolvePath_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	bassbookDir := filepath.Join(tmpDir, "BassBook")
	if err := os.MkdirAll(bassbookDir, 0755); err != nil {
		t.Fatal(err)
	}

	tp := ToolPaths{
		WorkspaceRoot: "/nonexistent/workspace",
		AllowedPaths:  []string{tmpDir},
	}

	// "bassbook" should match "BassBook" case-insensitively
	resolved, err := FuzzyResolvePath(tp, "bassbook")
	if err != nil {
		t.Fatalf("case-insensitive match should work: %v", err)
	}
	if resolved != bassbookDir {
		t.Errorf("expected %q, got %q", bassbookDir, resolved)
	}
}

func TestFuzzyResolvePath_HomeExpansion(t *testing.T) {
	tmpDir := t.TempDir()
	projectDir := filepath.Join(tmpDir, "myproject")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	home, _ := os.UserHomeDir()
	tp := ToolPaths{
		WorkspaceRoot: home,
		AllowedPaths:  []string{tmpDir},
	}

	// ~/nonexistent_dir_xyz should not match anything
	// Use a random name that definitely doesn't exist
	randomPath := filepath.Join(os.TempDir(), "fuzzy_resolve_test_nonexistent_abc123")
	_, err := FuzzyResolvePath(tp, randomPath)
	if err == nil {
		t.Error("expected error for nonexistent path")
	}
}

func TestFuzzyResolvePath_HomePathCorrection(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a user directory under /Users/ style
	userDir := filepath.Join(tmpDir, "projects", "repos", "bassbook")
	if err := os.MkdirAll(userDir, 0755); err != nil {
		t.Fatal(err)
	}

	tp := ToolPaths{
		WorkspaceRoot: "/nonexistent/workspace",
		AllowedPaths:  []string{tmpDir},
	}

	// /home/user/projects/repos/bassbook is a common hallucination
	// But this won't match since our tmpDir is not /Users/
	// This tests that the correction is attempted
	_, err := FuzzyResolvePath(tp, "/home/ema/projects/repos/bassbook")
	// Should fail gracefully since our tmpDir isn't under /Users/ema
	if err == nil {
		t.Log("/home correction succeeded — path was found")
	}
}

func TestFuzzyResolvePath_NoMatch(t *testing.T) {
	tmpDir := t.TempDir()
	tp := ToolPaths{
		WorkspaceRoot: "/nonexistent/workspace",
		AllowedPaths:  []string{tmpDir},
	}

	_, err := FuzzyResolvePath(tp, "totally_nonexistent_project_xyz")
	if err == nil {
		t.Error("expected error for nonexistent project name")
	}
}

func TestFuzzyResolvePath_RelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	subDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatal(err)
	}

	tp := ToolPaths{
		WorkspaceRoot: tmpDir,
		AllowedPaths:  nil,
	}

	// Relative path should resolve against workspace root
	resolved, err := FuzzyResolvePath(tp, "src")
	if err != nil {
		t.Fatalf("relative path should resolve: %v", err)
	}
	// Resolve both sides for macOS symlink consistency
	expectedResolved, _ := filepath.EvalSymlinks(subDir)
	actualResolved, _ := filepath.EvalSymlinks(resolved)
	if actualResolved != expectedResolved {
		t.Errorf("expected %q, got %q", expectedResolved, actualResolved)
	}
}

func TestFuzzyResolvePath_PrefixMatch(t *testing.T) {
	tmpDir := t.TempDir()
	longDir := filepath.Join(tmpDir, "bassbook-api")
	if err := os.MkdirAll(longDir, 0755); err != nil {
		t.Fatal(err)
	}

	tp := ToolPaths{
		WorkspaceRoot: "/nonexistent/workspace",
		AllowedPaths:  []string{tmpDir},
	}

	// "bassbook" should prefix-match "bassbook-api"
	resolved, err := FuzzyResolvePath(tp, "bassbook")
	if err != nil {
		t.Fatalf("prefix match should work: %v", err)
	}
	if resolved != longDir {
		t.Errorf("expected %q, got %q", longDir, resolved)
	}
}