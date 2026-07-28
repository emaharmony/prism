package trajectory

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists trajectory runs to SQLite.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// NewStoreFromPath opens a SQLite database for trajectory storage.
func NewStoreFromPath(dbPath string) (*Store, error) {
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open trajectory db: %w", err)
	}
	store := &Store{db: db}
	if err := store.Init(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init trajectory: %w", err)
	}
	return store, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

// Init creates the trajectory table.
func (s *Store) Init() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS trajectory_runs (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			session_key TEXT DEFAULT '',
			model TEXT DEFAULT '',
			provider TEXT DEFAULT '',
			trigger TEXT DEFAULT '',
			trigger_detail TEXT DEFAULT '',
			status TEXT DEFAULT 'pending',
			think_level TEXT DEFAULT '',
			system_prompt_hash TEXT DEFAULT '',
			tool_calls_json TEXT DEFAULT '[]',
			prompt_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			duration_ms INTEGER DEFAULT 0,
			started_at_ms INTEGER DEFAULT 0,
			ended_at_ms INTEGER DEFAULT 0,
			skills_json TEXT DEFAULT '[]',
			config_json TEXT DEFAULT '{}',
			error TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_trajectory_agent ON trajectory_runs(agent_id);
		CREATE INDEX IF NOT EXISTS idx_trajectory_status ON trajectory_runs(status);
		CREATE INDEX IF NOT EXISTS idx_trajectory_started ON trajectory_runs(started_at_ms);
	`)
	return err
}

// Save persists a trajectory run. If the run already exists (by ID), it's updated.
func (s *Store) Save(run TrajectoryRun) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	toolCallsJSON, _ := json.Marshal(run.ToolCalls)
	skillsJSON, _ := json.Marshal(run.SkillsLoaded)
	configJSON, _ := json.Marshal(run.ConfigSnapshot)

	startedMs := run.StartedAt.UnixMilli()
	endedMs := int64(0)
	if !run.EndedAt.IsZero() {
		endedMs = run.EndedAt.UnixMilli()
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO trajectory_runs
		(id, agent_id, session_key, model, provider, trigger, trigger_detail,
		 status, think_level, system_prompt_hash, tool_calls_json,
		 prompt_tokens, output_tokens, duration_ms, started_at_ms, ended_at_ms,
		 skills_json, config_json, error)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	`, run.ID, run.AgentID, run.SessionKey, run.Model, run.Provider,
		run.Trigger, run.TriggerDetail, run.Status, run.ThinkLevel,
		run.SystemPromptHash, string(toolCallsJSON),
		run.PromptTokens, run.OutputTokens, run.DurationMs, startedMs, endedMs,
		string(skillsJSON), string(configJSON), run.Error)
	return err
}

// Load retrieves a single trajectory run by ID.
func (s *Store) Load(id string) (*TrajectoryRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow(`SELECT id, agent_id, session_key, model, provider, trigger,
		trigger_detail, status, think_level, system_prompt_hash, tool_calls_json,
		prompt_tokens, output_tokens, duration_ms, started_at_ms, ended_at_ms,
		skills_json, config_json, error
		FROM trajectory_runs WHERE id = ?`, id)

	var run TrajectoryRun
	var toolCallsJSON, skillsJSON, configJSON string
	var startedMs, endedMs int64

	err := row.Scan(&run.ID, &run.AgentID, &run.SessionKey, &run.Model, &run.Provider,
		&run.Trigger, &run.TriggerDetail, &run.Status, &run.ThinkLevel,
		&run.SystemPromptHash, &toolCallsJSON, &run.PromptTokens, &run.OutputTokens,
		&run.DurationMs, &startedMs, &endedMs, &skillsJSON, &configJSON, &run.Error)
	if err != nil {
		return nil, fmt.Errorf("load trajectory %s: %w", id, err)
	}

	run.StartedAt = time.UnixMilli(startedMs)
	if endedMs > 0 {
		run.EndedAt = time.UnixMilli(endedMs)
	}
	json.Unmarshal([]byte(toolCallsJSON), &run.ToolCalls)
	json.Unmarshal([]byte(skillsJSON), &run.SkillsLoaded)
	json.Unmarshal([]byte(configJSON), &run.ConfigSnapshot)

	return &run, nil
}

// List returns recent trajectory runs, most recent first.
func (s *Store) List(limit int) ([]TrajectoryRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`SELECT id, agent_id, session_key, model, provider, trigger,
		trigger_detail, status, think_level, system_prompt_hash, tool_calls_json,
		prompt_tokens, output_tokens, duration_ms, started_at_ms, ended_at_ms,
		skills_json, config_json, error
		FROM trajectory_runs ORDER BY started_at_ms DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

// ListForAgent returns recent runs for a specific agent.
func (s *Store) ListForAgent(agentID string, limit int) ([]TrajectoryRun, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}

	rows, err := s.db.Query(`SELECT id, agent_id, session_key, model, provider, trigger,
		trigger_detail, status, think_level, system_prompt_hash, tool_calls_json,
		prompt_tokens, output_tokens, duration_ms, started_at_ms, ended_at_ms,
		skills_json, config_json, error
		FROM trajectory_runs WHERE agent_id = ? ORDER BY started_at_ms DESC LIMIT ?`,
		agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRuns(rows)
}

// ExportJSONL returns all runs as JSONL (one JSON object per line).
func (s *Store) ExportJSONL(limit int) (string, error) {
	runs, err := s.List(limit)
	if err != nil {
		return "", err
	}

	var result string
	for _, run := range runs {
		data, _ := json.Marshal(run)
		result += string(data) + "\n"
	}
	return result, nil
}

// HashPrompt computes a SHA-256 hash of the system prompt (for storage
// without storing the full prompt content).
func HashPrompt(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", h[:16]) // first 16 bytes = 32 hex chars
}

func scanRuns(rows *sql.Rows) ([]TrajectoryRun, error) {
	var runs []TrajectoryRun
	for rows.Next() {
		var run TrajectoryRun
		var toolCallsJSON, skillsJSON, configJSON string
		var startedMs, endedMs int64

		err := rows.Scan(&run.ID, &run.AgentID, &run.SessionKey, &run.Model, &run.Provider,
			&run.Trigger, &run.TriggerDetail, &run.Status, &run.ThinkLevel,
			&run.SystemPromptHash, &toolCallsJSON, &run.PromptTokens, &run.OutputTokens,
			&run.DurationMs, &startedMs, &endedMs, &skillsJSON, &configJSON, &run.Error)
		if err != nil {
			return nil, fmt.Errorf("scan trajectory: %w", err)
		}

		run.StartedAt = time.UnixMilli(startedMs)
		if endedMs > 0 {
			run.EndedAt = time.UnixMilli(endedMs)
		}
		json.Unmarshal([]byte(toolCallsJSON), &run.ToolCalls)
		json.Unmarshal([]byte(skillsJSON), &run.SkillsLoaded)
		json.Unmarshal([]byte(configJSON), &run.ConfigSnapshot)

		runs = append(runs, run)
	}
	return runs, nil
}