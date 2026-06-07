package discordbot

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitMessageShort(t *testing.T) {
	content := "Hello, world!"
	chunks := splitMessage(content, 2000)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0] != content {
		t.Errorf("expected %q, got %q", content, chunks[0])
	}
}

func TestSplitMessageLong(t *testing.T) {
	// Create a message longer than maxLen
	content := strings.Repeat("line content here\n", 200) // ~4000 chars
	chunks := splitMessage(content, 1900)

	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}

	// Verify total content is preserved
	total := strings.Join(chunks, "")
	if total != content {
		t.Error("split message content doesn't match original")
	}

	// Verify each chunk is within limits (except possibly the last)
	for i, chunk := range chunks {
		if i < len(chunks)-1 && len(chunk) > 1900 {
			t.Errorf("chunk %d is %d chars, exceeds 1900", i, len(chunk))
		}
	}
}

func TestSplitMessageNoNewlines(t *testing.T) {
	// A long string with no newlines
	content := strings.Repeat("a", 5000)
	chunks := splitMessage(content, 1900)

	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}

	total := strings.Join(chunks, "")
	if total != content {
		t.Error("split message content doesn't match original")
	}
}

func TestSplitMessageUTF8Safe(t *testing.T) {
	content := strings.Repeat("\U0001F389", 1000)
	chunks := SplitMessage(content, 1901)

	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks, got %d", len(chunks))
	}

	total := strings.Join(chunks, "")
	if total != content {
		t.Error("split message content doesn't match original")
	}
	for i, chunk := range chunks {
		if !utf8.ValidString(chunk) {
			t.Errorf("chunk %d is not valid UTF-8", i)
		}
		if len(chunk) > 1901 {
			t.Errorf("chunk %d is %d bytes, exceeds 1901", i, len(chunk))
		}
	}
}

func TestInboundMessageFields(t *testing.T) {
	msg := &InboundMessage{
		ChannelID: "12345",
		UserID:    "67890",
		UserName:  "testuser",
		Content:   "Hello!",
		GuildID:   "99999",
		IsDM:      false,
		MessageID: "abc123",
	}

	if msg.ChannelID != "12345" {
		t.Errorf("expected ChannelID '12345', got %q", msg.ChannelID)
	}
	if msg.IsDM {
		t.Error("expected IsDM false for guild message")
	}

	// DM has empty GuildID
	dm := &InboundMessage{
		ChannelID: "12345",
		UserID:    "67890",
		Content:   "Hello!",
		GuildID:   "",
		IsDM:      true,
	}
	if !dm.IsDM {
		t.Error("expected IsDM true for DM")
	}
}

func TestOutboundMessageFields(t *testing.T) {
	msg := &OutboundMessage{
		ChannelID: "12345",
		Content:   "Response from agent",
		IsReply:   true,
		ReplyTo:   "abc123",
	}

	if !msg.IsReply {
		t.Error("expected IsReply true")
	}
	if msg.ReplyTo != "abc123" {
		t.Errorf("expected ReplyTo 'abc123', got %q", msg.ReplyTo)
	}
}

func TestBotAdapterName(t *testing.T) {
	bot := NewBotAdapter("test-token")
	if bot.Name() != "discord-bot" {
		t.Errorf("expected name 'discord-bot', got %q", bot.Name())
	}
}

func TestBotAdapterNotConnected(t *testing.T) {
	bot := NewBotAdapter("test-token")
	if bot.IsReady() {
		t.Error("expected not ready before Start()")
	}

	// Send should fail when not connected
	err := bot.Send(&OutboundMessage{
		ChannelID: "12345",
		Content:   "test",
	})
	if err == nil {
		t.Error("expected error when sending without connection")
	}
}

func TestMessageHandlerRegistration(t *testing.T) {
	bot := NewBotAdapter("test-token")
	received := make([]*InboundMessage, 0)

	bot.OnMessage(func(msg *InboundMessage) {
		received = append(received, msg)
	})

	if len(bot.handlers) != 1 {
		t.Errorf("expected 1 handler, got %d", len(bot.handlers))
	}
}
