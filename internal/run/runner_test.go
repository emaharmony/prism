package run_test

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats-server/v2/server"

	"github.com/emaharmony/prism/internal/event"
	"github.com/emaharmony/prism/internal/provider"
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

	// Expected order for V2 no-memory run (provider replaced agent output/completed)
	expected := []string{
		event.V1EventTypes.TaskCreated,
		event.V1EventTypes.TaskStarted,
		event.V1EventTypes.AgentStarted,
		event.V2EventTypes.LLMRequested,
		event.V2EventTypes.LLMCompleted,
		event.V1EventTypes.AgentCompleted,
		event.V2EventTypes.OutputWritten,
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

func TestV1Remembrance404(t *testing.T) {
	// Start a test HTTP server that returns 404 for the context-build endpoint
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	// Simple HTTP server returning 404
	httpServer := &http.Server{Addr: "127.0.0.1:0"}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	go httpServer.Serve(listener)
	defer httpServer.Close()

	memoryURL := fmt.Sprintf("http://%s", listener.Addr().String())

	tmpDir := t.TempDir()
	cfg := run.RunConfig{
		Task:           "Test remembrance 404",
		Project:        "prism",
		Agent:          "lumi",
		BusURL:         busURL,
		MemoryEnabled:  true,
		RequireMemory:  false,
		MemoryURL:      memoryURL,
		RunDir:         filepath.Join(tmpDir, "runs"),
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("V1 run with 404 memory should not error: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", result.Status)
	}

	// Read events and verify memory.context_failed was emitted
	eventsPath := filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}

	foundContextRequested := false
	foundContextFailed := false
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		var evt event.Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		if evt.Type == "prism.memory.context_requested" {
			foundContextRequested = true
		}
		if evt.Type == "prism.memory.context_failed" {
			foundContextFailed = true
		}
	}

	if !foundContextRequested {
		t.Error("expected prism.memory.context_requested event")
	}
	if !foundContextFailed {
		t.Error("expected prism.memory.context_failed event for 404 response")
	}
}

func TestV1ParentIDInNATS(t *testing.T) {
	// Verify that events published to NATS include parent_id
	// This tests the fix for the emitWithParent bug
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()
	cfg := run.RunConfig{
		Task:          "Test parent_id in NATS",
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

	// Read events.jsonl and verify parent_ids are set
	eventsPath := filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}

	var events []event.Event
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		var evt event.Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		events = append(events, evt)
	}

	// Build a map of event types to their parent IDs
	parentIDs := make(map[string]string)
	for _, evt := range events {
		parentIDs[evt.Type] = evt.ParentID
	}

	// task.started should have parent = task.created
	taskCreatedID := ""
	for _, evt := range events {
		if evt.Type == "prism.task.created" {
			taskCreatedID = evt.ID
		}
	}

	if taskCreatedID == "" {
		t.Fatal("task.created event not found")
	}

	// task.started should be parented to task.created
	if parentIDs["prism.task.started"] != taskCreatedID {
		t.Errorf("task.started parent_id = %q, want %q (task.created ID)", parentIDs["prism.task.started"], taskCreatedID)
	}

	// agent.started should be parented to task.started
	taskStartedID := ""
	for _, evt := range events {
		if evt.Type == "prism.task.started" {
			taskStartedID = evt.ID
		}
	}
	if parentIDs["prism.agent.started"] != taskStartedID {
		t.Errorf("agent.started parent_id = %q, want %q (task.started ID)", parentIDs["prism.agent.started"], taskStartedID)
	}
}

// parseEventsFile reads events.jsonl and returns parsed events
func parseEventsFile(t *testing.T, eventsPath string) []event.Event {
	t.Helper()
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("failed to read events.jsonl: %v", err)
	}
	var events []event.Event
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		var evt event.Event
		if err := json.Unmarshal([]byte(line), &evt); err != nil {
			continue
		}
		events = append(events, evt)
	}
	return events
}

// eventTypes returns the ordered list of event types
func eventTypes(events []event.Event) []string {
	types := make([]string, len(events))
	for i, e := range events {
		types[i] = e.Type
	}
	return types
}

// assertContainsTypes verifies that expectedTypes appear in order within actualTypes.
// Not necessarily consecutively — just in the right relative order.
func assertContainsTypes(t *testing.T, actualTypes []string, expectedTypes []string) {
	t.Helper()
	ei := 0
	for _, at := range actualTypes {
		if ei < len(expectedTypes) && at == expectedTypes[ei] {
			ei++
		}
	}
	if ei != len(expectedTypes) {
		t.Errorf("expected event types in order %v, but not all found in %v", expectedTypes, actualTypes)
	}
}

// ============================================================================
// V2 Tests
// ============================================================================

func TestV2MockProviderSuccess(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test V2 event lifecycle",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: false,
		RunDir:        filepath.Join(tmpDir, "runs"),
		Provider:      provider.NewMockProvider(),
		Model:         "mock-model",
		Temperature:   0.2,
		MaxTokens:     2048,
		Timeout:       60 * time.Second,
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("V2 run failed: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", result.Status)
	}
	if result.RunID == "" {
		t.Error("expected non-empty run_id")
	}

	// Verify events.jsonl
	events := parseEventsFile(t, filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl"))
	types := eventTypes(events)

	// Expected V2 lifecycle: task.created, task.started, agent.started,
	// llm.requested, llm.completed, agent.completed, output.written, task.completed
	expected := []string{
		event.V1EventTypes.TaskCreated,
		event.V1EventTypes.TaskStarted,
		event.V1EventTypes.AgentStarted,
		event.V2EventTypes.LLMRequested,
		event.V2EventTypes.LLMCompleted,
		event.V1EventTypes.AgentCompleted,
		event.V2EventTypes.OutputWritten,
		event.V1EventTypes.TaskCompleted,
	}

	if len(events) < len(expected) {
		t.Fatalf("expected at least %d events, got %d", len(expected), len(events))
	}

	assertContainsTypes(t, types, expected)

	// Verify parent_id chains (V2 causal chain):
	// task.started -> task.created
	// agent.started -> task.started
	// llm.requested -> agent.started
	// llm.completed -> llm.requested
	// agent.completed -> llm.completed
	// output.written -> agent.completed
	// task.completed -> agent.completed
	parentMap := make(map[string]string)
	idMap := make(map[string]string)
	for _, evt := range events {
		parentMap[evt.Type] = evt.ParentID
		idMap[evt.Type] = evt.ID
	}

	if parentMap[event.V1EventTypes.TaskStarted] != idMap[event.V1EventTypes.TaskCreated] {
		t.Error("task.started parent should be task.created")
	}
	if parentMap[event.V1EventTypes.AgentStarted] != idMap[event.V1EventTypes.TaskStarted] {
		t.Error("agent.started parent should be task.started")
	}
	if parentMap[event.V2EventTypes.LLMRequested] != idMap[event.V1EventTypes.AgentStarted] {
		t.Error("llm.requested parent should be agent.started")
	}
	if parentMap[event.V2EventTypes.LLMCompleted] != idMap[event.V2EventTypes.LLMRequested] {
		t.Error("llm.completed parent should be llm.requested")
	}
	if parentMap[event.V1EventTypes.AgentCompleted] != idMap[event.V2EventTypes.LLMCompleted] {
		t.Error("agent.completed parent should be llm.completed")
	}
	if parentMap[event.V2EventTypes.OutputWritten] != idMap[event.V1EventTypes.AgentCompleted] {
		t.Error("output.written parent should be agent.completed")
	}
	if parentMap[event.V1EventTypes.TaskCompleted] != idMap[event.V1EventTypes.AgentCompleted] {
		t.Error("task.completed parent should be agent.completed")
	}

	// Verify correlation_id propagation
	var corrID string
	for _, evt := range events {
		if corrID == "" {
			corrID = evt.CorrelationID
		}
		if evt.CorrelationID != corrID {
			t.Errorf("event %s has different correlation_id: %s != %s", evt.Type, evt.CorrelationID, corrID)
		}
	}

	// Verify summary.json has V2 fields
	summaryPath := filepath.Join(tmpDir, "runs", result.RunID, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary.json: %v", err)
	}

	var summary event.Summary
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatalf("failed to parse summary.json: %v", err)
	}
	if summary.Provider == "" {
		t.Error("summary missing Provider field")
	}
	if summary.Model == "" {
		t.Error("summary missing Model field")
	}
	if summary.OutputPath == "" {
		t.Error("summary missing OutputPath field")
	}
	if summary.PromptPath == "" {
		t.Error("summary missing PromptPath field")
	}
	if summary.MemoryStatus != "none" {
		t.Errorf("expected MemoryStatus 'none', got '%s'", summary.MemoryStatus)
	}

	// Verify prompt.md and output.md exist
	promptPath := filepath.Join(tmpDir, "runs", result.RunID, "prompt.md")
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		t.Error("prompt.md does not exist")
	}
	outputPath := filepath.Join(tmpDir, "runs", result.RunID, "output.md")
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Error("output.md does not exist")
	}

	// Verify output.md has content
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output.md: %v", err)
	}
	if !strings.Contains(string(outputData), "Mock response") {
		t.Error("output.md does not contain mock response")
	}
}

func TestV2MockProviderWithMemory(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	// Start a mock remembrance server
	mockMemory := startMockMemoryServer(t)
	defer mockMemory.Close()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test V2 with memory",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: true,
		RequireMemory: false,
		MemoryURL:     mockMemory.URL,
		RunDir:        filepath.Join(tmpDir, "runs"),
		Provider:      provider.NewMockProvider(),
		Model:         "mock-model",
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("V2 run with memory failed: %v", err)
	}

	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", result.Status)
	}

	// Verify events contain V2 context events
	events := parseEventsFile(t, filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl"))
	types := eventTypes(events)

	expected := []string{
		event.V1EventTypes.TaskCreated,
		event.V1EventTypes.TaskStarted,
		event.V1EventTypes.MemoryContextRequested,
		event.V2EventTypes.ContextRequested,
		event.V1EventTypes.MemoryContextBuilt,
		event.V2EventTypes.ContextInjected,
		event.V1EventTypes.AgentStarted,
		event.V2EventTypes.LLMRequested,
		event.V2EventTypes.LLMCompleted,
		event.V1EventTypes.AgentCompleted,
		event.V2EventTypes.OutputWritten,
		event.V1EventTypes.TaskCompleted,
	}

	assertContainsTypes(t, types, expected)

	// Verify V2 context events have correct parent chains
	// context.requested -> task.started
	// context.injected -> task.started
	var taskStartedID string
	for _, evt := range events {
		if evt.Type == event.V1EventTypes.TaskStarted {
			taskStartedID = evt.ID
			break
		}
	}
	for _, evt := range events {
		if evt.Type == event.V2EventTypes.ContextRequested {
			if evt.ParentID != taskStartedID {
				t.Errorf("context.requested parent should be task.started, got %s", evt.ParentID)
			}
		}
		if evt.Type == event.V2EventTypes.ContextInjected {
			if evt.ParentID != taskStartedID {
				t.Errorf("context.injected parent should be task.started, got %s", evt.ParentID)
			}
		}
	}

	// Verify summary MemoryStatus is "injected"
	summaryPath := filepath.Join(tmpDir, "runs", result.RunID, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary.json: %v", err)
	}
	var summary event.Summary
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatalf("failed to parse summary.json: %v", err)
	}
	if summary.MemoryStatus != "injected" {
		t.Errorf("expected MemoryStatus 'injected', got '%s'", summary.MemoryStatus)
	}
	if !summary.MemoryUsed {
		t.Error("expected MemoryUsed to be true")
	}

	// Verify prompt.md includes context section
	promptPath := filepath.Join(tmpDir, "runs", result.RunID, "prompt.md")
	promptData, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("failed to read prompt.md: %v", err)
	}
	if !strings.Contains(string(promptData), "## Retrieved Context") {
		t.Error("prompt.md should contain context section")
	}
}

func TestV2LLMFailure(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test V2 LLM failure",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: false,
		RunDir:        filepath.Join(tmpDir, "runs"),
		Provider:      provider.NewFailingMockProvider(),
		Model:         "mock-model",
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err == nil {
		t.Fatal("expected error when provider fails")
	}
	if result == nil {
		t.Fatal("expected non-nil result even on failure")
	}
	if result.Status != "failed" {
		t.Errorf("expected status 'failed', got %s", result.Status)
	}

	// Verify event sequence: llm.failed -> agent.failed -> task.failed
	events := parseEventsFile(t, filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl"))
	types := eventTypes(events)

	expected := []string{
		event.V1EventTypes.TaskCreated,
		event.V1EventTypes.TaskStarted,
		event.V1EventTypes.AgentStarted,
		event.V2EventTypes.LLMRequested,
		event.V2EventTypes.LLMFailed,
		event.V2EventTypes.AgentFailed,
		event.V1EventTypes.TaskFailed,
	}

	assertContainsTypes(t, types, expected)

	// Verify parent_id chain for failure path:
	// task.started -> task.created
	// agent.started -> task.started
	// llm.requested -> agent.started
	// llm.failed -> llm.requested
	// agent.failed -> llm.failed
	// task.failed -> task.started
	parentMap := make(map[string]string)
	idMap := make(map[string]string)
	for _, evt := range events {
		parentMap[evt.Type] = evt.ParentID
		idMap[evt.Type] = evt.ID
	}
	if parentMap[event.V2EventTypes.AgentFailed] != idMap[event.V2EventTypes.LLMFailed] {
		t.Error("agent.failed parent should be llm.failed")
	}
	if parentMap[event.V2EventTypes.LLMFailed] != idMap[event.V2EventTypes.LLMRequested] {
		t.Error("llm.failed parent should be llm.requested")
	}

	// Verify llm.failed has error in payload
	for _, evt := range events {
		if evt.Type == event.V2EventTypes.LLMFailed {
			errMsg, ok := evt.Payload["error"].(string)
			if !ok || errMsg == "" {
				t.Error("llm.failed event missing error in payload")
			}
		}
	}

	// Verify summary has LLMError
	summaryPath := filepath.Join(tmpDir, "runs", result.RunID, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary.json: %v", err)
	}
	var summary event.Summary
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatalf("failed to parse summary.json: %v", err)
	}
	if summary.LLMError == "" {
		t.Error("summary should have LLMError field set")
	}
}

func TestV2DryRunPrompt(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test dry run",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: false,
		RunDir:        filepath.Join(tmpDir, "runs"),
		Provider:      provider.NewMockProvider(),
		Model:         "mock-model",
		DryRunPrompt:  true,
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("expected dry-run status 'completed', got %s", result.Status)
	}

	// Verify events: task.created, task.started, agent.started, agent.completed, task.completed
	// BUT no llm events, no output.written
	events := parseEventsFile(t, filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl"))
	types := eventTypes(events)

	// Should NOT contain LLM events
	for _, evt := range events {
		if evt.Type == event.V2EventTypes.LLMRequested ||
			evt.Type == event.V2EventTypes.LLMCompleted ||
			evt.Type == event.V2EventTypes.LLMFailed ||
			evt.Type == event.V2EventTypes.OutputWritten {
			t.Errorf("dry run should not have %s event", evt.Type)
		}
	}

	expected := []string{
		event.V1EventTypes.TaskCreated,
		event.V1EventTypes.TaskStarted,
		event.V1EventTypes.AgentStarted,
		event.V1EventTypes.AgentCompleted,
		event.V1EventTypes.TaskCompleted,
	}
	assertContainsTypes(t, types, expected)

	// Verify prompt.md exists (that's the whole point of dry-run)
	promptPath := filepath.Join(tmpDir, "runs", result.RunID, "prompt.md")
	if _, err := os.Stat(promptPath); os.IsNotExist(err) {
		t.Error("dry-run: prompt.md should exist")
	}

	// Verify output.md does NOT exist
	outputPath := filepath.Join(tmpDir, "runs", result.RunID, "output.md")
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Error("dry-run: output.md should NOT exist")
	}

	// Verify summary shows dry-run provider
	summaryPath := filepath.Join(tmpDir, "runs", result.RunID, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary.json: %v", err)
	}
	var summary event.Summary
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatalf("failed to parse summary.json: %v", err)
	}
	if summary.Provider != "dry-run" {
		t.Errorf("expected dry-run provider 'dry-run', got '%s'", summary.Provider)
	}
	if summary.PromptPath == "" {
		t.Error("dry-run summary should have PromptPath")
	}
}

func TestV2OutputArtifacts(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test output artifacts",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: false,
		RunDir:        filepath.Join(tmpDir, "runs"),
		Provider:      provider.NewMockProvider(),
		Model:         "mock-model",
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("V2 run failed: %v", err)
	}

	runDir := filepath.Join(tmpDir, "runs", result.RunID)

	// Verify events.jsonl exists
	eventsPath := filepath.Join(runDir, "events.jsonl")
	if _, err := os.Stat(eventsPath); os.IsNotExist(err) {
		t.Error("events.jsonl does not exist")
	}

	// Verify summary.json exists
	summaryPath := filepath.Join(runDir, "summary.json")
	if _, err := os.Stat(summaryPath); os.IsNotExist(err) {
		t.Error("summary.json does not exist")
	}

	// Verify prompt.md exists and has content
	promptPath := filepath.Join(runDir, "prompt.md")
	promptData, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("failed to read prompt.md: %v", err)
	}
	if !strings.Contains(string(promptData), "# Prism Agent Task") {
		t.Error("prompt.md missing expected content")
	}
	if !strings.Contains(string(promptData), "Test output artifacts") {
		t.Error("prompt.md missing task text")
	}

	// Verify output.md exists and has content
	outputPath := filepath.Join(runDir, "output.md")
	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("failed to read output.md: %v", err)
	}
	if len(outputData) == 0 {
		t.Error("output.md is empty")
	}
}

func TestV2ContextInjectionInPrompt(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	mockMemory := startMockMemoryServer(t)
	defer mockMemory.Close()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:          "Test context injection",
		Project:       "prism",
		Agent:         "lumi",
		BusURL:        busURL,
		MemoryEnabled: true,
		RequireMemory: false,
		MemoryURL:     mockMemory.URL,
		RunDir:        filepath.Join(tmpDir, "runs"),
		Provider:      provider.NewMockProvider(),
		Model:         "mock-model",
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("V2 run with memory failed: %v", err)
	}

	// Verify prompt.md includes the context content
	promptPath := filepath.Join(tmpDir, "runs", result.RunID, "prompt.md")
	promptData, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("failed to read prompt.md: %v", err)
	}

	promptStr := string(promptData)
	if !strings.Contains(promptStr, "## Retrieved Context") {
		t.Error("prompt.md missing ## Retrieved Context section")
	}
	if !strings.Contains(promptStr, "Test remembrance context for unit test") {
		t.Error("prompt.md missing injected context content")
	}

	// Verify agent.started is parented to task.started when no context,
	// but in this case it should come after context.injected
	events := parseEventsFile(t, filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl"))

	var taskStartedIdx, contextInjectedIdx, agentStartedIdx int
	for i, evt := range events {
		switch evt.Type {
		case event.V1EventTypes.TaskStarted:
			taskStartedIdx = i
		case event.V2EventTypes.ContextInjected:
			contextInjectedIdx = i
		case event.V1EventTypes.AgentStarted:
			agentStartedIdx = i
		}
	}

	if taskStartedIdx >= agentStartedIdx {
		t.Error("task.started should appear before agent.started")
	}
	if contextInjectedIdx >= agentStartedIdx {
		t.Error("context.injected should appear before agent.started")
	}
}

func TestV2MemoryFailureGraceful(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:           "Test memory failure graceful",
		Project:        "prism",
		Agent:          "lumi",
		BusURL:         busURL,
		MemoryEnabled:  true,
		RequireMemory:  false, // graceful: continue even if memory fails
		MemoryURL:      "http://localhost:18790", // Not running
		RunDir:         filepath.Join(tmpDir, "runs"),
		Provider:       provider.NewMockProvider(),
		Model:          "mock-model",
	}

	runner := run.NewRunner(cfg)
	result, err := runner.Run()

	if err != nil {
		t.Fatalf("graceful memory failure should not error: %v", err)
	}
	if result.Status != "completed" {
		t.Errorf("expected status 'completed', got %s", result.Status)
	}

	// Verify events include both V1 and V2 context failure events
	events := parseEventsFile(t, filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl"))
	types := eventTypes(events)

	expected := []string{
		event.V1EventTypes.MemoryContextRequested,
		event.V2EventTypes.ContextRequested,
		event.V1EventTypes.MemoryContextFailed,
		event.V2EventTypes.ContextFailed,
		event.V2EventTypes.LLMCompleted,
		event.V1EventTypes.TaskCompleted,
	}

	assertContainsTypes(t, types, expected)

	// Verify summary MemoryStatus is "failed"
	summaryPath := filepath.Join(tmpDir, "runs", result.RunID, "summary.json")
	summaryData, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read summary.json: %v", err)
	}
	var summary event.Summary
	if err := json.Unmarshal(summaryData, &summary); err != nil {
		t.Fatalf("failed to parse summary.json: %v", err)
	}
	if summary.MemoryStatus != "failed" {
		t.Errorf("expected MemoryStatus 'failed', got '%s'", summary.MemoryStatus)
	}
	if summary.MemoryUsed {
		t.Error("expected MemoryUsed to be false on failure")
	}
}

func TestV2MemoryFailureStrict(t *testing.T) {
	s, busURL := startTestServer(t)
	defer s.Shutdown()

	tmpDir := t.TempDir()

	cfg := run.RunConfig{
		Task:           "Test memory failure strict",
		Project:        "prism",
		Agent:          "lumi",
		BusURL:         busURL,
		MemoryEnabled:  true,
		RequireMemory:  true, // strict: fail if memory unavailable
		MemoryURL:      "http://localhost:18790", // Not running
		RunDir:         filepath.Join(tmpDir, "runs"),
		Provider:       provider.NewMockProvider(),
		Model:          "mock-model",
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

	// Verify events: context.requested, context.failed, task.failed
	// Should NOT have agent.started, llm events, etc.
	events := parseEventsFile(t, filepath.Join(tmpDir, "runs", result.RunID, "events.jsonl"))
	types := eventTypes(events)

	expected := []string{
		event.V1EventTypes.TaskCreated,
		event.V1EventTypes.TaskStarted,
		event.V1EventTypes.MemoryContextRequested,
		event.V2EventTypes.ContextRequested,
		event.V1EventTypes.MemoryContextFailed,
		event.V2EventTypes.ContextFailed,
		event.V1EventTypes.TaskFailed,
	}

	assertContainsTypes(t, types, expected)

	// Verify NO agent.started or LLM events exist
	for _, evt := range events {
		if evt.Type == event.V1EventTypes.AgentStarted ||
			evt.Type == event.V2EventTypes.LLMRequested ||
			evt.Type == event.V2EventTypes.LLMCompleted {
			t.Errorf("strict failure should not have %s event", evt.Type)
		}
	}
}

// ============================================================================
// Mock Remembrance Server for V2 tests
// ============================================================================

// mockMemoryServer is a simple HTTP server that returns a fake context.
type mockMemoryServer struct {
	URL      string
	listener net.Listener
	server   *http.Server
}

func (m *mockMemoryServer) Close() {
	m.server.Close()
}

func startMockMemoryServer(t *testing.T) *mockMemoryServer {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/context/build", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := `{"context": "Test remembrance context for unit test.", "sources": [{"id": "test-doc-1", "category": "test", "tier": "1", "snippet": "test snippet one", "score": 0.95}, {"id": "test-doc-2", "category": "test", "tier": "2", "snippet": "test snippet two", "score": 0.85}]}`
		w.Write([]byte(resp))
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	server := &http.Server{Handler: mux}
	go server.Serve(listener)

	return &mockMemoryServer{
		URL:      fmt.Sprintf("http://%s", listener.Addr().String()),
		listener: listener,
		server:   server,
	}
}