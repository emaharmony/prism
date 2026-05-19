// Package gemini streaming support — Server-Sent Events for generateContent.
//
// Gemini's streamGenerateContent endpoint returns SSE events with
// generateContentResponse chunks. We extract text from candidates[0].content.parts
// and batch tokens into 50ms intervals.
package gemini

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

// GenerateStream calls the Gemini streamGenerateContent API with streaming enabled.
func (p *Provider) GenerateStream(ctx context.Context, req provider.GenerateRequest) (<-chan provider.TokenChunk, error) {
	gemReq := generateContentRequest{
		Contents: []geminiContent{
			{Role: "user", Parts: []geminiPart{{Text: req.Prompt}}},
		},
		GenerationConfig: &generationConfig{
			Temperature:    req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		},
	}

	body, err := json.Marshal(gemReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	// streamGenerateContent uses alt=sse query parameter
	url := fmt.Sprintf("%s/v1beta/models/%s:streamGenerateContent?alt=sse&key=%s", p.BaseURL, req.Model, p.APIKey)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := p.HTTPClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: request failed: %w", err)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		return nil, retry.NewRetryableError(fmt.Errorf("gemini: rate limited (429)"))
	}
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("gemini: API error (%d): %s", resp.StatusCode, string(respBody))
	}

	ch := make(chan provider.TokenChunk, 64)
	go p.readSSEStream(resp, ch)
	return ch, nil
}

// readSSEStream reads Gemini's SSE events and batches tokens into 50ms intervals.
func (p *Provider) readSSEStream(resp *http.Response, ch chan<- provider.TokenChunk) {
	defer close(ch)
	defer resp.Body.Close()

	var tokens []string
	var index int
	lastBatch := time.Now()
	batchInterval := 50 * time.Millisecond

	decoder := newGeminiSSEDecoder(resp.Body)

	for {
		data, err := decoder.NextData()
		if err != nil {
			if err == io.EOF {
				break
			}
			ch <- provider.TokenChunk{Error: fmt.Errorf("gemini: read SSE: %w", err), Finished: true}
			return
		}

		var chunk generateContentResponse
		if err := json.Unmarshal(data, &chunk); err != nil {
			ch <- provider.TokenChunk{Error: fmt.Errorf("gemini: decode chunk: %w", err), Finished: true}
			return
		}

		for _, candidate := range chunk.Candidates {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					tokens = append(tokens, part.Text)
					index++

					if time.Since(lastBatch) >= batchInterval {
						batchTokens := make([]string, len(tokens))
						copy(batchTokens, tokens)
						ch <- provider.TokenChunk{Tokens: batchTokens, Index: index - len(tokens)}
						tokens = tokens[:0]
						lastBatch = time.Now()
					}
				}
			}

			// Check for finish
			if candidate.FinishReason == "STOP" || candidate.FinishReason == "SAFETY" {
				if len(tokens) > 0 {
					batchTokens := make([]string, len(tokens))
					copy(batchTokens, tokens)
					ch <- provider.TokenChunk{Tokens: batchTokens, Index: index - len(tokens), Finished: true}
				} else {
					ch <- provider.TokenChunk{Finished: true}
				}
				return
			}
		}
	}

	// Stream ended
	if len(tokens) > 0 {
		ch <- provider.TokenChunk{Tokens: tokens, Index: index - len(tokens), Finished: true}
	} else {
		ch <- provider.TokenChunk{Finished: true}
	}
}

// ---------- Gemini SSE decoder ----------

// geminiSSEDecoder reads SSE data lines from the response body.
type geminiSSEDecoder struct {
	scanner *lineScanner
}

func newGeminiSSEDecoder(r io.Reader) *geminiSSEDecoder {
	return &geminiSSEDecoder{scanner: newLineScanner(r)}
}

// NextData reads the next "data:" line from the SSE stream.
func (d *geminiSSEDecoder) NextData() (json.RawMessage, error) {
	for d.scanner.Scan() {
		line := d.scanner.Text()
		if len(line) > 5 && line[:5] == "data:" {
			value := line[5:]
			if len(value) > 0 && value[0] == ' ' {
				value = value[1:]
			}
			return json.RawMessage(value), nil
		}
	}
	return nil, io.EOF
}

// We reuse the lineScanner from the anthropic package by defining it here too.
// (Can't import anthropic from gemini — would create coupling between providers)

type lineScanner struct {
	reader io.Reader
	line   string
	eof    bool
}

func newLineScanner(r io.Reader) *lineScanner {
	return &lineScanner{reader: r}
}

func (ls *lineScanner) Scan() bool {
	if ls.eof {
		return false
	}
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

// Compile-time interface check.
var _ provider.StreamingProvider = (*Provider)(nil)