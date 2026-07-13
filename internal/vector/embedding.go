package vector

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
)

// MockEmbeddingProvider generates deterministic test embeddings.
// It produces embeddings based on a hash of the input text, so the same
// text always gets the same vector. This is useful for testing vector
// search without requiring an external embedding API.
type MockEmbeddingProvider struct {
	dimension int
}

// NewMockEmbeddingProvider creates a mock embedding provider with the given dimension.
func NewMockEmbeddingProvider(dimension int) *MockEmbeddingProvider {
	return &MockEmbeddingProvider{dimension: dimension}
}

// Embed generates a deterministic embedding for the given text.
func (m *MockEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	return deterministicVector(text, m.dimension), nil
}

// EmbedBatch generates embeddings for multiple texts.
func (m *MockEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, text := range texts {
		results[i] = deterministicVector(text, m.dimension)
	}
	return results, nil
}

// Dimension returns the embedding dimension.
func (m *MockEmbeddingProvider) Dimension() int {
	return m.dimension
}

// Name returns the provider name.
func (m *MockEmbeddingProvider) Name() string {
	return "mock"
}

// deterministicVector generates a deterministic float64 vector from text.
// Uses SHA-256 hash extended to fill the desired dimension.
// Values are normalized to produce a unit vector (cosine similarity friendly).
func deterministicVector(text string, dimension int) []float64 {
	vector := make([]float64, dimension)
	if len(text) == 0 {
		return vector
	}

	// Generate enough hash bytes to fill the dimension
	// Each float64 needs 8 bytes
	needed := dimension * 8
	hashBytes := make([]byte, 0, needed)

	// Extend hash by repeatedly hashing text + counter
	for i := 0; len(hashBytes) < needed; i++ {
		h := sha256.New()
		binary.Write(h, binary.LittleEndian, uint32(i))
		h.Write([]byte(text))
		hashBytes = append(hashBytes, h.Sum(nil)...)
	}

	// Convert bytes to float64 values spanning both positive and negative range
	var norm float64
	for i := 0; i < dimension; i++ {
		bits := binary.LittleEndian.Uint64(hashBytes[i*8 : (i+1)*8])
		// Map to [-0.5, 0.5] for better vector space coverage
		val := float64(int64(bits%1001)-500) / 1000.0
		vector[i] = val
		norm += val * val
	}

	// Normalize to unit vector
	if norm > 0 {
		invNorm := 1.0 / sqrt(norm)
		for i := range vector {
			vector[i] *= invNorm
		}
	}

	return vector
}
