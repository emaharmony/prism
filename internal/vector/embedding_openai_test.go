package vector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOpenAIEmbeddingProvider_Name(t *testing.T) {
	p := NewOpenAIEmbeddingProvider("test-key", OpenAIEmbeddingSmall, 0)
	if p.Name() != "openai" {
		t.Errorf("Name() = %q, want openai", p.Name())
	}
}

func TestOpenAIEmbeddingProvider_Dimension(t *testing.T) {
	p := NewOpenAIEmbeddingProvider("test-key", OpenAIEmbeddingSmall, 0)
	if p.Dimension() != OpenAIEmbeddingSmallDim {
		t.Errorf("Dimension() = %d, want %d", p.Dimension(), OpenAIEmbeddingSmallDim)
	}

	p2 := NewOpenAIEmbeddingProvider("test-key", OpenAIEmbeddingLarge, 0)
	if p2.Dimension() != OpenAIEmbeddingLargeDim {
		t.Errorf("Dimension() = %d, want %d", p2.Dimension(), OpenAIEmbeddingLargeDim)
	}
}

func TestOpenAIEmbeddingProvider_CustomDimension(t *testing.T) {
	p := NewOpenAIEmbeddingProvider("test-key", OpenAIEmbeddingSmall, 512)
	if p.Dimension() != 512 {
		t.Errorf("Dimension() = %d, want 512", p.Dimension())
	}
}

func TestOpenAIEmbeddingProvider_Embed_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want Bearer test-key", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIEmbeddingResponse{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float64{0.1, 0.2, 0.3}, Index: 0},
			},
			Model: "text-embedding-3-small",
		})
	}))
	defer server.Close()

	p := NewOpenAIEmbeddingProviderWithBaseURL("test-key", server.URL, OpenAIEmbeddingSmall, 3)
	embed, err := p.Embed(context.Background(), "hello world")
	if err != nil {
		t.Fatalf("Embed() error = %v", err)
	}
	if len(embed) != 3 {
		t.Errorf("Embed() dimension = %d, want 3", len(embed))
	}
}

func TestOpenAIEmbeddingProvider_EmbedBatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openAIEmbeddingResponse{
			Data: []struct {
				Embedding []float64 `json:"embedding"`
				Index     int       `json:"index"`
			}{
				{Embedding: []float64{0.1, 0.2}, Index: 0},
				{Embedding: []float64{0.3, 0.4}, Index: 1},
			},
			Model: "text-embedding-3-small",
		})
	}))
	defer server.Close()

	p := NewOpenAIEmbeddingProviderWithBaseURL("test-key", server.URL, OpenAIEmbeddingSmall, 2)
	results, err := p.EmbedBatch(context.Background(), []string{"hello", "world"})
	if err != nil {
		t.Fatalf("EmbedBatch() error = %v", err)
	}
	if len(results) != 2 {
		t.Errorf("EmbedBatch() returned %d results, want 2", len(results))
	}
}

func TestOpenAIEmbeddingProvider_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "invalid api key"}}`))
	}))
	defer server.Close()

	p := NewOpenAIEmbeddingProviderWithBaseURL("bad-key", server.URL, OpenAIEmbeddingSmall, 3)
	_, err := p.Embed(context.Background(), "hello")
	if err == nil {
		t.Fatal("expected error for 401")
	}
}

func TestOpenAIEmbeddingProvider_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p := NewOpenAIEmbeddingProvider("test-key", OpenAIEmbeddingSmall, 3)
	_, err := p.Embed(ctx, "hello")
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestOpenAIEmbeddingProviderWithBaseURL(t *testing.T) {
	p := NewOpenAIEmbeddingProviderWithBaseURL("test-key", "https://api.custom.ai/v1", OpenAIEmbeddingSmall, 0)
	if p.baseURL != "https://api.custom.ai/v1" {
		t.Errorf("baseURL = %q, want https://api.custom.ai/v1", p.baseURL)
	}
}

func TestOpenAIEmbeddingProvider_DefaultBaseURL(t *testing.T) {
	p := NewOpenAIEmbeddingProvider("test-key", OpenAIEmbeddingSmall, 0)
	if p.baseURL != "https://api.openai.com/v1" {
		t.Errorf("baseURL = %q, want https://api.openai.com/v1", p.baseURL)
	}
}
