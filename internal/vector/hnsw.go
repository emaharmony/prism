package vector

import (
	"container/heap"
	"math/rand"
	"sort"
	"sync"
)

// distItem holds a node ID and its distance for sorting.
type distItem struct {
	id   string
	dist float64
}

// HNSWIndex implements a Hierarchical Navigable Small World graph for
// approximate nearest neighbor search. This provides O(log n) search
// complexity instead of O(n) brute-force, with configurable recall/latency tradeoff.
//
// Based on "Efficient and robust approximate nearest neighbor search using
// Hierarchical Navigable Small World graphs" (Malkov & Yashunin, 2016).
// Simplified: single-layer with configurable neighbors per node.
//
// For datasets smaller than the threshold (default: 2 * neighbors), the index
// falls back to brute-force scan since the graph overhead isn't worth it.
//
// Thread-safe via RWMutex.
type HNSWIndex struct {
	mu         sync.RWMutex
	nodes      map[string]*hnswNode
	neighbors  int // max connections per node
	dim        int
	rng        *rand.Rand
	smallGraph int // threshold for brute-force fallback in Search
}

type hnswNode struct {
	id        string
	vector    []float64
	neighbors []string // neighbor node IDs
}

// NewHNSWIndex creates a new HNSW index with the given dimension and max neighbors.
// Typical neighbors values: 16-32. Higher = better recall, slower build.
func NewHNSWIndex(dimension, neighbors int) *HNSWIndex {
	if neighbors <= 0 {
		neighbors = 24
	}
	return &HNSWIndex{
		nodes:      make(map[string]*hnswNode),
		neighbors:  neighbors,
		dim:        dimension,
		rng:        rand.New(rand.NewSource(42)), // deterministic for reproducibility
		smallGraph: neighbors * 2,                 // fallback threshold
	}
}

// Insert adds a vector to the index, connecting it to its nearest neighbors.
// Silently ignores nil or empty vectors, or vectors with wrong dimension.
func (h *HNSWIndex) Insert(id string, vector []float64) {
	if vector == nil || len(vector) == 0 || len(vector) != h.dim {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.insertUnlocked(id, vector)
}

// InsertBulk adds multiple vectors to the index efficiently under a single lock.
// This is significantly faster than individual Insert calls for bulk loading
// because it acquires the write lock only once.
func (h *HNSWIndex) InsertBulk(entries []struct{ ID string; Vector []float64 }) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, e := range entries {
		if e.Vector != nil && len(e.Vector) == h.dim {
			h.insertUnlocked(e.ID, e.Vector)
		}
	}
}

// insertUnlocked adds a vector without acquiring the mutex.
// Caller must hold h.mu.
func (h *HNSWIndex) insertUnlocked(id string, vector []float64) {
	node := &hnswNode{
		id:        id,
		vector:    vector,
		neighbors: make([]string, 0, h.neighbors),
	}

	if len(h.nodes) == 0 {
		h.nodes[id] = node
		return
	}

	// Find nearest neighbors to connect to
	neighbors := h.searchNearestUnlocked(vector, h.neighbors)

	h.nodes[id] = node

	// Connect new node to neighbors
	for _, neighborID := range neighbors {
		if neighbor, ok := h.nodes[neighborID]; ok {
			node.neighbors = append(node.neighbors, neighborID)
			// Add reverse connection
			if len(neighbor.neighbors) < h.neighbors {
				neighbor.neighbors = append(neighbor.neighbors, id)
			} else {
				// Replace weakest connection if new one is closer
				h.maybeReplaceWeakest(neighbor, id, vector)
			}
		}
	}
}

// Delete removes a vector from the index.
func (h *HNSWIndex) Delete(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	node, ok := h.nodes[id]
	if !ok {
		return
	}

	// Remove this node from all neighbors' lists
	for _, neighborID := range node.neighbors {
		if neighbor, ok := h.nodes[neighborID]; ok {
			neighbor.neighbors = removeString(neighbor.neighbors, id)
		}
	}

	delete(h.nodes, id)
}

// Search finds the top-K nearest neighbors using graph traversal.
// ef controls the beam width — higher = better recall, slower.
// Returns nil for empty index, nil/empty query, or dimension mismatch.
func (h *HNSWIndex) Search(query []float64, k int, ef int) []SearchResult {
	if query == nil || len(query) == 0 || len(query) != h.dim {
		return nil
	}
	if ef < k {
		ef = k
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	if len(h.nodes) == 0 {
		return nil
	}

	// For small graphs, brute-force scan is faster and exact
	if len(h.nodes) <= h.smallGraph {
		return h.bruteForceSearch(query, k)
	}

	// Use deterministic entry point: lexicographically smallest ID
	var entryID string
	for id := range h.nodes {
		if entryID == "" || id < entryID {
			entryID = id
		}
	}

	return h.graphSearch(query, k, ef, entryID)
}

// bruteForceSearch does a linear scan of all nodes. Used for small graphs.
func (h *HNSWIndex) bruteForceSearch(query []float64, k int) []SearchResult {
	var results []SearchResult
	for id, node := range h.nodes {
		score := CosineSimilarity(query, node.vector)
		results = append(results, SearchResult{
			Entry: VectorEntry{ID: id, Vector: node.vector},
			Score: score,
		})
	}
	sort.Slice(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > k {
		results = results[:k]
	}
	return results
}

// graphSearch performs HNSW graph traversal starting from the given entry point.
func (h *HNSWIndex) graphSearch(query []float64, k int, ef int, entryID string) []SearchResult {
	visited := make(map[string]bool)
	candidates := &minHeap{}
	heap.Init(candidates)

	// Initialize with entry point
	entryDist := cosineDistance(query, h.nodes[entryID].vector)
	heap.Push(candidates, &heapItem{id: entryID, dist: entryDist})
	visited[entryID] = true

	// Track best results
	results := &maxHeap{}
	heap.Init(results)

	for candidates.Len() > 0 {
		current := heap.Pop(candidates).(*heapItem)

		// If current is farther than the k-th best and we have enough results, stop
		if results.Len() >= k && current.dist > (*results)[0].dist {
			break
		}

		node := h.nodes[current.id]
		if node == nil {
			continue
		}

		for _, neighborID := range node.neighbors {
			if visited[neighborID] {
				continue
			}
			visited[neighborID] = true

			neighbor, ok := h.nodes[neighborID]
			if !ok {
				continue
			}

			dist := cosineDistance(query, neighbor.vector)

			heap.Push(candidates, &heapItem{id: neighborID, dist: dist})
			heap.Push(results, &heapItem{id: neighborID, dist: dist})

			if results.Len() > ef {
				heap.Pop(results)
			}
		}
	}

	// Collect top-K results
	var searchResults []SearchResult
	for results.Len() > 0 {
		item := heap.Pop(results).(*heapItem)
		if node, ok := h.nodes[item.id]; ok {
			searchResults = append(searchResults, SearchResult{
				Entry: VectorEntry{
					ID:     node.id,
					Vector: node.vector,
				},
				Score: 1 - item.dist, // Convert distance back to similarity
			})
		}
	}

	// Reverse to get highest score first (results come out max-heap order)
	for i, j := 0, len(searchResults)-1; i < j; i, j = i+1, j-1 {
		searchResults[i], searchResults[j] = searchResults[j], searchResults[i]
	}

	if len(searchResults) > k {
		searchResults = searchResults[:k]
	}

	return searchResults
}

// Len returns the number of entries in the index.
func (h *HNSWIndex) Len() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.nodes)
}

// searchNearestUnlocked finds nearest neighbors without acquiring locks.
// Used internally during insert.
func (h *HNSWIndex) searchNearestUnlocked(query []float64, k int) []string {
	if len(h.nodes) <= k {
		ids := make([]string, 0, len(h.nodes))
		for id := range h.nodes {
			ids = append(ids, id)
		}
		return ids
	}

	candidates := make([]distItem, 0, len(h.nodes))
	for id, node := range h.nodes {
		dist := cosineDistance(query, node.vector)
		candidates = append(candidates, distItem{id: id, dist: dist})
	}

	if len(candidates) > k {
		partialSort(candidates, k)
		candidates = candidates[:k]
	}

	result := make([]string, len(candidates))
	for i, c := range candidates {
		result[i] = c.id
	}
	return result
}

// maybeReplaceWeakest replaces the weakest connection if the new one is closer.
func (h *HNSWIndex) maybeReplaceWeakest(node *hnswNode, newID string, newVector []float64) {
	if len(node.neighbors) == 0 {
		return
	}

	// Find the weakest (most distant) neighbor
	worstIdx := 0
	worstDist := cosineDistance(node.vector, h.nodes[node.neighbors[0]].vector)

	for i, neighborID := range node.neighbors {
		if neighbor, ok := h.nodes[neighborID]; ok {
			dist := cosineDistance(node.vector, neighbor.vector)
			if dist > worstDist {
				worstDist = dist
				worstIdx = i
			}
		}
	}

	newDist := cosineDistance(node.vector, newVector)
	if newDist < worstDist {
		node.neighbors[worstIdx] = newID
	}
}

// cosineDistance returns 1 - cosine_similarity (lower = closer).
func cosineDistance(a, b []float64) float64 {
	return 1 - CosineSimilarity(a, b)
}

// partialSort does a partial sort to find the k smallest items by selection.
func partialSort(items []distItem, k int) {
	for i := 0; i < k && i < len(items); i++ {
		minIdx := i
		for j := i + 1; j < len(items); j++ {
			if items[j].dist < items[minIdx].dist {
				minIdx = j
			}
		}
		items[i], items[minIdx] = items[minIdx], items[i]
	}
}

func removeString(slice []string, s string) []string {
	for i, v := range slice {
		if v == s {
			return append(slice[:i], slice[i+1:]...)
		}
	}
	return slice
}

// Heap types for HNSW search

type heapItem struct {
	id   string
	dist float64
}

type minHeap []*heapItem

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].dist < h[j].dist }
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(*heapItem)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}

type maxHeap []*heapItem

func (h maxHeap) Len() int           { return len(h) }
func (h maxHeap) Less(i, j int) bool { return h[i].dist > h[j].dist } // max-heap
func (h maxHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *maxHeap) Push(x any)        { *h = append(*h, x.(*heapItem)) }
func (h *maxHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[:n-1]
	return item
}