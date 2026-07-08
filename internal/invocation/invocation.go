// Package invocation implements Prism's Agent Invocation API: a minimal,
// public, single-shot "ask a configured agent one question, get a
// structured result back" primitive over HTTP.
//
// This exists because Prism had no production-ready external contract for
// "submit a prompt to a named agent, get a result" — the session/stage
// pipeline is interactive-turn-oriented, and the workflow/delegation
// machinery is either in-process-only or feature-flagged off by default.
// Invocation is deliberately the lightest-weight path in the codebase for
// this: no session persistence, no tool loop, no approval-gate machinery —
// just a resolved agent's provider/model, one prompt in, one JSON-ish
// result out. Any consequential action stays on the caller's side.
//
// Agents must opt in via AgentConfig.InvocableViaAPI — this is a new
// network-reachable surface, so nothing is invocable by default.
package invocation

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Status is the lifecycle state of an invocation.
type Status string

const (
	StatusPending   Status = "pending"
	StatusCompleted Status = "completed"
	StatusFailed    Status = "failed"
)

// Invocation is a single request to an agent and its eventual result.
type Invocation struct {
	ID          string         `json:"invocation_id"`
	AgentID     string         `json:"agent_id"`
	Status      Status         `json:"status"`
	Result      map[string]any `json:"result,omitempty"`
	Error       string         `json:"error,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
}

// Store holds invocations in memory, keyed by ID. Invocations are
// short-lived (seconds, not sessions) — callers are expected to poll or
// stream for a result within the invocation's lifetime, so durability
// across a Prism restart isn't a requirement here, unlike session state.
type Store struct {
	mu          sync.RWMutex
	invocations map[string]*Invocation
}

// NewStore creates an empty invocation store.
func NewStore() *Store {
	return &Store{invocations: make(map[string]*Invocation)}
}

// Create registers a new pending invocation for the given agent and
// returns it.
func (s *Store) Create(agentID string) *Invocation {
	inv := &Invocation{
		ID:        "inv_" + ulid.MustNew(ulid.Timestamp(time.Now()), rand.Reader).String(),
		AgentID:   agentID,
		Status:    StatusPending,
		CreatedAt: time.Now(),
	}
	s.mu.Lock()
	s.invocations[inv.ID] = inv
	s.mu.Unlock()
	return inv
}

// Get looks up an invocation by ID.
func (s *Store) Get(id string) (*Invocation, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	inv, ok := s.invocations[id]
	return inv, ok
}

// Complete marks an invocation as completed with a result.
func (s *Store) Complete(id string, result map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.invocations[id]
	if !ok {
		return
	}
	now := time.Now()
	inv.Status = StatusCompleted
	inv.Result = result
	inv.CompletedAt = &now
}

// Fail marks an invocation as failed with an error message.
func (s *Store) Fail(id string, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, ok := s.invocations[id]
	if !ok {
		return
	}
	now := time.Now()
	inv.Status = StatusFailed
	inv.Error = errMsg
	inv.CompletedAt = &now
}

// ParseResult converts an LLM's raw text response into a result map. Agents
// invoked through this API are expected to respond with strict JSON (per
// their conversation_postfix instructions); if the response doesn't parse
// as a JSON object, it's wrapped so callers still get a stable shape rather
// than a parse error.
func ParseResult(content string) map[string]any {
	trimmed := strings.TrimSpace(content)
	var parsed map[string]any
	if err := json.Unmarshal([]byte(trimmed), &parsed); err == nil {
		return parsed
	}
	return map[string]any{"text": content}
}

// DefaultMaxTokens is used when a request doesn't specify max_tokens.
const DefaultMaxTokens = 512

// ValidateRequest returns an error if the request is missing required
// fields.
func ValidateRequest(prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	return nil
}
