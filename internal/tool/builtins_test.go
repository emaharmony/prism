package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestEchoTool(t *testing.T) {
	tool := &EchoTool{}
	result, err := tool.Execute(context.Background(), map[string]any{"text": "hello world"})
	if err != nil {
		t.Fatalf("echo should not error: %v", err)
	}
	if !result.Success {
		t.Errorf("echo should succeed, got: %s", result.Error)
	}
	if result.Output["text"] != "hello world" {
		t.Errorf("expected 'hello world', got %v", result.Output["text"])
	}
}

func TestEchoToolMissingText(t *testing.T) {
	tool := &EchoTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("echo should not return Go error: %v", err)
	}
	if result.Success {
		t.Error("echo without text should fail")
	}
}

func TestEchoToolSchema(t *testing.T) {
	tool := &EchoTool{}
	if tool.Name() != "echo" {
		t.Errorf("expected name 'echo', got %s", tool.Name())
	}
	schema := tool.Schema()
	if _, ok := schema.Input["text"]; !ok {
		t.Error("echo schema missing 'text' input parameter")
	}
}

func TestListDirTool(t *testing.T) {
	tmpDir := t.TempDir()
	// Create test structure
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)

	tool := &ListDirTool{WorkspaceRoot: tmpDir}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "."})
	if err != nil {
		t.Fatalf("list_dir should not error: %v", err)
	}
	if !result.Success {
		t.Errorf("list_dir should succeed, got: %s", result.Error)
	}

	count, ok := result.Output["count"].(int)
	if !ok {
		t.Errorf("expected count to be int, got %T", result.Output["count"])
	}
	if count != 3 {
		t.Errorf("expected 3 entries (2 files + 1 dir), got %d", count)
	}

	// Verify file/dir prefixes
	files, ok := result.Output["files"].([]string)
	if !ok {
		t.Fatalf("expected files to be []string, got %T", result.Output["files"])
	}
	hasFile := false
	hasDir := false
	for _, f := range files {
		if len(f) > 5 && f[:5] == "file:" {
			hasFile = true
		}
		if len(f) > 4 && f[:4] == "dir:" {
			hasDir = true
		}
	}
	if !hasFile {
		t.Error("list_dir should include file: prefix entries")
	}
	if !hasDir {
		t.Error("list_dir should include dir: prefix entries")
	}
}

func TestListDirToolMissingPath(t *testing.T) {
	tool := &ListDirTool{WorkspaceRoot: "."}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("list_dir should not return Go error: %v", err)
	}
	if result.Success {
		t.Error("list_dir without path should fail")
	}
}

func TestReadFileTool(t *testing.T) {
	tmpDir := t.TempDir()
	testContent := "hello world\nline 2\nline 3"
	os.WriteFile(filepath.Join(tmpDir, "test.txt"), []byte(testContent), 0644)

	tool := &ReadFileTool{WorkspaceRoot: tmpDir, MaxFileSize: 1024 * 1024}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "test.txt"})
	if err != nil {
		t.Fatalf("read_file should not error: %v", err)
	}
	if !result.Success {
		t.Errorf("read_file should succeed, got: %s", result.Error)
	}
	if result.Output["content"] != testContent {
		t.Errorf("content mismatch: expected %q, got %q", testContent, result.Output["content"])
	}
	if result.Output["path"] != "test.txt" {
		t.Errorf("path mismatch: expected 'test.txt', got %v", result.Output["path"])
	}
}

func TestReadFileToolMissingPath(t *testing.T) {
	tool := &ReadFileTool{WorkspaceRoot: ".", MaxFileSize: 1024 * 1024}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("read_file should not return Go error: %v", err)
	}
	if result.Success {
		t.Error("read_file without path should fail")
	}
}

func TestReadFileToolSizeLimit(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a file larger than the size limit
	largeContent := make([]byte, 2000) // 2KB file, 1KB limit
	for i := range largeContent {
		largeContent[i] = 'A'
	}
	os.WriteFile(filepath.Join(tmpDir, "large.txt"), largeContent, 0644)

	tool := &ReadFileTool{WorkspaceRoot: tmpDir, MaxFileSize: 1024} // 1KB limit
	result, err := tool.Execute(context.Background(), map[string]any{"path": "large.txt"})
	if err != nil {
		t.Fatalf("read_file should not error: %v", err)
	}
	if result.Success {
		t.Error("read_file should fail when file exceeds size limit")
	}
	if result.Error == "" {
		t.Error("expected error message for size limit exceeded")
	}
}

func TestReadFileToolNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	tool := &ReadFileTool{WorkspaceRoot: tmpDir, MaxFileSize: 1024 * 1024}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "nonexistent.txt"})
	if err != nil {
		t.Fatalf("read_file should not return Go error for missing file: %v", err)
	}
	if result.Success {
		t.Error("read_file should fail for nonexistent file")
	}
}

func TestWriteFileDryRunNoDiskWrite(t *testing.T) {
	tool := &WriteFileDryRun{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"path":    "test.txt",
		"content": "hello world",
	})
	if err != nil {
		t.Fatalf("write_file_dry_run should not error: %v", err)
	}
	if !result.Success {
		t.Errorf("write_file_dry_run should succeed, got: %s", result.Error)
	}

	// Verify it returns a preview diff
	preview, ok := result.Output["preview_diff"].(string)
	if !ok || preview == "" {
		t.Error("write_file_dry_run should return preview_diff")
	}
	if !contains(preview, "test.txt") {
		t.Error("preview diff should contain the file path")
	}

	// Verify it reports not written
	if result.Output["written"] != false {
		t.Error("write_file_dry_run should have written=false")
	}
}

func TestWriteFileDryRunMissingParams(t *testing.T) {
	tool := &WriteFileDryRun{}

	// Missing path
	result, err := tool.Execute(context.Background(), map[string]any{"content": "hello"})
	if err != nil {
		t.Fatalf("should not return Go error: %v", err)
	}
	if result.Success {
		t.Error("missing path should cause failure")
	}

	// Missing content
	result, err = tool.Execute(context.Background(), map[string]any{"path": "test.txt"})
	if err != nil {
		t.Fatalf("should not return Go error: %v", err)
	}
	if result.Success {
		t.Error("missing content should cause failure")
	}
}

func TestRegisterBuiltins(t *testing.T) {
	reg := NewRegistry()
	RegisterBuiltins(reg, ".", 1024*1024)

	names := reg.List()
	expectedTools := []string{"echo", "list_dir", "read_file", "write_file_dry_run", "read_project"}
	for _, name := range expectedTools {
		found := false
		for _, n := range names {
			if n == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected tool %q in registry, not found", name)
		}
	}
	if len(names) != len(expectedTools) {
		t.Errorf("expected %d builtins, got %d: %v", len(expectedTools), len(names), names)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// ── Symlink Traversal Security Tests ────────────────────────────────

func TestReadFileTool_BlocksSymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file outside the workspace
	outsideDir := filepath.Join(t.TempDir(), "outside")
	os.MkdirAll(outsideDir, 0755)
	os.WriteFile(filepath.Join(outsideDir, "secret.txt"), []byte("secret data"), 0644)

	// Create symlink inside workspace pointing outside
	linkPath := filepath.Join(tmpDir, "escape_link")
	os.Symlink(outsideDir, linkPath)

	tool := &ReadFileTool{WorkspaceRoot: tmpDir, MaxFileSize: 1024 * 1024}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "escape_link/secret.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("ReadFileTool should block symlink escape")
	}
}

func TestReadFileTool_BlocksDirectSymlink(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a file outside the workspace
	outsideFile := filepath.Join(t.TempDir(), "outside_secret.txt")
	os.WriteFile(outsideFile, []byte("secret data"), 0644)

	// Create symlink inside workspace pointing directly to outside file
	linkPath := filepath.Join(tmpDir, "secret_link")
	os.Symlink(outsideFile, linkPath)

	tool := &ReadFileTool{WorkspaceRoot: tmpDir, MaxFileSize: 1024 * 1024}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "secret_link"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("ReadFileTool should block direct symlink to outside file")
	}
}

func TestReadFileTool_AllowsNormalFiles(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "normal.txt"), []byte("hello world"), 0644)

	tool := &ReadFileTool{WorkspaceRoot: tmpDir, MaxFileSize: 1024 * 1024}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "normal.txt"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("ReadFileTool should allow normal files, got error: %s", result.Error)
	}
}

func TestListDirTool_BlocksSymlinkEscape(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a directory outside the workspace
	outsideDir := filepath.Join(t.TempDir(), "outside_dir")
	os.MkdirAll(outsideDir, 0755)

	// Create symlink inside workspace pointing outside
	linkPath := filepath.Join(tmpDir, "escape_link")
	os.Symlink(outsideDir, linkPath)

	tool := &ListDirTool{WorkspaceRoot: tmpDir}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "escape_link"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("ListDirTool should block symlink escape")
	}
}

func TestListDirTool_AllowsNormalDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	os.MkdirAll(filepath.Join(tmpDir, "subdir"), 0755)
	os.WriteFile(filepath.Join(tmpDir, "file.txt"), []byte("data"), 0644)

	tool := &ListDirTool{WorkspaceRoot: tmpDir}
	result, err := tool.Execute(context.Background(), map[string]any{"path": "."})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Errorf("ListDirTool should allow normal directories, got error: %s", result.Error)
	}
}