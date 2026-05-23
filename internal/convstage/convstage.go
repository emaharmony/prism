// Package convstage provides a lightweight conversation stage for the Prism
// Discord pipeline.
//
// DEPRECATED: This package is superseded by the stage.Pipeline (V21-5).
// The LLMStage with StreamCallback now handles streaming directly.
// This package is kept for reference and will be removed in a future version.
package convstage

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/emaharmony/prism/internal/provider"
)

// MessageSender is the interface for sending/editing Discord messages.
// This decouples the conversation stage from the concrete discordbot.BotAdapter,
// making it testable without a real Discord connection.
type MessageSender interface {
	Send(channelID, content string) error
	SendPlaceholder(channelID, content string) (string, error)
	EditMessage(channelID, messageID, content string) error
}

// ConversationStage handles LLM generation and response delivery for
// the Discord conversation pipeline.
type ConversationStage struct {
	sender   MessageSender
	provider provider.Provider
	model    string
	agentID  string
}

// NewConversationStage creates a conversation stage for the given agent and provider.
func NewConversationStage(sender MessageSender, prov provider.Provider, model, agentID string) *ConversationStage {
	return &ConversationStage{
		sender:   sender,
		provider: prov,
		model:    model,
		agentID:  agentID,
	}
}

// Result holds the outcome of a conversation stage execution.
type Result struct {
	Text         string
	PromptTokens int
	OutputTokens int
	LatencyMS    int64
	Streamed     bool   // true if response was delivered via streaming
	MessageID    string // Discord message ID (for follow-up operations)
}

// Execute generates a response and sends it to Discord synchronously.
// Falls back to this if streaming fails or is not supported.
func (s *ConversationStage) Execute(ctx context.Context, prompt string, channelID string) (*Result, error) {
	resp, err := s.provider.Generate(ctx, provider.GenerateRequest{
		Agent:  s.agentID,
		Task:   prompt,
		Prompt: prompt,
		Model:  s.model,
	})
	if err != nil {
		return nil, fmt.Errorf("conversation stage: generate: %w", err)
	}

	// Send the complete response to Discord
	err = s.sender.Send(channelID, resp.Text)
	if err != nil {
		return nil, fmt.Errorf("conversation stage: send: %w", err)
	}

	return &Result{
		Text:         resp.Text,
		PromptTokens: resp.PromptTokens,
		OutputTokens: resp.OutputTokens,
		LatencyMS:    resp.LatencyMS,
		Streamed:     false,
	}, nil
}

// ExecuteStreaming generates a response with streaming tokens to Discord.
// Falls back to synchronous Execute if the provider doesn't support streaming.
// IMPORTANT: The ctx parameter should carry the Run's cancel context so that
// /cancel and timeout enforcement work correctly during streaming.
//
// Streaming strategy:
//  1. Send a placeholder message to Discord ("✧ ...")
//  2. Read tokens from the stream channel
//  3. Accumulate tokens, flush to Discord every ~900ms (respects 5 edits/5s rate limit)
//  4. On stream completion, do a final flush with the complete response
//  5. On error, edit the placeholder with partial text + error message
func (s *ConversationStage) ExecuteStreaming(ctx context.Context, prompt string, channelID string) (*Result, error) {
	// Check if provider supports streaming
	streamingProv, ok := s.provider.(provider.StreamingProvider)
	if !ok {
		log.Printf("[STREAM] provider doesn't support streaming, falling back to sync")
		return s.Execute(ctx, prompt, channelID)
	}

	// Step 1: Send placeholder message
	placeholderMsgID, err := s.sender.SendPlaceholder(channelID, "✧ ...")
	if err != nil {
		log.Printf("[STREAM] placeholder send failed, falling back to sync: %v", err)
		return s.Execute(ctx, prompt, channelID)
	}

	// Step 2: Start the streaming LLM call
	chunkCh, err := streamingProv.GenerateStream(ctx, provider.GenerateRequest{
		Agent:  s.agentID,
		Task:   prompt,
		Prompt: prompt,
		Model:  s.model,
	})
	if err != nil {
		// Streaming setup failed — fall back to sync
		// Edit the placeholder to show the sync response instead of sending a new message
		// to avoid ghost placeholder + new message
		syncResult, syncErr := s.Execute(ctx, prompt, channelID)
		if syncErr != nil {
			// Both streaming and sync failed — edit placeholder with error
			s.sender.EditMessage(channelID, placeholderMsgID, "⚠️ I couldn't start thinking. Please try again.")
			return nil, fmt.Errorf("conversation stage: streaming and sync both failed: %w", err)
		}
		// Sync succeeded — update the placeholder with the response
		s.sender.EditMessage(channelID, placeholderMsgID, syncResult.Text)
		syncResult.Streamed = false
		syncResult.MessageID = placeholderMsgID
		return syncResult, nil
	}

	// Step 3: Read tokens, batch, and flush to Discord
	result := s.processStream(ctx, chunkCh, channelID, placeholderMsgID)
	result.MessageID = placeholderMsgID
	return result, nil
}

// processStream reads from the token channel and batches edits to Discord.
func (s *ConversationStage) processStream(ctx context.Context, chunkCh <-chan provider.TokenChunk, channelID, messageID string) *Result {
	var (
		fullText     strings.Builder
		totalTokens  int
		editCount    int
		startTime    = time.Now()
		flushTimer   = time.NewTicker(900 * time.Millisecond)
		deferredEdit string // accumulated text waiting for next flush
		mu           sync.Mutex
		streamErr    error
	)

	defer flushTimer.Stop()

	// Channel to signal when the stream is done
	streamDone := make(chan struct{})

	// Goroutine: read tokens from the LLM stream
	go func() {
		defer close(streamDone)

		for chunk := range chunkCh {
			// Cooperative cancellation: abort reading if context is done,
			// even if the provider doesn't check ctx.Done() itself.
			select {
			case <-ctx.Done():
				mu.Lock()
				streamErr = ctx.Err()
				mu.Unlock()
				return
			default:
			}

			if chunk.Error != nil {
				mu.Lock()
				streamErr = chunk.Error
				mu.Unlock()
				return
			}

			mu.Lock()
			for _, token := range chunk.Tokens {
				// Defense-in-depth: stop accumulating if text exceeds 10KB.
				// Discord's limit is 2000 chars; 10KB is well beyond useful output.
				if fullText.Len()+len(token) > 10240 {
					streamErr = fmt.Errorf("response exceeded maximum size (10KB)")
					mu.Unlock()
					return
				}
				fullText.WriteString(token)
				deferredEdit += token
				totalTokens++
			}
			mu.Unlock()

			if chunk.Finished {
				return
			}
		}
	}()

	// Main loop: batch edits to Discord
	for {
		select {
		case <-streamDone:
			// Stream finished — do a final flush
			mu.Lock()
			finalText := fullText.String()
			err := streamErr
			mu.Unlock()

			if err != nil {
				// Mid-stream error — show partial text if available
				if len(finalText) > 0 {
					errMsg := finalText + "\n\n⚠️ I started thinking but hit an error. Here's what I had so far."
					s.sender.EditMessage(channelID, messageID, errMsg)
				} else {
					s.sender.EditMessage(channelID, messageID, "⚠️ I started thinking but hit an error. Please try again.")
				}
				return &Result{
					Text:      finalText,
					Streamed:  true,
					LatencyMS: time.Since(startTime).Milliseconds(),
				}
			}

			// Final edit with complete response
			if len(finalText) > 0 {
				s.editMessageSafe(channelID, messageID, finalText)
				editCount++
			}

			log.Printf("[STREAM] completed: %d tokens, %d edits, %dms",
				totalTokens, editCount, time.Since(startTime).Milliseconds())

			return &Result{
				Text:         finalText,
				OutputTokens: totalTokens,
				LatencyMS:    time.Since(startTime).Milliseconds(),
				Streamed:     true,
			}

		case <-flushTimer.C:
			// Time to flush accumulated tokens to Discord
			mu.Lock()
			toFlush := deferredEdit
			deferredEdit = ""
			current := fullText.String()
			mu.Unlock()

			// Skip edit if nothing accumulated since last flush
			if len(toFlush) > 0 {
				s.editMessageSafe(channelID, messageID, current)
				editCount++
			}

		case <-ctx.Done():
			// Context cancelled (user /cancel or shutdown)
			mu.Lock()
			partialText := fullText.String()
			mu.Unlock()

			if len(partialText) > 0 {
				s.sender.EditMessage(channelID, messageID, partialText+"\n\n⚠️ Canceled.")
			} else {
				s.sender.EditMessage(channelID, messageID, "⚠️ Canceled.")
			}
			return &Result{
				Text:      partialText,
				Streamed:  true,
				LatencyMS: time.Since(startTime).Milliseconds(),
			}
		}
	}
}

// editMessageSafe edits a Discord message, logging errors instead of propagating them.
// A failed edit is not fatal — the next edit will include the accumulated text.
// Uses rune-aware truncation to avoid corrupting multi-byte UTF-8 characters
// while staying within Discord's 2000-byte limit.
func (s *ConversationStage) editMessageSafe(channelID, messageID, content string) {
	if len(content) > 2000 {
		// Discord message limit — truncate safely.
		// Walk backwards through runes until we fit in 1997 bytes + "..." (3 bytes).
		runes := []rune(content)
		for len(runes) > 1 && len(string(runes))+3 > 2000 {
			runes = runes[:len(runes)-1]
		}
		content = string(runes) + "..."
	}
	if err := s.sender.EditMessage(channelID, messageID, content); err != nil {
		log.Printf("[STREAM] edit failed: %v", err)
	}
}