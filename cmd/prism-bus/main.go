package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"

	"sync/atomic"
	"syscall"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// Event is the canonical Prism event schema.
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

// EventMetadata tracks LLM provenance, cost, and session context.
type EventMetadata struct {
	Model      string  `json:"model,omitempty"`
	PromptHash string  `json:"prompt_hash,omitempty"`
	TokenCost  int     `json:"token_cost,omitempty"`
	SessionID  string  `json:"session_id,omitempty"`
	LatencyMs  int     `json:"latency_ms,omitempty"`
}

var eventCounter atomic.Uint64

func newEvent(eventType, source string, payload map[string]any) Event {
	now := time.Now().UTC()
	n := eventCounter.Add(1)
	return Event{
		ID:        fmt.Sprintf("evt_%d_%06d", now.UnixMilli(), n),
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

func main() {
	log.SetFlags(log.Ltime | log.Lshortfile)
	log.Println("prism: starting event bus...")

	// ── Embedded NATS Server ──────────────────────────────────────
	opts := &server.Options{
		ServerName: "prism-local",
		Port:       4222,
		NoLog:      false,
		Debug:      false,
		Trace:      false,
		JetStream:  true, // Enable JetStream for persistence + replay
		StoreDir:   "./prism-data",
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		log.Fatalf("prism: failed to create NATS server: %v", err)
	}
	go ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		log.Fatalf("prism: NATS server did not start in time")
	}
	log.Printf("prism: NATS server running on %s", ns.ClientURL())

	// ── Connect as a client ───────────────────────────────────────
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		log.Fatalf("prism: failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	// ── Initialize JetStream ──────────────────────────────────────
	js, err := nc.JetStream()
	if err != nil {
		log.Fatalf("prism: failed to init JetStream: %v", err)
	}

	// Create the PRISM stream — all events flow through here
	streamName := "PRISM"
	_, err = js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{"prism.>"},
		Retention: nats.LimitsPolicy, // Keep until limit reached
		MaxMsgs:  1000000,
		MaxBytes: 1024 * 1024 * 1024, // 1GB
		MaxAge:   7 * 24 * time.Hour,  // 7 days retention
		Storage:  nats.FileStorage,     // Durable
	})
	if err != nil {
		log.Fatalf("prism: failed to create stream: %v", err)
	}
	log.Printf("prism: stream '%s' created (subjects: prism.>)", streamName)

	// ── Subscribe: catch-all logger ────────────────────────────────
	logSub, err := js.Subscribe("prism.>", func(msg *nats.Msg) {
		evt, err := eventFromBytes(msg.Data)
		if err != nil {
			log.Printf("  ⚡ [%s] (invalid event: %v)", msg.Subject, err)
			return
		}
		log.Printf("  💎 [%s] id=%s source=%s", evt.Type, evt.ID[:20], evt.Source)
	}, nats.Durable("logger"), nats.DeliverAll())
	if err != nil {
		log.Fatalf("prism: failed to create logger subscription: %v", err)
	}
	defer logSub.Unsubscribe()

	// ── Subscribe: agent decision handler ──────────────────────────
	decisionSub, err := js.Subscribe("prism.agent.decision", func(msg *nats.Msg) {
		evt, err := eventFromBytes(msg.Data)
		if err != nil {
			return
		}
		action, _ := evt.Payload["action"].(string)
		confidence, _ := evt.Payload["confidence"].(float64)
		log.Printf("  🧠 AGENT DECISION: action=%s confidence=%.0f%% source=%s", action, confidence*100, evt.Source)

		// React: emit a follow-up event (demonstrates event chain)
		if action == "spawn_coder" {
			spawnEvt := newEvent("prism.agent.spawned", "prism-bus", map[string]any{
				"agent_type": "coder",
				"reason":     "agent decision requested spawn",
			})
			spawnEvt.CorrelationID = evt.CorrelationID
			spawnEvt.ParentID = evt.ID
			data, _ := json.Marshal(spawnEvt)
			js.Publish("prism.agent.spawned", data)
		}
	}, nats.Durable("decision-handler"), nats.DeliverAll())
	if err != nil {
		log.Fatalf("prism: failed to create decision subscription: %v", err)
	}
	defer decisionSub.Unsubscribe()

	// ── Subscribe: memory store handler ────────────────────────────
	memorySub, err := js.Subscribe("prism.memory.stored", func(msg *nats.Msg) {
		evt, err := eventFromBytes(msg.Data)
		if err != nil {
			return
		}
		category, _ := evt.Payload["category"].(string)
		tier, _ := evt.Payload["tier"].(string)
		log.Printf("  📝 MEMORY STORED: category=%s tier=%s", category, tier)
	}, nats.Durable("memory-store"), nats.DeliverAll())
	if err != nil {
		log.Fatalf("prism: failed to create memory subscription: %v", err)
	}
	defer memorySub.Unsubscribe()

	// ── Publish test events ───────────────────────────────────────
	log.Println("prism: publishing test events...")
	correlationID := fmt.Sprintf("corr_%d", time.Now().UnixMilli())

	// Event 1: A message arrives from Discord
	evt1 := newEvent("prism.channel.received", "discord", map[string]any{
		"channel":    "discord",
		"channel_id": "1491622581348864162",
		"sender":    "ema",
		"text":      "deploy the new feature",
	})
	evt1.CorrelationID = correlationID
	data1, _ := json.Marshal(evt1)
	js.Publish("prism.channel.received", data1)

	time.Sleep(100 * time.Millisecond)

	// Event 2: Agent makes a decision (fan-out reaction)
	evt2 := newEvent("prism.agent.decision", "lumi", map[string]any{
		"reasoning":  "User wants deployment, spawning coder agent",
		"action":     "spawn_coder",
		"confidence": 0.92,
	})
	evt2.CorrelationID = correlationID
	evt2.ParentID = evt1.ID
	data2, _ := json.Marshal(evt2)
	js.Publish("prism.agent.decision", data2)

	time.Sleep(100 * time.Millisecond)

	// Event 3: Memory is stored
	evt3 := newEvent("prism.memory.stored", "lumi", map[string]any{
		"category": "decision",
		"tier":     "session",
		"content":  "Ema requested deployment of new feature",
	})
	evt3.CorrelationID = correlationID
	evt3.ParentID = evt2.ID
	data3, _ := json.Marshal(evt3)
	js.Publish("prism.memory.stored", data3)

	log.Println("prism: 3 test events published")
	log.Println("prism: event bus running. press ctrl+c to stop.")

	// ── Print stream stats ────────────────────────────────────────
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			info, err := js.StreamInfo(streamName)
			if err != nil {
				continue
			}
			log.Printf("  📊 stream: %d messages, %d bytes", info.State.Msgs, info.State.Bytes)
		}
	}()



	// ── Wait for shutdown signal ───────────────────────────────────
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("prism: shutting down...")
	logSub.Unsubscribe()
	decisionSub.Unsubscribe()
	memorySub.Unsubscribe()
	nc.Close()
	ns.Shutdown()
	log.Println("prism: stopped.")
}