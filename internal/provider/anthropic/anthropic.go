// Package anthropic provides an LLM provider for Anthropic's Messages API (Claude).
//
// Design decisions:
//   - Raw HTTP, no SDK. The Anthropic Go SDK adds unnecessary weight.
//   - Anthropic uses x-api-key header (not Bearer token) and requires anthropic-version.
//   - Streaming uses SSE with event types (message_start, content_block_delta, message_stop).
//   - Tier-based paid guard via TieredProvider interface.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/retry"
)

const (
	// DefaultBaseURL is the Anthropic Messages API endpoint.
	DefaultBaseURL = "https://api.anthropic.com"

	// APIVersion is the required anthropic-version header value.
	APIVersion = "2023-06-01"

	// Name is the provider identifier.
	Name = "anthropic"
)

// Provider calls Anthropic's Messages API using raw HTTP.
type Provider struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	TierVal    provider.ProviderTier
}

// New creates a new Anthropic provider with the given API key.
func New(apiKey string) *Provider {
	return &Provider{
		APIKey:  apiKey,
		BaseURL: DefaultBaseURL,
		HTTPClient: &http.Client{
			Timeout: 120 * time.Second,
		},
		TierVal: provider.TierPaid,
	}
}

// NewWithBaseURL creates a provider with a custom base URL (for proxies).
func NewWithBaseURL(apiKey, baseURL string) *Provider {
	return &Provider{
		APIKey:     apiKey,
		BaseURL:    baseURL,
		HTTPClient: &http.Client{Timeout: 120 * time.Second},
		TierVal:    provider.TierPaid,
	}
}

// Name returns the provider name.
func (p *Provider) Name() string { return Name }

// Tier returns the provider cost tier.
func (p *Provider) Tier() provider.ProviderTier { return p.TierVal }

// ---------- Anthropic request/response types ----------

type messagesRequest struct {
	Model     string        `json:"model"`
	MaxTokens int           `json:"max_tokens"`
	Messages  []anthropicMessage `json:"messages"`
	System    string        `json:"system,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	Stream    bool          `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type messagesResponse struct {
	ID      string              `json:"id"`
	Type    string              `json:"type"`
	Role    string              `json:"role"`
	Content []contentBlock      `json:"content"`
	Model   string              `json:"model"`
	Usage   anthropicUsage      `json:"usage"`
	StopReason string           `json:"stop_reason"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ---------- Generate ----------

// Generate calls the Anthropic Messages API synchronously.
func (p *Provider) Generate(ctx context.Context, req provider.GenerateRequest) (provider.GenerateResponse, error) {
	start := time.Now()

	anthReq := messagesRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
		Messages:  []anthropicMessage{{Role: "user", Content: req.Prompt}},
		Temperature: req.Temperature,
	}

	body, err := json.Marshal(anthReq)
	if err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("anthropic-version", APIVersion)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("anthropic: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		return provider.GenerateResponse{}, retry.NewRetryableError(fmt.Errorf("anthropic: rate limited (429)"))
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		return provider.GenerateResponse{}, retry.NewRetryableError(fmt.Errorf("anthropic: service unavailable (503)"))
	}
	if resp.StatusCode == http.StatusBadGateway {
		return provider.GenerateResponse{}, retry.NewRetryableError(fmt.Errorf("anthropic: bad gateway (502)"))
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return provider.GenerateResponse{}, fmt.Errorf("anthropic: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	var msgResp messagesResponse
	if err := json.NewDecoder(resp.Body).Decode(&msgResp); err != nil {
		return provider.GenerateResponse{}, fmt.Errorf("anthropic: decode response: %w", err)
	}

	// Extract text from content blocks
	text := ""
	for _, block := range msgResp.Content {
		if block.Type == "text" {
			text += block.Text
		}
	}

	if text == "" {
		return provider.GenerateResponse{}, fmt.Errorf("anthropic: no text content in response")
	}

	duration := time.Since(start).Milliseconds()

	return provider.GenerateResponse{
		Text:         text,
		Model:        msgResp.Model,
		Provider:     Name,
		LatencyMS:    duration,
		PromptTokens: msgResp.Usage.InputTokens,
		OutputTokens: msgResp.Usage.OutputTokens,
		Raw: map[string]any{
			"id":           msgResp.ID,
			"stop_reason":  msgResp.StopReason,
			"model":        msgResp.Model,
			"input_tokens": msgResp.Usage.InputTokens,
			"output_tokens": msgResp.Usage.OutputTokens,
		},
	}, nil
}

// IsRetryableError checks if an Anthropic error is retryable.
func IsRetryableError(err error) bool {
	return retry.IsRetryable(err)
}

// Compile-time interface check.
var _ provider.Provider = (*Provider)(nil)