package convstage

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/emaharmony/prism/internal/provider"
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

func (m *mockSender) getAllEdits() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.edits))
	copy(result, m.edits)
	return result
}

// mockPartialFailProvider succeeds at sync Generate but fails GenerateStream.
type mockPartialFailProvider struct {
	mock.MockProvider
}

func (m *mockPartialFailProvider) GenerateStream(ctx context.Context, req provider.GenerateRequest) (<-chan provider.TokenChunk, error) {
	return nil, fmt.Errorf("streaming not available for this provider")
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
	if result.Text == "" {
		t.Error("streaming result should have non-empty text")
	}
	if len(sender.sends) < 1 {
		t.Error("should have at least a placeholder send")
	}
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
	if result.Streamed {
		t.Error("fallback to sync should not report Streamed=true")
	}
}

func TestConversationStage_StreamingFallback_SyncSuccess(t *testing.T) {
	// Provider that fails streaming but succeeds at sync Generate
	sender := &mockSender{}
	prov := &mockPartialFailProvider{MockProvider: *mock.New()}

	stage := NewConversationStage(sender, prov, "mock-model", "lumi")

	result, err := stage.ExecuteStreaming(context.Background(), "prompt", "channel-1")
	if err != nil {
		t.Fatalf("ExecuteStreaming failed: %v", err)
	}

	// Should fall back to sync successfully
	if result.Streamed {
		t.Error("fallback to sync should not report Streamed=true")
	}

	// Placeholder should exist and be edited to show the sync response
	if sender.getEditCount() < 1 {
		t.Error("placeholder should be edited with sync response")
	}
	lastEdit := sender.getLastEdit()
	if !strings.Contains(lastEdit, "Mock response") {
		t.Errorf("placeholder edit should contain sync response, got %q", lastEdit)
	}
}

func TestConversationStage_StreamingError_WithPartialText(t *testing.T) {
	sender := &mockSender{}

	// Create a streaming provider that sends partial text then errors
	chunkCh := make(chan provider.TokenChunk, 10)
	go func() {
		chunkCh <- provider.TokenChunk{Tokens: []string{"Hello from the model. "}, Index: 0}
		chunkCh <- provider.TokenChunk{Tokens: []string{}, Index: 1, Error: fmt.Errorf("connection lost")}
		close(chunkCh)
	}()

	stage := NewConversationStage(sender, mock.New(), "mock-model", "lumi")
	result := stage.processStream(context.Background(), chunkCh, "channel-1", "msg-123")

	if !result.Streamed {
		t.Error("streaming with error should still report Streamed=true")
	}
	if result.Text != "Hello from the model. " {
		t.Errorf("partial text should be preserved, got %q", result.Text)
	}

	// Last edit should include partial text + error indicator
	lastEdit := sender.getLastEdit()
	if !strings.Contains(lastEdit, "Hello from the model.") {
		t.Errorf("error message should include partial text, got %q", lastEdit)
	}
	if !strings.Contains(lastEdit, "⚠️") {
		t.Errorf("error message should contain warning indicator, got %q", lastEdit)
	}
}

func TestConversationStage_StreamingError_NoPartialText(t *testing.T) {
	sender := &mockSender{}

	chunkCh := make(chan provider.TokenChunk, 10)
	go func() {
		chunkCh <- provider.TokenChunk{Tokens: []string{}, Index: 0, Error: fmt.Errorf("immediate failure")}
		close(chunkCh)
	}()

	stage := NewConversationStage(sender, mock.New(), "mock-model", "lumi")
	result := stage.processStream(context.Background(), chunkCh, "channel-1", "msg-123")

	if !result.Streamed {
		t.Error("streaming with error should still report Streamed=true")
	}

	lastEdit := sender.getLastEdit()
	if !strings.Contains(lastEdit, "⚠️") {
		t.Errorf("should show generic error when no partial text, got %q", lastEdit)
	}
}

func TestConversationStage_StreamingCancel_WithPartialText(t *testing.T) {
	sender := &mockSender{}

	chunkCh := make(chan provider.TokenChunk, 10)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		chunkCh <- provider.TokenChunk{Tokens: []string{"Starting to think..."}, Index: 0}
		time.Sleep(50 * time.Millisecond)
		cancel()
		close(chunkCh)
	}()

	stage := NewConversationStage(sender, mock.New(), "mock-model", "lumi")
	result := stage.processStream(ctx, chunkCh, "channel-1", "msg-123")

	if !result.Streamed {
		t.Error("canceled stream should report Streamed=true")
	}
	if result.Text != "Starting to think..." {
		t.Errorf("partial text should be preserved on cancel, got %q", result.Text)
	}

	lastEdit := sender.getLastEdit()
	if !strings.Contains(lastEdit, "Canceled") {
		t.Errorf("canceled stream should show 'Canceled', got %q", lastEdit)
	}
	if !strings.Contains(lastEdit, "Starting to think...") {
		t.Errorf("canceled edit should include partial text, got %q", lastEdit)
	}
}

func TestEditMessageSafe_Truncation(t *testing.T) {
	sender := &mockSender{}
	stage := NewConversationStage(sender, nil, "mock-model", "lumi")

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

func TestEditMessageSafe_UTF8Truncation(t *testing.T) {
	sender := &mockSender{}
	stage := NewConversationStage(sender, nil, "mock-model", "lumi")

	// Create content with emoji that spans >2000 runes
	emoji := "🎉🎊🎈🎁🎄🎅🎆✨💫🌟⭐🔥💯🎵🎶🎸🎹🎺🎻🥁🪘🪇🪈🪉🪊🪋🪌🪍🪎🪏"
	longContent := strings.Repeat(emoji, 200) // Well over 2000 chars with multi-byte chars
	stage.editMessageSafe("channel-1", "msg-123", longContent)

	if sender.getEditCount() != 1 {
		t.Fatalf("expected 1 edit, got %d", sender.getEditCount())
	}

	lastEdit := sender.getLastEdit()
	// Verify the result is valid UTF-8 (no corrupted multi-byte sequences)
	for _, r := range lastEdit {
		if r == '\ufffd' {
			t.Error("truncation produced invalid UTF-8 (replacement character found)")
			break
		}
	}
	if len(lastEdit) > 2000 {
		t.Errorf("edit should be <= 2000 chars, got %d", len(lastEdit))
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

func TestConversationStage_StreamingBatching(t *testing.T) {
	sender := &mockSender{}
	prov := mock.New()

	stage := NewConversationStage(sender, prov, "mock-model", "lumi")

	result, err := stage.ExecuteStreaming(context.Background(), "prompt", "channel-1")
	if err != nil {
		t.Fatalf("ExecuteStreaming failed: %v", err)
	}

	if result.Text == "" {
		t.Error("streaming result should have non-empty text")
	}
	if len(sender.sends) < 1 {
		t.Error("should have at least a placeholder send")
	}
}

// --- Feedback Loop 3: Additional edge-case tests ---

func TestEditMessageSafe_EditError(t *testing.T) {
	sender := &mockSender{editErr: fmt.Errorf("Discord rate limited")}
	stage := NewConversationStage(sender, nil, "mock-model", "lumi")

	// Should not panic when EditMessage fails — just logs
	stage.editMessageSafe("channel-1", "msg-123", "Hello world!")

	// No edits actually recorded since editErr is set
	if sender.getEditCount() != 0 {
		t.Errorf("expected 0 edits (all fail), got %d", sender.getEditCount())
	}
}

func TestConversationStage_BothStreamAndSyncFail(t *testing.T) {
	sender := &mockSender{}
	prov := mock.NewFailing() // Both GenerateStream and Generate fail

	stage := NewConversationStage(sender, prov, "mock-model", "lumi")

	result, err := stage.ExecuteStreaming(context.Background(), "prompt", "channel-1")
	if err == nil {
		t.Error("expected error when both streaming and sync fail")
	}
	if result != nil {
		t.Error("expected nil result when both fail")
	}

	// Placeholder should exist and be edited with error message
	edits := sender.getAllEdits()
	if len(edits) == 0 {
		t.Error("placeholder should be edited with error message")
	} else {
		lastEdit := edits[len(edits)-1]
		if !strings.Contains(lastEdit, "⚠️") {
			t.Errorf("error edit should contain warning, got %q", lastEdit)
		}
	}
}

func TestConversationStage_MaxTextSizeGuard(t *testing.T) {
	sender := &mockSender{}

	// Create a provider that sends tokens exceeding the 10KB limit
	chunkCh := make(chan provider.TokenChunk, 10)
	go func() {
		// Send 11KB of text in chunks
		bigToken := strings.Repeat("x", 2048)
		for i := 0; i < 6; i++ { // 6 * 2048 = 12KB
			chunkCh <- provider.TokenChunk{Tokens: []string{bigToken}, Index: i}
		}
		chunkCh <- provider.TokenChunk{Tokens: []string{}, Index: 6, Finished: true}
		close(chunkCh)
	}()

	stage := NewConversationStage(sender, mock.New(), "mock-model", "lumi")
	result := stage.processStream(context.Background(), chunkCh, "channel-1", "msg-123")

	if !result.Streamed {
		t.Error("should still report streamed=true")
	}
	// Text should be truncated due to size guard
	if len(result.Text) > 10240 {
		t.Errorf("text should be capped at ~10KB, got %d bytes", len(result.Text))
	}
}

func TestConversationStage_CooperativeCancellation(t *testing.T) {
	sender := &mockSender{}

	// Create a provider that sends tokens slowly and doesn't check ctx.Done()
	chunkCh := make(chan provider.TokenChunk, 20)
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		// Send tokens every 50ms
		for i := 0; i < 20; i++ {
			select {
			case chunkCh <- provider.TokenChunk{Tokens: []string{fmt.Sprintf("word%d ", i)}, Index: i}:
			case <-ctx.Done():
				close(chunkCh)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		chunkCh <- provider.TokenChunk{Tokens: []string{}, Index: 20, Finished: true}
		close(chunkCh)
	}()

	// Cancel after 150ms — should get ~3 tokens
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()

	stage := NewConversationStage(sender, mock.New(), "mock-model", "lumi")
	result := stage.processStream(ctx, chunkCh, "channel-1", "msg-123")

	if !result.Streamed {
		t.Error("should report streamed=true")
	}
	// Should have some partial text
	if len(result.Text) == 0 {
		t.Error("should have captured some tokens before cancellation")
	}
}