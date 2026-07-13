package vector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOllamaEmbeddingProvider_Name(t *testing.T) {
	p := NewOllamaEmbeddingProvider("", "nomic-embed-text")
	if p.Name() != "ollama" {
		t.Errorf("Name() = %q, want ollama", p.Name())
	}
}

func TestOllamaEmbeddingProvider_DefaultURL(t *testing.T) {
	p := NewOllamaEmbeddingProvider("", "nomic-embed-text")
	if p.baseURL != "http://localhost:11434" {
		t.Errorf("baseURL = %q, want http://localhost:11434", p.baseURL)
	}
}

func TestOllamaEmbeddingProvider_Embed_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/embeddings" {
			t.Errorf("Path = %q, want /api/embeddings", r.URL.Path)
		}
		var req ollamaEmbedRequest
		json.NewDecoder(r.Body).Decode(&req)
		if req.Model != "nomic-embed-text" {
			t.Errorf("Model = %q, want nomic-embed-text", req.Model)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaEmbedResponse{
			Embedding: []float64{0.1, 0.2, 0.3},
		})
	}))
	defer server.Close()

	p := NewOllamaEmbeddingProvider(server.URL, "nomic-embed-text")
	embed, err := p.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(embed) != 3 {
		t.Errorf("Embed() dimension = %d, want 3", len(embed))
	}
}

func TestOllamaEmbeddingProvider_EmbedBatch(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaEmbedResponse{
			Embedding: []float64{0.1 * float64(callCount), 0.2, 0.3},
		})
	}))
	defer server.Close()

	p := NewOllamaEmbeddingProvider(server.URL, "nomic-embed-text")
	results, err := p.EmbedBatch(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("EmbedBatch() returned %d results, want 2", len(results))
	}
	if callCount != 2 {
		t.Errorf("Ollama called %d times, want 2 (sequential)", callCount)
	}
}

func TestOllamaEmbeddingProvider_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("model not found"))
	}))
	defer server.Close()

	p := NewOllamaEmbeddingProvider(server.URL, "nonexistent-model")
	_, err := p.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for 500")
	}
}

func TestOllamaEmbeddingProvider_Dimension(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(ollamaEmbedResponse{
			Embedding: []float64{0.1, 0.2, 0.3, 0.4, 0.5},
		})
	}))
	defer server.Close()

	p := NewOllamaEmbeddingProvider(server.URL, "nomic-embed-text")
	dim := p.Dimension()
	if dim != 5 {
		t.Errorf("Dimension() = %d, want 5", dim)
	}

	// Second call should use cached value
	dim2 := p.Dimension()
	if dim2 != 5 {
		t.Errorf("Dimension() cached = %d, want 5", dim2)
	}
	if callCount != 1 {
		t.Errorf("Ollama called %d times, want 1 (cached)", callCount)
	}
}

func TestOllamaEmbeddingProvider_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewOllamaEmbeddingProvider("http://localhost:11434", "nomic-embed-text")
	_, err := p.Embed(ctx, "hello")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}
