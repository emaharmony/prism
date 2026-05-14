package agent

import (
	"testing"
)

func TestParseFinalResponse(t *testing.T) {
	input := `{"type": "final", "content": "The project uses event-driven architecture."}`
	resp := ParseAgentOutput(input)

	if resp.Type != ResponseFinal {
		t.Errorf("expected type 'final', got %s", resp.Type)
	}
	if resp.Content != "The project uses event-driven architecture." {
		t.Errorf("unexpected content: %s", resp.Content)
	}
}

func TestParseToolRequest(t *testing.T) {
	input := `{"type": "tool_request", "tool": "read_file", "input": {"path": "README.md"}}`
	resp := ParseAgentOutput(input)

	if resp.Type != ResponseToolRequest {
		t.Errorf("expected type 'tool_request', got %s", resp.Type)
	}
	if resp.ToolName != "read_file" {
		t.Errorf("expected tool 'read_file', got %s", resp.ToolName)
	}
	if resp.ToolInput["path"] != "README.md" {
		t.Errorf("expected input path 'README.md', got %v", resp.ToolInput["path"])
	}
}

func TestParseInvalidJSON(t *testing.T) {
	input := "This is just plain text, not JSON."
	resp := ParseAgentOutput(input)

	if resp.Type != ResponseFinal {
		t.Errorf("invalid JSON should fall back to 'final', got %s", resp.Type)
	}
	if resp.Content != input {
		t.Errorf("fallback content should be the raw input, got %s", resp.Content)
	}
}

func TestParseUnknownType(t *testing.T) {
	input := `{"type": "unknown_type", "data": "something"}`
	resp := ParseAgentOutput(input)

	if resp.Type != ResponseFinal {
		t.Errorf("unknown type should fall back to 'final', got %s", resp.Type)
	}
}

func TestParseToolRequestMissingToolName(t *testing.T) {
	input := `{"type": "tool_request", "input": {"path": "README.md"}}`
	resp := ParseAgentOutput(input)

	// Missing tool name should fall back to final response
	if resp.Type != ResponseFinal {
		t.Errorf("tool_request without tool name should fall back to 'final', got %s", resp.Type)
	}
}

func TestParseToolRequestWithEchoTool(t *testing.T) {
	input := `{"type": "tool_request", "tool": "echo", "input": {"text": "hello"}}`
	resp := ParseAgentOutput(input)

	if resp.Type != ResponseToolRequest {
		t.Errorf("expected type 'tool_request', got %s", resp.Type)
	}
	if resp.ToolName != "echo" {
		t.Errorf("expected tool 'echo', got %s", resp.ToolName)
	}
	if resp.ToolInput["text"] != "hello" {
		t.Errorf("expected input text 'hello', got %v", resp.ToolInput["text"])
	}
}

func TestParseJSONWithMarkdownFence(t *testing.T) {
	input := "```json\n{\"type\": \"final\", \"content\": \"Hello world\"}\n```"
	resp := ParseAgentOutput(input)

	if resp.Type != ResponseFinal {
		t.Errorf("expected type 'final', got %s", resp.Type)
	}
	if resp.Content != "Hello world" {
		t.Errorf("expected content 'Hello world', got %s", resp.Content)
	}
}

func TestParseToolRequestWithMarkdownFence(t *testing.T) {
	input := "```json\n{\"type\": \"tool_request\", \"tool\": \"list_dir\", \"input\": {\"path\": \".\"}}\n```"
	resp := ParseAgentOutput(input)

	if resp.Type != ResponseToolRequest {
		t.Errorf("expected type 'tool_request', got %s", resp.Type)
	}
	if resp.ToolName != "list_dir" {
		t.Errorf("expected tool 'list_dir', got %s", resp.ToolName)
	}
}

func TestBuildToolPromptSuffix(t *testing.T) {
	suffix := BuildToolPromptSuffix([]string{"echo", "read_file", "list_dir", "write_file_dry_run"})

	if suffix == "" {
		t.Error("tool prompt suffix should not be empty")
	}
	if !containsStr(suffix, "echo") {
		t.Error("tool prompt suffix should list available tools including echo")
	}
	if !containsStr(suffix, "tool_request") {
		t.Error("tool prompt suffix should explain tool_request format")
	}
	if !containsStr(suffix, "final") {
		t.Error("tool prompt suffix should explain final format")
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}