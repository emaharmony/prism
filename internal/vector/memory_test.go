package vector

import (
	"context"
	"fmt"
	"sync"
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
	de, ok := err.(*DimensionError)
	if !ok {
		t.Errorf("expected *DimensionError, got %T", err)
	} else {
		if de.Expected != 3 || de.Got != 2 {
			t.Errorf("DimensionError: expected %d got %d, but got Expected=%d Got=%d", 3, 2, de.Expected, de.Got)
		}
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

func TestMemoryVectorStore_Search_OrderByScore(t *testing.T) {
	store := NewMemoryVectorStore(3)
	store.UpsertBatch(context.Background(), []VectorEntry{
		{ID: "v1", Content: "exact", Vector: []float64{1, 0, 0}},
		{ID: "v2", Content: "mid", Vector: []float64{0.7, 0.7, 0}},
		{ID: "v3", Content: "orthogonal", Vector: []float64{0, 0, 1}},
	})

	results, err := store.Search(context.Background(), []float64{1, 0, 0}, SearchOptions{TopK: 10, MinScore: 0.0})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	// Results should be ordered by descending score
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("Results not ordered by descending score: result[%d].Score=%.4f > result[%d].Score=%.4f",
				i-1, results[i-1].Score, i, results[i].Score)
		}
	}
}

func TestMemoryVectorStore_Search_EmptyResults(t *testing.T) {
	store := NewMemoryVectorStore(3)

	results, err := store.Search(context.Background(), []float64{1, 0, 0}, SearchOptions{TopK: 10})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search() on empty store returned %d results, want 0", len(results))
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

func TestMemoryVectorStore_Search_AllFilteredOut(t *testing.T) {
	store := NewMemoryVectorStore(3)
	store.UpsertBatch(context.Background(), []VectorEntry{
		{ID: "v1", Content: "first", Vector: []float64{1, 0, 0}, Source: "event"},
	})

	results, err := store.Search(context.Background(), []float64{1, 0, 0}, SearchOptions{
		TopK:         10,
		MinScore:     0.5,
		SourceFilter: "artifact", // no artifacts in store
	})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Search() returned %d results, want 0 (all filtered)", len(results))
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

func TestMemoryVectorStore_ConcurrentAccess(t *testing.T) {
	store := NewMemoryVectorStore(3)
	var wg sync.WaitGroup
	const numOps = 100

	// Concurrent upserts
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			entry := VectorEntry{
				ID:      fmt.Sprintf("v%d", idx),
				Content: fmt.Sprintf("entry %d", idx),
				Vector:  []float64{float64(idx) / 100, 0.5, 0.5},
				Source:  "event",
			}
			store.Upsert(context.Background(), entry)
		}(i)
	}

	// Concurrent searches
	for i := 0; i < numOps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			store.Search(context.Background(), []float64{1, 0, 0}, SearchOptions{TopK: 5})
		}()
	}

	wg.Wait()

	if store.Len() != numOps {
		t.Errorf("Len() = %d, want %d after concurrent upserts", store.Len(), numOps)
	}
}