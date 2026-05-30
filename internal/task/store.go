// Package task provides end-to-end task tracking for agent delegation.
//
// Tasks represent units of work delegated between agents (e.g., Lumi
// delegates a coding task to Mango). Each task has a lifecycle:
//
//	created → assigned → in_progress → completed | failed | cancelled
//
// Tasks are stored in SQLite for durability and event-sourced via NATS
// for real-time notification.
package task

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/emaharmony/prism/internal/sqlite"
)

// Status represents the lifecycle state of a task.
type Status string

const (
	StatusCreated    Status = "created"
	StatusAssigned   Status = "assigned"
	StatusInProgress Status = "in_progress"
	StatusCompleted  Status = "completed"
	StatusFailed     Status = "failed"
	StatusCancelled  Status = "cancelled"
)

// ValidStatuses is the set of allowed status values.
var ValidStatuses = map[Status]bool{
	StatusCreated:    true,
	StatusAssigned:   true,
	StatusInProgress: true,
	StatusCompleted:  true,
	StatusFailed:     true,
	StatusCancelled:  true,
}

// Task represents a unit of work delegated between agents.
type Task struct {
	// ID is the unique task identifier (e.g., "task-abc123").
	ID string `json:"id"`

	// ParentID references the parent task for subtask delegation.
	ParentID string `json:"parent_id,omitempty"`

	// Type categorizes the task (e.g., "code_implementation", "review", "research").
	Type string `json:"type"`

	// Status is the current lifecycle state.
	Status Status `json:"status"`

	// DelegatedBy is the agent that created the task.
	DelegatedBy string `json:"delegated_by"`

	// DelegatedTo is the agent assigned to complete the task.
	DelegatedTo string `json:"delegated_to"`

	// Description is a human-readable task description.
	Description string `json:"description"`

	// Context holds task-specific metadata (JSON-encoded).
	Context map[string]any `json:"context,omitempty"`

	// Result holds the task completion output (JSON-encoded).
	Result map[string]any `json:"result,omitempty"`

	// Priority is the task priority (normal, high, critical).
	Priority string `json:"priority,omitempty"`

	// CreatedAt is when the task was created.
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the task was last updated.
	UpdatedAt time.Time `json:"updated_at"`

	// CompletedAt is when the task was completed (nil if not yet).
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

// Store provides durable task storage backed by SQLite.
type Store struct {
	db *sql.DB
}

// NewStore creates a new task store backed by the given SQLite database path.
// It initializes the schema if it doesn't exist.
func NewStore(dbPath string) (*Store, error) {
	db, err := sql.Open(sqlite.DriverName, dbPath)
	if err != nil {
		return nil, fmt.Errorf("task: open database: %w", err)
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec("PRAGMA busy_timeout=5000"); err != nil {
		db.Close()
		return nil, fmt.Errorf("task: set busy timeout: %w", err)
	}
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("task: set WAL mode: %w", err)
	}

	if err := initSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("task: init schema: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Create inserts a new task into the store.
func (s *Store) Create(t *Task) error {
	if t.ID == "" {
		return fmt.Errorf("task: ID is required")
	}
	if !ValidStatuses[t.Status] {
		return fmt.Errorf("task: invalid status %q", t.Status)
	}

	contextJSON, err := json.Marshal(t.Context)
	if err != nil {
		return fmt.Errorf("task: marshal context: %w", err)
	}

	resultJSON, err := json.Marshal(t.Result)
	if err != nil {
		return fmt.Errorf("task: marshal result: %w", err)
	}

	var completedAt *string
	if t.CompletedAt != nil {
		ca := t.CompletedAt.Format(time.RFC3339)
		completedAt = &ca
	}

	_, err = s.db.Exec(`
		INSERT INTO tasks (id, parent_id, type, status, delegated_by, delegated_to, description, context, result, priority, created_at, updated_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.ParentID, t.Type, string(t.Status), t.DelegatedBy, t.DelegatedTo, t.Description, string(contextJSON), string(resultJSON), t.Priority, t.CreatedAt, t.UpdatedAt, completedAt,
	)

	if err != nil {
		return fmt.Errorf("task: insert: %w", err)
	}

	return nil
}

// Get retrieves a task by ID.
func (s *Store) Get(id string) (*Task, error) {
	row := s.db.QueryRow(`
		SELECT id, parent_id, type, status, delegated_by, delegated_to, description, context, result, priority, created_at, updated_at, completed_at
		FROM tasks WHERE id = ?`, id)

	t, err := scanTask(row)
	if err != nil {
		return nil, fmt.Errorf("task: get %q: %w", id, err)
	}
	return t, nil
}

// UpdateStatus updates a task's status and optionally its result.
func (s *Store) UpdateStatus(id string, status Status, result map[string]any) error {
	if !ValidStatuses[status] {
		return fmt.Errorf("task: invalid status %q", status)
	}

	now := time.Now()

	var resultJSON []byte
	var err error
	if result != nil {
		resultJSON, err = json.Marshal(result)
		if err != nil {
			return fmt.Errorf("task: marshal result: %w", err)
		}
	}

	// If completing or failing, set completed_at
	var completedAt *string
	if status == StatusCompleted || status == StatusFailed || status == StatusCancelled {
		ca := now.Format(time.RFC3339)
		completedAt = &ca
	}

	_, err = s.db.Exec(`
		UPDATE tasks SET status = ?, result = COALESCE(?, result), updated_at = ?, completed_at = COALESCE(?, completed_at)
		WHERE id = ?`,
		string(status), string(resultJSON), now, completedAt, id,
	)

	if err != nil {
		return fmt.Errorf("task: update status %q: %w", id, err)
	}

	return nil
}

// ListByAgent returns all tasks delegated to a given agent.
func (s *Store) ListByAgent(agentID string) ([]*Task, error) {
	rows, err := s.db.Query(`
		SELECT id, parent_id, type, status, delegated_by, delegated_to, description, context, result, priority, created_at, updated_at, completed_at
		FROM tasks WHERE delegated_to = ? ORDER BY created_at DESC`, agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("task: list by agent %q: %w", agentID, err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

// ListByStatus returns all tasks with the given status.
func (s *Store) ListByStatus(status Status) ([]*Task, error) {
	rows, err := s.db.Query(`
		SELECT id, parent_id, type, status, delegated_by, delegated_to, description, context, result, priority, created_at, updated_at, completed_at
		FROM tasks WHERE status = ? ORDER BY created_at DESC`, string(status),
	)
	if err != nil {
		return nil, fmt.Errorf("task: list by status %q: %w", status, err)
	}
	defer rows.Close()

	var tasks []*Task
	for rows.Next() {
		t, err := scanTaskRows(rows)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, t)
	}

	return tasks, rows.Err()
}

// initSchema creates the tasks table if it doesn't exist.
func initSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			parent_id TEXT,
			type TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'created',
			delegated_by TEXT,
			delegated_to TEXT,
			description TEXT,
			context TEXT,
			result TEXT,
			priority TEXT DEFAULT 'normal',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			completed_at DATETIME
		);
		CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
		CREATE INDEX IF NOT EXISTS idx_tasks_agent ON tasks(delegated_to);
		CREATE INDEX IF NOT EXISTS idx_tasks_parent ON tasks(parent_id);
	`)
	return err
}

func scanTask(row *sql.Row) (*Task, error) {
	t := &Task{}
	var contextJSON, resultJSON sql.NullString
	var completedAt sql.NullString
	var statusStr string

	err := row.Scan(&t.ID, &t.ParentID, &t.Type, &statusStr, &t.DelegatedBy, &t.DelegatedTo, &t.Description, &contextJSON, &resultJSON, &t.Priority, &t.CreatedAt, &t.UpdatedAt, &completedAt)
	if err != nil {
		return nil, err
	}

	t.Status = Status(statusStr)

	if contextJSON.Valid && contextJSON.String != "" {
		if err := json.Unmarshal([]byte(contextJSON.String), &t.Context); err != nil {
			t.Context = make(map[string]any)
		}
	}

	if resultJSON.Valid && resultJSON.String != "" {
		if err := json.Unmarshal([]byte(resultJSON.String), &t.Result); err != nil {
			t.Result = make(map[string]any)
		}
	}

	if completedAt.Valid {
		ca, err := time.Parse(time.RFC3339, completedAt.String)
		if err == nil {
			t.CompletedAt = &ca
		}
	}

	return t, nil
}

// Scanner interface for rows that can be scanned.
type scanner interface {
	Scan(dest ...any) error
}

func scanTaskRows(rows scanner) (*Task, error) {
	t := &Task{}
	var contextJSON, resultJSON sql.NullString
	var completedAt sql.NullString
	var statusStr string

	err := rows.Scan(&t.ID, &t.ParentID, &t.Type, &statusStr, &t.DelegatedBy, &t.DelegatedTo, &t.Description, &contextJSON, &resultJSON, &t.Priority, &t.CreatedAt, &t.UpdatedAt, &completedAt)
	if err != nil {
		return nil, err
	}

	t.Status = Status(statusStr)

	if contextJSON.Valid && contextJSON.String != "" {
		if err := json.Unmarshal([]byte(contextJSON.String), &t.Context); err != nil {
			t.Context = make(map[string]any)
		}
	}

	if resultJSON.Valid && resultJSON.String != "" {
		if err := json.Unmarshal([]byte(resultJSON.String), &t.Result); err != nil {
			t.Result = make(map[string]any)
		}
	}

	if completedAt.Valid {
		ca, err := time.Parse(time.RFC3339, completedAt.String)
		if err == nil {
			t.CompletedAt = &ca
		}
	}

	return t, nil
}
