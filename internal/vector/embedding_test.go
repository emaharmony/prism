package vector

import (
	"context"
	"testing"
)

func TestMockEmbeddingProvider_Deterministic(t *testing.T) {
	provider := NewMockEmbeddingProvider(128)

	embed1, err := provider.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	embed2, err := provider.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	if len(embed1) != 128 {
		t.Errorf("Dimension = %d, want 128", len(embed1))
	}
	for i := range embed1 {
		if embed1[i] != embed2[i] {
			t.Errorf("Embedding not deterministic: position %d differs", i)
			break
		}
	}
}

func TestMockEmbeddingProvider_Different(t *testing.T) {
	provider := NewMockEmbeddingProvider(128)

	embed1, err := provider.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	embed2, err := provider.Embed(context.Background(), "goodbye")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	// Different texts should produce different embeddings
	similarity := CosineSimilarity(embed1, embed2)
	if similarity > 0.99 {
		t.Errorf("Different texts produced near-identical embeddings (similarity = %f)", similarity)
	}
}

func TestMockEmbeddingProvider_Batch(t *testing.T) {
	provider := NewMockEmbeddingProvider(64)

	texts := []string{"alpha", "beta", "gamma"}
	results, err := provider.EmbedBatch(context.Background(), texts)
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if len(results) != 3 {
		t.Errorf("EmbedBatch() returned %d results, want 3", len(results))
	}
	for i, result := range results {
		if len(result) != 64 {
			t.Errorf("Result %d dimension = %d, want 64", i, len(result))
		}
	}
}

func TestMockEmbeddingProvider_Dimension(t *testing.T) {
	provider := NewMockEmbeddingProvider(256)
	if provider.Dimension() != 256 {
		t.Errorf("Dimension() = %d, want 256", provider.Dimension())
	}
}

func TestMockEmbeddingProvider_Name(t *testing.T) {
	provider := NewMockEmbeddingProvider(128)
	if provider.Name() != "mock" {
		t.Errorf("Name() = %q, want mock", provider.Name())
	}
}

func TestMockEmbeddingProvider_EmptyText(t *testing.T) {
	provider := NewMockEmbeddingProvider(128)
	embed, err := provider.Embed(context.Background(), "")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(embed) != 128 {
		t.Errorf("Dimension = %d, want 128", len(embed))
	}
	// Empty text should produce a zero vector
	for _, v := range embed {
		if v != 0 {
			t.Error("Empty text should produce zero vector")
			break
		}
	}
}

func TestMockEmbeddingProvider_UnitVector(t *testing.T) {
	provider := NewMockEmbeddingProvider(128)
	embed, err := provider.Embed(context.Background(), "test")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}

	// Check that the result is approximately a unit vector
	var norm float64
	for _, v := range embed {
		norm += v * v
	}
	if norm < 0.99 || norm > 1.01 {
		t.Errorf("Norm = %f, want ~1.0 (unit vector)", norm)
	}
}