package memory

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func tempStore(t *testing.T) *MarkdownStore {
	t.Helper()
	dir := t.TempDir()
	return NewMarkdownStore(dir)
}

func TestStoreAndRetrieve(t *testing.T) {
	s := tempStore(t)

	id, err := s.Store(context.Background(), Memory{
		Content:   "Decided to use local models for memory extraction",
		Category:  "decision",
		Tier:      "active",
		Summary:   "Use local models for memory extraction",
		KeyTopics: []string{"memory", "local-models", "architecture"},
		AgentID:   "lumi",
		Source:    "prizm:lumi",
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty ID")
	}

	got, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected memory, got nil")
	}
	if got.Content != "Decided to use local models for memory extraction" {
		t.Errorf("Content = %q, want matching", got.Content)
	}
	if got.Category != "decision" {
		t.Errorf("Category = %q, want decision", got.Category)
	}
	if got.AgentID != "lumi" {
		t.Errorf("AgentID = %q, want lumi", got.AgentID)
	}
}

func TestSearchByKeyword(t *testing.T) {
	s := tempStore(t)

	_, err := s.Store(context.Background(), Memory{
		Content:   "Decided to use nemotron for memory gate",
		Category:  "decision",
		Tier:      "active",
		Summary:   "Nemotron memory gate",
		KeyTopics: []string{"memory", "nemotron"},
		Source:    "prizm:lumi",
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	_, err = s.Store(context.Background(), Memory{
		Content:   "Prizm uses event-driven architecture",
		Category:  "fact",
		Tier:      "persist",
		Summary:   "Event-driven architecture",
		KeyTopics: []string{"events", "architecture"},
		Source:    "prizm:lumi",
	})
	if err != nil {
		t.Fatalf("Store 2: %v", err)
	}

	results, err := s.Search(context.Background(), "nemotron", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for nemotron search")
	}

	// nemotron should be in top result
	found := false
	for _, r := range results {
		if r.Category == "decision" {
			found = true
		}
	}
	if !found {
		t.Error("nemotron search should find the decision memory")
	}
}

func TestListRecent(t *testing.T) {
	s := tempStore(t)

	for i := 0; i < 5; i++ {
		_, err := s.Store(context.Background(), Memory{
			Content:  "memory item",
			Category: "fact",
			Summary:  "test memory",
		})
		if err != nil {
			t.Fatalf("Store %d: %v", i, err)
		}
	}

	results, err := s.ListRecent(context.Background(), 3)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(results) != 3 {
		t.Errorf("ListRecent returned %d items, want 3", len(results))
	}
}

func TestConcurrentWrites(t *testing.T) {
	s := tempStore(t)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var ids []string

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			id, err := s.Store(context.Background(), Memory{
				Content:  "concurrent memory",
				Category: "fact",
				Summary:  "test",
			})
			if err != nil {
				t.Errorf("Store %d: %v", n, err)
				return
			}
			mu.Lock()
			ids = append(ids, id)
			mu.Unlock()
		}(i)
	}
	wg.Wait()

	if len(ids) != 10 {
		t.Errorf("stored %d memories, want 10", len(ids))
	}
}

func TestGetNotFound(t *testing.T) {
	s := tempStore(t)
	got, err := s.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != nil {
		t.Error("expected nil for nonexistent ID")
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	s := tempStore(t)
	_, err := s.Store(context.Background(), Memory{
		Content:  "test",
		Category: "fact",
		Summary:  "test",
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}

	results, err := s.Search(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Error("empty query should return all memories")
	}
}

func TestParseExistingMarkdown(t *testing.T) {
	dir := t.TempDir()
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0755)

	content := `# 2026-08-10 Memory Log

### 01HXYZABC — Test summary

- **Category:** decision
- **Tier:** active
- **Source:** prizm:lumi
- **Agent:** lumi
- **Key Topics:** memory, test

This is the memory content.
`
	os.WriteFile(filepath.Join(memDir, "2026-08-10.md"), []byte(content), 0644)

	s := NewMarkdownStore(dir)
	got, err := s.Get(context.Background(), "01HXYZABC")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("expected to find existing memory")
	}
	if got.Category != "decision" {
		t.Errorf("Category = %q, want decision", got.Category)
	}
	if got.AgentID != "lumi" {
		t.Errorf("AgentID = %q, want lumi", got.AgentID)
	}
}