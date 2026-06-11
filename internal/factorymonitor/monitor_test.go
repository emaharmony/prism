package factorymonitor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakePublisher struct {
	events []publishedEvent
}

type publishedEvent struct {
	subject string
	data    []byte
}

func (p *fakePublisher) Publish(subject string, data []byte) error {
	p.events = append(p.events, publishedEvent{subject: subject, data: append([]byte(nil), data...)})
	return nil
}

func TestSnapshotReadsFactoryQueue(t *testing.T) {
	root := makeFactoryRoot(t)
	writeJSONFile(t, filepath.Join(root, "inbox", "queued.json"), map[string]any{"task_id": "queued"})
	writeJSONFile(t, filepath.Join(root, "processing", "active.json"), map[string]any{"task_id": "active"})
	writeJSONFile(t, filepath.Join(root, "failed", "failed.json"), map[string]any{"task_id": "failed"})
	writeJSONFile(t, filepath.Join(root, "archive", "archived.json"), map[string]any{"task_id": "archived"})
	writeFile(t, filepath.Join(root, "outbox", "build_result.md"), "done")
	writeJSONFile(t, filepath.Join(root, "outbox", "build_status.json"), map[string]any{
		"task_id":    "build",
		"status":     "completed",
		"message":    "finished",
		"updated_at": time.Now().UTC().Add(-2 * time.Minute).Format(time.RFC3339),
	})
	writeJSONFile(t, filepath.Join(root, "outbox", "review_status.json"), map[string]any{
		"task_id":    "review",
		"status":     "running",
		"message":    "checking",
		"updated_at": time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
	})

	mon := New(Config{Root: root}, nil)
	snap, err := mon.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}

	if snap.Counts.Inbox != 1 || snap.Counts.Processing != 1 || snap.Counts.Failed != 1 || snap.Counts.Archive != 1 {
		t.Fatalf("queue counts = %+v", snap.Counts)
	}
	if snap.Counts.Active != 1 || snap.Counts.Completed != 1 {
		t.Fatalf("task counts = %+v", snap.Counts)
	}
	if len(snap.Tasks) != 2 {
		t.Fatalf("expected 2 status tasks, got %d", len(snap.Tasks))
	}
	if snap.Tasks[0].TaskID != "build" || snap.Tasks[0].ResultPath == "" {
		t.Fatalf("completed task did not resolve result path: %+v", snap.Tasks[0])
	}
}

func TestPollPublishesChangedAndStuckOnce(t *testing.T) {
	root := makeFactoryRoot(t)
	statusPath := filepath.Join(root, "outbox", "slow_status.json")
	oldUpdate := time.Now().UTC().Add(-45 * time.Minute).Format(time.RFC3339)
	writeJSONFile(t, statusPath, map[string]any{
		"task_id":    "slow",
		"status":     "running",
		"message":    "still working",
		"updated_at": oldUpdate,
	})

	pub := &fakePublisher{}
	mon := New(Config{Root: root, StuckAfter: 30 * time.Minute}, pub)

	mon.poll()
	assertSubjects(t, pub.events, []string{EventStatusChanged, EventStatusStuck})

	pub.events = nil
	mon.poll()
	assertSubjects(t, pub.events, nil)

	writeJSONFile(t, statusPath, map[string]any{
		"task_id":    "slow",
		"status":     "running",
		"message":    "still working on phase 2",
		"updated_at": oldUpdate,
	})
	mon.poll()
	assertSubjects(t, pub.events, []string{EventStatusChanged, EventStatusStuck})
}

func TestBaselineSuppressesExistingStatuses(t *testing.T) {
	root := makeFactoryRoot(t)
	writeJSONFile(t, filepath.Join(root, "outbox", "existing_status.json"), map[string]any{
		"task_id":    "existing",
		"status":     "running",
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})

	pub := &fakePublisher{}
	mon := New(Config{Root: root}, pub)
	snap, err := mon.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot failed: %v", err)
	}
	mon.baseline(snap)
	mon.poll()

	if len(pub.events) != 0 {
		t.Fatalf("expected no events after baseline, got %+v", pub.events)
	}
}

func TestPublishDigestPublishesSnapshot(t *testing.T) {
	root := makeFactoryRoot(t)
	writeJSONFile(t, filepath.Join(root, "outbox", "task_status.json"), map[string]any{
		"task_id":    "task",
		"status":     "running",
		"updated_at": time.Now().UTC().Format(time.RFC3339),
	})

	pub := &fakePublisher{}
	mon := New(Config{Root: root}, pub)
	message := mon.PublishDigest()

	if message == "" {
		t.Fatal("expected formatted digest message")
	}
	assertSubjects(t, pub.events, []string{EventStatusDigest})
}

func makeFactoryRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"inbox", "processing", "failed", "archive", "outbox"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", name, err)
		}
	}
	return root
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	writeFile(t, path, string(data))
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertSubjects(t *testing.T, events []publishedEvent, want []string) {
	t.Helper()
	if len(events) != len(want) {
		t.Fatalf("subjects len = %d, want %d (%+v)", len(events), len(want), events)
	}
	for i := range want {
		if events[i].subject != want[i] {
			t.Fatalf("subject[%d] = %q, want %q", i, events[i].subject, want[i])
		}
	}
}
