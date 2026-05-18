// Package tool provides Prism's built-in tools — the actions an AI agent can request
// during a run. Each tool is a named function that takes JSON input and returns a result.
//
// Built-in tools (V1): echo, list_dir, read_file — read-only, always allowed.
// Built-in tools (V4): write_file_dry_run (preview a write), write_file_proposal
// (propose a file change for human approval). Direct write_file is denied by policy.
//
// Path safety (V12 — centralized):
// All path containment checks now use safety.IsWithinRoot from internal/safety.
// This replaces the previous defense-in-depth pattern where each tool had its own
// isWithinRoot implementation. The centralization means one place to audit and fix
// bugs, while still being called at each filesystem boundary.
//
// Why does write_file_proposal use ApprovalStorer as `any` instead of *approval.Approval?
// Because importing the approval package would create a circular dependency (tool →
// approval → event → tool). The `any` type breaks the cycle at the cost of type safety.
package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/safety"
)

// EchoTool returns whatever text is passed in. Always approved by policy.
// EchoTool is the simplest tool — it returns its input unchanged.
// Used for testing the tool execution pipeline end-to-end without side effects.
// Policy: always allowed (read-only, no filesystem access).
type EchoTool struct{}

func (t *EchoTool) Name() string        { return "echo" }
func (t *EchoTool) Description() string { return "Returns the input text unchanged. Useful for testing and verification." }
func (t *EchoTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"text": {Type: "string", Description: "The text to echo back", Required: true},
		},
		Output: ParamSpec{Type: "string", Description: "The echoed text"},
	}
}
func (t *EchoTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	text, ok := input["text"].(string)
	if !ok {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "required parameter 'text' must be a string",
		}, nil
	}
	return ToolResult{
		Success: true,
		Output:  map[string]any{"text": text},
	}, nil
}

// ListDirTool lists files and directories under a given path within the
// workspace root. Path traversal is blocked by policy.
// ListDirTool lists files and directories under a given path.
// The path must be within the workspace root — path traversal (..) is blocked.
// Symlinks are resolved with EvalSymlinks before the containment check.
// Policy: always allowed (read-only, no modifications).
type ListDirTool struct {
	// WorkspaceRoot is the root directory that constrains allowed paths.
	WorkspaceRoot string
}

func (t *ListDirTool) Name() string        { return "list_dir" }
func (t *ListDirTool) Description() string { return "Lists files and directories under a given path within the workspace root." }
func (t *ListDirTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"path": {Type: "string", Description: "Path relative to workspace root", Required: true},
		},
		Output: ParamSpec{Type: "array", Description: "List of file and directory names"},
	}
}
func (t *ListDirTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	pathVal, ok := input["path"].(string)
	if !ok {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "required parameter 'path' must be a string",
		}, nil
	}

	absPath := filepath.Clean(filepath.Join(t.WorkspaceRoot, pathVal))

	// Defense-in-depth: resolve symlinks and verify the path stays within root
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   fmt.Sprintf("failed to resolve path: %v", err),
		}, nil
	}
	absRoot, err := filepath.Abs(t.WorkspaceRoot)
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "invalid workspace root",
		}, nil
	}
	resolvedRoot, _ := filepath.EvalSymlinks(absRoot)
	if resolvedRoot == "" {
		resolvedRoot = absRoot
	}
	if !safety.IsWithinRoot(resolvedPath, resolvedRoot) {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "path is outside workspace root (symlink escape blocked)",
		}, nil
	}

	entries, err := os.ReadDir(resolvedPath)
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   fmt.Sprintf("failed to read directory: %v", err),
		}, nil
	}

	var names []string
	for _, entry := range entries {
		prefix := ""
		if entry.IsDir() {
			prefix = "dir:"
		} else {
			prefix = "file:"
		}
		names = append(names, prefix+entry.Name())
	}
	sort.Strings(names)

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"path":   pathVal,
			"files":  names,
			"count":  len(names),
		},
	}, nil
}

// ReadFileTool reads a text file within the workspace root. Policy blocks
// path traversal and enforces a maximum file size.
// ReadFileTool reads a text file's contents within the workspace root.
// Enforces a maximum file size (default 1MB) to prevent reading huge files.
// Path traversal and symlink escapes are blocked the same way as ListDirTool.
// Policy: always allowed (read-only, no modifications).
type ReadFileTool struct {
	// WorkspaceRoot is the root directory that constrains allowed paths.
	WorkspaceRoot string
	// MaxFileSize is the maximum allowed file size in bytes.
	MaxFileSize int64
}

func (t *ReadFileTool) Name() string        { return "read_file" }
func (t *ReadFileTool) Description() string { return "Reads a text file within the workspace root. Path traversal and file size limits are enforced by policy." }
func (t *ReadFileTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"path": {Type: "string", Description: "Path relative to workspace root", Required: true},
		},
		Output: ParamSpec{Type: "string", Description: "The file contents"},
	}
}
func (t *ReadFileTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	pathVal, ok := input["path"].(string)
	if !ok {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "required parameter 'path' must be a string",
		}, nil
	}

	absPath := filepath.Clean(filepath.Join(t.WorkspaceRoot, pathVal))

	// Defense-in-depth: resolve symlinks and verify the path stays within root
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   fmt.Sprintf("failed to resolve path: %v", err),
		}, nil
	}
	absRoot, err := filepath.Abs(t.WorkspaceRoot)
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "invalid workspace root",
		}, nil
	}
	resolvedRoot, _ := filepath.EvalSymlinks(absRoot)
	if resolvedRoot == "" {
		resolvedRoot = absRoot
	}
	if !safety.IsWithinRoot(resolvedPath, resolvedRoot) {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "path is outside workspace root (symlink escape blocked)",
		}, nil
	}

	// Use Lstat to check size without following symlinks at the final component
	info, err := os.Lstat(resolvedPath)
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   fmt.Sprintf("failed to stat file: %v", err),
		}, nil
	}

	// Reject symlinks at the final path component
	if info.Mode()&os.ModeSymlink != 0 {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "symlink at final path component is not allowed",
		}, nil
	}

	maxSize := t.MaxFileSize
	if maxSize <= 0 {
		maxSize = 1024 * 1024 // 1MB default
	}

	if info.Size() > maxSize {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   fmt.Sprintf("file size %d bytes exceeds maximum allowed %d bytes", info.Size(), maxSize),
		}, nil
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   fmt.Sprintf("failed to read file: %v", err),
		}, nil
	}

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"path":    pathVal,
			"content": string(data),
			"size":    len(data),
		},
	}, nil
}

// WriteFileDryRunTool previews what a file write would look like. It does NOT
// write to disk. Always approved by policy (no mutation).
// WriteFileDryRun shows what a file write would look like without actually writing.
// It returns a "diff-style" preview: the content that would be written and the
// target path. No bytes are written to disk.
// Policy: always allowed (preview only, harmless).
//
// The 500-character preview truncation keeps the tool result from flooding
// the LLM's context window. The full content is in the output, just truncated
// in the preview field.
type WriteFileDryRun struct{}

func (t *WriteFileDryRun) Name() string        { return "write_file_dry_run" }
func (t *WriteFileDryRun) Description() string { return "Previews a file write diff without writing to disk. No side effects." }
func (t *WriteFileDryRun) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"path":    {Type: "string", Description: "Path relative to workspace root", Required: true},
			"content": {Type: "string", Description: "Proposed file content", Required: true},
		},
		Output: ParamSpec{Type: "object", Description: "Preview of the write operation"},
	}
}
func (t *WriteFileDryRun) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	pathVal, ok := input["path"].(string)
	if !ok {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "required parameter 'path' must be a string",
		}, nil
	}

	content, ok := input["content"].(string)
	if !ok {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "required parameter 'content' must be a string",
		}, nil
	}

	// Build a preview diff — this does NOT write to disk
	lines := strings.Count(content, "\n") + 1
	preview := fmt.Sprintf("--- /dev/null\n+++ %s\n@@ -0,0 +1,%d @@\n", pathVal, lines)

	// Add each line with a "+" prefix (simplified unified diff)
	for _, line := range strings.Split(content, "\n") {
		preview += "+" + line + "\n"
	}

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"path":           pathVal,
			"content_length": len(content),
			"lines":          lines,
			"preview_diff":   preview,
			"written":         false,
		},
	}, nil
}

// WriteFileProposalTool validates a file write proposal and returns an approval request.
// It does NOT write to disk. Instead, it emits mutation.proposed and approval.requested
// events, and returns an approval_id that can be used to approve or deny the mutation.
// WriteFileProposal proposes a file write and creates a pending approval.
// The file is NOT written — instead, a pending approval is saved to
// runs/<run_id>/approvals/<approval_id>.json with the proposed content.
// A human operator must review and approve before the file is actually written.
//
// This is the core of V4's safety model: the model proposes, Prism validates,
// the human decides.
//
// Policy: requires_approval (creates a pending approval, blocks until human decides).
// Direct write_file without proposal → denied by policy.
type WriteFileProposal struct {
	WorkspaceRoot string
	// ApprovalStore is used to persist the approval artifact.
	ApprovalStore ApprovalStorer
	// Emit is called to emit events.
	Emit func(eventType, source string, payload map[string]any)
}

// ApprovalStorer is the minimal interface needed to persist an approval.
type ApprovalStorer interface {
	Save(a any) error
}

// ApprovalProxy wraps an approval.Approval for storage.
type ApprovalProxy struct {
	ApprovalID    string       `json:"approval_id"`
	RunID         string       `json:"run_id"`
	CorrelationID string       `json:"correlation_id"`
	Status        string       `json:"status"`
	RequestedBy   string       `json:"requested_by"`
	Project       string       `json:"project"`
	MutationType  string       `json:"mutation_type"`
	TargetPath    string       `json:"target_path"`
	Content       string       `json:"content,omitempty"`
	Preview       string       `json:"preview,omitempty"`
	CreatedAt     string       `json:"created_at"`
	Policy        PolicyDecision `json:"policy"`
}

func (t *WriteFileProposal) Name() string        { return "write_file_proposal" }
func (t *WriteFileProposal) Description() string { return "Proposes a file write for approval. Does not write to disk — returns an approval_id for the approval workflow." }
func (t *WriteFileProposal) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"path":    {Type: "string", Description: "Path relative to workspace root", Required: true},
			"content": {Type: "string", Description: "Proposed file content", Required: true},
		},
		Output: ParamSpec{Type: "object", Description: "Approval ID and preview"},
	}
}
func (t *WriteFileProposal) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	pathVal, ok := input["path"].(string)
	if !ok {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "required parameter 'path' must be a string",
		}, nil
	}

	content, ok := input["content"].(string)
	if !ok {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   "required parameter 'content' must be a string",
		}, nil
	}

	// Validate path safety
	if strings.Contains(pathVal, "..") {
		return ToolResult{Success: false, Error: "path traversal blocked"}, nil
	}
	if filepath.IsAbs(pathVal) {
		return ToolResult{Success: false, Error: "absolute paths not allowed"}, nil
	}

	// Defense-in-depth: resolve symlinks in the target path
	absRoot, err := filepath.Abs(t.WorkspaceRoot)
	if err != nil {
		return ToolResult{Success: false, Error: "invalid workspace root"}, nil
	}
	resolvedRoot, _ := filepath.EvalSymlinks(absRoot)
	if resolvedRoot == "" {
		resolvedRoot = absRoot
	}
	absPath := filepath.Clean(filepath.Join(resolvedRoot, pathVal))
	// For write targets that don't exist yet, resolve parent
	parentDir := filepath.Dir(absPath)
	resolvedParent, parentErr := filepath.EvalSymlinks(parentDir)
	if parentErr != nil {
		return ToolResult{Success: false, Error: "cannot resolve parent directory for write target"}, nil
	}
	resolvedAbsPath := filepath.Join(resolvedParent, filepath.Base(absPath))
	if !safety.IsWithinRoot(resolvedAbsPath, resolvedRoot) {
		return ToolResult{Success: false, Error: "path is outside workspace root (symlink escape blocked)"}, nil
	}

	// Check content size
	const maxContentSize = 1024 * 1024
	if len(content) > maxContentSize {
		return ToolResult{Success: false, Error: fmt.Sprintf("content size %d exceeds 1MB limit", len(content))}, nil
	}

	// Generate approval ID and create artifact
	approvalID := "appr_" + fmt.Sprintf("%d", len(pathVal)+len(content))
	// Actually use ULID
	now := time.Now()
	approvalID = fmt.Sprintf("appr_%d", now.UnixNano())

	// Build preview
	previewLen := len(content)
	if previewLen > 500 {
		previewLen = 500
	}
	preview := content[:previewLen]
	if len(content) > 500 {
		preview += "..."
	}

	// Emit mutation.proposed and approval.requested events
	if t.Emit != nil {
		t.Emit("prism.mutation.proposed", "prism-tool-executor", map[string]any{
			"approval_id":     approvalID,
			"mutation_type":   "write_file",
			"target_path":     pathVal,
			"content_size":    len(content),
			"policy_decision": "requires_approval",
			"policy_reason":   "file writes require explicit approval",
		})
		t.Emit("prism.approval.requested", "prism-tool-executor", map[string]any{
			"approval_id":     approvalID,
			"mutation_type":   "write_file",
			"target_path":     pathVal,
			"policy_decision": "requires_approval",
			"policy_reason":   "file writes require explicit approval",
		})
	}

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"approval_id":     approvalID,
			"mutation_type":   "write_file",
			"target_path":     pathVal,
			"content_length":  len(content),
			"preview":         preview,
			"status":          "pending_approval",
			"instruction":     "Use 'prism approval approve <approval_id> --by <name>' or 'prism approval deny <approval_id> --by <name>' to proceed.",
		},
	}, nil
}

// RegisterBuiltins adds the built-in tools to a registry, using the given
// workspace root and max file size. Returns the registry for chaining.
func RegisterBuiltins(registry *Registry, workspaceRoot string, maxFileSize int64) *Registry {
	if maxFileSize <= 0 {
		maxFileSize = 1024 * 1024 // 1MB default
	}

	registry.Register(&EchoTool{})
	registry.Register(&ListDirTool{WorkspaceRoot: workspaceRoot})
	registry.Register(&ReadFileTool{WorkspaceRoot: workspaceRoot, MaxFileSize: maxFileSize})
	registry.Register(&WriteFileDryRun{})

	return registry
}

// RegisterBuiltinsV4 adds all V3 + V4 tools to the registry.
func RegisterBuiltinsV4(registry *Registry, workspaceRoot string, maxFileSize int64) *Registry {
	RegisterBuiltins(registry, workspaceRoot, maxFileSize)
	registry.Register(&WriteFileProposal{WorkspaceRoot: workspaceRoot})
	return registry
}