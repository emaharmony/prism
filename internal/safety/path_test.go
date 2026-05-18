package safety

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsWithinRoot_NormalPath(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "src", "main.go")
	os.MkdirAll(filepath.Dir(target), 0755)
	os.WriteFile(target, []byte("hello"), 0644)

	if !IsWithinRoot(target, root) {
		t.Errorf("IsWithinRoot(%q, %q) = false, want true", target, root)
	}
}

func TestIsWithinRoot_PathTraversal(t *testing.T) {
	root := t.TempDir()
	escape := filepath.Join(root, "..", "..", "etc", "passwd")

	absRoot, _ := filepath.Abs(root)
	absEscape, _ := filepath.Abs(escape)

	if IsWithinRoot(absEscape, absRoot) {
		t.Errorf("IsWithinRoot should block path traversal")
	}
}

func TestIsWithinRoot_SamePath(t *testing.T) {
	root := t.TempDir()
	absRoot, _ := filepath.Abs(root)

	if !IsWithinRoot(absRoot, absRoot) {
		t.Errorf("IsWithinRoot(same, same) = false, want true")
	}
}

func TestIsWithinRoot_NestedPath(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "a", "b", "c", "file.txt")

	if !IsWithinRoot(nested, root) {
		t.Errorf("IsWithinRoot nested path = false, want true")
	}
}

func TestIsWithinRoot_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	// Create a symlink inside root that points outside
	linkPath := filepath.Join(root, "escape_link")
	os.Symlink(os.TempDir(), linkPath)

	target := filepath.Join(linkPath, "file.txt")
	if IsWithinRoot(target, root) {
		t.Errorf("IsWithinRoot should block symlink escape")
	}
}

func TestResolveAndContain_NormalPath(t *testing.T) {
	root := t.TempDir()
	resolved, err := ResolveAndContain(root, "src/main.go")
	if err != nil {
		t.Fatalf("ResolveAndContain() error = %v", err)
	}
	expected := filepath.Join(root, "src", "main.go")
	if resolved != expected {
		t.Errorf("ResolveAndContain() = %q, want %q", resolved, expected)
	}
}

func TestResolveAndContain_PathTraversal(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveAndContain(root, "../../etc/passwd")
	if err == nil {
		t.Error("ResolveAndContain should block path traversal")
	}
}

func TestResolveAndContain_AbsolutePath(t *testing.T) {
	root := t.TempDir()
	_, err := ResolveAndContain(root, "/etc/passwd")
	if err == nil {
		t.Error("ResolveAndContain should block absolute paths outside root")
	}
}