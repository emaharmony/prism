// Package ollama provides an LLM provider for local Ollama instances.
//
// OllamaProvider calls Ollama's native /api/generate endpoint using raw HTTP.
// No SDK dependency — just net/http and encoding/json.
//
// For OpenAI-compatible Ollama mode, use the openai package with
// NewWithBaseURL("http://localhost:11434/v1") instead.
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/emaharmony/prism/internal/provider"
)

// Name is the provider name reported in responses.
const Name = "ollama"

// DefaultBaseURL is the default Ollama API endpoint.
const DefaultBaseURL = "http://localhost:11434"

// DefaultHTTPTimeout is the timeout for individual HTTP calls.
// Local inference can be slow, especially for large models.
const DefaultHTTPTimeout = 5 * time.Minute

// Provider generates completions via a local Ollama instance.
type Provider struct {
	BaseURL    string
	HTTPClient *http.Client
}

// New creates an OllamaProvider with sensible defaults.
// If baseURL is empty it defaults to http://localhost:11434.
func New(baseURL string) *Provider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultBaseURL
	}
	return &Provider{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: DefaultHTTPTimeout},
	}
}

// ---------- request / response types for /api/generate ----------

type generateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Options generateOptions `json:"options,omitempty"`
}

type generateOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type generateResponse struct {
	Response        string `json:"response"`
	Model           string `json:"model"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	TotalDuration   int64  `json:"total_duration"`
	Done            bool   `json:"done"`
	Error           string `json:"error,omitempty"`
}

// ---------- Provider interface ----------

// Generate sends a completion request to Ollama and returns the result.
func (o *Provider) Generate(ctx context.Context, req provider.GenerateRequest) (provider.GenerateResponse, error) {
	start := time.Now()

	body := generateRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		Stream: false,
		Options: generateOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.HTTPClient.Do(httpReq)
	if err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return provider.GenerateResponse{}, fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var oResp generateResponse
	if err := json.Unmarshal(respBody, &oResp); err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	if oResp.Error != "" {
		return provider.GenerateResponse{}, fmt.Errorf("ollama: %s", oResp.Error)
	}

	latency := time.Since(start).Milliseconds()

	promptTokens := oResp.PromptEvalCount
	outputTokens := oResp.EvalCount
	if promptTokens <= 0 {
		promptTokens = len(req.Prompt) / 4
	}
	if outputTokens <= 0 {
		outputTokens = len(oResp.Response) / 4
	}

	return provider.GenerateResponse{
		Text:         oResp.Response,
		Model:        oResp.Model,
		Provider:     Name,
		LatencyMS:    latency,
		PromptTokens: promptTokens,
		OutputTokens: outputTokens,
		Raw: map[string]any{
			"prompt_eval_count": oResp.PromptEvalCount,
			"eval_count":        oResp.EvalCount,
			"total_duration_ns": oResp.TotalDuration,
		},
	}, nil
}

// Compile-time interface check.
var _ provider.Provider = (*Provider)(nil)