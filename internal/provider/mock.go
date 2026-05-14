package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

const MockProviderName = "mock"

// MockProvider always succeeds with deterministic output.
// Used for testing — no external dependencies.
type MockProvider struct {
	shouldFail bool
}

// NewMockProvider creates a MockProvider that always succeeds.
func NewMockProvider() *MockProvider {
	return &MockProvider{shouldFail: false}
}

// NewFailingMockProvider creates a MockProvider that always returns an error.
func NewFailingMockProvider() *MockProvider {
	return &MockProvider{shouldFail: true}
}

// Generate returns a deterministic response or an error depending on configuration.
func (m *MockProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	if m.shouldFail {
		return GenerateResponse{}, fmt.Errorf("mock provider configured to fail")
	}

	start := time.Now()

	// Simulate a brief processing delay
	select {
	case <-ctx.Done():
		return GenerateResponse{}, ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}

	latency := time.Since(start).Milliseconds()

	return GenerateResponse{
		Text:         fmt.Sprintf("Mock response for task '%s' by agent '%s' in project '%s'.", req.Task, req.Agent, req.Project),
		Model:        "mock-model",
		Provider:     MockProviderName,
		LatencyMS:    latency,
		PromptTokens: len(req.Prompt) / 4, // rough estimate
		OutputTokens: 50,
		Raw: map[string]any{
			"mock": true,
			"request_echo": map[string]string{
				"run_id":  req.RunID,
				"agent":   req.Agent,
				"project": req.Project,
				"task":    req.Task,
			},
		},
	}, nil
}

// ToolRequestMockProvider returns JSON simulating an LLM that requests a tool call.
// This is used for V3 tool execution tests.
type ToolRequestMockProvider struct {
	toolName  string
	toolInput map[string]any
}

// NewToolRequestMockProvider creates a provider that returns a tool_request JSON response.
func NewToolRequestMockProvider(toolName string, toolInput map[string]any) *ToolRequestMockProvider {
	return &ToolRequestMockProvider{
		toolName:  toolName,
		toolInput: toolInput,
	}
}

// Generate returns a JSON string representing a tool request.
func (p *ToolRequestMockProvider) Generate(ctx context.Context, req GenerateRequest) (GenerateResponse, error) {
	start := time.Now()
	select {
	case <-ctx.Done():
		return GenerateResponse{}, ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}
	latency := time.Since(start).Milliseconds()

	// Build tool_request JSON
	inputJSON, _ := json.Marshal(p.toolInput)
	text := fmt.Sprintf(`{"type": "tool_request", "tool": "%s", "input": %s}`, p.toolName, string(inputJSON))

	return GenerateResponse{
		Text:         text,
		Model:        "mock-model",
		Provider:     MockProviderName,
		LatencyMS:    latency,
		PromptTokens: len(req.Prompt) / 4,
		OutputTokens: 50,
	}, nil
}