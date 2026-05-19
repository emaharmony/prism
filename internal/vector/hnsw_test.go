package vector

import (
	"fmt"
	"math"
	"sort"
	"testing"
)

func TestHNSWInsertAndSearch(t *testing.T) {
	idx := NewHNSWIndex(3, 16)

	vectors := map[string][]float64{
		"a": {1.0, 0.0, 0.0},
		"b": {0.0, 1.0, 0.0},
		"c": {0.0, 0.0, 1.0},
		"d": {0.9, 0.1, 0.0},
		"e": {0.1, 0.9, 0.0},
	}
	for id, vec := range vectors {
		idx.Insert(id, vec)
	}

	if idx.Len() != 5 {
		t.Errorf("expected 5 entries, got %d", idx.Len())
	}

	results := idx.Search([]float64{1.0, 0.0, 0.0}, 3, 50)
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}

	if results[0].Entry.ID != "a" {
		t.Errorf("expected top result 'a', got '%s'", results[0].Entry.ID)
	}
	if results[0].Score < 0.99 {
		t.Errorf("expected score > 0.99 for exact match, got %f", results[0].Score)
	}
}

func TestHNSWDelete(t *testing.T) {
	idx := NewHNSWIndex(3, 16)

	idx.Insert("a", []float64{1.0, 0.0, 0.0})
	idx.Insert("b", []float64{0.0, 1.0, 0.0})

	if idx.Len() != 2 {
		t.Errorf("expected 2, got %d", idx.Len())
	}

	idx.Delete("a")
	if idx.Len() != 1 {
		t.Errorf("expected 1 after delete, got %d", idx.Len())
	}

	results := idx.Search([]float64{1.0, 0.0, 0.0}, 1, 10)
	if len(results) > 0 && results[0].Entry.ID == "a" {
		t.Error("deleted entry 'a' should not appear in results")
	}
}

func TestHNSWEmpty(t *testing.T) {
	idx := NewHNSWIndex(3, 16)
	results := idx.Search([]float64{1.0, 0.0, 0.0}, 5, 50)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty index, got %d", len(results))
	}
}

func TestHNSWSingleEntry(t *testing.T) {
	idx := NewHNSWIndex(3, 16)
	idx.Insert("only", []float64{1.0, 0.0, 0.0})

	results := idx.Search([]float64{1.0, 0.0, 0.0}, 5, 10)
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}
	if results[0].Entry.ID != "only" {
		t.Errorf("expected 'only', got '%s'", results[0].Entry.ID)
	}
}

func TestHNSWLargeDataset(t *testing.T) {
	idx := NewHNSWIndex(3, 24)

	for i := 0; i < 200; i++ {
		angle := float64(i) * math.Pi / 100.0
		vec := []float64{math.Cos(angle), math.Sin(angle), 0.0}
		id := fmt.Sprintf("v%d", i)
		idx.Insert(id, vec)
	}

	if idx.Len() != 200 {
		t.Errorf("expected 200 entries, got %d", idx.Len())
	}

	results := idx.Search([]float64{1.0, 0.0, 0.0}, 10, 100)
	if len(results) == 0 {
		t.Fatal("expected results from large dataset")
	}

	if results[0].Score < 0.9 {
		t.Errorf("expected top score > 0.9, got %f", results[0].Score)
	}
}

func TestHNSWRecall(t *testing.T) {
	idx := NewHNSWIndex(3, 24)

	type entry struct {
		id  string
		vec []float64
	}
	var entries []entry

	for i := 0; i < 50; i++ {
		angle := float64(i) * math.Pi * 2 / 50.0
		vec := []float64{math.Cos(angle), math.Sin(angle), 0.0}
		id := fmt.Sprintf("v%d", i)
		idx.Insert(id, vec)
		entries = append(entries, entry{id: id, vec: vec})
	}

	query := []float64{1.0, 0.0, 0.0}

	// HNSW search
	hnswResults := idx.Search(query, 5, 50)

	// Brute-force search (sorted by score)
	var bruteForce []SearchResult
	for _, e := range entries {
		score := CosineSimilarity(query, e.vec)
		bruteForce = append(bruteForce, SearchResult{
			Entry: VectorEntry{ID: e.id, Vector: e.vec},
			Score: score,
		})
	}

	// Sort brute-force results by descending score
	sort.Slice(bruteForce, func(i, j int) bool {
		return bruteForce[i].Score > bruteForce[j].Score
	})

	if len(hnswResults) == 0 {
		t.Fatal("no HNSW results")
	}

	// Check that top HNSW result is in top 5 brute-force results
	topHNSW := hnswResults[0].Entry.ID
	found := false
	for i := 0; i < 5 && i < len(bruteForce); i++ {
		if bruteForce[i].Entry.ID == topHNSW {
			found = true
		}
	}

	if !found {
		t.Errorf("top HNSW result %s not in brute-force top 5", topHNSW)
	}
}

func TestHNSWConcurrency(t *testing.T) {
	idx := NewHNSWIndex(3, 16)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			vec := []float64{float64(n), 0.0, 0.0}
			idx.Insert(fmt.Sprintf("c%d", n), vec)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	if idx.Len() != 10 {
		t.Errorf("expected 10 entries after concurrent inserts, got %d", idx.Len())
	}
}

func TestHNSWMaybeReplaceWeakest(t *testing.T) {
	idx := NewHNSWIndex(3, 2) // Only 2 neighbors per node

	idx.Insert("a", []float64{1.0, 0.0, 0.0})
	idx.Insert("b", []float64{0.0, 1.0, 0.0})
	idx.Insert("c", []float64{0.0, 0.0, 1.0})
	idx.Insert("d", []float64{0.95, 0.05, 0.0})

	results := idx.Search([]float64{1.0, 0.0, 0.0}, 2, 20)
	if len(results) == 0 {
		t.Error("expected results after neighbor replacement")
	}
}

func TestHNSWNilVector(t *testing.T) {
	idx := NewHNSWIndex(3, 16)
	idx.Insert("nil", nil)
	idx.Insert("empty", []float64{})
	idx.Insert("wrong_dim", []float64{1.0, 0.0})

	if idx.Len() != 0 {
		t.Errorf("expected 0 entries for nil/empty/wrong-dim vectors, got %d", idx.Len())
	}
}

func TestHNSWWrongDimensionSearch(t *testing.T) {
	idx := NewHNSWIndex(3, 16)
	idx.Insert("a", []float64{1.0, 0.0, 0.0})

	results := idx.Search([]float64{1.0, 0.0}, 1, 10) // 2D query on 3D index
	if results != nil {
		t.Errorf("expected nil for wrong dimension, got %d results", len(results))
	}
}

func TestHNSWDeleteNonExistent(t *testing.T) {
	idx := NewHNSWIndex(3, 16)
	idx.Insert("a", []float64{1.0, 0.0, 0.0})

	idx.Delete("nonexistent") // should not panic
	if idx.Len() != 1 {
		t.Errorf("expected 1 after deleting nonexistent, got %d", idx.Len())
	}
}

func TestHNSWInsertDuplicate(t *testing.T) {
	idx := NewHNSWIndex(3, 16)
	idx.Insert("a", []float64{1.0, 0.0, 0.0})
	idx.Insert("a", []float64{0.9, 0.1, 0.0}) // update

	results := idx.Search([]float64{0.9, 0.1, 0.0}, 1, 10)
	if len(results) == 0 {
		t.Fatal("expected results after duplicate insert")
	}
	// Should find the updated vector
	if results[0].Score < 0.95 {
		t.Errorf("expected high score for updated vector, got %f", results[0].Score)
	}
}

func TestHNSWInsertBulk(t *testing.T) {
	idx := NewHNSWIndex(3, 16)

	entries := []struct{ ID string; Vector []float64 }{
		{"a", []float64{1.0, 0.0, 0.0}},
		{"b", []float64{0.0, 1.0, 0.0}},
		{"c", []float64{0.0, 0.0, 1.0}},
		{"bad1", nil},          // filtered
		{"bad2", []float64{}}, // filtered
		{"bad3", []float64{1.0}}, // filtered (wrong dim)
	}
	idx.InsertBulk(entries)

	if idx.Len() != 3 {
		t.Errorf("expected 3 entries (3 valid, 3 filtered), got %d", idx.Len())
	}
}

func TestHNSWDimensionMismatch(t *testing.T) {
	idx := NewHNSWIndex(3, 16)

	// Insert wrong dimension
	idx.Insert("wrong", []float64{1.0, 0.0})
	if idx.Len() != 0 {
		t.Errorf("expected 0 for wrong dimension insert, got %d", idx.Len())
	}

	// Search with nil
	results := idx.Search(nil, 1, 10)
	if results != nil {
		t.Errorf("expected nil for nil query, got %d results", len(results))
	}
}

func BenchmarkHNSWInsert(b *testing.B) {
	idx := NewHNSWIndex(3, 24)
	vecs := make([][]float64, b.N)
	for i := range vecs {
		angle := float64(i) * math.Pi / 1000.0
		vecs[i] = []float64{math.Cos(angle), math.Sin(angle), 0.0}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Insert(fmt.Sprintf("v%d", i), vecs[i])
	}
}

func BenchmarkHNSWSearch(b *testing.B) {
	idx := NewHNSWIndex(3, 24)
	// Pre-populate with 1000 vectors
	for i := 0; i < 1000; i++ {
		angle := float64(i) * math.Pi * 2 / 1000.0
		idx.Insert(fmt.Sprintf("v%d", i), []float64{math.Cos(angle), math.Sin(angle), 0.0})
	}
	query := []float64{1.0, 0.0, 0.0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search(query, 10, 50)
	}
}

func BenchmarkHNSWInsertBulk(b *testing.B) {
	entries := make([]struct{ ID string; Vector []float64 }, 100)
	for i := range entries {
		angle := float64(i) * math.Pi * 2 / 100.0
		entries[i] = struct{ ID string; Vector []float64 }{
			ID:     fmt.Sprintf("v%d", i),
			Vector: []float64{math.Cos(angle), math.Sin(angle), 0.0},
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx2 := NewHNSWIndex(3, 24)
		idx2.InsertBulk(entries)
	}
}

func BenchmarkBruteForceSearch(b *testing.B) {
	store := NewMemoryVectorStore(3)
	for i := 0; i < 1000; i++ {
		angle := float64(i) * math.Pi * 2 / 1000.0
		store.Upsert(nil, VectorEntry{
			ID:     fmt.Sprintf("v%d", i),
			Vector: []float64{math.Cos(angle), math.Sin(angle), 0.0},
		})
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		store.Search(nil, []float64{1.0, 0.0, 0.0}, SearchOptions{TopK: 10, MinScore: 0.0})
	}
}