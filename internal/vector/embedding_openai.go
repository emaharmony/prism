package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIEmbeddingProvider generates embeddings using OpenAI's API.
// Supports text-embedding-3-small (default) and text-embedding-3-large.
type OpenAIEmbeddingProvider struct {
	apiKey     string
	baseURL    string
	model      string
	dim        int
	httpClient *http.Client
}

// OpenAI embedding models and their default dimensions.
const (
	OpenAIEmbeddingSmall      = "text-embedding-3-small"
	OpenAIEmbeddingLarge      = "text-embedding-3-large"
	OpenAIEmbeddingAda002     = "text-embedding-ada-002"
	OpenAIEmbeddingSmallDim   = 1536
	OpenAIEmbeddingLargeDim   = 3072
	OpenAIEmbeddingAda002Dim  = 1536
)

// NewOpenAIEmbeddingProvider creates an OpenAI embedding provider.
// If dimension is 0, uses the model's default dimension.
func NewOpenAIEmbeddingProvider(apiKey, model string, dimension int) *OpenAIEmbeddingProvider {
	if dimension <= 0 {
		switch model {
		case OpenAIEmbeddingSmall, OpenAIEmbeddingAda002:
			dimension = OpenAIEmbeddingSmallDim
		case OpenAIEmbeddingLarge:
			dimension = OpenAIEmbeddingLargeDim
		default:
			dimension = OpenAIEmbeddingSmallDim
		}
	}
	return &OpenAIEmbeddingProvider{
		apiKey:  apiKey,
		baseURL: "https://api.openai.com/v1",
		model:   model,
		dim:     dimension,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// NewOpenAIEmbeddingProviderWithBaseURL creates an OpenAI embedding provider
// with a custom base URL (for OpenAI-compatible APIs).
func NewOpenAIEmbeddingProviderWithBaseURL(apiKey, baseURL, model string, dimension int) *OpenAIEmbeddingProvider {
	p := NewOpenAIEmbeddingProvider(apiKey, model, dimension)
	p.baseURL = baseURL
	return p
}

// openAIEmbeddingRequest is the request body for the embeddings API.
type openAIEmbeddingRequest struct {
	Model     string `json:"model"`
	Input     any    `json:"input"` // string or []string
	Dimensions int   `json:"dimensions,omitempty"`
}

// openAIEmbeddingResponse is the response from the embeddings API.
type openAIEmbeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Model string `json:"model"`
	Usage struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// openAIErrorResponse is an error response from the OpenAI API.
type openAIErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// Embed generates an embedding for a single text.
func (o *OpenAIEmbeddingProvider) Embed(ctx context.Context, text string) ([]float64, error) {
	results, err := o.EmbedBatch(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("openai embedding: no results returned")
	}
	return results[0], nil
}

// EmbedBatch generates embeddings for multiple texts.
func (o *OpenAIEmbeddingProvider) EmbedBatch(ctx context.Context, texts []string) ([][]float64, error) {
	reqBody := openAIEmbeddingRequest{
		Model:      o.model,
		Input:      texts,
		Dimensions: o.dim,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("openai embedding: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+"/embeddings", bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("openai embedding: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+o.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("openai embedding: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai embedding: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp openAIErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("openai embedding: %s: %s", resp.Status, errResp.Error.Message)
		}
		return nil, fmt.Errorf("openai embedding: %s", resp.Status)
	}

	var embedResp openAIEmbeddingResponse
	if err := json.Unmarshal(body, &embedResp); err != nil {
		return nil, fmt.Errorf("openai embedding: decode response: %w", err)
	}

	results := make([][]float64, len(embedResp.Data))
	for i, d := range embedResp.Data {
		results[i] = d.Embedding
	}

	return results, nil
}

// Dimension returns the embedding dimension.
func (o *OpenAIEmbeddingProvider) Dimension() int {
	return o.dim
}

// Name returns the provider name.
func (o *OpenAIEmbeddingProvider) Name() string {
	return "openai"
}