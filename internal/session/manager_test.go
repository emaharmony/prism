package session

import (
	"path/filepath"
	"testing"
	"time"
)

func TestCreateAndGet(t *testing.T) {
	dbPath := tempDB(t)
	m := newTestManager(t, dbPath)
	defer m.Close()

	s, err := m.Create("lumi", "discord", "channel123", "user456")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if s.AgentID != "lumi" {
		t.Errorf("expected agent 'lumi', got %q", s.AgentID)
	}
	if s.Channel != "discord" {
		t.Errorf("expected channel 'discord', got %q", s.Channel)
	}
	if s.ChannelID != "channel123" {
		t.Errorf("expected channel_id 'channel123', got %q", s.ChannelID)
	}
	if s.UserID != "user456" {
		t.Errorf("expected user_id 'user456', got %q", s.UserID)
	}

	// Get by ID
	got, err := m.Get(s.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != s.ID {
		t.Errorf("expected session ID %q, got %q", s.ID, got.ID)
	}
}

func TestFindActive(t *testing.T) {
	dbPath := tempDB(t)
	m := newTestManager(t, dbPath)
	defer m.Close()

	// No session exists yet
	found, err := m.FindActive("discord", "channel123", "user456")
	if err != nil {
		t.Fatalf("FindActive: %v", err)
	}
	if found != nil {
		t.Error("expected nil for no active session")
	}

	// Create a session
	s, err := m.Create("lumi", "discord", "channel123", "user456")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Should find it
	found, err = m.FindActive("discord", "channel123", "user456")
	if err != nil {
		t.Fatalf("FindActive: %v", err)
	}
	if found == nil {
		t.Fatal("expected to find active session")
	}
	if found.ID != s.ID {
		t.Errorf("expected session ID %q, got %q", s.ID, found.ID)
	}

	// Different user should not find it
	found2, err := m.FindActive("discord", "channel123", "other_user")
	if err != nil {
		t.Fatalf("FindActive other user: %v", err)
	}
	if found2 != nil {
		t.Error("expected nil for different user")
	}
}

func TestAddMessage(t *testing.T) {
	dbPath := tempDB(t)
	m := newTestManager(t, dbPath)
	defer m.Close()

	s, err := m.Create("lumi", "discord", "channel123", "user456")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	msg, err := m.AddMessage(s.ID, "user", "Hello!", "")
	if err != nil {
		t.Fatalf("AddMessage: %v", err)
	}
	if msg.Role != "user" {
		t.Errorf("expected role 'user', got %q", msg.Role)
	}
	if msg.Content != "Hello!" {
		t.Errorf("expected content 'Hello!', got %q", msg.Content)
	}

	// Add agent response
	msg2, err := m.AddMessage(s.ID, "agent", "Hi there!", "lumi")
	if err != nil {
		t.Fatalf("AddMessage agent: %v", err)
	}
	if msg2.AgentID != "lumi" {
		t.Errorf("expected agent_id 'lumi', got %q", msg2.AgentID)
	}

	// Verify messages are in the session
	got, err := m.Get(s.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Messages) != 2 {
		t.Errorf("expected 2 messages, got %d", len(got.Messages))
	}
}

func TestCompaction(t *testing.T) {
	dbPath := tempDB(t)
	m := newTestManager(t, dbPath, WithMaxContext(5))
	defer m.Close()

	s, err := m.Create("lumi", "discord", "channel123", "user456")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Add 7 messages (exceeds max of 5)
	for i := 0; i < 7; i++ {
		_, err := m.AddMessage(s.ID, "user", "message", "")
		if err != nil {
			t.Fatalf("AddMessage %d: %v", i, err)
		}
	}

	// Should be compacted to 5 messages
	got, err := m.Get(s.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Messages) > 5 {
		t.Errorf("expected at most 5 messages after compaction, got %d", len(got.Messages))
	}
}

func TestList(t *testing.T) {
	dbPath := tempDB(t)
	m := newTestManager(t, dbPath)
	defer m.Close()

	_, err := m.Create("lumi", "discord", "ch1", "u1")
	if err != nil {
		t.Fatalf("Create 1: %v", err)
	}
	_, err = m.Create("mango", "discord", "ch2", "u2")
	if err != nil {
		t.Fatalf("Create 2: %v", err)
	}

	ids := m.List()
	if len(ids) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(ids))
	}
}

// Helpers

type managerOption struct {
	maxContext int
}

type optionFunc func(*managerOption)

func WithMaxContext(n int) optionFunc {
	return func(o *managerOption) { o.maxContext = n }
}

func newTestManager(t *testing.T, dbPath string, opts ...optionFunc) *Manager {
	t.Helper()
	opt := managerOption{maxContext: 100}
	for _, o := range opts {
		o(&opt)
	}

	m, err := NewManager(dbPath, opt.maxContext, 30*time.Minute, 4, "truncate")
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return m
}

func tempDB(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test_sessions.db")
}