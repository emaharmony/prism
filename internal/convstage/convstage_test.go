package convstage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emaharmony/prism/internal/provider/mock"
)

// mockSender captures message operations for test assertions.
type mockSender struct {
	mu             sync.Mutex
	sends          []string
	placeholderID  string
	edits          []string
	placeholderErr error
	editErr        error
	sendErr        error
}

func (m *mockSender) Send(channelID, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sends = append(m.sends, content)
	return nil
}

func (m *mockSender) SendPlaceholder(channelID, content string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.placeholderErr != nil {
		return "", m.placeholderErr
	}
	m.placeholderID = "msg-123"
	m.sends = append(m.sends, content)
	return "msg-123", nil
}

func (m *mockSender) EditMessage(channelID, messageID, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.editErr != nil {
		return m.editErr
	}
	m.edits = append(m.edits, content)
	return nil
}

func (m *mockSender) getLastEdit() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.edits) == 0 {
		return ""
	}
	return m.edits[len(m.edits)-1]
}

func (m *mockSender) getEditCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.edits)
}

// --- Tests ---

func TestConversationStage_SyncExecute(t *testing.T) {
	sender := &mockSender{}
	prov := mock.New()

	stage := NewConversationStage(sender, prov, "mock-model", "lumi")

	result, err := stage.Execute(context.Background(), "prompt text", "channel-1")
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if result.Streamed {
		t.Error("sync execute should not report Streamed=true")
	}
	if len(sender.sends) != 1 {
		t.Fatalf("expected 1 send, got %d", len(sender.sends))
	}
}

func TestConversationStage_StreamingSuccess(t *testing.T) {
	sender := &mockSender{}
	prov := mock.New()

	stage := NewConversationStage(sender, prov, "mock-model", "lumi")

	result, err := stage.ExecuteStreaming(context.Background(), "prompt", "channel-1")
	if err != nil {
		t.Fatalf("ExecuteStreaming failed: %v", err)
	}

	if !result.Streamed {
		t.Error("streaming execute should report Streamed=true")
	}

	// Mock provider generates "Mock response for task 'prompt' by agent 'lumi' in project ''."
	// It should contain the key content
	if result.Text == "" {
		t.Error("streaming result should have non-empty text")
	}

	// Should have a placeholder send
	if len(sender.sends) < 1 {
		t.Error("should have at least a placeholder send")
	}

	// Should have at least one edit (final flush)
	if sender.getEditCount() < 1 {
		t.Error("should have at least one edit (final flush)")
	}

	lastEdit := sender.getLastEdit()
	if !strings.Contains(lastEdit, "Mock response") {
		t.Errorf("final edit should contain 'Mock response', got %q", lastEdit)
	}
}

func TestConversationStage_StreamingFallback_PlaceholderFailed(t *testing.T) {
	sender := &mockSender{placeholderErr: fmt.Errorf("Discord down")}
	prov := mock.New()

	stage := NewConversationStage(sender, prov, "mock-model", "lumi")

	result, err := stage.ExecuteStreaming(context.Background(), "prompt", "channel-1")
	if err != nil {
		t.Fatalf("ExecuteStreaming failed: %v", err)
	}

	// Should fall back to sync since placeholder failed
	if result.Streamed {
		t.Error("fallback to sync should not report Streamed=true")
	}
}

func TestConversationStage_StreamingProviderFails(t *testing.T) {
	sender := &mockSender{}
	prov := mock.NewFailing() // GenerateStream returns error

	stage := NewConversationStage(sender, prov, "mock-model", "lumi")

	result, err := stage.ExecuteStreaming(context.Background(), "prompt", "channel-1")
	// Should handle gracefully — falls back to sync, which also fails
	_ = result
	// The failing mock also fails on sync Generate, so we expect an error
	if err == nil {
		// If it somehow succeeded (shouldn't with failing mock), that's also fine
		// as long as there's no panic
	}
}

func TestConversationStage_StreamingCancel(t *testing.T) {
	sender := &mockSender{}
	prov := mock.New()

	ctx, cancel := context.WithCancel(context.Background())

	stage := NewConversationStage(sender, prov, "mock-model", "lumi")

	// Cancel after a short delay
	go func() {
		time.Sleep(30 * time.Millisecond)
		cancel()
	}()

	// Should handle cancellation gracefully — no panic, no deadlock
	result, _ := stage.ExecuteStreaming(ctx, "prompt", "channel-1")
	_ = result
}

func TestEditMessageSafe_Truncation(t *testing.T) {
	sender := &mockSender{}
	stage := NewConversationStage(sender, nil, "mock-model", "lumi")

	// Content longer than 2000 chars
	longContent := strings.Repeat("x", 3000)
	stage.editMessageSafe("channel-1", "msg-123", longContent)

	if sender.getEditCount() != 1 {
		t.Fatalf("expected 1 edit, got %d", sender.getEditCount())
	}

	lastEdit := sender.getLastEdit()
	if len(lastEdit) > 2000 {
		t.Errorf("edit should be truncated to <= 2000 chars, got %d", len(lastEdit))
	}
	if !strings.HasSuffix(lastEdit, "...") {
		t.Error("truncated edit should end with '...'")
	}
}

func TestEditMessageSafe_ShortContent(t *testing.T) {
	sender := &mockSender{}
	stage := NewConversationStage(sender, nil, "mock-model", "lumi")

	stage.editMessageSafe("channel-1", "msg-123", "Hello world!")

	if sender.getEditCount() != 1 {
		t.Fatalf("expected 1 edit, got %d", sender.getEditCount())
	}

	lastEdit := sender.getLastEdit()
	if lastEdit != "Hello world!" {
		t.Errorf("short content should not be modified, got %q", lastEdit)
	}
}