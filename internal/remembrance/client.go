// Package remembrance provides an HTTP client for the Remembrance memory layer.
//
// V2 adds: entity management, graph traversal, dream cycle, hybrid search,
// and context building. The client communicates with Remembrance's REST API
// (default: http://localhost:8788).
//
// All V1 endpoints are preserved. V2 adds new endpoints for the knowledge
// graph, fact store, dream cycle, and hybrid search.
package remembrance

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ── V1 Types ───────────────────────────────────────────────────

// ContextResponse represents the response from Remembrance's context-build endpoint.
type ContextResponse struct {
	Context string          `json:"context"`
	Sources []ContextSource `json:"sources,omitempty"`
	Meta    map[string]any  `json:"meta,omitempty"`
}

// ContextSource represents a single memory source used in context building.
type ContextSource struct {
	ID       string  `json:"id"`
	Category string  `json:"category"`
	Tier     string  `json:"tier"`
	Snippet  string  `json:"snippet"`
	Score    float64 `json:"score"`
}

// ── V2 Types ────────────────────────────────────────────────────

// CaptureResponse represents the response from a memory capture.
type CaptureResponse struct {
	ID           string   `json:"id"`
	Decision     string   `json:"decision"`
	Confidence   float64  `json:"confidence"`
	Backend      string   `json:"backend"`
	Category     string   `json:"category"`
	Tier         string   `json:"tier"`
	Summary      string   `json:"summary"`
	Topics       []string `json:"topics"`
	Entities     []string `json:"entities"`
	NewEntities  []string `json:"new_entities"`
	EdgesCreated int      `json:"edges_created"`
}

// SearchResponse represents the response from a hybrid search.
type SearchResponse struct {
	Results []map[string]any `json:"results"`
	Count   int              `json:"count"`
}

// EntityResponse represents an entity with compiled truth and timeline.
type EntityResponse struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	Aliases       []string `json:"aliases"`
	CompiledTruth string   `json:"compiled_truth"`
	Timeline      string   `json:"timeline"`
	Tier          string   `json:"tier"`
	CreatedAt     float64  `json:"created_at"`
	UpdatedAt     float64  `json:"updated_at"`
	Edges         []EdgeResponse `json:"edges,omitempty"`
}

// EdgeResponse represents a typed relationship between entities.
type EdgeResponse struct {
	SourceID  string  `json:"source_id"`
	TargetID  string  `json:"target_id"`
	EdgeType  string  `json:"edge_type"`
	SinceDate float64 `json:"since_date"`
	Confidence float64 `json:"confidence"`
	Evidence  string  `json:"evidence"`
}

// GraphResponse represents a graph traversal result.
type GraphResponse struct {
	Center    string           `json:"center"`
	Entities  []EntityResponse `json:"entities"`
	Edges     []EdgeResponse   `json:"edges"`
}

// DreamResponse represents the result of a dream cycle run.
type DreamResponse struct {
	LogID       string `json:"log_id"`
	Status      string `json:"status"`
	DurationMs  int    `json:"duration_ms"`
	Phases      []any  `json:"phases"`
	Totals      map[string]any `json:"totals"`
}

// ContextBuildResponse represents the full context for a task.
type ContextBuildResponse struct {
	Query        string           `json:"query"`
	Project      string           `json:"project,omitempty"`
	Agent        string           `json:"agent,omitempty"`
	Memories     []map[string]any `json:"memories"`
	Entities     []map[string]any `json:"entities"`
	OpenThreads  []map[string]any `json:"open_threads"`
	TotalResults int              `json:"total_results"`
}

// ── Client ───────────────────────────────────────────────────────

// Client is an HTTP client for the Remembrance memory layer.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewClient creates a new Remembrance client with default timeout.
func NewClient(baseURL string) *Client {
	return NewClientWithTimeout(baseURL, 30*time.Second)
}

// NewClientWithTimeout creates a new Remembrance client with custom timeout.
func NewClientWithTimeout(baseURL string, timeout time.Duration) *Client {
	return &Client{
		BaseURL: strings.TrimSuffix(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ── V1 Methods ──────────────────────────────────────────────────

// BuildContext requests context from Remembrance for a task.
// V1 compatibility: calls GET /context/build?task=<task>&project=<project>&agent=<agent>
func (c *Client) BuildContext(task, project, agent string, limit int) (*ContextBuildResponse, error) {
	params := url.Values{}
	params.Set("task", task)
	if project != "" {
		params.Set("project", project)
	}
	if agent != "" {
		params.Set("agent", agent)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	reqURL := fmt.Sprintf("%s/context/build?%s", c.BaseURL, params.Encode())
	var resp ContextBuildResponse
	if err := c.doGet(reqURL, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// IsAvailable checks if Remembrance is reachable.
func (c *Client) IsAvailable() bool {
	resp, err := c.HTTPClient.Get(c.BaseURL + "/v1/health")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// ── V2 Methods ──────────────────────────────────────────────────

// Capture sends text through the full pipeline (gate → extract → graph → store).
func (c *Client) Capture(text, source, category, tier string) (map[string]any, error) {
	body := map[string]any{
		"text":   text,
		"source": source,
	}
	if category != "" {
		body["category"] = category
	}
	if tier != "" {
		body["tier"] = tier
	}
	return c.doPost(c.BaseURL+"/capture", body)
}

// Search performs hybrid search (FTS5 + vector + graph + RRF).
func (c *Client) Search(query, mode, category, tier string, limit int) (map[string]any, error) {
	params := url.Values{}
	params.Set("q", query)
	if mode != "" {
		params.Set("mode", mode)
	}
	if category != "" {
		params.Set("category", category)
	}
	if tier != "" {
		params.Set("tier", tier)
	}
	if limit > 0 {
		params.Set("limit", strconv.Itoa(limit))
	}

	reqURL := fmt.Sprintf("%s/search?%s", c.BaseURL, params.Encode())
	return c.doGetMap(reqURL)
}

// EntityGet retrieves an entity by name (compiled truth + timeline).
func (c *Client) EntityGet(name string) (map[string]any, error) {
	reqURL := fmt.Sprintf("%s/entity/%s", c.BaseURL, url.PathEscape(name))
	return c.doGetMap(reqURL)
}

// GraphQuery performs N-hop graph traversal from an entity.
func (c *Client) GraphQuery(entity string, depth int, edgeTypes []string) (map[string]any, error) {
	params := url.Values{}
	params.Set("depth", strconv.Itoa(depth))
	if len(edgeTypes) > 0 {
		params.Set("edge_types", strings.Join(edgeTypes, ","))
	}

	reqURL := fmt.Sprintf("%s/graph/%s?%s", c.BaseURL, url.PathEscape(entity), params.Encode())
	return c.doGetMap(reqURL)
}

// Dream triggers the dream cycle manually.
func (c *Client) Dream(phases []string, dryRun bool) (map[string]any, error) {
	body := map[string]any{}
	if len(phases) > 0 {
		body["phases"] = phases
	}
	body["dry_run"] = dryRun
	return c.doPost(c.BaseURL+"/dream", body)
}

// ── HTTP Helpers ─────────────────────────────────────────────────

func (c *Client) doGet(reqURL string, target any) error {
	resp, err := c.HTTPClient.Get(reqURL)
	if err != nil {
		return fmt.Errorf("remembrance request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found")
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("remembrance returned %d: %s", resp.StatusCode, string(body))
	}

	return json.NewDecoder(resp.Body).Decode(target)
}

func (c *Client) doGetMap(reqURL string) (map[string]any, error) {
	var result map[string]any
	if err := c.doGet(reqURL, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func (c *Client) doPost(reqURL string, body map[string]any) (map[string]any, error) {
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := c.HTTPClient.Post(reqURL, "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("remembrance request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("remembrance returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}
	return result, nil
}