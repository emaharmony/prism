// Package remembrance provides an HTTP client for the Remembrance memory layer.
// For V1, this is an optional context hook — failure is graceful unless
// --require-memory is passed.
package remembrance

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ContextResponse represents the response from Remembrance's context-build endpoint.
type ContextResponse struct {
	Context string            `json:"context"`
	Sources []ContextSource   `json:"sources,omitempty"`
	Meta    map[string]any    `json:"meta,omitempty"`
}

// ContextSource represents a single memory source used in context building.
type ContextSource struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Tier     string `json:"tier"`
	Snippet  string `json:"snippet"`
	Score    float64 `json:"score"`
}

// Client is an HTTP client for the Remembrance memory layer.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new Remembrance client.
func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		HTTPClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// BuildContext requests context from Remembrance for a task.
// It calls GET /context/build?task=<task>&project=<project>&agent=<agent>
func (c *Client) BuildContext(task, project, agent string) (*ContextResponse, error) {
	params := url.Values{}
	params.Set("task", task)
	if project != "" {
		params.Set("project", project)
	}
	if agent != "" {
		params.Set("agent", agent)
	}

	reqURL := fmt.Sprintf("%s/context/build?%s", c.BaseURL, params.Encode())

	resp, err := c.HTTPClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("remembrance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Endpoint doesn't exist — Remembrance may not have context for this
		return nil, nil
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("remembrance returned %d: %s", resp.StatusCode, string(body))
	}

	var ctxResp ContextResponse
	if err := json.NewDecoder(resp.Body).Decode(&ctxResp); err != nil {
		return nil, fmt.Errorf("failed to decode remembrance response: %w", err)
	}

	return &ctxResp, nil
}

// IsAvailable checks if Remembrance is reachable.
func (c *Client) IsAvailable() bool {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}