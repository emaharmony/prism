// Package main implements the `prism watch` subcommand: a live terminal view of a
// running gated-loop workflow.
//
// Usage:
//
//	prism watch [--config prism.yaml] [--subject prism.workflow] [--url <base>]
//
// It connects to the running daemon's SSE event stream (GET /api/v1/events/stream)
// and renders a continuously-updated view: the phase tree with gate scores, a
// token/budget burn-down meter, current tool activity, verification result, and
// delegation status. It is a pure consumer of events the engine already emits, so
// it never affects a run — just observes it.
//
// The live model and its ASCII rendering live in internal/tracker, shared with the
// desktop panel (cmd/prism-panel); this file is only the terminal transport + I/O.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/sse"
	"github.com/emaharmony/prism/internal/tracker"
)

// executeWatch is the `prism watch` entry point.
func executeWatch(args []string) {
	fs := flag.NewFlagSet("watch", flag.ExitOnError)
	configPath := fs.String("config", "prism.yaml", "Path to prism.yaml configuration file")
	subject := fs.String("subject", "prism.workflow.>", "NATS subject filter to subscribe to")
	urlOverride := fs.String("url", "", "Override the base API URL (default derived from config)")
	fs.Parse(args)

	baseURL := *urlOverride
	token := ""
	if baseURL == "" {
		cfg, err := orchestrator.LoadConfig(*configPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error loading config: %v\n(use --url to point at a running daemon)\n", err)
			os.Exit(1)
		}
		host := cfg.Prism.BindHost
		if host == "" || host == "0.0.0.0" {
			host = "127.0.0.1"
		}
		baseURL = fmt.Sprintf("http://%s:%d", host, cfg.Prism.Port+1) // API on health port + 1
		token = cfg.API.ResolveAuthToken()
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := runWatch(ctx, baseURL, *subject, token, os.Stdout); err != nil && ctx.Err() == nil {
		fmt.Fprintf(os.Stderr, "watch: %v\n", err)
		os.Exit(1)
	}
}

// runWatch connects to the SSE stream and drives the model→render loop until the
// context is cancelled or the stream ends.
func runWatch(ctx context.Context, baseURL, subject, token string, out io.Writer) error {
	url := strings.TrimRight(baseURL, "/") + "/api/v1/events/stream?subject=" + subject
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("connect to %s: %w (is `prism serve` running?)", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream returned %s", resp.Status)
	}

	model := tracker.New()
	start := time.Now()
	repaint(out, model, frameAt(start))

	dec := sse.NewDecoder(resp.Body)
	// Throttle repaints so a burst of events doesn't thrash the terminal.
	last := time.Time{}
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		ev, err := dec.Next()
		if err != nil {
			return err
		}
		if ev.Event == "" || ev.Event == "connected" || ev.Event == "heartbeat" {
			continue
		}
		var envelope struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if json.Unmarshal(ev.Data, &envelope) != nil || envelope.Type == "" {
			continue
		}
		model.Apply(envelope.Type, envelope.Payload)
		if now := time.Now(); now.Sub(last) > 80*time.Millisecond {
			repaint(out, model, frameAt(start))
			last = now
		}
	}
}

// frameAt maps elapsed wall-clock time to an animation frame index (~8 fps) so the
// current-phase spinner advances between repaints.
func frameAt(start time.Time) int {
	return int(time.Since(start) / (125 * time.Millisecond))
}

// repaint clears the screen and writes the rendered model frame.
func repaint(out io.Writer, m *tracker.Model, frame int) {
	fmt.Fprint(out, "\033[2J\033[H") // clear + cursor home
	fmt.Fprint(out, tracker.RenderFrame(m.Snapshot(), frame))
}
