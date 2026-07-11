package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// OllamaEmbeddingProvider generates embeddings using a local Ollama instance.
// This is the zero-cost option: run embeddings locally with no API key needed.
type OllamaEmbeddingProvider struct {
	baseURL    string
	model      string
	dim        int // cached dimension, 0 = unknown
	dimOnce    sync.Once
	httpClient *http.Client
}

// NewOllamaEmbeddingProvider creates an Ollama embedding provider.
func NewOllamaEmbeddingProvider(baseURL, model string) *OllamaEmbeddingProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	return &OllamaEmbeddingProvider{
		baseURL: baseURL,
		model:   model,
		httpClient: &http.Client{
			Timeout: 60 * time.Second, // Local inference can be slow
		},
	}
}

// ollamaEmbedRequest is the request body for Ollama's /api/embeddings endpoint.
type ollamaEmbedRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// ollamaEmbedResponse is the response from Ollama's embeddings API.
type ollamaEmbedResponse struct {
	Embedding []float64 `json:"embedding"`
}

// ollamaEmbedBatchRequest is the request body for Ollama's /api/embed endpoint (batch).
type ollamaEmbedBatchRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

// ollamaEmbedBatchResponse is the batch response from Ollama's /api/embed endpoint.
type ollamaEmbedBatchResponse struct {
	Embeddings [][]float64 `json:"embeddings"`
}

// Embed generates an embedding for a single text.
func (o *OllamaEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	reqBody := ollamaEmbedRequest{
		Model:  o.model,
		Prompt: text,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama embedding: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/api/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("ollama embedding: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embedding: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama embedding: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama embedding: %s: %s", resp.Status, string(body))
	}

	var embedResp ollamaEmbedResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("ollama embedding: decode response: %w", err)
	}

	return embedResp.Embedding, nil
}

// EmbedBatch generates embeddings for multiple texts.
// Ollama doesn't have a native batch endpoint, so we embed one at a time.
func (o *OllamaEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	results := make([][]float64, len(texts))
	for i, text := range texts {
		embed, err := o.Embed(ctx, text)
		if err != nil {
			return nil, fmt.Errorf("ollama embedding: batch item %d: %w", i, err)
		}
		results[i] = embed
	}
	return results, nil
}

// Dimension returns the embedding dimension.
// For Ollama, we need to embed a test string to find out.
// Uses sync.Once for thread-safe lazy initialization.
func (o *OllamaEmbeddingProvider) Dimension() int {
	o.dimOnce.Do(func() {
		embed, err := o.Embed(context.Background(), "dimension probe")
		if err == nil {
			o.dim = len(embed)
		}
	})
	return o.dim
}

// Name returns the provider name.
func (o *OllamaEmbeddingProvider) Name() string {
	return "ollama"
}
