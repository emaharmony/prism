package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nats-io/nats.go"
)

// AgentConfig defines an agent that subscribes to events and processes them.
type AgentConfig struct {
	Name       string   `json:"name"`
	Subscribes []string `json:"subscribes"`
	Model      string   `json:"model,omitempty"`
	MaxPending  int      `json:"max_pending,omitempty"`
}

// AgentRuntime connects to the Prism bus, subscribes to events,
// and dispatches them to registered agent handlers.
type AgentRuntime struct {
	nc      *nats.Conn
	js      nats.JetStreamContext
	config  AgentConfig
	handler func(Event)
	mu      sync.Mutex
	pending int
}

var (
	busURL    = flag.String("bus", "nats://localhost:4222", "Prism bus URL")
	agentName = flag.String("name", "", "Agent name (required)")
	subs      = flag.String("subs", "", "Comma-separated event subjects to subscribe to")
	model     = flag.String("model", "", "LLM model to use for this agent")
)

func main() {
	flag.Parse()

	if *agentName == "" {
		log.Fatal("prism-agent: -name is required")
	}

	config := AgentConfig{
		Name:       *agentName,
		Subscribes: parseSubs(*subs),
		Model:      *model,
		MaxPending: 100,
	}

	if len(config.Subscribes) == 0 {
		config.Subscribes = []string{"prism.>"}
		log.Printf("prism-agent: no subscriptions specified, defaulting to prism.>")
	}

	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Printf("prism-agent: starting agent '%s'...", config.Name)

	// ── Connect to NATS ──────────────────────────────────────────
	nc, err := nats.Connect(*busURL, nats.Name(fmt.Sprintf("agent-%s", config.Name)))
	if err != nil {
		log.Fatalf("prism-agent: failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("prism-agent: failed to init JetStream: %v", err)
	}

	log.Printf("prism-agent: connected to %s", *busURL)

	// ── Create Agent Runtime ──────────────────────────────────────
	runtime := &AgentRuntime{
		nc:     nc,
		js:     js,
		config: config,
		handler: func(evt Event) {
			// Default handler: log the event and emit a processed event
			log.Printf("  🔮 [%s] received %s from %s", config.Name, evt.Type, evt.Source)
		},
		mu:      sync.Mutex{},
		pending: 0,
	}

	// ── Subscribe to configured subjects ───────────────────────────
	for _, subject := range config.Subscribes {
		durableName := fmt.Sprintf("agent-%s-%s", config.Name, sanitizeSubject(subject))

		sub, err := js.Subscribe(subject, runtime.handleMessage, nats.Durable(durableName))
		if err != nil {
			log.Fatalf("prism-agent: failed to subscribe to %s: %v", subject, err)
		}
		defer sub.Unsubscribe()
		log.Printf("prism-agent: subscribed to %s (durable=%s)", subject, durableName)
	}

	// ── Publish agent.started event ────────────────────────────────
	runtime.publish("prism.agent.started", map[string]any{
		"agent_name": config.Name,
		"model":      config.Model,
		"subscribes": config.Subscribes,
	})

	log.Printf("prism-agent: '%s' running. press ctrl+c to stop.", config.Name)

	// ── Wait for shutdown ──────────────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	// Periodic heartbeat
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			runtime.mu.Lock()
			pending := runtime.pending
			runtime.mu.Unlock()
			runtime.publish("prism.agent.heartbeat", map[string]any{
				"agent_name": config.Name,
				"pending":    pending,
				"status":     "running",
			})
		}
	}()

	<-sigCh

	log.Printf("prism-agent: '%s' shutting down...", config.Name)
	runtime.publish("prism.agent.stopped", map[string]any{
		"agent_name": config.Name,
		"status":     "graceful_shutdown",
	})
	time.Sleep(500 * time.Millisecond) // Flush pending messages
}

func (r *AgentRuntime) handleMessage(msg *nats.Msg) {
	evt, err := eventFromBytes(msg.Data)
	if err != nil {
		log.Printf("  ⚠️ [%s] invalid event: %v", r.config.Name, err)
		return
	}

	r.mu.Lock()
	r.pending++
	pending := r.pending
	r.mu.Unlock()

	// Call the handler
	r.handler(evt)

	r.mu.Lock()
	r.pending--
	r.mu.Unlock()

	// Emit agent.decision event (agents process events and produce decisions)
	// This is a placeholder — real agents will have custom logic
	log.Printf("  🔮 [%s] processed %s (pending=%d)", r.config.Name, evt.Type, pending-1)
}

func (r *AgentRuntime) publish(eventType string, payload map[string]any) Event {
	evt := newEvent(eventType, r.config.Name, payload)
	data, err := json.Marshal(evt)
	if err != nil {
		log.Printf("  ⚠️ [%s] failed to marshal event: %v", r.config.Name, err)
		return evt
	}
	if _, err := r.js.Publish(eventType, data); err != nil {
		log.Printf("  ⚠️ [%s] failed to publish %s: %v", r.config.Name, eventType, err)
	}
	return evt
}

// sanitizeSubject replaces NATS-invalid characters for durable consumer names.
func sanitizeSubject(s string) string {
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, "*", "all")
	s = strings.ReplaceAll(s, ">", "all")
	if len(s) > 48 {
		s = s[:48]
	}
	return s
}

func parseSubs(s string) []string {
	if s == "" {
		return nil
	}
	subs := strings.Split(s, ",")
	result := make([]string, 0, len(subs))
	for _, sub := range subs {
		sub = strings.TrimSpace(sub)
		if sub != "" {
			result = append(result, sub)
		}
	}
	return result
}

// ── Event types (shared with prism-bus) ────────────────────────────
// In production, these would be in a shared package.

type Event struct {
	ID            string         `json:"id"`
	Type          string         `json:"type"`
	Source        string         `json:"source"`
	Timestamp     string         `json:"timestamp"`
	CorrelationID string         `json:"correlation_id"`
	ParentID      string         `json:"parent_id,omitempty"`
	Payload       map[string]any `json:"payload"`
	Metadata      EventMetadata  `json:"metadata"`
}

type EventMetadata struct {
	Model      string `json:"model,omitempty"`
	PromptHash string `json:"prompt_hash,omitempty"`
	TokenCost  int    `json:"token_cost,omitempty"`
	SessionID  string `json:"session_id,omitempty"`
	LatencyMs  int    `json:"latency_ms,omitempty"`
}

var eventCounter int64

func newEvent(eventType, source string, payload map[string]any) Event {
	eventCounter++
	now := time.Now().UTC()
	return Event{
		ID:        fmt.Sprintf("evt_%d_%06d", now.UnixMilli(), eventCounter),
		Type:      eventType,
		Source:    source,
		Timestamp: now.Format(time.RFC3339Nano),
		Payload:   payload,
	}
}

func eventFromBytes(data []byte) (Event, error) {
	var evt Event
	err := json.Unmarshal(data, &evt)
	return evt, err
}

// Unused import for context (will be needed for LLM calls)
var _ = context.Background