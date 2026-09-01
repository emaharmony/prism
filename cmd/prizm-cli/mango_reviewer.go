// Package main: Mango review subscriber for automatic code review.
//
// When a file-mutating tool succeeds (write_file, edit_file, git_commit, git_push),
// the tool loop publishes prizm.review.requested with the changed files and context.
// This subscriber picks up those events and creates a delegation task for the
// mango agent, which reviews the change for correctness, safety, and quality.
//
// V77: Option A — quick wire. Mango receives the review request and responds
// through the normal agent pipeline. The response is published back on
// prizm.review.completed so other systems can observe review outcomes.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/emaharmony/prizm/internal/delegation"
	"github.com/emaharmony/prizm/internal/orchestrator"
	"github.com/nats-io/nats.go"
)

// mangoReviewer listens for review requests and delegates them to the mango agent.
type mangoReviewer struct {
	nc       *nats.Conn
	deleg    *delegation.Engine
	cfg      *orchestrator.Config
	reviewCh chan reviewRequest
}

type reviewRequest struct {
	AgentID        string   `json:"agent_id"`
	FilesChanged   []string `json:"files_changed"`
	TaskDesc       string   `json:"task_description"`
	ToolName       string   `json:"tool_name,omitempty"`
	SessionID      string   `json:"session_id,omitempty"`
	CorrelationID  string   `json:"correlation_id,omitempty"`
}

// startMangoReviewer subscribes to prizm.review.requested and delegates
// review tasks to the mango agent.
func startMangoReviewer(nc *nats.Conn, deleg *delegation.Engine, cfg *orchestrator.Config) error {
	if nc == nil {
		return fmt.Errorf("mango reviewer requires NATS connection")
	}
	if deleg == nil {
		return fmt.Errorf("mango reviewer requires delegation engine")
	}

	// Check if mango agent is configured
	mangoConfigured := false
	for _, a := range cfg.Agents {
		if a.ID == "mango" {
			mangoConfigured = true
			break
		}
	}
	if !mangoConfigured {
		log.Printf("[MANGO-REVIEW] mango agent not configured, reviewer disabled")
		return nil // not an error — just no mango
	}

	mr := &mangoReviewer{
		nc:       nc,
		deleg:    deleg,
		cfg:      cfg,
		reviewCh: make(chan reviewRequest, 16),
	}

	// Subscribe to review events
	sub, err := nc.Subscribe("prizm.review.requested", mr.handleMsg)
	if err != nil {
		return fmt.Errorf("subscribe review.requested: %w", err)
	}
	_ = sub

	// Start review worker goroutine
	go mr.processReviews()

	log.Printf("[MANGO-REVIEW] watching prizm.review.requested (delegating to mango)")
	return nil
}

func (mr *mangoReviewer) handleMsg(msg *nats.Msg) {
	var req reviewRequest
	if err := json.Unmarshal(msg.Data, &req); err != nil {
		log.Printf("[MANGO-REVIEW] bad review.requested event: %v", err)
		return
	}

	select {
	case mr.reviewCh <- req:
	default:
		log.Printf("[MANGO-REVIEW] review channel full, dropping request for %s", req.TaskDesc)
	}
}

func (mr *mangoReviewer) processReviews() {
	for req := range mr.reviewCh {
		mr.review(req)
	}
}

func (mr *mangoReviewer) review(req reviewRequest) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build review prompt
	filesStr := strings.Join(req.FilesChanged, ", ")
	if filesStr == "" {
		filesStr = "unknown files"
	}

	reviewPrompt := fmt.Sprintf(
		"Review the following change:\n\n"+
			"Tool: %s\n"+
			"Files changed: %s\n"+
			"Context: %s\n\n"+
			"Check for: correctness, safety, regressions, and whether the change accomplishes its stated purpose.\n"+
			"Report findings as structured JSON: {\"decision\": \"pass\"|\"fail\", \"issues\": [...], \"suggestions\": [...]}",
		req.ToolName, filesStr, req.TaskDesc,
	)

	// Delegate to mango
	task, err := mr.deleg.Delegate(ctx, "lumi", "mango", "review_code", reviewPrompt, map[string]any{
		"files_changed":   req.FilesChanged,
		"tool_name":       req.ToolName,
		"session_id":      req.SessionID,
		"correlation_id":  req.CorrelationID,
		"review_type":     "post_mutation",
	})

	if err != nil {
		log.Printf("[MANGO-REVIEW] failed to delegate review task: %v", err)
		// Publish failure event
		mr.publishReviewCompleted(req, "delegation_failed", err.Error())
		return
	}

	log.Printf("[MANGO-REVIEW] delegated review task %s to mango (files: %s)", task.ID, filesStr)

	// Publish review requested event so other systems can track
	mr.publishReviewCompleted(req, "delegated", fmt.Sprintf("task_id: %s", task.ID))
}

func (mr *mangoReviewer) publishReviewCompleted(req reviewRequest, status, detail string) {
	payload := map[string]any{
		"reviewer":      "mango",
		"status":        status,
		"detail":        detail,
		"files_changed": req.FilesChanged,
		"agent_id":      req.AgentID,
		"v":             1,
	}
	data, _ := json.Marshal(payload)
	if err := mr.nc.Publish("prizm.review.completed", data); err != nil {
		log.Printf("[MANGO-REVIEW] failed to publish review.completed: %v", err)
	}
}