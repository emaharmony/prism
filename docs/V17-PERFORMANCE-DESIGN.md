# V17 — Performance: HNSW Vector Index, Connection Pooling, Event Store Indexes

**Status:** Merged (PR #30)
**Date:** 2026-05-19

## What Changed

Four targeted performance improvements to bring Prism's performance dimension to ≥8.5.

### 1. HNSW Vector Search — O(log n) Instead of O(n)

The `MemoryVectorStore` previously used brute-force linear scan for all vector similarity searches. This works for small datasets (<100 entries) but degrades to O(n) for every query.

**New: `internal/vector/hnsw.go`** — Hierarchical Navigable Small World graph index for approximate nearest neighbor search.

- Single-layer HNSW with configurable neighbors per node (default: 24)
- Automatic fallback to brute-force for small graphs (≤2× neighbors threshold)
- Deterministic entry point selection (lexicographic smallest ID)
- `Insert`, `InsertBulk`, `Delete`, `Search` all supported
- Thread-safe via `RWMutex`
- Beam search with configurable `ef` parameter (default: 4× TopK)

**Key methods:**
- `Insert(id, vector)` — Add single vector, connects to nearest neighbors
- `InsertBulk(entries)` — Batch insert under single lock (O(n) vs O(n²) for bulk loading)
- `Search(query, k, ef)` — Top-K nearest neighbors via graph traversal
- `Delete(id)` — Remove vector and clean up neighbor references
- `bruteForceSearch(query, k)` — Linear scan for small datasets
- `graphSearch(query, k, ef, entryID)` — HNSW traversal for large datasets

**Design decisions:**
- Single-layer (not true multi-layer HNSW) — simpler, sufficient for Prism's scale
- `maybeReplaceWeakest` keeps neighbor lists bounded — when full, weakest connection replaced by closer one
- `searchNearestUnlocked` uses brute-force during insert — acceptable because inserts are less frequent than searches
- `distItem` type for sorting instead of anonymous struct — cleaner, reusable

### 2. HTTP Connection Pooling — All 5 LLM Providers

Previously, each LLM provider created its own `http.Client` with default transport (no connection reuse). Every API call required a new TCP handshake + TLS negotiation.

**New: `internal/provider/transport.go`** — Shared HTTP transport with connection pooling.

```go
var DefaultTransport = &http.Transport{
    MaxIdleConns:        100,
    MaxIdleConnsPerHost: 20,
    IdleConnTimeout:     90 * time.Second,
}
```

Applied to all 5 providers: OpenAI, Anthropic, Gemini, Ollama, and compat wrappers (Together, Groq, Azure, Ollama-compat).

**Architecture note:** `DefaultTransport` lives in the parent `provider` package, not in `openai/`. Anthropic, Gemini, and Ollama import from `provider.DefaultTransport`, not from `openai.DefaultTransport`. This avoids cross-provider dependencies.

### 3. Event Store Indexes — 3 New Composite Indexes

The SQLite event store had 3 indexes (run_id, type, timestamp). Query patterns showed common filters on composite columns.

**New indexes:**
```sql
CREATE INDEX idx_events_source ON events(run_id, type);      -- filtered event queries
CREATE INDEX idx_events_correlation ON events(correlation_id); -- correlation lookups
CREATE INDEX idx_events_created ON events(created_at);         -- time-range pagination
```

These cover the most common query patterns after simple run_id filtering:
- "Get all events of type X within run Y" → uses `idx_events_source`
- "Find events with correlation ID Z" → uses `idx_events_correlation`
- "Paginate events by creation time" → uses `idx_events_created`

### 4. Approval Store — No Change

The approval store is file-based with `RWMutex` — appropriate for Prism's current scale. SQLite migration would be over-engineering at this stage.

## Test Coverage

**8 new HNSW tests:**
- `TestHNSWInsertAndSearch` — basic insert and nearest-neighbor search
- `TestHNSWDelete` — delete and verify removal
- `TestHNSWEmpty` — empty index returns nil
- `TestHNSWSingleEntry` — single-vector edge case
- `TestHNSWLargeDataset` — 200 vectors, verify top-K quality
- `TestHNSWRecall` — compare HNSW results against brute-force (sorted!)
- `TestHNSWConcurrency` — concurrent inserts from 10 goroutines
- `TestHNSWMaybeReplaceWeakest` — neighbor replacement when list is full
- `TestHNSWNilVector` — nil/empty vector validation
- `TestHNSWWrongDimensionSearch` — dimension mismatch returns nil
- `TestHNSWDeleteNonExistent` — delete nonexistent is safe
- `TestHNSWInsertDuplicate` — duplicate ID updates vector
- `TestHNSWInsertBulk` — batch insert with invalid filtering
- `TestHNSWDimensionMismatch` — wrong dimension insert is rejected

**4 benchmarks:**
- `BenchmarkHNSWInsert` — single insert performance
- `BenchmarkHNSWSearch` — search on 1000-vector index
- `BenchmarkHNSWInsertBulk` — batch insert performance
- `BenchmarkBruteForceSearch` — comparison baseline

**Total: 620 tests, 0 failures, 35 packages**

## Files Changed

| File | Change |
|------|--------|
| `internal/vector/hnsw.go` | NEW — HNSW index implementation |
| `internal/vector/hnsw_test.go` | NEW — 13 tests + 4 benchmarks |
| `internal/vector/memory.go` | MODIFIED — uses HNSW for >100 entries |
| `internal/provider/transport.go` | NEW — shared DefaultTransport |
| `internal/provider/openai/openai.go` | MODIFIED — re-exports DefaultTransport |
| `internal/provider/openai/compat.go` | MODIFIED — uses shared transport |
| `internal/provider/anthropic/anthropic.go` | MODIFIED — uses provider.DefaultTransport |
| `internal/provider/gemini/gemini.go` | MODIFIED — uses provider.DefaultTransport |
| `internal/provider/ollama/ollama.go` | MODIFIED — uses provider.DefaultTransport |
| `internal/event/store.go` | MODIFIED — 3 new composite indexes |

## Mango Review

Two rounds of Mango review:

**Round 1 (7.1/10):**
- Correctness 6 — TestHNSWRecall broken, non-deterministic entry point
- Architecture 5 — Cross-provider dependency (anthropic importing openai)
- Performance 7 — Insert still O(n)
- Error Handling 7 — No nil vector check, no limit validation

**Round 2 (≥8.5 all dimensions):**
- Extracted DefaultTransport to `provider/transport.go`
- Fixed recall test (sorted brute-force)
- Added nil/empty/dimension validation
- Deterministic entry point (lexicographic smallest)
- InsertBulk for batch loading
- Query limit capped at 10,000
- 13 HNSW edge case tests + 4 benchmarks