package linkunderstanding

import (
	"testing"
)

func TestDetectURLs(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single URL",
			input: "Check this out https://github.com/user/repo",
			want:  []string{"https://github.com/user/repo"},
		},
		{
			name:  "multiple URLs",
			input: "See https://example.com and http://test.org/page",
			want:  []string{"https://example.com", "http://test.org/page"},
		},
		{
			name:  "no URLs",
			input: "Just regular text without any links",
			want:  nil,
		},
		{
			name:  "URL with trailing punctuation",
			input: "Check https://example.com, and also https://test.org.",
			want:  []string{"https://example.com", "https://test.org"},
		},
		{
			name:  "duplicate URLs",
			input: "https://example.com and https://example.com again",
			want:  []string{"https://example.com"},
		},
		{
			name:  "URL with query params",
			input: "https://api.example.com/v1/search?q=test&limit=10",
			want:  []string{"https://api.example.com/v1/search?q=test&limit=10"},
		},
		{
			name:  "URL in markdown link",
			input: "Check [this](https://example.com) out",
			want:  []string{"https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectURLs(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("DetectURLs(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("DetectURLs(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple paragraph",
			input: "<p>Hello world</p>",
			want:  "Hello world",
		},
		{
			name:  "with script block",
			input: "<script>alert('xss')</script><p>Content</p>",
			want:  "Content",
		},
		{
			name:  "with entities",
			input: "<p>Hello &amp; goodbye</p>",
			want:  "Hello & goodbye",
		},
		{
			name:  "nested tags",
			input: "<div><p>Nested <b>bold</b> text</p></div>",
			want:  "Nested bold text",
		},
		{
			name:  "empty",
			input: "",
			want:  "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTML(tt.input)
			if got != tt.want {
				t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestExtractTitle(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"<html><head><title>My Page</title></head></html>", "My Page"},
		{"<title>Test</title>", "Test"},
		{"<html><body>No title</body></html>", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractTitle(tt.input)
		if got != tt.want {
			t.Errorf("extractTitle(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatForPrompt(t *testing.T) {
	links := []DetectedLink{
		{
			URL:     "https://example.com",
			Title:   "Example Page",
			Content: "This is the page content.",
		},
		{
			URL:   "https://failed.com",
			Error: "fetch failed: timeout",
		},
	}

	result := FormatForPrompt(links)
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !contains(result, "## Fetched Link Content") {
		t.Error("expected header")
	}
	if !contains(result, "https://example.com") {
		t.Error("expected first URL")
	}
	if !contains(result, "Example Page") {
		t.Error("expected title")
	}
	if !contains(result, "This is the page content") {
		t.Error("expected content")
	}
	if !contains(result, "fetch failed: timeout") {
		t.Error("expected error for failed URL")
	}
}

func TestFormatForPrompt_Empty(t *testing.T) {
	result := FormatForPrompt(nil)
	if result != "" {
		t.Error("expected empty string for no links")
	}
}

func TestProcessLinks_NoURLs(t *testing.T) {
	result := ProcessLinks("just text", 3, 1000, 5000000000)
	if result != "" {
		t.Error("expected empty string for text without URLs")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || containsStr(s, substr))
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}