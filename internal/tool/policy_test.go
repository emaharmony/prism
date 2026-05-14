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

// ── V4 Policy Tests ──────────────────────────────────────────────

func TestPolicyV4_WriteFileDeniedDirectly(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "write_file", map[string]any{
		"path":    "test.txt",
		"content": "data",
	})
	if result.Decision != PolicyDenied {
		t.Errorf("direct write_file should be denied, got %s", result.Decision)
	}
}

func TestPolicyV4_WriteFileProposalRequiresApproval(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "write_file_proposal", map[string]any{
		"path":    "test.txt",
		"content": "Hello world",
	})
	if result.Decision != PolicyRequiresApproval {
		t.Errorf("write_file_proposal should require_approval, got %s", result.Decision)
	}
}

func TestPolicyV4_WriteFileProposalPathTraversal(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "write_file_proposal", map[string]any{
		"path":    "../../../etc/passwd",
		"content": "malicious",
	})
	if result.Decision != PolicyDenied {
		t.Errorf("write_file_proposal with '..' should be denied, got %s", result.Decision)
	}
}

func TestPolicyV4_WriteFileProposalAbsolutePath(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "write_file_proposal", map[string]any{
		"path":    "/etc/passwd",
		"content": "malicious",
	})
	if result.Decision != PolicyDenied {
		t.Errorf("write_file_proposal with absolute path should be denied, got %s", result.Decision)
	}
}

func TestPolicyV4_ApplyPatchProposalDenied(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "apply_patch_proposal", map[string]any{
		"patch": "some diff",
	})
	if result.Decision != PolicyDenied {
		t.Errorf("apply_patch_proposal should be denied (V5 candidate), got %s", result.Decision)
	}
}

func TestPolicyV4_ShellDenied(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "shell", map[string]any{
		"command": "ls",
	})
	if result.Decision != PolicyDenied {
		t.Errorf("shell should be denied, got %s", result.Decision)
	}
}

func TestPolicyV4_WriteFileProposalMissingContent(t *testing.T) {
	cfg := DefaultPolicyConfig()
	result := EvaluatePolicy(cfg, "write_file_proposal", map[string]any{
		"path": "test.txt",
	})
	if result.Decision != PolicyDenied {
		t.Errorf("write_file_proposal without content should be denied, got %s", result.Decision)
	}
}

func TestPolicyV4_WriteFileProposalOversizedContent(t *testing.T) {
	cfg := DefaultPolicyConfig()
	// Content over 1MB
	bigContent := make([]byte, 1024*1024+1)
	for i := range bigContent {
		bigContent[i] = 'x'
	}
	result := EvaluatePolicy(cfg, "write_file_proposal", map[string]any{
		"path":    "test.txt",
		"content": string(bigContent),
	})
	if result.Decision != PolicyDenied {
		t.Errorf("write_file_proposal with oversized content should be denied, got %s", result.Decision)
	}
}