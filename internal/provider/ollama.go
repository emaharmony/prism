package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// OllamaProviderName is the name this provider reports in responses.
	OllamaProviderName = "ollama"

	// DefaultOllamaBaseURL is the default Ollama API endpoint when none is configured.
	DefaultOllamaBaseURL = "http://localhost:11434"

	// DefaultOllamaHTTPTimeout is a sensible timeout for individual HTTP calls.
	DefaultOllamaHTTPTimeout = 5 * time.Minute
)

// OllamaProvider generates completions via a local Ollama instance.
// It implements the Provider interface.
type OllamaProvider struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewOllamaProvider returns an OllamaProvider with sensible defaults.
// If baseURL is empty it defaults to http://localhost:11434.
func NewOllamaProvider(baseURL string) *OllamaProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = DefaultOllamaBaseURL
	}
	return &OllamaProvider{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{Timeout: DefaultOllamaHTTPTimeout},
	}
}

// ---------- request / response types for the Ollama /api/generate endpoint ----------

type ollamaRequest struct {
	Model  string         `json:"model"`
	Prompt string         `json:"prompt"`
	Stream bool           `json:"stream"`
	Options ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	Temperature float64 `json:"temperature,omitempty"`
	NumPredict  int     `json:"num_predict,omitempty"`
}

type ollamaResponse struct {
	Response        string `json:"response"`
	Model           string `json:"model"`
	PromptEvalCount int    `json:"prompt_eval_count"`
	EvalCount       int    `json:"eval_count"`
	TotalDuration   int64  `json:"total_duration"` // nanoseconds
	Done            bool   `json:"done"`
	Error           string `json:"error,omitempty"`
}

// ---------- Provider interface ----------

// Generate sends a completion request to Ollama and returns the result.
func (o *OllamaProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	start := time.Now()

	// Build request body
	body := ollamaRequest{
		Model:  req.Model,
		Prompt: req.Prompt,
		Stream: false,
		Options: ollamaOptions{
			Temperature: req.Temperature,
			NumPredict:  req.MaxTokens,
		},
	}

	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.BaseURL+"/api/generate", bytes.NewReader(bodyBytes))
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("ollama: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := o.HTTPClient.Do(httpReq)
	if err != nil {
		// Context deadline exceeded and connection refused should not crash Prism —
		// they propagate as normal errors so the runner can emit llm.failed.
		return GenerateResponse{}, fmt.Errorf("ollama: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return GenerateResponse{}, fmt.Errorf("ollama: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var oResp ollamaResponse
	if err := json.Unmarshal(respBody, &oResp); err != nil {
		return GenerateResponse{}, fmt.Errorf("ollama: unmarshal response: %w", err)
	}

	// Ollama may return an error inside a 200 response (e.g. model not found).
	if oResp.Error != "" {
		return GenerateResponse{}, fmt.Errorf("ollama: %s", oResp.Error)
	}

	latency := time.Since(start).Milliseconds()

	// Token counts: use Ollama's numbers when available, fall back to rough estimate.
	promptTokens := oResp.PromptEvalCount
	outputTokens := oResp.EvalCount
	if promptTokens <= 0 {
		promptTokens = len(req.Prompt) / 4
	}
	if outputTokens <= 0 {
		outputTokens = len(oResp.Response) / 4
	}

	return GenerateResponse{
		Text:         oResp.Response,
		Model:        oResp.Model,
		Provider:     OllamaProviderName,
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

// Ensure OllamaProvider implements Provider at compile time.
var _ Provider = (*OllamaProvider)(nil)
