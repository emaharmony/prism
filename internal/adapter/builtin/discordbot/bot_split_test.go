package discordbot

import (
	"testing"
	"unicode/utf8"
)

func TestSplitMessage_ShortMessage(t *testing.T) {
	result := SplitMessage("hello", 2000)
	if len(result) != 1 || result[0] != "hello" {
		t.Errorf("expected single chunk 'hello', got %v", result)
	}
}

func TestSplitMessage_MultipleNewlines(t *testing.T) {
	content := "line1\nline2\nline3\nline4\nline5"
	result := SplitMessage(content, 10)
	if len(result) < 2 {
		t.Errorf("expected multiple chunks, got %d", len(result))
	}
	// Verify no content is lost
	joined := ""
	for _, chunk := range result {
		joined += chunk
	}
	if joined != content {
		t.Errorf("content lost after splitting: got %q, want %q", joined, content)
	}
}

func TestSplitMessage_UTF8Boundary(t *testing.T) {
	// Create a string with multi-byte characters (emoji)
	// Each emoji is 4 bytes. Create a string that exceeds the chunk limit.
	content := "🎉🎊🎁🎈" + "hello world" + "🎉🎊🎁🎈"
	// Force splitting within the multi-byte section
	result := SplitMessage(content, 15)
	for _, chunk := range result {
		// Verify no chunk starts or ends with a partial rune
		if !utf8.ValidString(chunk) {
			t.Errorf("invalid UTF-8 in chunk: %q", chunk)
		}
	}
	// Verify no content is lost
	joined := ""
	for _, chunk := range result {
		joined += chunk
	}
	if joined != content {
		t.Errorf("content lost after UTF-8 splitting: got %q, want %q", joined, content)
	}
}

func TestSplitMessage_MaxLenZero(t *testing.T) {
	result := SplitMessage("hello world", 0)
	// Should fall back to MessageChunkLimit (1900)
	if len(result) != 1 || result[0] != "hello world" {
		t.Errorf("expected single chunk with default limit, got %v", result)
	}
}

func TestSplitMessage_ExactLimit(t *testing.T) {
	// Content exactly at MessageLimit (2000 chars)
	content := ""
	for i := 0; i < 200; i++ {
		content += "abcdefghij"
	}
	if len(content) != 2000 {
		t.Fatalf("test setup: expected 2000 chars, got %d", len(content))
	}
	result := SplitMessage(content, MessageLimit)
	if len(result) != 1 {
		t.Errorf("expected single chunk for exact-limit message, got %d chunks", len(result))
	}
}

func TestSplitMessage_JustOverLimit(t *testing.T) {
	// Content just over MessageLimit (2001 chars)
	content := ""
	for i := 0; i < 200; i++ {
		content += "abcdefghij"
	}
	content += "x" // 2001 chars
	result := SplitMessage(content, MessageLimit)
	if len(result) != 2 {
		t.Errorf("expected 2 chunks for just-over-limit message, got %d", len(result))
	}
	joined := ""
	for _, chunk := range result {
		joined += chunk
	}
	if joined != content {
		t.Errorf("content lost: got %d chars, want %d", len(joined), len(content))
	}
}

func TestSafeSplitIndex(t *testing.T) {
	tests := []struct {
		name    string
		content string
		maxLen  int
		want    int
	}{
		{"short content", "hello", 10, 5},
		{"exact length", "hello", 5, 5},
		{"at ASCII boundary", "hello world", 5, 5},
		{"within multi-byte", "hello 🎉 world", 7, 6}, // 🎉 starts at byte 6 (4 bytes)
		{"empty content", "", 10, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := safeSplitIndex(tt.content, tt.maxLen)
			if got != tt.want {
				t.Errorf("safeSplitIndex(%q, %d) = %d, want %d", tt.content, tt.maxLen, got, tt.want)
			}
			// Verify the split point is a valid rune boundary
			if got > 0 && got < len(tt.content) {
				if !utf8.RuneStart(tt.content[got]) {
					t.Errorf("split point %d is not a rune start in %q", got, tt.content)
				}
			}
		})
	}
}
