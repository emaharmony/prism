package context

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuilder_NamedContext(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("# SOUL\nYou are Lumi, a helpful AI."), 0644)
	os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("# AGENTS\nLumi is the lead developer."), 0644)

	builder := NewBuilder(tmpDir).WithNamedContexts([]string{"soul", "agents"})
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.Files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(result.Files))
	}
	if result.TotalTokens == 0 {
		t.Error("expected non-zero total tokens")
	}
	if result.FormattedString == "" {
		t.Error("expected non-empty formatted string")
	}
	if !strings.Contains(result.FormattedString, "## Workspace: soul") {
		t.Error("expected soul section in formatted string")
	}
}

func TestBuilder_MissingNamedContext(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("soul content"), 0644)
	// AGENTS.md doesn't exist

	builder := NewBuilder(tmpDir).WithNamedContexts([]string{"soul", "agents"})
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Missing files should be skipped, not error
	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file (agents missing), got %d", len(result.Files))
	}
}

func TestBuilder_TokenBudget(t *testing.T) {
	tmpDir := t.TempDir()
	// Write a large file
	largeContent := strings.Repeat("This is test content for budget testing. ", 500) // ~10KB, ~2500 tokens
	os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte(largeContent), 0644)
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("You are Lumi."), 0644)

	// Budget of 100 tokens should truncate MEMORY.md
	builder := NewBuilder(tmpDir).WithNamedContexts([]string{"soul", "memory"}).WithTokenBudget(100)
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Soul should not be truncated
	soulTruncated := false
	for _, f := range result.Files {
		if f.Name == "soul" && f.Truncated {
			soulTruncated = true
		}
	}
	if soulTruncated {
		t.Error("soul should never be truncated")
	}

	// Memory should be truncated
	memoryTruncated := false
	for _, f := range result.Files {
		if f.Name == "memory" && f.Truncated {
			memoryTruncated = true
		}
	}
	if !memoryTruncated {
		t.Error("memory should be truncated due to budget")
	}
}

func TestBuilder_SoulNeverTruncated(t *testing.T) {
	tmpDir := t.TempDir()
	largeContent := strings.Repeat("soul content ", 10000) // very large
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte(largeContent), 0644)

	builder := NewBuilder(tmpDir).WithNamedContexts([]string{"soul"}).WithTokenBudget(50)
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, f := range result.Files {
		if f.Name == "soul" && f.Truncated {
			t.Error("soul should never be truncated, even when over budget")
		}
	}
}

func TestBuilder_ContentHash(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("soul content"), 0644)

	builder := NewBuilder(tmpDir).WithNamedContexts([]string{"soul"})
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.ContentHash == "" {
		t.Error("expected non-empty content hash")
	}
	if len(result.ContentHash) != 64 { // SHA-256 hex
		t.Errorf("expected 64-char hash, got %d chars", len(result.ContentHash))
	}
}

func TestBuilder_ExplicitFiles(t *testing.T) {
	tmpDir := t.TempDir()
	docsDir := filepath.Join(tmpDir, "docs")
	os.MkdirAll(docsDir, 0755)
	os.WriteFile(filepath.Join(docsDir, "design.md"), []byte("# Design\nArchitecture doc."), 0644)

	builder := NewBuilder(tmpDir).WithExplicitFiles([]string{filepath.Join(docsDir, "design.md")})
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.Files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(result.Files))
	}
	if result.Files[0].Source != "file" {
		t.Errorf("expected source 'file', got %s", result.Files[0].Source)
	}
}

func TestEstimateTokens(t *testing.T) {
	// 400 characters should be ~100 tokens
	tokens := estimateTokens(strings.Repeat("a", 400))
	if tokens != 100 {
		t.Errorf("expected 100 tokens for 400 chars, got %d", tokens)
	}
}

func TestDiscoverFiles(t *testing.T) {
	tmpDir := t.TempDir()
	docsDir := filepath.Join(tmpDir, "docs")
	os.MkdirAll(docsDir, 0755)
	os.WriteFile(filepath.Join(docsDir, "approval-design.md"), []byte("This document describes the approval engine and mutation gates."), 0644)
	os.WriteFile(filepath.Join(docsDir, "vector-search.md"), []byte("Vector search uses HNSW for fast nearest neighbor queries."), 0644)
	os.WriteFile(filepath.Join(docsDir, "unrelated.md"), []byte("This file has nothing relevant."), 0644)

	results := DiscoverFiles(tmpDir, "fix the approval bug")
	if len(results) == 0 {
		t.Fatal("expected at least one discovered file")
	}

	// approval-design.md should rank first (filename + content match)
	if filepath.Base(results[0]) != "approval-design.md" {
		t.Errorf("expected approval-design.md to rank first, got %s", filepath.Base(results[0]))
	}
}

func TestDiscoverFiles_NoDocsDir(t *testing.T) {
	tmpDir := t.TempDir()
	results := DiscoverFiles(tmpDir, "fix something")
	if results != nil {
		t.Errorf("expected nil results when docs/ doesn't exist, got %v", results)
	}
}

func TestExtractKeywords(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"fix the approval bug", []string{"approval", "bug"}},
		{"implement vector search", []string{"vector", "search"}},
		{"update the event store", []string{"event", "store"}},
	}

	for _, tt := range tests {
		keywords := extractKeywords(tt.input)
		// Check that expected keywords are present
		for _, exp := range tt.expected {
			found := false
			for _, kw := range keywords {
				if kw == exp {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected keyword %q in %v for input %q", exp, keywords, tt.input)
			}
		}
	}
}

func TestAvailableNamedContexts(t *testing.T) {
	contexts := AvailableNamedContexts()
	if len(contexts) != len(NamedSources) {
		t.Errorf("expected %d contexts, got %d", len(NamedSources), len(contexts))
	}
}

func TestBuilder_NoBudget(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("soul content"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "AGENTS.md"), []byte("agents content"), 0644)

	builder := NewBuilder(tmpDir).WithNamedContexts([]string{"soul", "agents"})
	// No token budget set — should inject everything
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	for _, f := range result.Files {
		if f.Truncated {
			t.Errorf("file %s should not be truncated without budget", f.Name)
		}
	}
}

func TestBuilder_FormattedString(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("You are Lumi."), 0644)

	builder := NewBuilder(tmpDir).WithNamedContexts([]string{"soul"})
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(result.FormattedString, "## Workspace: soul") {
		t.Error("expected '## Workspace: soul' header in formatted string")
	}
	if !strings.Contains(result.FormattedString, "You are Lumi.") {
		t.Error("expected soul content in formatted string")
	}
}

func TestBuilder_TruncationMessage(t *testing.T) {
	tmpDir := t.TempDir()
	largeContent := strings.Repeat("memory content ", 1000)
	os.WriteFile(filepath.Join(tmpDir, "MEMORY.md"), []byte(largeContent), 0644)
	os.WriteFile(filepath.Join(tmpDir, "SOUL.md"), []byte("soul"), 0644)

	builder := NewBuilder(tmpDir).WithNamedContexts([]string{"soul", "memory"}).WithTokenBudget(20)
	result, err := builder.Build()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Check that truncation message appears in formatted string
	if result.Truncated {
		foundTruncationMsg := false
		for _, f := range result.Files {
			if f.Truncated && strings.Contains(f.Content, "truncated") {
				foundTruncationMsg = true
			}
		}
		if !foundTruncationMsg {
			t.Error("expected truncation message in truncated file content")
		}
	}
}
