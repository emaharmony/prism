// Package convstage provides a lightweight conversation stage for the Prism
// Discord pipeline. It encapsulates LLM generation and response delivery,
// supporting both synchronous and streaming modes.
//
// V21-4: This is a stepping stone to the full stage pipeline (Option C).
// The stage isolates the LLM-call + response-delivery logic from the
// message handler, making it testable and providing a clean migration path.
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
	Streamed     bool // true if response was delivered via streaming
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
//
// Streaming strategy:
//  1. Send a placeholder message to Discord ("✧ ...")
//  2. Read tokens from the stream channel
//  3. Accumulate tokens, flush to Discord every ~900ms (respects 5 edits/5s rate limit)
//  4. On stream completion, do a final flush with the complete response
//  5. On error, edit the placeholder with an error message
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
		// Streaming setup failed — edit placeholder with error, fall back
		s.sender.EditMessage(channelID, placeholderMsgID, "⚠️ I couldn't start thinking. Please try again.")
		log.Printf("[STREAM] GenerateStream failed: %v", err)
		return s.Execute(ctx, prompt, channelID)
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
			if chunk.Error != nil {
				mu.Lock()
				streamErr = chunk.Error
				mu.Unlock()
				return
			}

			mu.Lock()
			for _, token := range chunk.Tokens {
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
				// Mid-stream error
				errMsg := "⚠️ I started thinking but hit an error. Please try again."
				s.sender.EditMessage(channelID, messageID, errMsg)
				return &Result{
					Text:     finalText,
					Streamed: true,
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
				s.sender.EditMessage(channelID, messageID, partialText+"\n\n✧ Canceled.")
			} else {
				s.sender.EditMessage(channelID, messageID, "✧ Canceled.")
			}
			return &Result{
				Text:     partialText,
				Streamed: true,
				LatencyMS: time.Since(startTime).Milliseconds(),
			}
		}
	}
}

// editMessageSafe edits a Discord message, logging errors instead of propagating them.
// A failed edit is not fatal — the next edit will include the accumulated text.
func (s *ConversationStage) editMessageSafe(channelID, messageID, content string) {
	if len(content) > 2000 {
		// Discord message limit — truncate for the edit, we'll send follow-ups if needed
		content = content[:1997] + "..."
	}
	if err := s.sender.EditMessage(channelID, messageID, content); err != nil {
		log.Printf("[STREAM] edit failed: %v", err)
	}
}