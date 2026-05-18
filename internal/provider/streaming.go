// Package provider defines Prism's LLM provider interface and implementations.
//
// V14a adds the StreamingProvider interface for streaming LLM responses.
// Providers that support streaming implement both Provider and StreamingProvider.
// Providers that don't fall back to the synchronous Generate() path.
//
// Streaming is essential to Prism's thesis: events should be visible as they
// happen. If the LLM produces output in one blob at the end, the event stream
// is blind during generation. Streaming makes the LLM's output visible in
// real time — each token batch is an event.
package provider

import (
	"context"
)

// TokenChunk represents a batch of tokens from a streaming LLM response.
//
// Tokens are batched every 50ms (or every 5 tokens, whichever comes first)
// to prevent event flooding. A streaming response of 2000 tokens produces
// ~40 token events instead of 2000 individual events.
//
// Token events go to a separate NATS stream (PRISM_TOKENS) with 1h retention
// and are NOT written to events.jsonl. Only the llm.completed event is
// persisted to disk.
type TokenChunk struct {
	// Tokens is a batch of tokens from the LLM.
	Tokens []string

	// Index is the position of the first token in the batch
	// relative to the start of the response.
	Index int

	// Finished is true when the LLM has completed generation.
	// The last TokenChunk will have Finished=true and may contain
	// the final tokens or be empty.
	Finished bool

	// Error is non-nil if the stream encountered an error.
	// After an error, no more chunks will be sent on the channel.
	Error error
}

// StreamingProvider extends Provider with streaming generation capability.
//
// Providers that support streaming implement this interface in addition
// to Provider. The pipeline checks at runtime:
//
//	if sp, ok := provider.(StreamingProvider); ok {
//	    // Use streaming path
//	} else {
//	    // Fall back to synchronous Generate()
//	}
//
// This means every provider works with the pipeline — streaming is
// opt-in, not required.
type StreamingProvider interface {
	Provider

	// GenerateStream returns a channel of TokenChunks for streaming output.
	// The caller reads from the channel until it's closed (successful completion)
	// or an error is sent (TokenChunk with Error set).
	//
	// The caller MUST read all chunks from the channel to prevent goroutine
	// leaks. If the caller needs to cancel, use context cancellation.
	//
	// Example:
	//   ch, err := sp.GenerateStream(ctx, req)
	//   for chunk := range ch {
	//       if chunk.Error != nil {
	//           return chunk.Error
	//       }
	//       processTokens(chunk.Tokens)
	//   }
	GenerateStream(ctx context.Context, req GenerateRequest) (<-chan TokenChunk, error)
}