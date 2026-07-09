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
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/sse"
)

// phaseView is the accumulated live state of a single workflow phase.
type phaseView struct {
	name       string
	status     string // entered | paused | passed | fallback | stuck
	gateScore  float64
	gatePassed bool
	gateSeen   bool
	lastTool   string
	toolRetry  int
	verifyText string
}

// watchModel accumulates workflow state from the event stream. It is intentionally
// decoupled from any I/O so its update + render logic is unit-testable.
type watchModel struct {
	workflow    string
	order       []string // phase names in the order first seen
	phases      map[string]*phaseView
	current     string
	status      string // running | paused | completed | blocked
	tokTotal    int
	tokMax      int
	delegations map[string]string // task_id -> "agent:status"
	lastEvent   string
	events      int
}

func newWatchModel() *watchModel {
	return &watchModel{
		phases:      map[string]*phaseView{},
		delegations: map[string]string{},
		status:      "connecting",
	}
}

func (m *watchModel) phase(name string) *phaseView {
	if name == "" {
		return &phaseView{}
	}
	pv, ok := m.phases[name]
	if !ok {
		pv = &phaseView{name: name, status: "pending"}
		m.phases[name] = pv
		m.order = append(m.order, name)
	}
	return pv
}

// num safely reads a JSON number (decoded as float64) from a payload.
func num(p map[string]any, key string) float64 {
	if v, ok := p[key].(float64); ok {
		return v
	}
	return 0
}

func str(p map[string]any, key string) string {
	if v, ok := p[key].(string); ok {
		return v
	}
	return ""
}

// apply updates the model from one event. evType is the engine event type
// (e.g. "phase.entered"); payload is the event's payload map.
func (m *watchModel) apply(evType string, payload map[string]any) {
	m.events++
	m.lastEvent = evType
	switch evType {
	case "workflow.started":
		m.workflow = str(payload, "workflow")
		m.status = "running"
	case "phase.entered":
		name := str(payload, "phase")
		m.current = name
		pv := m.phase(name)
		pv.status = "running"
		if m.status == "connecting" {
			m.status = "running"
		}
	case "phase.tokens":
		m.tokTotal = int(num(payload, "total"))
		if mx := int(num(payload, "max")); mx > 0 {
			m.tokMax = mx
		}
		if name := str(payload, "phase"); name != "" {
			m.current = name
		}
	case "phase.gate_check":
		pv := m.phase(str(payload, "phase"))
		pv.gateSeen = true
		pv.gateScore = num(payload, "score")
		pv.gatePassed, _ = payload["passed"].(bool)
		if pv.gatePassed {
			pv.status = "passed"
		}
	case "phase.verification":
		pv := m.phase(str(payload, "phase"))
		passed, _ := payload["passed"].(bool)
		mark := "FAIL"
		if passed {
			mark = "pass"
		}
		pv.verifyText = fmt.Sprintf("%s %s(exit %d)", str(payload, "profile"), mark, int(num(payload, "exit_code")))
	case "phase.fallback":
		m.phase(str(payload, "phase")).status = "fallback"
	case "phase.stuck":
		pv := m.phase(str(payload, "phase"))
		pv.status = "stuck"
		pv.lastTool = str(payload, "tool")
	case "tool.retry":
		pv := m.phase(str(payload, "phase"))
		pv.lastTool = str(payload, "tool")
		pv.toolRetry = int(num(payload, "attempt"))
	case "workflow.paused":
		m.status = "paused"
		if name := str(payload, "phase"); name != "" {
			m.phase(name).status = "paused"
		}
	case "workflow.budget_exhausted":
		m.status = "budget exhausted"
	case "phase.budget_exhausted":
		if name := str(payload, "phase"); name != "" {
			m.phase(name).status = "budget"
		}
	case "workflow.completed":
		// The terminal event carries the real outcome so a budget-killed run
		// isn't masked as "completed" in the live view.
		if s := str(payload, "status"); s != "" && s != "completed" {
			m.status = strings.ReplaceAll(s, "_", " ")
		} else {
			m.status = "completed"
		}
	case "workflow.blocked":
		m.status = "blocked"
	case "task.delegated":
		if id := str(payload, "task_id"); id != "" {
			m.delegations[id] = str(payload, "agent") + ":sent"
		}
	case "delegation.retry":
		m.markDelegations(payload, "retrying")
	case "delegation.timeout":
		m.markDelegations(payload, "timed_out")
	}
}

func (m *watchModel) markDelegations(payload map[string]any, status string) {
	tasks, _ := payload["tasks"].([]any)
	for _, t := range tasks {
		id, _ := t.(string)
		if id == "" {
			continue
		}
		agent := "?"
		if prev, ok := m.delegations[id]; ok {
			agent = strings.SplitN(prev, ":", 2)[0]
		}
		m.delegations[id] = agent + ":" + status
	}
}

// render produces the full-screen view for the current model state.
func (m *watchModel) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "🔮 prism watch")
	if m.workflow != "" {
		fmt.Fprintf(&b, " — %s", m.workflow)
	}
	fmt.Fprintf(&b, "   [%s]\n", m.status)
	b.WriteString(strings.Repeat("─", 56) + "\n")

	// Token / budget burn-down meter.
	if m.tokTotal > 0 || m.tokMax > 0 {
		b.WriteString("Budget  " + tokenMeter(m.tokTotal, m.tokMax) + "\n\n")
	}

	// Phase tree.
	b.WriteString("Phases\n")
	for _, name := range m.order {
		pv := m.phases[name]
		fmt.Fprintf(&b, "  %s %-13s", phaseGlyph(pv, name == m.current), name)
		if pv.gateSeen {
			fmt.Fprintf(&b, " gate %.2f", pv.gateScore)
		}
		if pv.verifyText != "" {
			fmt.Fprintf(&b, " · ✓ %s", pv.verifyText)
		}
		if pv.lastTool != "" {
			fmt.Fprintf(&b, " · %s", pv.lastTool)
			if pv.toolRetry > 0 {
				fmt.Fprintf(&b, " (retry %d)", pv.toolRetry)
			}
		}
		b.WriteString("\n")
	}

	// Delegations.
	if len(m.delegations) > 0 {
		b.WriteString("\nDelegations\n")
		ids := make([]string, 0, len(m.delegations))
		for id := range m.delegations {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			fmt.Fprintf(&b, "  %-8s %s\n", id, m.delegations[id])
		}
	}

	fmt.Fprintf(&b, "\n%d events · last: %s\n", m.events, m.lastEvent)
	return b.String()
}

func phaseGlyph(pv *phaseView, isCurrent bool) string {
	switch pv.status {
	case "passed":
		return "✅"
	case "fallback":
		return "⚠️ "
	case "stuck", "blocked":
		return "❌"
	case "paused":
		return "⏸️ "
	}
	if isCurrent {
		return "▶️ "
	}
	return "·"
}

// tokenMeter renders a 20-cell progress bar for token usage against the budget.
func tokenMeter(total, max int) string {
	if max <= 0 {
		return fmt.Sprintf("%s tokens (no ceiling)", humanInt(total))
	}
	const width = 20
	frac := float64(total) / float64(max)
	if frac > 1 {
		frac = 1
	}
	filled := int(frac * width)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
	return fmt.Sprintf("[%s] %s/%s (%.0f%%)", bar, humanInt(total), humanInt(max), frac*100)
}

func humanInt(n int) string {
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}

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

	model := newWatchModel()
	repaint(out, model)

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
		model.apply(envelope.Type, envelope.Payload)
		if now := time.Now(); now.Sub(last) > 80*time.Millisecond {
			repaint(out, model)
			last = now
		}
	}
}

// repaint clears the screen and writes the rendered model.
func repaint(out io.Writer, m *watchModel) {
	fmt.Fprint(out, "\033[2J\033[H") // clear + cursor home
	fmt.Fprint(out, m.render())
}
