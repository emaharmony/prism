package approval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Store provides file-based persistence for approvals under
// runs/<run_id>/approvals/<approval_id>.json.
type Store struct {
	mu      sync.RWMutex
	runsDir string
}

// NewStore creates a new file-based approval store.
// runsDir is the base directory containing run directories (e.g., "./runs").
func NewStore(runsDir string) *Store {
	return &Store{runsDir: runsDir}
}

// approvalDir returns the approvals directory for a given run.
func (s *Store) approvalDir(runID string) string {
	return filepath.Join(s.runsDir, runID, "approvals")
}

// approvalPath returns the full path for a given approval file.
func (s *Store) approvalPath(runID, approvalID string) string {
	return filepath.Join(s.approvalDir(runID), fmt.Sprintf("%s.json", approvalID))
}

// Save persists an approval to disk.
func (s *Store) Save(a *Approval) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.approvalDir(a.RunID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create approvals directory: %w", err)
	}

	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal approval: %w", err)
	}

	path := s.approvalPath(a.RunID, a.ApprovalID)
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
		return fmt.Errorf("failed to write approval file: %w", err)
	}

	return nil
}

// Load reads an approval from disk.
// Returns nil and an error if the approval does not exist.
func (s *Store) Load(runID, approvalID string) (*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := s.approvalPath(runID, approvalID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("approval %s not found", approvalID)
		}
		return nil, fmt.Errorf("failed to read approval: %w", err)
	}

	var a Approval
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("failed to unmarshal approval: %w", err)
	}

	return &a, nil
}

// List returns all approvals for a given run, sorted by creation time.
func (s *Store) List(runID string) ([]*Approval, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	dir := s.approvalDir(runID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read approvals directory: %w", err)
	}

	var approvals []*Approval
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var a Approval
		if err := json.Unmarshal(data, &a); err != nil {
			continue
		}
		approvals = append(approvals, &a)
	}

	return approvals, nil
}
