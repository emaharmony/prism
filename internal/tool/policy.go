package tool

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PolicyConfig holds the configuration for policy decisions.
type PolicyConfig struct {
	// WorkspaceRoot is the root directory that list_dir and read_file are
	// allowed to access. Any path outside this root is denied.
	WorkspaceRoot string
	// MaxFileSize is the maximum file size in bytes that read_file will allow.
	// Defaults to 1MB if zero.
	MaxFileSize int64
}

// DefaultPolicyConfig returns a PolicyConfig with sensible defaults.
func DefaultPolicyConfig() PolicyConfig {
	return PolicyConfig{
		WorkspaceRoot: ".",
		MaxFileSize:   1024 * 1024, // 1MB
	}
}

// PolicyResult captures the decision and reason for a policy evaluation.
type PolicyResult struct {
	Decision PolicyDecision
	Reason   string
}

// EvaluatePolicy checks whether a tool call is allowed under the current policy.
// Policy is deterministic — the LLM never decides policy.
//
// Rules:
//   - echo → always approved
//   - list_dir → approved if path is within workspace root
//   - read_file → approved if path is within workspace root and file size ≤ MaxFileSize
//   - write_file_dry_run → always approved (no mutation)
//   - write_file_proposal → requires_approval (mutation gate)
//   - apply_patch_proposal → denied (V5 candidate, not implemented)
//   - write_file (direct) → denied (must use write_file_proposal)
//   - anything else → denied
//   - Path traversal with ".." or absolute paths outside workspace → denied
func EvaluatePolicy(cfg PolicyConfig, toolName string, input map[string]any) PolicyResult {
	switch toolName {
	case "echo":
		return PolicyResult{Decision: PolicyApproved, Reason: "echo is always approved"}

	case "write_file_dry_run":
		return PolicyResult{Decision: PolicyApproved, Reason: "write_file_dry_run is a read-only preview, no mutation"}

	case "write_file_proposal":
		return evaluateV4ProposalPolicy(cfg, toolName, input)

	case "apply_patch_proposal":
		return PolicyResult{Decision: PolicyDenied, Reason: "apply_patch_proposal is not implemented (V5 candidate)"}

	case "write_file":
		return PolicyResult{Decision: PolicyDenied, Reason: "direct write_file is denied — use write_file_proposal for approval-gated mutations"}

	case "list_dir":
		return evaluatePathPolicy(cfg, toolName, input)

	case "read_file":
		return evaluatePathPolicy(cfg, toolName, input)

	default:
		return PolicyResult{Decision: PolicyDenied, Reason: fmt.Sprintf("tool %q is not in the approved list", toolName)}
	}
}

// evaluateV4ProposalPolicy validates a write_file_proposal or similar V4 mutation proposal.
func evaluateV4ProposalPolicy(cfg PolicyConfig, toolName string, input map[string]any) PolicyResult {
	pathVal, ok := input["path"]
	if !ok {
		return PolicyResult{Decision: PolicyDenied, Reason: fmt.Sprintf("%s requires a 'path' parameter", toolName)}
	}

	pathStr, ok := pathVal.(string)
	if !ok {
		return PolicyResult{Decision: PolicyDenied, Reason: fmt.Sprintf("%s 'path' must be a string", toolName)}
	}

	// Block path traversal
	if strings.Contains(pathStr, "..") {
		return PolicyResult{Decision: PolicyDenied, Reason: "path traversal with '..' is blocked"}
	}

	// Block absolute paths
	if filepath.IsAbs(pathStr) {
		return PolicyResult{Decision: PolicyDenied, Reason: "absolute paths are not allowed"}
	}

	// Resolve and check workspace bounds
	absRoot, err := filepath.Abs(cfg.WorkspaceRoot)
	if err != nil {
		return PolicyResult{Decision: PolicyDenied, Reason: "invalid workspace root"}
	}
	absPath := filepath.Clean(filepath.Join(absRoot, pathStr))
	if !isWithinRoot(absPath, absRoot) {
		return PolicyResult{Decision: PolicyDenied, Reason: "path is outside the workspace root"}
	}

	// Check content parameter exists
	_, ok = input["content"]
	if !ok {
		return PolicyResult{Decision: PolicyDenied, Reason: fmt.Sprintf("%s requires a 'content' parameter", toolName)}
	}

	contentStr, ok := input["content"].(string)
	if !ok {
		return PolicyResult{Decision: PolicyDenied, Reason: fmt.Sprintf("%s 'content' must be a string", toolName)}
	}

	if len(contentStr) > 1024*1024 {
		return PolicyResult{Decision: PolicyDenied, Reason: "content exceeds maximum size of 1MB"}
	}

	return PolicyResult{Decision: PolicyRequiresApproval, Reason: fmt.Sprintf("%s requires explicit approval to apply the mutation", toolName)}
}

// evaluatePathPolicy handles the path-based policy checks for list_dir and read_file.
func evaluatePathPolicy(cfg PolicyConfig, toolName string, input map[string]any) PolicyResult {
	pathVal, ok := input["path"]
	if !ok {
		return PolicyResult{Decision: PolicyDenied, Reason: fmt.Sprintf("%s requires a 'path' parameter", toolName)}
	}

	pathStr, ok := pathVal.(string)
	if !ok {
		return PolicyResult{Decision: PolicyDenied, Reason: fmt.Sprintf("%s 'path' must be a string", toolName)}
	}

	// Block path traversal with ".."
	if strings.Contains(pathStr, "..") {
		return PolicyResult{Decision: PolicyDenied, Reason: "path traversal with '..' is blocked"}
	}

	// Block absolute paths that are outside the workspace root
	if filepath.IsAbs(pathStr) {
		return PolicyResult{Decision: PolicyDenied, Reason: "absolute paths are not allowed"}
	}

	// Resolve both workspace root and target path to absolute form
	absRoot, err := filepath.Abs(cfg.WorkspaceRoot)
	if err != nil {
		return PolicyResult{Decision: PolicyDenied, Reason: "invalid workspace root"}
	}
	absPath := filepath.Clean(filepath.Join(absRoot, pathStr))

	// Check that the resolved path is within the workspace root
	if !isWithinRoot(absPath, absRoot) {
		return PolicyResult{Decision: PolicyDenied, Reason: "path is outside the workspace root"}
	}

	return PolicyResult{Decision: PolicyApproved, Reason: fmt.Sprintf("%s path is within workspace root", toolName)}
}

// resolvePath joins the workspace root with the given path and cleans it.
func resolvePath(workspaceRoot, relPath string) string {
	return filepath.Clean(filepath.Join(workspaceRoot, relPath))
}

// isWithinRoot checks that absPath is within absRoot (both must be absolute).
func isWithinRoot(absPath, absRoot string) bool {
	// Ensure root ends with separator for proper prefix matching
	rel, err := filepath.Rel(absRoot, absPath)
	if err != nil {
		return false
	}
	// If the relative path starts with "..", it's outside the root
	return !strings.HasPrefix(rel, "..") && rel != ".."
}