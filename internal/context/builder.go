// Package context provides workspace context injection for Prism runs.
// It reads OpenClaw workspace files, applies selection rules, respects token budgets,
// and produces formatted context strings for LLM prompts.
//
// V19 introduces smart context injection as a pipeline stage:
//
//	Connection → Context → Remembrance → LLM → Tool → Approval → Validation → Review
//
// Context is read-only — Prism never writes back to the workspace.
package context

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// NamedSource maps short names to workspace files.
var NamedSources = map[string]string{
	"soul":      "SOUL.md",
	"agents":    "AGENTS.md",
	"user":      "USER.md",
	"heartbeat": "HEARTBEAT.md",
	"memory":    "MEMORY.md",
	"identity":  "IDENTITY.md",
}

// SourcePriority controls truncation order. Lower = truncated first.
var SourcePriority = map[string]int{
	"soul":      100, // Never truncate
	"agents":    80,
	"user":      80,
	"heartbeat": 50,
	"memory":    50,
}

// ContextFile represents a workspace file that has been read and processed.
type ContextFile struct {
	Name            string // Short name (e.g., "soul")
	Path            string // Full file path
	Content         string // Raw file content
	SizeBytes       int    // File size in bytes
	EstimatedTokens int    // Rough token estimate (content / 4)
	Priority        int    // Truncation priority (higher = kept longer)
	Source          string // "named", "auto", or "file"
	Truncated       bool   // Whether content was truncated
	TruncatedBy     int    // Number of tokens removed by truncation
}

// InjectedContext is the result of context injection.
type InjectedContext struct {
	Files           []ContextFile
	TotalTokens     int
	Truncated       bool
	ContentHash     string // SHA-256 of raw concatenated content
	FormattedString string // The formatted context string for LLM prompt
}

// Builder constructs injected context from workspace files.
type Builder struct {
	WorkspaceRoot string
	NamedContexts []string // e.g., ["soul", "agents"]
	AutoFiles     []string // Files discovered by keyword matching
	ExplicitFiles []string // Files specified via --context-file
	TokenBudget   int      // Max tokens to inject (0 = no limit)

	// Cache for BuildCached(). Invalidated when workspace files change.
	cache   *cachedContext
	cacheMu sync.Mutex
}

// cachedContext holds a previously-built InjectedContext along with file
// mtimes and sizes. BuildCached() checks these to determine if a rebuild
// is needed. This eliminates redundant file reads when workspace files
// haven't changed between messages.
type cachedContext struct {
	result   *InjectedContext
	fileInfo map[string]os.FileInfo // path -> Stat result
}

// NewBuilder creates a context builder with the given workspace root.
func NewBuilder(workspaceRoot string) *Builder {
	return &Builder{
		WorkspaceRoot: workspaceRoot,
		TokenBudget:   0, // No limit by default
	}
}

// WithNamedContexts sets which named context sources to include.
func (b *Builder) WithNamedContexts(names []string) *Builder {
	b.NamedContexts = names
	return b
}

// WithAutoFiles sets keyword-matched files.
func (b *Builder) WithAutoFiles(files []string) *Builder {
	b.AutoFiles = files
	return b
}

// WithExplicitFiles sets explicitly-specified files.
func (b *Builder) WithExplicitFiles(files []string) *Builder {
	b.ExplicitFiles = files
	return b
}

// WithTokenBudget sets the maximum number of tokens to inject.
func (b *Builder) WithTokenBudget(budget int) *Builder {
	b.TokenBudget = budget
	return b
}

// Build reads workspace files and constructs the injected context.
func (b *Builder) Build() (*InjectedContext, error) {
	var files []ContextFile

	// 1. Read named sources (highest priority)
	for _, name := range b.NamedContexts {
		filename, ok := NamedSources[name]
		if !ok {
			continue
		}
		cf, err := b.readFile(name, filename, "named", SourcePriority[name])
		if err != nil {
			continue // Skip missing files
		}
		files = append(files, cf)
	}

	// 2. Read auto-discovered files (lowest priority)
	for _, path := range b.AutoFiles {
		name := filepath.Base(path)
		cf, err := b.readFile(name, path, "auto", 30)
		if err != nil {
			continue
		}
		files = append(files, cf)
	}

	// 3. Read explicit files (medium priority, after named but before auto)
	for _, path := range b.ExplicitFiles {
		name := filepath.Base(path)
		cf, err := b.readFile(name, path, "file", 60)
		if err != nil {
			continue
		}
		files = append(files, cf)
	}

	// 4. Compute content hash (before truncation)
	rawContent := ""
	for _, f := range files {
		rawContent += f.Content
	}
	hash := sha256.Sum256([]byte(rawContent))

	// 5. Apply token budget
	if b.TokenBudget > 0 {
		files = b.applyBudget(files)
	}

	// 6. Format context string
	formatted := b.formatContext(files)

	totalTokens := 0
	truncated := false
	for _, f := range files {
		totalTokens += f.EstimatedTokens
		if f.Truncated {
			truncated = true
		}
	}

	return &InjectedContext{
		Files:           files,
		TotalTokens:     totalTokens,
		Truncated:       truncated,
		ContentHash:     fmt.Sprintf("%x", hash),
		FormattedString: formatted,
	}, nil
}

// BuildCached is a cached version of Build. It returns the cached result
// if none of the workspace files have changed (checked via os.Stat mtime+size).
// If any file has changed or this is the first call, it delegates to Build()
// and caches the result. This is the preferred method for hot-path calls
// like per-message context injection where workspace files rarely change.
func (b *Builder) BuildCached() (*InjectedContext, error) {
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()

	// Collect all file paths that would be read
	filePath := func(name, filename, source string) string {
		if !filepath.IsAbs(filename) && source == "named" {
			return filepath.Join(b.WorkspaceRoot, filename)
		}
		return filename
	}

	needRebuild := b.cache == nil

	// Build the set of expected file paths
	expectedPaths := make(map[string]bool)
	for _, name := range b.NamedContexts {
		if filename, ok := NamedSources[name]; ok {
			p := filePath(name, filename, "named")
			expectedPaths[p] = true
		}
	}
	for _, path := range b.AutoFiles {
		expectedPaths[path] = true
	}
	for _, path := range b.ExplicitFiles {
		expectedPaths[path] = true
	}

	if b.cache != nil {
		// Check if any file changed or was added/removed
		for p, oldInfo := range b.cache.fileInfo {
			if !expectedPaths[p] {
				// File was removed from the expected set
				needRebuild = true
				break
			}
			newInfo, err := os.Stat(p)
			if err != nil {
				// File no longer exists
				needRebuild = true
				break
			}
			if newInfo.Size() != oldInfo.Size() || !newInfo.ModTime().Equal(oldInfo.ModTime()) {
				needRebuild = true
				break
			}
		}
		// Check for new files not in cache
		if !needRebuild {
			for p := range expectedPaths {
				if _, ok := b.cache.fileInfo[p]; !ok {
					needRebuild = true
					break
				}
			}
		}
	}

	if needRebuild {
		result, err := b.Build()
		if err != nil {
			return nil, err
		}

		// Collect current file mtimes/sizes for next call
		fileInfo := make(map[string]os.FileInfo)
		for p := range expectedPaths {
			if info, err := os.Stat(p); err == nil {
				fileInfo[p] = info
			}
		}

		b.cache = &cachedContext{
			result:   result,
			fileInfo: fileInfo,
		}
	}

	return b.cache.result, nil
}

// readFile reads a single file from the workspace.
func (b *Builder) readFile(name, filename, source string, priority int) (ContextFile, error) {
	// Resolve path: named sources are relative to workspace root
	fullPath := filename
	if !filepath.IsAbs(filename) && source == "named" {
		fullPath = filepath.Join(b.WorkspaceRoot, filename)
	}

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ContextFile{}, fmt.Errorf("read %s: %w", fullPath, err)
	}

	content := string(data)
	tokens := estimateTokens(content)

	return ContextFile{
		Name:            name,
		Path:            fullPath,
		Content:         content,
		SizeBytes:       len(data),
		EstimatedTokens: tokens,
		Priority:        priority,
		Source:          source,
	}, nil
}

// applyBudget truncates files to fit within the token budget.
// Truncation priority: file (60) > auto (30) > named sources (50-100).
// Soul (100) is never truncated.
func (b *Builder) applyBudget(files []ContextFile) []ContextFile {
	// Calculate total tokens
	totalTokens := 0
	for _, f := range files {
		totalTokens += f.EstimatedTokens
	}

	if totalTokens <= b.TokenBudget {
		return files // Fits within budget
	}

	// Sort by priority ascending (truncate lowest priority first)
	sorted := make([]ContextFile, len(files))
	copy(sorted, files)

	// Simple insertion sort by priority ascending
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j].Priority < sorted[j-1].Priority; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}

	// Truncate from lowest priority until we fit
	overage := totalTokens - b.TokenBudget
	for i := range sorted {
		if overage <= 0 {
			break
		}

		// Never truncate soul (priority 100)
		if sorted[i].Name == "soul" {
			continue
		}

		// Truncate a file only if it actually has tokens AND it is either a
		// non-named source or a low-priority (<80) named source. High-priority
		// named sources (soul, agents, …) are preserved. Parenthesised to match
		// this documented intent rather than rely on operator precedence.
		if sorted[i].EstimatedTokens > 0 && (sorted[i].Source != "named" || sorted[i].Priority < 80) {
			// Truncate this file
			keepTokens := sorted[i].EstimatedTokens - overage
			if keepTokens < 0 {
				keepTokens = 0
			}

			if keepTokens == 0 {
				// Remove entirely
				overage -= sorted[i].EstimatedTokens
				sorted[i].Content = ""
				sorted[i].Truncated = true
				sorted[i].TruncatedBy = sorted[i].EstimatedTokens
				sorted[i].EstimatedTokens = 0
			} else {
				// Truncate content
				keepChars := keepTokens * 4 // Rough estimate
				originalTokens := sorted[i].EstimatedTokens
				if keepChars < len(sorted[i].Content) {
					sorted[i].Content = sorted[i].Content[:keepChars] + "\n\n[... truncated: " + fmt.Sprintf("%d", originalTokens-keepTokens) + " tokens omitted ...]\n"
					sorted[i].Truncated = true
					sorted[i].TruncatedBy = originalTokens - keepTokens
					sorted[i].EstimatedTokens = keepTokens
				}
				overage -= (originalTokens - keepTokens)
			}
		}
	}

	return sorted
}

// formatContext produces the formatted context string for LLM injection.
func (b *Builder) formatContext(files []ContextFile) string {
	var sb strings.Builder

	for _, f := range files {
		if f.Content == "" {
			continue
		}

		sb.WriteString(fmt.Sprintf("## Workspace: %s\n", f.Name))
		if f.Truncated {
			sb.WriteString(fmt.Sprintf("[Truncated: %d tokens omitted]\n\n", f.TruncatedBy))
		}
		sb.WriteString(f.Content)
		sb.WriteString("\n\n")
	}

	return sb.String()
}

// estimateTokens provides a rough token estimate.
// English text averages ~4 characters per token.
func estimateTokens(content string) int {
	return len(content) / 4
}

// DiscoverFiles searches docs/ for files matching keywords from a task description.
// Returns ranked file paths sorted by relevance (hit count descending).
func DiscoverFiles(workspaceRoot, task string) []string {
	keywords := extractKeywords(task)
	docsDir := filepath.Join(workspaceRoot, "docs")

	// Check if docs/ exists
	if _, err := os.Stat(docsDir); os.IsNotExist(err) {
		return nil
	}

	type fileScore struct {
		path  string
		score int
	}

	var scored []fileScore

	// Walk docs/ directory
	filepath.Walk(docsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content := strings.ToLower(string(data))
		filename := strings.ToLower(filepath.Base(path))

		score := 0
		for _, kw := range keywords {
			lkw := strings.ToLower(kw)
			// Filename match counts more
			if strings.Contains(filename, lkw) {
				score += 3
			}
			// Content match
			score += strings.Count(content, lkw)
		}

		if score > 0 {
			scored = append(scored, fileScore{path: path, score: score})
		}
		return nil
	})

	// Sort by score descending
	for i := 1; i < len(scored); i++ {
		for j := i; j > 0 && scored[j].score > scored[j-1].score; j-- {
			scored[j], scored[j-1] = scored[j-1], scored[j]
		}
	}

	var result []string
	for _, s := range scored {
		result = append(result, s.path)
	}
	return result
}

// extractKeywords splits a task description into search keywords.
// Removes common English stopwords.
func extractKeywords(task string) []string {
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true, "but": true,
		"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
		"with": true, "by": true, "from": true, "is": true, "it": true, "that": true,
		"this": true, "was": true, "are": true, "be": true, "has": true, "had": true,
		"have": true, "will": true, "would": true, "could": true, "should": true,
		"can": true, "do": true, "did": true, "not": true, "no": true, "fix": true,
		"implement": true, "create": true, "remove": true, "delete": true,
	}

	words := strings.Fields(strings.ToLower(task))
	var keywords []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?()[]{}\"'")
		if len(w) > 2 && !stopwords[w] {
			keywords = append(keywords, w)
		}
	}
	return keywords
}

// AvailableNamedContexts returns the list of available context source names.
func AvailableNamedContexts() []string {
	names := make([]string, 0, len(NamedSources))
	for name := range NamedSources {
		names = append(names, name)
	}
	return names
}
