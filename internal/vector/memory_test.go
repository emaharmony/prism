package vector

import (
	"context"
	"testing"
)

func TestMemoryVectorStore_UpsertAndGet(t *testing.T) {
	store := NewMemoryVectorStore(3)
	entry := VectorEntry{
		ID:      "v1",
		Content: "hello world",
		Vector:  []float64{1, 0, 0},
		Source:  "event",
	}

	err := store.Upsert(context.Background(), entry)
	if err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	got, err := store.Get(context.Background(), "v1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil {
		t.Fatal("Get() returned nil, want entry")
	}
	if got.Content != "hello world" {
		t.Errorf("Content = %q, want hello world", got.Content)
	}
}

func TestMemoryVectorStore_Upsert_WrongDimension(t *testing.T) {
	store := NewMemoryVectorStore(3)
	entry := VectorEntry{
		ID:      "v1",
		Content: "test",
		Vector:  []float64{1, 0}, // wrong dimension
	}

	err := store.Upsert(context.Background(), entry)
	if err == nil {
		t.Fatal("expected dimension error")
	}
}

func TestMemoryVectorStore_UpsertBatch(t *testing.T) {
	store := NewMemoryVectorStore(3)
	entries := []VectorEntry{
		{ID: "v1", Content: "first", Vector: []float64{1, 0, 0}, Source: "event"},
		{ID: "v2", Content: "second", Vector: []float64{0, 1, 0}, Source: "event"},
		{ID: "v3", Content: "third", Vector: []float64{0, 0, 1}, Source: "run_summary"},
	}

	err := store.UpsertBatch(context.Background(), entries)
	if err != nil {
		t.Fatalf("UpsertBatch() error = %v", err)
	}
	if store.Len() != 3 {
		t.Errorf("Len() = %d, want 3", store.Len())
	}
}

func TestMemoryVectorStore_Search_TopK(t *testing.T) {
	store := NewMemoryVectorStore(3)
	store.UpsertBatch(context.Background(), []VectorEntry{
		{ID: "v1", Content: "first", Vector: []float64{1, 0, 0}, Source: "event"},
		{ID: "v2", Content: "second", Vector: []float64{0, 1, 0}, Source: "event"},
		{ID: "v3", Content: "third", Vector: []float64{0.9, 0.1, 0}, Source: "event"},
	})

	results, err := store.Search(context.Background(), []float64{1, 0, 0}, SearchOptions{TopK: 2})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) > 2 {
		t.Errorf("Search() returned %d results, want at most 2", len(results))
	}
	// First result should be v1 (exact match, similarity = 1.0)
	if len(results) > 0 && results[0].Entry.ID != "v1" {
		t.Errorf("First result ID = %q, want v1", results[0].Entry.ID)
	}
}

func TestMemoryVectorStore_Search_MinScore(t *testing.T) {
	store := NewMemoryVectorStore(3)
	store.UpsertBatch(context.Background(), []VectorEntry{
		{ID: "v1", Content: "first", Vector: []float64{1, 0, 0}},
		{ID: "v2", Content: "second", Vector: []float64{0, 1, 0}},
	})

	// MinScore=0.99 should only return exact or near-exact matches
	results, err := store.Search(context.Background(), []float64{1, 0, 0}, SearchOptions{TopK: 10, MinScore: 0.99})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Errorf("Search() returned %d results, want 1 (only exact match)", len(results))
	}
}

func TestMemoryVectorStore_Search_SourceFilter(t *testing.T) {
	store := NewMemoryVectorStore(3)
	store.UpsertBatch(context.Background(), []VectorEntry{
		{ID: "v1", Content: "first", Vector: []float64{1, 0, 0}, Source: "event"},
		{ID: "v2", Content: "second", Vector: []float64{0.9, 0.1, 0}, Source: "run_summary"},
	})

	results, err := store.Search(context.Background(), []float64{1, 0, 0}, SearchOptions{
		TopK:         10,
		MinScore:     0.5,
		SourceFilter: "event",
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	for _, r := range results {
		if r.Entry.Source != "event" {
			t.Errorf("Found source %q, want event only", r.Entry.Source)
		}
	}
}

func TestMemoryVectorStore_Delete_ByID(t *testing.T) {
	store := NewMemoryVectorStore(3)
	store.UpsertBatch(context.Background(), []VectorEntry{
		{ID: "v1", Content: "first", Vector: []float64{1, 0, 0}},
		{ID: "v2", Content: "second", Vector: []float64{0, 1, 0}},
	})

	err := store.Delete(context.Background(), VectorFilter{IDs: []string{"v1"}})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if store.Len() != 1 {
		t.Errorf("Len() = %d, want 1 after delete", store.Len())
	}
}

func TestMemoryVectorStore_Delete_BySource(t *testing.T) {
	store := NewMemoryVectorStore(3)
	store.UpsertBatch(context.Background(), []VectorEntry{
		{ID: "v1", Content: "first", Vector: []float64{1, 0, 0}, Source: "event"},
		{ID: "v2", Content: "second", Vector: []float64{0, 1, 0}, Source: "run_summary"},
		{ID: "v3", Content: "third", Vector: []float64{0, 0, 1}, Source: "event"},
	})

	err := store.Delete(context.Background(), VectorFilter{Source: "event"})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if store.Len() != 1 {
		t.Errorf("Len() = %d, want 1 after delete by source", store.Len())
	}
}

func TestMemoryVectorStore_Delete_All(t *testing.T) {
	store := NewMemoryVectorStore(3)
	store.UpsertBatch(context.Background(), []VectorEntry{
		{ID: "v1", Content: "first", Vector: []float64{1, 0, 0}},
		{ID: "v2", Content: "second", Vector: []float64{0, 1, 0}},
	})

	err := store.Delete(context.Background(), VectorFilter{})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if store.Len() != 0 {
		t.Errorf("Len() = %d, want 0 after delete all", store.Len())
	}
}

func TestMemoryVectorStore_Get_NotFound(t *testing.T) {
	store := NewMemoryVectorStore(3)
	got, err := store.Get(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got != nil {
		t.Error("Get() should return nil for nonexistent ID")
	}
}

func TestMemoryVectorStore_Update(t *testing.T) {
	store := NewMemoryVectorStore(3)
	entry := VectorEntry{
		ID:      "v1",
		Content: "original",
		Vector:  []float64{1, 0, 0},
	}
	store.Upsert(context.Background(), entry)

	// Update with new content
	entry.Content = "updated"
	store.Upsert(context.Background(), entry)

	got, _ := store.Get(context.Background(), "v1")
	if got.Content != "updated" {
		t.Errorf("Content = %q, want updated", got.Content)
	}
	if store.Len() != 1 {
		t.Errorf("Len() = %d, want 1 after update", store.Len())
	}
}

func TestMemoryVectorStore_Close(t *testing.T) {
	store := NewMemoryVectorStore(3)
	if err := store.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}