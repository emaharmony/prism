// Package provider implements Prism's LLM provider interface.
//
// V14c adds the OpenAI-compatible provider. This gives Prism access to GPT-4,
// GPT-3.5, and any OpenAI-compatible endpoint (Ollama's OpenAI compatibility
// mode, Azure OpenAI, Together AI, etc.).
//
// Design decisions:
//   - Raw HTTP, no SDK. The OpenAI Go SDK is 2MB. We need ~200 lines.
//   - Server-Sent Events (SSE) for streaming. No WebSockets.
//   - Tier-based paid guard. Providers have a Tier (Free/Paid) and the
//     chain skips paid providers unless --allow-paid-fallback is set.
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

	"github.com/emaharmony/prism/internal/retry"
)

// ProviderTier classifies providers by cost.
type ProviderTier string

const (
	TierFree ProviderTier = "free" // Local models (Ollama, mock)
	TierPaid ProviderTier = "paid" // Cloud APIs (OpenAI, etc.)
)

// OpenAIProvider calls OpenAI's chat completion API using raw HTTP.
// No SDK dependency — just net/http and encoding/json.
//
// It supports both synchronous Generate() and streaming GenerateStream().
// The streaming implementation uses Server-Sent Events (SSE) with
// 50ms batching for token events.
type OpenAIProvider struct {
	apiKey     string
	baseURL    string       // defaults to https://api.openai.com/v1
	httpClient *http.Client
	tier       ProviderTier // free or paid
}

// NewOpenAIProvider creates a new OpenAI provider with the given API key.
// The base URL defaults to https://api.openai.com/v1 but can be overridden
// for OpenAI-compatible endpoints (Azure, Together AI, etc.).
func NewOpenAIProvider(apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: "https://api.openai.com/v1",
		httpClient: &http.Client{
			Timeout: 120 * time.Second, // 2 minutes for long generations
		},
		tier: TierPaid,
	}
}

// NewOpenAIProviderWithBaseURL creates a provider with a custom base URL.
// Use this for OpenAI-compatible endpoints (Azure, Together AI, Ollama OpenAI mode).
func NewOpenAIProviderWithBaseURL(apiKey, baseURL string) *OpenAIProvider {
	return &OpenAIProvider{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		tier: TierPaid,
	}
}

// Name returns the provider name.
func (p *OpenAIProvider) Name() string {
	return "openai"
}

// Tier returns the provider tier (Free or Paid).
func (p *OpenAIProvider) Tier() ProviderTier {
	return p.tier
}

// chatCompletionRequest is the JSON body sent to OpenAI.
type chatCompletionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Stream      bool          `json:"stream,omitempty"`
}

// chatMessage is a single message in the chat completion request.
type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatCompletionResponse is the JSON response from OpenAI.
type chatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []chatCompletionChoice `json:"choices"`
	Usage   chatCompletionUsage    `json:"usage"`
}

// chatCompletionChoice is a single choice in the response.
type chatCompletionChoice struct {
	Index        int         `json:"index"`
	Message      chatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

// chatCompletionUsage is the token usage in the response.
type chatCompletionUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Generate calls the OpenAI chat completion API synchronously.
func (p *OpenAIProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	start := time.Now()

	chatReq := chatCompletionRequest{
		Model:       req.Model,
		Messages:    []chatMessage{{Role: "user", Content: req.Prompt}},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      false,
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return GenerateResponse{}, fmt.Errorf("openai: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return GenerateResponse{}, retry.NewRetryableError(fmt.Errorf("openai: rate limited (429)"))
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return GenerateResponse{}, retry.NewRetryableError(fmt.Errorf("openai: service unavailable (503)"))
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return GenerateResponse{}, fmt.Errorf("openai: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var chatResp chatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return GenerateResponse{}, fmt.Errorf("openai: decode response: %w", err)
	}

	if len(chatResp.Choices) == 0 {
		return GenerateResponse{}, fmt.Errorf("openai: no choices in response")
	}

	duration := time.Since(start).Milliseconds()

	return GenerateResponse{
		Text:         chatResp.Choices[0].Message.Content,
		Model:        chatResp.Model,
		Provider:     "openai",
		LatencyMS:    duration,
		PromptTokens: chatResp.Usage.PromptTokens,
		OutputTokens: chatResp.Usage.CompletionTokens,
		Raw: map[string]any{
			"id":             chatResp.ID,
			"finish_reason":  chatResp.Choices[0].FinishReason,
			"model":          chatResp.Model,
			"prompt_tokens":  chatResp.Usage.PromptTokens,
			"completion_tokens": chatResp.Usage.CompletionTokens,
		},
	}, nil
}

// GenerateStream calls the OpenAI chat completion API with streaming enabled.
// It returns a channel of TokenChunk batches. The caller reads from the
// channel until it's closed (successful completion) or an error is sent.
//
// Token chunks are batched every 50ms as per V14a design.
func (p *OpenAIProvider) GenerateStream(ctx context.Context, req GenerateRequest) (<-chan TokenChunk, error) {
	chatReq := chatCompletionRequest{
		Model:       req.Model,
		Messages:    []chatMessage{{Role: "user", Content: req.Prompt}},
		Temperature: req.Temperature,
		MaxTokens:   req.MaxTokens,
		Stream:      true, // Enable SSE streaming
	}

	body, err := json.Marshal(chatReq)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: request failed: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		return nil, retry.NewRetryableError(fmt.Errorf("openai: rate limited (429)"))
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		resp.Body.Close()
		return nil, retry.NewRetryableError(fmt.Errorf("openai: service unavailable (503)"))
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("openai: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan TokenChunk, 64)
	go p.readSSEStream(resp, ch)

	return ch, nil
}

// readSSEStream reads Server-Sent Events from the OpenAI response
// and batches tokens into 50ms intervals.
func (p *OpenAIProvider) readSSEStream(resp *http.Response, ch chan<- TokenChunk) {
	defer close(ch)
	defer resp.Body.Close()

	var tokens []string
	var index int
	lastBatch := time.Now()
	batchInterval := 50 * time.Millisecond

	decoder := json.NewDecoder(resp.Body)
	for decoder.More() {
		// Read each SSE line
		var line struct {
			Choices []struct {
				Delta struct {
					Content string `json:"content"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}

		if err := decoder.Decode(&line); err != nil {
			ch <- TokenChunk{Error: fmt.Errorf("openai: decode SSE: %w", err), Finished: true}
			return
		}

		for _, choice := range line.Choices {
			if choice.Delta.Content != "" {
				tokens = append(tokens, choice.Delta.Content)
				index++

				// Batch tokens every 50ms
				if time.Since(lastBatch) >= batchInterval {
					batchTokens := make([]string, len(tokens))
					copy(batchTokens, tokens)
					ch <- TokenChunk{
						Tokens:   batchTokens,
						Index:    index - len(tokens),
						Finished: false,
					}
					tokens = tokens[:0]
					lastBatch = time.Now()
				}
			}

			if choice.FinishReason != nil && *choice.FinishReason == "stop" {
				// Send remaining tokens
				if len(tokens) > 0 {
					batchTokens := make([]string, len(tokens))
					copy(batchTokens, tokens)
					ch <- TokenChunk{
						Tokens:   batchTokens,
						Index:    index - len(tokens),
						Finished: true,
					}
				} else {
					ch <- TokenChunk{Finished: true}
				}
				return
			}
		}
	}

	// Stream ended without finish_reason
	if len(tokens) > 0 {
		ch <- TokenChunk{Tokens: tokens, Index: index - len(tokens), Finished: true}
	} else {
		ch <- TokenChunk{Finished: true}
	}
}

// IsRetryableError checks if an error from the OpenAI provider is retryable.
// This wraps the generic IsRetryable function from the retry package
// with OpenAI-specific error classification.
func IsOpenAIErrorRetryable(err error) bool {
	return retry.IsRetryable(err)
}

// containsSubstring checks if s contains substr (case-sensitive).
// This is a local helper to avoid importing strings package for one function.
func containsSubstring(s, substr string) bool {
	return len(s) >= len(substr) && strings.Contains(s, substr)
}