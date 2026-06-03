// Package session provides the session manager for Prism V20.
//
// A session tracks a conversation between a user and an agent on a channel.
// Sessions persist across messages, compact when they grow too large, and
// reset on idle timeout or daily boundary.
//
// V20 uses truncation compaction: when a session exceeds MaxContextMessages,
// the oldest messages are removed. Full Remembrance-based summarization
// comes in V21.
package session

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// Session represents an active conversation between a user and an agent.
type Session struct {
	ID          string    `json:"id"`
	AgentID     string    `json:"agent_id"`
	Channel     string    `json:"channel"`    // discord, telegram, webchat
	ChannelID   string    `json:"channel_id"` // e.g., Discord channel ID
	UserID      string    `json:"user_id"`    // e.g., Discord user ID
	StartedAt   time.Time `json:"started_at"`
	LastActive  time.Time `json:"last_active"`
	CompactedAt time.Time `json:"compacted_at,omitempty"`

	// Messages holds the conversation history (in order, oldest first).
	Messages []Message `json:"messages"`
}

// Message is a single message within a session.
type Message struct {
	ID        string    `json:"id"`
	Role      string    `json:"role"` // "user", "agent", "system"
	Content   string    `json:"content"`
	AgentID   string    `json:"agent_id"` // which agent sent this (for agent role)
	Timestamp time.Time `json:"timestamp"`
}

// Manager handles session lifecycle: create, get, update, compact, reset.
type Manager struct {
	mu       sync.RWMutex
	db       *sql.DB
	sessions map[string]*Session // in-memory cache, keyed by session ID

	// Configuration
	maxContextMessages int
	idleTimeout        time.Duration
	dailyResetHour     int
	compactionStrategy string // "truncate" (V20) or "summarize" (V21)
}

var idCounter atomic.Uint64

// NewManager creates a session manager with the given configuration.
func NewManager(dbPath string, maxContextMessages int, idleTimeout time.Duration, dailyResetHour int, compactionStrategy string) (*Manager, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("session: open db: %w", err)
	}

	// Enable WAL mode for concurrent reads
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("session: set WAL mode: %w", err)
	}

	m := &Manager{
		db:                 db,
		sessions:           make(map[string]*Session),
		maxContextMessages: maxContextMessages,
		idleTimeout:        idleTimeout,
		dailyResetHour:     dailyResetHour,
		compactionStrategy: compactionStrategy,
	}

	if err := m.migrate(ctx(context.Background())); err != nil {
		db.Close()
		return nil, fmt.Errorf("session: migrate: %w", err)
	}

	return m, nil
}

// ctx is a helper to create a background context.
func ctx(bg context.Context) context.Context { return bg }

// migrate creates the sessions and messages tables.
func (m *Manager) migrate(_ context.Context) error {
	_, err := m.db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			channel TEXT NOT NULL,
			channel_id TEXT NOT NULL,
			user_id TEXT NOT NULL,
			started_at DATETIME NOT NULL,
			last_active DATETIME NOT NULL,
			compacted_at DATETIME
		);
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			agent_id TEXT NOT NULL DEFAULT '',
			timestamp DATETIME NOT NULL,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		);
		CREATE INDEX IF NOT EXISTS idx_messages_session ON messages(session_id, timestamp);
		CREATE INDEX IF NOT EXISTS idx_sessions_channel_user ON sessions(channel, channel_id, user_id);
	`)
	return err
}

// Create creates a new session for a user on a channel with a specific agent.
func (m *Manager) Create(agentID, channel, channelID, userID string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	s := &Session{
		ID:         generateSessionID(),
		AgentID:    agentID,
		Channel:    channel,
		ChannelID:  channelID,
		UserID:     userID,
		StartedAt:  now,
		LastActive: now,
		Messages:   []Message{},
	}

	// Persist to SQLite
	_, err := m.db.Exec(
		"INSERT INTO sessions (id, agent_id, channel, channel_id, user_id, started_at, last_active) VALUES (?, ?, ?, ?, ?, ?, ?)",
		s.ID, s.AgentID, s.Channel, s.ChannelID, s.UserID, s.StartedAt, s.LastActive,
	)
	if err != nil {
		return nil, fmt.Errorf("session: insert: %w", err)
	}

	m.sessions[s.ID] = s
	return s, nil
}

// Get retrieves a session by ID.
func (m *Manager) Get(id string) (*Session, error) {
	m.mu.RLock()
	if s, ok := m.sessions[id]; ok {
		m.mu.RUnlock()
		return s, nil
	}
	m.mu.RUnlock()

	// Try loading from SQLite
	return m.loadFromDB(id)
}

// FindActive finds an active session for a user on a channel.
// Returns nil if no active session exists.
func (m *Manager) FindActive(channel, channelID, userID string) (*Session, error) {
	m.mu.RLock()
	for _, s := range m.sessions {
		if s.Channel == channel && s.ChannelID == channelID && s.UserID == userID {
			// Check if session has timed out
			if time.Since(s.LastActive) > m.idleTimeout {
				m.mu.RUnlock()
				return nil, nil
			}
			m.mu.RUnlock()
			return s, nil
		}
	}
	m.mu.RUnlock()

	// Try loading from SQLite
	row := m.db.QueryRow(
		"SELECT id, agent_id, channel, channel_id, user_id, started_at, last_active, compacted_at FROM sessions WHERE channel = ? AND channel_id = ? AND user_id = ? ORDER BY last_active DESC LIMIT 1",
		channel, channelID, userID,
	)

	var s Session
	var compactedAt sql.NullTime
	err := row.Scan(&s.ID, &s.AgentID, &s.Channel, &s.ChannelID, &s.UserID, &s.StartedAt, &s.LastActive, &compactedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("session: find active: %w", err)
	}

	// Check idle timeout
	if time.Since(s.LastActive) > m.idleTimeout {
		return nil, nil
	}

	// Load messages
	msgs, err := m.loadMessages(s.ID)
	if err != nil {
		return nil, err
	}
	s.Messages = msgs

	m.mu.Lock()
	m.sessions[s.ID] = &s
	m.mu.Unlock()

	return &s, nil
}

// AddMessage adds a message to a session and persists it.
func (m *Manager) AddMessage(sessionID, role, content, agentID string) (*Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s, ok := m.sessions[sessionID]
	if !ok {
		return nil, fmt.Errorf("session: not found: %s", sessionID)
	}

	msg := Message{
		ID:        generateMessageID(),
		Role:      role,
		Content:   content,
		AgentID:   agentID,
		Timestamp: time.Now().UTC(),
	}

	s.Messages = append(s.Messages, msg)
	s.LastActive = time.Now().UTC()

	// Persist message
	_, err := m.db.Exec(
		"INSERT INTO messages (id, session_id, role, content, agent_id, timestamp) VALUES (?, ?, ?, ?, ?, ?)",
		msg.ID, sessionID, msg.Role, msg.Content, msg.AgentID, msg.Timestamp,
	)
	if err != nil {
		return nil, fmt.Errorf("session: insert message: %w", err)
	}

	// Update last_active
	_, err = m.db.Exec(
		"UPDATE sessions SET last_active = ? WHERE id = ?",
		s.LastActive, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("session: update last_active: %w", err)
	}

	// Compact if needed
	if len(s.Messages) > m.maxContextMessages {
		if err := m.compact(s); err != nil {
			// Log compaction error but don't fail the message add
			// In production this would go to structured logging
			fmt.Printf("session: compaction warning: %v\n", err)
		}
	}

	return &msg, nil
}

// compact truncates the oldest messages when the session exceeds the budget.
// V20 uses truncation. V21 will use Remembrance summarization.
func (m *Manager) compact(s *Session) error {
	if m.compactionStrategy != "truncate" {
		// V21: Remembrance summarization goes here
		return nil
	}

	// Keep only the most recent MaxContextMessages messages
	excess := len(s.Messages) - m.maxContextMessages
	if excess <= 0 {
		return nil
	}

	// Remove oldest messages from memory
	removed := s.Messages[:excess]
	s.Messages = s.Messages[excess:]
	s.CompactedAt = time.Now().UTC()

	// Remove from SQLite
	for _, msg := range removed {
		if _, err := m.db.Exec("DELETE FROM messages WHERE id = ?", msg.ID); err != nil {
			return fmt.Errorf("session: delete message %s: %w", msg.ID, err)
		}
	}

	// Update compacted_at
	if _, err := m.db.Exec("UPDATE sessions SET compacted_at = ? WHERE id = ?", s.CompactedAt, s.ID); err != nil {
		return fmt.Errorf("session: update compacted_at: %w", err)
	}

	return nil
}

// Close shuts down the session manager and closes the database.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.db.Close()
}

// loadFromDB loads a session from SQLite by ID.
func (m *Manager) loadFromDB(id string) (*Session, error) {
	row := m.db.QueryRow(
		"SELECT id, agent_id, channel, channel_id, user_id, started_at, last_active, compacted_at FROM sessions WHERE id = ?",
		id,
	)

	var s Session
	var compactedAt sql.NullTime
	err := row.Scan(&s.ID, &s.AgentID, &s.Channel, &s.ChannelID, &s.UserID, &s.StartedAt, &s.LastActive, &compactedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("session: not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("session: load: %w", err)
	}

	msgs, err := m.loadMessages(s.ID)
	if err != nil {
		return nil, err
	}
	s.Messages = msgs

	return &s, nil
}

// loadMessages loads all messages for a session from SQLite.
func (m *Manager) loadMessages(sessionID string) ([]Message, error) {
	rows, err := m.db.Query(
		"SELECT id, role, content, agent_id, timestamp FROM messages WHERE session_id = ? ORDER BY timestamp ASC",
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("session: load messages: %w", err)
	}
	defer rows.Close()

	var msgs []Message
	for rows.Next() {
		var msg Message
		if err := rows.Scan(&msg.ID, &msg.Role, &msg.Content, &msg.AgentID, &msg.Timestamp); err != nil {
			return nil, fmt.Errorf("session: scan message: %w", err)
		}
		msgs = append(msgs, msg)
	}
	return msgs, nil
}

// ListActive returns all sessions that are currently in memory.
func (m *Manager) ListActive() ([]*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var active []*Session
	for _, s := range m.sessions {
		active = append(active, s)
	}
	return active, nil
}

// List returns all active session IDs.
func (m *Manager) List() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	return ids
}

// generateSessionID creates a unique session ID.
func generateSessionID() string {
	return fmt.Sprintf("ses_%d_%d", time.Now().UnixNano(), idCounter.Add(1))
}

// generateMessageID creates a unique message ID.
func generateMessageID() string {
	return fmt.Sprintf("msg_%d_%d", time.Now().UnixNano(), idCounter.Add(1))
}
