package run_test

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats-server/v2/server"

	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/run"
)

// startTestServer starts an embedded NATS server for testing.
func startTestServer(t *testing.T) (*server.Server, string) {
	t.Helper()
	s, err := server.NewServer(&server.Options{
		Port:      -1, // Random available port
		StoreDir:  t.TempDir(),
		JetStream: true,
	})
	if err != nil {
		t.Fatalf("failed to create NATS server: %v", err)
	}
	go s.Start()
	if !s.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server did not start in time")
	}
	url := fmt.Sprintf("nats://127.0.0.1:%d", s.Addr().(*net.TCPAddr).Port)
	return s, url
}

func TestV1LifecycleWithoutMemory(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test V1 event lifecycle",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: false,
		RunDir:        filepath.Join(tmpDir, "runs"),
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("V1 run failed: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", result.Status)
	}
	if result.RunID == "" {
		t.Error("expected non-empty run_id")
	}
	if result.EventCount < 6 {
		// task.created, task.started, agent.started, agent.output, agent.completed, task.completed
		t.Errorf("expected at least 6 events, got %d", result.EventCount)
	}

	// Verify events.jsonl exists and is valid
	eventsPath := filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 6 {
		t.Errorf("expected at least 6 lines in events.jsonl, got %d", len(lines))
	}

	// Verify each line is valid JSON with correct structure
	for i, line := range lines {
		var evt event.Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
			continue
		}
		if !strings.HasPrefix(evt.ID, "evt_") {
			t.Errorf("line %d: event ID doesn't start with evt_: %s", i, evt.ID)
		}
		if !strings.HasPrefix(evt.Type, "prism.") {
			t.Errorf("line %d: event type doesn't start with prism.: %s", i, evt.Type)
		}
		if evt.CorrelationID == "" {
			t.Errorf("line %d: event has no correlation_id", i)
		}
	}

	// Verify summary.json exists
	summaryPath := filepath.Join(tmpDir, "runs", result.RunID, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary.json: %v", err)
	}

	var summary event.Summary
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatalf("failed to parse summary.json: %v", err)
	}
	if summary.Status != "completed" {
		t.Errorf("expected summary status 'completed', got %s", summary.Status)
	}
	if summary.RunID != result.RunID {
		t.Errorf("summary run_id mismatch: %s != %s", summary.RunID, result.RunID)
	}
}

func TestV1LifecycleWithMemoryFailure(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:           "Test with memory failure",
		Project:        "prism",
		Agent:          "lumi",
		BusURL:         busURL,
		MemoryEnabled:  true,
		RequireMemory:  false, // Should continue even if memory fails
		MemoryURL:      "http://localhost:18790", // Not running
		RunDir:         filepath.Join(tmpDir, "runs"),
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("V1 run with memory failure should not error: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", result.Status)
	}
	// Should have memory.context_requested + memory.context_failed events
	if result.EventCount < 8 {
		t.Errorf("expected at least 8 events (incl. memory events), got %d", result.EventCount)
	}
}

func TestV1LifecycleRequireMemoryFailure(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:           "Test require-memory failure",
		Project:        "prism",
		Agent:          "lumi",
		BusURL:         busURL,
		MemoryEnabled:  true,
		RequireMemory:  true, // Should fail if memory is unavailable
		MemoryURL:      "http://localhost:18790", // Not running
		RunDir:         filepath.Join(tmpDir, "runs"),
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err == nil {
		t.Fatal("expected error when require-memory=true and memory is unavailable")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on failure")
	}
	if result.Status != "failed" {
		t.Errorf("expected status 'failed', got %s", result.Status)
	}
}

func TestV1EventCorrelationID(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test correlation ID propagation",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: false,
		RunDir:        filepath.Join(tmpDir, "runs"),
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()
	if err != nil {
		t.Fatalf("V1 run failed: %v", err)
	}

	// Read events and verify all have the same correlation_id
	eventsPath := filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}

	var correlationIDs []string
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		var evt event.Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		correlationIDs = append(correlationIDs, evt.CorrelationID)
	}

	if len(correlationIDs) == 0 {
		t.Fatal("no events found")
	}

	firstCorrID := correlationIDs[0]
	for i, cid := range correlationIDs {
		if cid != firstCorrID {
			t.Errorf("event %d: correlation_id mismatch: %s != %s", i, cid, firstCorrID)
		}
	}
}

func TestV1EventTypesInOrder(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test event order",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: false,
		RunDir:        filepath.Join(tmpDir, "runs"),
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()
	if err != nil {
		t.Fatalf("V1 run failed: %v", err)
	}

	// Read events and verify they appear in expected order
	eventsPath := filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}

	var types []string
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		var evt event.Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		types = append(types, evt.Type)
	}

	// Expected order for no-memory run
	expected := []string{
		event.V1EventTypes.TaskCreated,
		event.V1EventTypes.TaskStarted,
		event.V1EventTypes.AgentStarted,
		event.V1EventTypes.AgentOutput,
		event.V1EventTypes.AgentCompleted,
		event.V1EventTypes.TaskCompleted,
	}

	if len(types) < len(expected) {
		t.Fatalf("expected at least %d events, got %d", len(expected), len(types))
	}

	for i, exp := range expected {
		if types[i] != exp {
			t.Errorf("event %d: expected type %s, got %s", i, exp, types[i])
		}
	}
}

func TestV1NATSPublishAndSubscribe(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	// Set up a subscriber BEFORE the run to capture events
	nc, err := nats.Connect(busURL)
	if err != nil {
		t.Fatalf("failed to connect to NATS: %v", err)
	}
	defer nc.Close()

	js, err := nc.JetStream()
	if err != nil {
		t.Fatalf("failed to get JetStream context: %v", err)
	}

	// Create stream
	_, _ = js.AddStream(&nats.StreamConfig{
		Name:      "PRISM",
		Subjects: []string{"prism.>"},
		Retention: nats.LimitsPolicy,
		MaxMsgs:   1000000,
		Storage:   nats.MemoryStorage,
	})

	// Subscribe to all events
	received := make([]event.Event, 0)
	sub, err := js.Subscribe("prism.>", func(msg *nats.Msg) {
		var evt event.Event
		if err := json.Unmarshal(msg.Data, &evt); err == nil {
			received = append(received, evt)
		}
		msg.Ack()
	}, nats.Durable("test-sub"), nats.ManualAck())
	if err != nil {
		t.Fatalf("failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Give subscriber time to start
	time.Sleep(100 * time.Millisecond)

	// Run the lifecycle
	tmpDir := t.TempDir()
	cfg := run.RunConfig{
		Task:          "Test NATS pub/sub",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: false,
		RunDir:        filepath.Join(tmpDir, "runs"),
	}

	runner := run.NewRunner(cfg)
	_, err = runner.Run()
	if err != nil {
		t.Fatalf("V1 run failed: %v", err)
	}

	// Give events time to arrive
	time.Sleep(500 * time.Millisecond)

	// Should have received at least 6 events
	if len(received) < 6 {
		t.Errorf("expected at least 6 events received via NATS, got %d", len(received))
	}
}