package vector

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// MemoryVectorStore is an in-memory vector store with brute-force search.
// It's suitable for datasets up to ~100K entries. For larger datasets,
// use SQLiteVectorStore which persists to disk and can use index structures.
//
// This is the default store for tests and small deployments. It's thread-safe
// and uses no external dependencies.
type MemoryVectorStore struct {
	mu       sync.RWMutex
	entries  map[string]VectorEntry
	dim      int
}

// NewMemoryVectorStore creates a new in-memory vector store.
func NewMemoryVectorStore(dimension int) *MemoryVectorStore {
	return &MemoryVectorStore{
		entries: make(map[string]VectorEntry),
		dim:     dimension,
	}
}

// Upsert adds or updates a vector entry.
func (m *MemoryVectorStore) Upsert(ctx context.Context, entry VectorEntry) error {
	if !ValidateDimension(entry.Vector, m.dim) {
		return &DimensionError{Expected: m.dim, Got: len(entry.Vector)}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[entry.ID] = entry
	return nil
}

// UpsertBatch adds or updates multiple entries atomically.
func (m *MemoryVectorStore) UpsertBatch(ctx context.Context, entries []VectorEntry) error {
	for _, entry := range entries {
		if !ValidateDimension(entry.Vector, m.dim) {
			return &DimensionError{Expected: m.dim, Got: len(entry.Vector)}
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, entry := range entries {
		m.entries[entry.ID] = entry
	}
	return nil
}

// Search finds the top-K entries most similar to the query vector.
func (m *MemoryVectorStore) Search(ctx context.Context, query []float64, opts SearchOptions) ([]SearchResult, error) {
	if opts.TopK <= 0 {
		opts.TopK = 10
	}
	if opts.MinScore <= 0 {
		opts.MinScore = 0.5
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []SearchResult
	for _, entry := range m.entries {
		// Apply source filter
		if opts.SourceFilter != "" && entry.Source != opts.SourceFilter {
			continue
		}

		score := CosineSimilarity(query, entry.Vector)
		if score >= opts.MinScore {
			results = append(results, SearchResult{
				Entry: entry,
				Score: score,
			})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Limit to top-K
	if len(results) > opts.TopK {
		results = results[:opts.TopK]
	}

	return results, nil
}

// Get retrieves a single entry by ID.
func (m *MemoryVectorStore) Get(ctx context.Context, id string) (*VectorEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	entry, ok := m.entries[id]
	if !ok {
		return nil, nil
	}
	return &entry, nil
}

// Delete removes vectors matching the filter.
func (m *MemoryVectorStore) Delete(ctx context.Context, filter VectorFilter) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch {
	case len(filter.IDs) > 0:
		for _, id := range filter.IDs {
			delete(m.entries, id)
		}
	case filter.Source != "":
		for id, entry := range m.entries {
			if entry.Source == filter.Source {
				delete(m.entries, id)
			}
		}
	default:
		m.entries = make(map[string]VectorEntry)
	}
	return nil
}

// Close releases resources (no-op for in-memory store).
func (m *MemoryVectorStore) Close() error {
	return nil
}

// Len returns the number of entries in the store.
func (m *MemoryVectorStore) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// DimensionError is returned when a vector has the wrong dimension.
type DimensionError struct {
	Expected int
	Got      int
}

func (e *DimensionError) Error() string {
	return fmt.Sprintf("vector dimension mismatch: expected %d, got %d", e.Expected, e.Got)
}