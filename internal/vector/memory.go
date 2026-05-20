package vector

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// MemoryVectorStore is an in-memory vector store with HNSW-accelerated search.
// For small datasets (<100 entries), it falls back to brute-force for correctness.
// For larger datasets, it uses the HNSW index for O(log n) search complexity.
//
// It's thread-safe and uses no external dependencies.
type MemoryVectorStore struct {
	mu      sync.RWMutex
	entries map[string]VectorEntry
	index   *HNSWIndex
	dim     int
	useHNSW bool // whether to use HNSW index for searches
}

// NewMemoryVectorStore creates a new in-memory vector store.
func NewMemoryVectorStore(dimension int) *MemoryVectorStore {
	return &MemoryVectorStore{
		entries: make(map[string]VectorEntry),
		index:   NewHNSWIndex(dimension, 24),
		dim:     dimension,
		useHNSW: true,
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
	if m.useHNSW {
		m.index.Insert(entry.ID, entry.Vector)
	}
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
		if m.useHNSW {
			m.index.Insert(entry.ID, entry.Vector)
		}
	}
	return nil
}

// Search finds the top-K entries most similar to the query vector.
// Uses HNSW index for O(log n) search when available, brute-force fallback.
func (m *MemoryVectorStore) Search(ctx context.Context, query []float64, opts SearchOptions) ([]SearchResult, error) {
	if opts.TopK <= 0 {
		opts.TopK = 10
	}
	if opts.MinScore <= 0 {
		opts.MinScore = 0.5
	}

	m.mu.RLock()
	defer m.mu.RUnlock()

	// Use HNSW index for larger datasets
	if m.useHNSW && len(m.entries) > 100 {
		ef := opts.TopK * 4 // beam width = 4x requested results for better recall
		if ef < 50 {
			ef = 50
		}
		results := m.index.Search(query, opts.TopK*2, ef)

		// Fill in content and metadata from entries, apply filters
		var filtered []SearchResult
		for _, r := range results {
			entry, ok := m.entries[r.Entry.ID]
			if !ok {
				continue
			}
			// Apply source filter
			if opts.SourceFilter != "" && entry.Source != opts.SourceFilter {
				continue
			}
			if r.Score >= opts.MinScore {
				filtered = append(filtered, SearchResult{
					Entry: entry,
					Score: r.Score,
				})
			}
		}

		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Score > filtered[j].Score
		})

		if len(filtered) > opts.TopK {
			filtered = filtered[:opts.TopK]
		}
		return filtered, nil
	}

	// Brute-force fallback for small datasets
	var results []SearchResult
	for _, entry := range m.entries {
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

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

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
			if m.useHNSW {
				m.index.Delete(id)
			}
		}
	case filter.Source != "":
		for id, entry := range m.entries {
			if entry.Source == filter.Source {
				delete(m.entries, id)
				if m.useHNSW {
					m.index.Delete(id)
				}
			}
		}
	default:
		m.entries = make(map[string]VectorEntry)
		m.index = NewHNSWIndex(m.dim, 24)
	}
	return nil
}

// Close releases resources (no-op for in-memory store).
func (m *MemoryVectorStore) Close() error { return nil }

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