// Package anthropic streaming support — Server-Sent Events for the Messages API.
//
// Anthropic SSE uses typed events: message_start, content_block_start,
// content_block_delta, content_block_stop, message_delta, message_stop.
// We extract text from content_block_delta events and batch into 50ms intervals.
package anthropic

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/retry"
)

// GenerateStream calls the Anthropic Messages API with streaming enabled.
func (p *Provider) GenerateStream(ctx context.Context, req provider.GenerateRequest) (<-chan provider.TokenChunk, error) {
	anthReq := messagesRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Messages:    []anthropicMessage{{Role: "user", Content: req.Prompt}},
		Temperature: req.Temperature,
		Stream:      true,
	}

	body, err := json.Marshal(anthReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.BaseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.APIKey)
	httpReq.Header.Set("anthropic-version", APIVersion)

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: request failed: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		return nil, retry.NewRetryableError(fmt.Errorf("anthropic: rate limited (429)"))
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("anthropic: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan provider.TokenChunk, 64)
	go p.readSSEStream(resp, ch)
	return ch, nil
}

// ---------- Anthropic SSE types ----------

// sseEvent represents a parsed Server-Sent Event from Anthropic.
type sseEvent struct {
	Event string          // event type (message_start, content_block_delta, etc.)
	Data  json.RawMessage // event payload
}

// contentBlockDelta is the content_block_delta event payload.
type contentBlockDelta struct {
	Type  string `json:"type"`
	Index int    `json:"index"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
}

// messageDeltaEvent is the message_delta event payload (stop_reason).
type messageDeltaEvent struct {
	Type  string `json:"type"`
	Delta struct {
		StopReason string `json:"stop_reason"`
	} `json:"delta"`
}

// ---------- SSE reader ----------

// readSSEStream reads Anthropic's typed SSE events and batches tokens.
func (p *Provider) readSSEStream(resp *http.Response, ch chan<- provider.TokenChunk) {
	defer close(ch)
	defer resp.Body.Close()

	var tokens []string
	var index int
	lastBatch := time.Now()
	batchInterval := 50 * time.Millisecond

	// Read SSE line by line
	decoder := newSSEDecoder(resp.Body)

	for {
		ev, err := decoder.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			ch <- provider.TokenChunk{Error: fmt.Errorf("anthropic: read SSE: %w", err), Finished: true}
			return
		}

		switch ev.Event {
		case "content_block_delta":
			var delta contentBlockDelta
			if err := json.Unmarshal(ev.Data, &delta); err != nil {
				ch <- provider.TokenChunk{Error: fmt.Errorf("anthropic: decode delta: %w", err), Finished: true}
				return
			}
			if delta.Delta.Type == "text_delta" && delta.Delta.Text != "" {
				tokens = append(tokens, delta.Delta.Text)
				index++

				if time.Since(lastBatch) >= batchInterval {
					batchTokens := make([]string, len(tokens))
					copy(batchTokens, tokens)
					ch <- provider.TokenChunk{Tokens: batchTokens, Index: index - len(tokens)}
					tokens = tokens[:0]
					lastBatch = time.Now()
				}
			}

		case "message_stop":
			if len(tokens) > 0 {
				batchTokens := make([]string, len(tokens))
				copy(batchTokens, tokens)
				ch <- provider.TokenChunk{Tokens: batchTokens, Index: index - len(tokens), Finished: true}
			} else {
				ch <- provider.TokenChunk{Finished: true}
			}
			return

		case "error":
			ch <- provider.TokenChunk{
				Error:    fmt.Errorf("anthropic: stream error: %s", string(ev.Data)),
				Finished: true,
			}
			return
		}
	}

	// Stream ended without message_stop
	if len(tokens) > 0 {
		ch <- provider.TokenChunk{Tokens: tokens, Index: index - len(tokens), Finished: true}
	} else {
		ch <- provider.TokenChunk{Finished: true}
	}
}

// ---------- SSE line decoder ----------

// sseDecoder parses Server-Sent Events from an HTTP response body.
type sseDecoder struct {
	scanner interface {
		Scan() bool
		Text() string
	}
}

// We use a simple line-based approach instead of bufio.Scanner
// to avoid the 64KB line limit.
type lineReader struct {
	r io.Reader
}

func newSSEDecoder(r io.Reader) *sseDecoder {
	return &sseDecoder{scanner: newLineScanner(r)}
}

type lineScanner struct {
	buf    []byte
	reader io.Reader
	line   string
	eof    bool
}

func newLineScanner(r io.Reader) *lineScanner {
	return &lineScanner{reader: r, buf: make([]byte, 4096)}
}

func (ls *lineScanner) Scan() bool {
	if ls.eof {
		return false
	}
	// Simple line reading - read byte by byte for correctness
	var line []byte
	b := make([]byte, 1)
	for {
		_, err := ls.reader.Read(b)
		if err != nil {
			ls.eof = true
			if len(line) > 0 {
				ls.line = string(line)
				return true
			}
			return false
		}
		if b[0] == '\n' {
			ls.line = string(line)
			return true
		}
		if b[0] != '\r' {
			line = append(line, b[0])
		}
	}
}

func (ls *lineScanner) Text() string {
	return ls.line
}

// Next reads the next SSE event from the stream.
func (d *sseDecoder) Next() (*sseEvent, error) {
	ev := &sseEvent{}
	for d.scanner.Scan() {
		line := d.scanner.Text()

		// Empty line = event boundary
		if line == "" {
			if ev.Event != "" {
				return ev, nil
			}
			continue
		}

		// Parse field: value
		if line[0] == ':' {
			continue // comment
		}

		colon := -1
		for i, c := range line {
			if c == ':' {
				colon = i
				break
			}
		}

		if colon == -1 {
			continue
		}

		field := line[:colon]
		value := line[colon+1:]
		if len(value) > 0 && value[0] == ' ' {
			value = value[1:]
		}

		switch field {
		case "event":
			ev.Event = value
		case "data":
			ev.Data = json.RawMessage(value)
		}
	}

	if ev.Event != "" {
		return ev, nil
	}
	return nil, io.EOF
}

// Compile-time interface check.
var _ provider.StreamingProvider = (*Provider)(nil)