package tool

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// EchoTool returns whatever text is passed in. Always approved by policy.
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

	entries, err := os.ReadDir(absPath)
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

	// Check file size before reading
	info, err := os.Stat(absPath)
	if err != nil {
		return ToolResult{
			Success: false,
			Output:  nil,
			Error:   fmt.Sprintf("failed to stat file: %v", err),
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

	data, err := os.ReadFile(absPath)
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

// RegisterBuiltins adds the four built-in tools to a registry, using the given
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