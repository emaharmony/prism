// Package tool provides project-level tools for reading and searching codebases.
package tool

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// SearchFilesTool searches for a text pattern across project files, like grep.
// Returns matching lines with file paths and line numbers.
// Policy: always allowed (read-only, no modifications).
type SearchFilesTool struct {
	WorkspaceRoot string
	AllowedPaths  []string
}

func (t *SearchFilesTool) Name() string        { return "search_files" }
func (t *SearchFilesTool) Description() string {
	return "Searches for a text pattern across project files (like grep). Returns matching lines with file paths and line numbers. Use this to find where functions, types, or patterns are defined or used."
}
func (t *SearchFilesTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"pattern":    {Type: "string", Description: "The text pattern to search for", Required: true},
			"path":       {Type: "string", Description: "Directory to search in, relative to workspace root (default: '.')", Required: false},
			"max_results": {Type: "integer", Description: "Maximum number of matching lines to return (default: 30)", Required: false},
		},
		Output: ParamSpec{Type: "array", Description: "List of matches with file, line number, and text"},
	}
}

func (t *SearchFilesTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	pattern, ok := input["pattern"].(string)
	if !ok {
		return ToolResult{Success: false, Error: "required parameter 'pattern' must be a string"}, nil
	}
	if pattern == "" {
		return ToolResult{Success: false, Error: "pattern must not be empty"}, nil
	}

	searchDir := "."
	if dir, ok := input["path"].(string); ok && dir != "" {
		searchDir = dir
	}

	maxResults := 30
	if mr, ok := input["max_results"].(float64); ok && mr > 0 {
		maxResults = int(mr)
	}

	absRoot, err := filepath.Abs(t.WorkspaceRoot)
	if err != nil {
		return ToolResult{Success: false, Error: "invalid workspace root"}, nil
	}
	resolvedRoot, _ := filepath.EvalSymlinks(absRoot)
	if resolvedRoot == "" {
		resolvedRoot = absRoot
	}

	absSearchDir := filepath.Clean(filepath.Join(resolvedRoot, searchDir))
	if !isWithinRoot(absSearchDir, resolvedRoot) {
		return ToolResult{Success: false, Error: "path is outside workspace root"}, nil
	}

	var matches []map[string]any
	var totalMatches int

	err = filepath.Walk(absSearchDir, func(walkPath string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		ext := strings.ToLower(filepath.Ext(walkPath))
		if skipExtensions[ext] {
			return nil
		}
		if info.Size() > 10*1024*1024 { // skip files > 10MB
			return nil
		}

		f, err := os.Open(walkPath)
		if err != nil {
			return nil
		}
		defer f.Close()

		relPath, _ := filepath.Rel(resolvedRoot, walkPath)
		scanner := bufio.NewScanner(f)
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if strings.Contains(scanner.Text(), pattern) {
				totalMatches++
				if len(matches) < maxResults {
					lineText := scanner.Text()
					if len(lineText) > 200 {
						lineText = lineText[:200] + "..."
					}
					matches = append(matches, map[string]any{
						"file":  relPath,
						"line":  lineNum,
						"text":  lineText,
					})
				}
			}
		}
		return nil
	})
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("search failed: %v", err)}, nil
	}

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"pattern":       pattern,
			"matches":       matches,
			"match_count":   len(matches),
			"total_matches": totalMatches,
			"truncated":     totalMatches > maxResults,
		},
	}, nil
}

// ProjectOverviewTool reads key project metadata files and builds a directory
// tree summary. This gives a quick understanding of a project without reading
// every file — it targets README, package manifests, config files, and the
// directory structure.
// Policy: always allowed (read-only, no modifications).
type ProjectOverviewTool struct {
	WorkspaceRoot string
	AllowedPaths  []string
}

func (t *ProjectOverviewTool) Name() string        { return "project_overview" }
func (t *ProjectOverviewTool) Description() string {
	return "Provides a quick overview of a project: reads README, package manifest, config files, and builds a directory tree. Use this first to understand a project's structure before diving into specific files."
}
func (t *ProjectOverviewTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"path": {Type: "string", Description: "Project root relative to workspace root (default: '.')", Required: false},
		},
		Output: ParamSpec{Type: "object", Description: "Project overview with key files and directory tree"},
	}
}

// keyFiles are filenames that are especially useful for understanding a project.
var keyFiles = map[string]bool{
	"README.md": true, "README.rst": true, "README.txt": true, "README": true,
	"go.mod": true, "go.sum": true, "package.json": true, "Cargo.toml": true,
	"pyproject.toml": true, "requirements.txt": true, "Gemfile": true,
	"pom.xml": true, "build.gradle": true, "CMakeLists.txt": true,
	"Makefile": true, "Dockerfile": true, "docker-compose.yaml": true, "docker-compose.yml": true,
	".env.example": true, "prism.yaml": true, "prism.yaml.example": true,
	"tsconfig.json": true, "next.config.js": true, "next.config.mjs": true,
	"vite.config.ts": true, "webpack.config.js": true,
}

func (t *ProjectOverviewTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	projectPath := "."
	if p, ok := input["path"].(string); ok && p != "" {
		projectPath = p
	}

	absRoot, err := filepath.Abs(t.WorkspaceRoot)
	if err != nil {
		return ToolResult{Success: false, Error: "invalid workspace root"}, nil
	}
	resolvedRoot, _ := filepath.EvalSymlinks(absRoot)
	if resolvedRoot == "" {
		resolvedRoot = absRoot
	}

	absProjectDir := filepath.Clean(filepath.Join(resolvedRoot, projectPath))
	if !isWithinRoot(absProjectDir, resolvedRoot) {
		return ToolResult{Success: false, Error: "path is outside workspace root"}, nil
	}

	// Read key files
	var keyFileContents []map[string]any
	for filename := range keyFiles {
		fpath := filepath.Join(absProjectDir, filename)
		data, err := os.ReadFile(fpath)
		if err != nil {
			continue // file doesn't exist, skip
		}
		content := string(data)
		if len(content) > 5000 { // cap key file reads at 5KB
			content = content[:5000] + "\n... (truncated)"
		}
		keyFileContents = append(keyFileContents, map[string]any{
			"file":    filename,
			"content": content,
			"size":    len(data),
		})
	}
	sort.Slice(keyFileContents, func(i, j int) bool {
		return keyFileContents[i]["file"].(string) < keyFileContents[j]["file"].(string)
	})

	// Build directory tree (2 levels deep for overview)
	tree := buildDirectoryTree(absProjectDir, resolvedRoot, 2)

	return ToolResult{
		Success: true,
		Output: map[string]any{
			"path":              projectPath,
			"key_files":         keyFileContents,
			"directory_tree":    tree,
		},
	}, nil
}

// buildDirectoryTree creates a compact directory tree representation.
func buildDirectoryTree(root, workspaceRoot string, maxDepth int) []map[string]any {
	var entries []map[string]any

	items, err := os.ReadDir(root)
	if err != nil {
		return entries
	}

	sort.Slice(items, func(i, j int) bool {
		// Directories first, then files
		if items[i].IsDir() != items[j].IsDir() {
			return items[i].IsDir()
		}
		return items[i].Name() < items[j].Name()
	})

	for _, item := range items {
		name := item.Name()
		if skipDirs[name] {
			continue
		}

		entry := map[string]any{
			"name": name,
			"type": "file",
		}
		if item.IsDir() {
			entry["type"] = "dir"
			if maxDepth > 1 {
				subPath := filepath.Join(root, name)
				entry["children"] = buildDirectoryTree(subPath, workspaceRoot, maxDepth-1)
			}
		}
		entries = append(entries, entry)
	}

	return entries
}

// isWithinRoot checks if a path is within the workspace root.
// Reuses the safety package logic.
func isWithinRoot(path, root string) bool {
	// Use filepath.Rel — if the relative path tries to escape, it will start with ".."
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, "..") && rel != "."
}

// runGitCommand is a helper for git tools to execute git commands safely.
func runGitCommand(workspaceRoot string, args ...string) (string, int, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workspaceRoot
	out, err := cmd.CombinedOutput()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return "", -1, fmt.Errorf("git command failed: %v", err)
		}
	}
	return strings.TrimSpace(string(out)), exitCode, nil
}