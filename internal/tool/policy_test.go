package tool

import (
	"testing"
)

func TestPolicyEchoApproved(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "echo", map[string]any{"text": "hello"})
	if result.Decision != PolicyApproved {
		t.Errorf("echo should be approved, got %s", result.Decision)
	}
}

func TestPolicyWriteFileDryRunApproved(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "write_file_dry_run", map[string]any{
		"path":    "test.txt",
		"content": "hello",
	})
	if result.Decision != PolicyApproved {
		t.Errorf("write_file_dry_run should be approved, got %s", result.Decision)
	}
}

func TestPolicyListDirApprovedWithinRoot(t *testing.T) {
	cfg := PolicyConfig{WorkspaceRoot: ".", MaxFileSize: 1024 * 1024}
	result := EvaluatePolicy(cfg, "list_dir", map[string]any{"path": "."})
	if result.Decision != PolicyApproved {
		t.Errorf("list_dir with '.' should be approved, got %s: %s", result.Decision, result.Reason)
	}
}

func TestPolicyListDirDeniedTraversal(t *testing.T) {
	cfg := PolicyConfig{WorkspaceRoot: ".", MaxFileSize: 1024 * 1024}
	result := EvaluatePolicy(cfg, "list_dir", map[string]any{"path": "../etc"})
	if result.Decision != PolicyDenied {
		t.Errorf("list_dir with '..' path should be denied, got %s", result.Decision)
	}
}

func TestPolicyListDirDeniedAbsolutePath(t *testing.T) {
	cfg := PolicyConfig{WorkspaceRoot: "/tmp/workspace", MaxFileSize: 1024 * 1024}
	// Absolute paths that resolve outside workspace should be denied
	result := EvaluatePolicy(cfg, "list_dir", map[string]any{"path": "/etc/passwd"})
	if result.Decision != PolicyDenied {
		t.Errorf("list_dir with absolute path outside root should be denied, got %s", result.Decision)
	}
}

func TestPolicyReadFileApprovedWithinRoot(t *testing.T) {
	cfg := PolicyConfig{WorkspaceRoot: ".", MaxFileSize: 1024 * 1024}
	result := EvaluatePolicy(cfg, "read_file", map[string]any{"path": "README.md"})
	if result.Decision != PolicyApproved {
		t.Errorf("read_file with relative path should be approved, got %s: %s", result.Decision, result.Reason)
	}
}

func TestPolicyReadFileDeniedTraversal(t *testing.T) {
	cfg := PolicyConfig{WorkspaceRoot: ".", MaxFileSize: 1024 * 1024}
	result := EvaluatePolicy(cfg, "read_file", map[string]any{"path": "../../etc/passwd"})
	if result.Decision != PolicyDenied {
		t.Errorf("read_file with '..' path should be denied, got %s", result.Decision)
	}
}

func TestPolicyUnknownToolDenied(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "dangerous_tool", map[string]any{})
	if result.Decision != PolicyDenied {
		t.Errorf("unknown tool should be denied, got %s", result.Decision)
	}
}

func TestPolicyShellDenied(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "shell", map[string]any{"command": "rm -rf /"})
	if result.Decision != PolicyDenied {
		t.Errorf("shell tool should be denied, got %s", result.Decision)
	}
}

func TestPolicyListDirMissingPath(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "list_dir", map[string]any{})
	if result.Decision != PolicyDenied {
		t.Errorf("list_dir without path should be denied, got %s", result.Decision)
	}
}

func TestPolicyListDirNonStringPath(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "list_dir", map[string]any{"path": 123})
	if result.Decision != PolicyDenied {
		t.Errorf("list_dir with non-string path should be denied, got %s", result.Decision)
	}
}

func TestPolicyReadFileMissingPath(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "read_file", map[string]any{})
	if result.Decision != PolicyDenied {
		t.Errorf("read_file without path should be denied, got %s", result.Decision)
	}
}