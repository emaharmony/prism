// Package sse provides a shared Server-Sent Events (SSE) decoder for LLM provider
// streaming implementations. Both Anthropic and Gemini (and potentially future
// providers) use SSE for streaming token delivery.
//
// This package exists to avoid duplicating line-scanning and SSE-parsing logic
// across provider sub-packages.
package sse

import (
	"encoding/json"
	"io"
	"strings"
)

// Event represents a parsed Server-Sent Event.
type Event struct {
	// Event is the SSE event type (e.g., "message_start", "content_block_delta").
	// Empty string means no event type was specified (default SSE event).
	Event string

	// Data is the raw JSON payload from the "data:" field.
	Data json.RawMessage
}

// Decoder reads Server-Sent Events from an HTTP response body.
type Decoder struct {
	scanner *lineScanner
}

// NewDecoder creates an SSE decoder that reads from r.
func NewDecoder(r io.Reader) *Decoder {
	return &Decoder{scanner: newLineScanner(r)}
}

// Next reads the next complete SSE event from the stream.
// Returns io.EOF when the stream ends.
//
// Per the SSE spec (HTML Living Standard §9.2.6), multiple data: lines
// within a single event are concatenated with LF between them.
func (d *Decoder) Next() (*Event, error) {
	ev := &Event{}
	var dataLines []string

	for d.scanner.Scan() {
		line := d.scanner.Text()

		// Empty line = event boundary
		if line == "" {
			if len(dataLines) > 0 || ev.Event != "" {
				// Concatenate data lines with LF per SSE spec
				if len(dataLines) > 0 {
					ev.Data = json.RawMessage(strings.Join(dataLines, "\n"))
				}
				return ev, nil
			}
			continue
		}

		// Comment line (starts with :)
		if len(line) > 0 && line[0] == ':' {
			continue
		}

		// Parse "field: value"
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
			dataLines = append(dataLines, value)
		}
	}

	// Return last event if we have partial data
	if len(dataLines) > 0 || ev.Event != "" {
		if len(dataLines) > 0 {
			ev.Data = json.RawMessage(strings.Join(dataLines, "\n"))
		}
		return ev, nil
	}
	return nil, io.EOF
}

// NextData reads the next "data:" line from a simple SSE stream
// (used by providers like Gemini that only use data fields, not typed events).
// Returns io.EOF when the stream ends.
func (d *Decoder) NextData() (json.RawMessage, error) {
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

// ---------- line scanner ----------

// lineScanner reads lines from an io.Reader one byte at a time.
// This avoids bufio.Scanner's 64KB line limit, which can be hit
// by large SSE data payloads.
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