package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// research_tools.go provides the read-only RESEARCH-phase tools: web_search and
// memory_search. Both are PolicyApproved (no filesystem mutation). They let the
// gated loop's PROBE/RESEARCH phases reach outside the codebase — the web and
// Prism's Remembrance memory — instead of only grepping local files.

// MemorySearcher is the minimal surface the memory_search tool needs. The
// *remembrance.Client satisfies it; using an interface here avoids a
// tool→remembrance import cycle.
type MemorySearcher interface {
	Search(query, mode, category, tier string, limit int) (map[string]any, error)
}

// MemorySearchTool queries Prism's Remembrance memory service mid-loop.
type MemorySearchTool struct {
	Searcher MemorySearcher
}

func (t *MemorySearchTool) Name() string { return "memory_search" }
func (t *MemorySearchTool) Description() string {
	return "Searches Prism's long-term memory (Remembrance) for relevant past context, decisions, and facts."
}
func (t *MemorySearchTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"query": {Type: "string", Description: "What to search memory for", Required: true},
			"limit": {Type: "number", Description: "Max results (default 5)", Required: false},
		},
		Output: ParamSpec{Type: "object", Description: "Matching memories with scores and snippets"},
	}
}
func (t *MemorySearchTool) Execute(_ context.Context, input map[string]any) (ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return ToolResult{Success: false, Error: "required parameter 'query' must be a non-empty string"}, nil
	}
	if t.Searcher == nil {
		return ToolResult{Success: false, Error: "memory search is not configured (Remembrance disabled)"}, nil
	}
	limit := 5
	if l, ok := input["limit"].(float64); ok && l > 0 {
		limit = int(l)
	}
	results, err := t.Searcher.Search(query, "hybrid", "", "", limit)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("memory search failed: %v", err)}, nil
	}
	return ToolResult{Success: true, Output: results}, nil
}

// WebSearchConfig configures the web_search tool. It targets a generic JSON
// search endpoint so it stays provider-agnostic: any service that accepts a
// query string and returns JSON works (Brave, Serper, SearXNG, etc.).
type WebSearchConfig struct {
	// Endpoint is the search API base URL. The query is added as the QueryParam.
	// Read from PRISM_WEBSEARCH_URL when empty.
	Endpoint string
	// QueryParam is the query string parameter name (default "q").
	QueryParam string
	// APIKey, when set, is sent as a Bearer token. Read from PRISM_WEBSEARCH_KEY.
	APIKey string
	// AuthHeader overrides the header name for the key (default "Authorization").
	AuthHeader string
}

// WebSearchTool performs a web search against a configured JSON endpoint.
type WebSearchTool struct {
	Config WebSearchConfig
	Client *http.Client
}

func (t *WebSearchTool) Name() string { return "web_search" }
func (t *WebSearchTool) Description() string {
	return "Searches the web for up-to-date information. Returns search results as JSON."
}
func (t *WebSearchTool) Schema() ToolSchema {
	return ToolSchema{
		Input: map[string]ParamSpec{
			"query": {Type: "string", Description: "The web search query", Required: true},
		},
		Output: ParamSpec{Type: "object", Description: "Search results from the configured provider"},
	}
}
func (t *WebSearchTool) Execute(ctx context.Context, input map[string]any) (ToolResult, error) {
	query, ok := input["query"].(string)
	if !ok || strings.TrimSpace(query) == "" {
		return ToolResult{Success: false, Error: "required parameter 'query' must be a non-empty string"}, nil
	}

	endpoint := t.Config.Endpoint
	if endpoint == "" {
		endpoint = os.Getenv("PRISM_WEBSEARCH_URL")
	}
	if endpoint == "" {
		return ToolResult{Success: false, Error: "web search is not configured — set PRISM_WEBSEARCH_URL (and PRISM_WEBSEARCH_KEY if required)"}, nil
	}
	apiKey := t.Config.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("PRISM_WEBSEARCH_KEY")
	}
	queryParam := t.Config.QueryParam
	if queryParam == "" {
		queryParam = "q"
	}
	authHeader := t.Config.AuthHeader
	if authHeader == "" {
		authHeader = "Authorization"
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("invalid PRISM_WEBSEARCH_URL: %v", err)}, nil
	}
	q := u.Query()
	q.Set(queryParam, query)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return ToolResult{Success: false, Error: err.Error()}, nil
	}
	req.Header.Set("Accept", "application/json")
	if apiKey != "" {
		if authHeader == "Authorization" {
			req.Header.Set(authHeader, "Bearer "+apiKey)
		} else {
			req.Header.Set(authHeader, apiKey)
		}
	}

	client := t.Client
	if client == nil {
		client = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return ToolResult{Success: false, Error: fmt.Sprintf("web search request failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ToolResult{Success: false, Error: fmt.Sprintf("web search returned %d: %s", resp.StatusCode, truncateStr(string(body), 500))}, nil
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		// Non-JSON response — return raw text.
		return ToolResult{Success: true, Output: map[string]any{"results": string(body)}}, nil
	}
	return ToolResult{Success: true, Output: map[string]any{"results": parsed}}, nil
}

// RegisterResearchTools adds web_search and memory_search to the registry.
// Pass a nil searcher to register memory_search in a disabled state.
func RegisterResearchTools(registry *Registry, searcher MemorySearcher, webCfg WebSearchConfig) *Registry {
	registry.Register(&MemorySearchTool{Searcher: searcher})
	registry.Register(&WebSearchTool{Config: webCfg})
	return registry
}

// truncateStr shortens a string for inclusion in an error message.
func truncateStr(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
