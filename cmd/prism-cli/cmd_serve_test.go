package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emaharmony/prism/internal/context"
	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/session"
)

func TestBuildPrompt_WithContextInjection(t *testing.T) {
	// Create a temp workspace with SOUL.md and USER.md
	workspace := t.TempDir()
	soulContent := "# SOUL\nYou are Lumi. Be warm, playful, and confident."
	userContent := "# User\nEma. He/him. ADHD-aware support."

	if err := os.WriteFile(filepath.Join(workspace, "SOUL.md"), []byte(soulContent), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "USER.md"), []byte(userContent), 0644); err != nil {
		t.Fatal(err)
	}

	ctxBuilder := context.NewBuilder(workspace)
	convCtx := &conversationContext{
		cfg: &orchestrator.Config{
			Agents: []orchestrator.AgentConfig{
				{ID: "lumi", Role: "lead", Context: []string{"soul", "user"}},
			},
		},
		ctxBuilder: ctxBuilder,
	}

	sess := &session.Session{
		ID: "test-session",
		Messages: []session.Message{
			{Role: "user", Content: "Hello!"},
		},
	}

	agentCfg := &orchestrator.AgentConfig{
		ID:      "lumi",
		Role:    "lead",
		Context: []string{"soul", "user"},
	}

	prompt := convCtx.buildPrompt(sess, agentCfg)

	// Verify agent identity is in the prompt
	if !strings.Contains(prompt, "You are lumi, a lead assistant") {
		t.Error("prompt should contain agent identity")
	}

	// Verify SOUL.md content was injected
	if !strings.Contains(prompt, soulContent) {
		t.Error("prompt should contain SOUL.md content")
	}

	// Verify USER.md content was injected
	if !strings.Contains(prompt, userContent) {
		t.Error("prompt should contain USER.md content")
	}

	// Verify conversation history is in the prompt
	if !strings.Contains(prompt, "User: Hello!") {
		t.Error("prompt should contain conversation history")
	}

	// Verify the prompt ends with the agent name (ready for completion)
	if !strings.HasSuffix(strings.TrimSpace(prompt), "lumi:") {
		t.Errorf("prompt should end with 'lumi:', got: %q", prompt[len(prompt)-20:])
	}
}

func TestBuildPrompt_WithoutContextInjection(t *testing.T) {
	convCtx := &conversationContext{
		cfg: &orchestrator.Config{},
		// No ctxBuilder — context injection should be skipped gracefully
	}

	sess := &session.Session{
		ID: "test-session",
		Messages: []session.Message{
			{Role: "user", Content: "Hello!"},
		},
	}

	agentCfg := &orchestrator.AgentConfig{
		ID:      "lumi",
		Role:    "lead",
		Context: []string{"soul"}, // Requested, but no builder available
	}

	prompt := convCtx.buildPrompt(sess, agentCfg)

	// Should still have agent identity
	if !strings.Contains(prompt, "You are lumi, a lead assistant") {
		t.Error("prompt should contain agent identity even without context injection")
	}

	// Should still have conversation history
	if !strings.Contains(prompt, "User: Hello!") {
		t.Error("prompt should contain conversation history even without context injection")
	}

	// Should NOT contain SOUL.md content (no builder)
	if strings.Contains(prompt, "SOUL") {
		t.Error("prompt should not contain SOUL.md when no builder is configured")
	}
}

func TestBuildPrompt_EmptyContextList(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "SOUL.md"), []byte("soul content"), 0644); err != nil {
		t.Fatal(err)
	}

	ctxBuilder := context.NewBuilder(workspace)
	convCtx := &conversationContext{
		cfg:       &orchestrator.Config{},
		ctxBuilder: ctxBuilder,
	}

	sess := &session.Session{
		ID:       "test-session",
		Messages: []session.Message{},
	}

	agentCfg := &orchestrator.AgentConfig{
		ID:      "lumi",
		Role:    "lead",
		Context: []string{}, // Empty context list — no injection
	}

	prompt := convCtx.buildPrompt(sess, agentCfg)

	// Should have identity but no SOUL.md
	if !strings.Contains(prompt, "You are lumi, a lead assistant") {
		t.Error("prompt should contain agent identity")
	}
	if strings.Contains(prompt, "soul content") {
		t.Error("prompt should not inject context when context list is empty")
	}
}

func TestBuildPrompt_MissingWorkspaceFiles(t *testing.T) {
	// Workspace with no files — context injection should fail gracefully
	workspace := t.TempDir()
	ctxBuilder := context.NewBuilder(workspace)

	convCtx := &conversationContext{
		cfg:       &orchestrator.Config{},
		ctxBuilder: ctxBuilder,
	}

	sess := &session.Session{
		ID:       "test-session",
		Messages: []session.Message{},
	}

	agentCfg := &orchestrator.AgentConfig{
		ID:      "lumi",
		Role:    "lead",
		Context: []string{"soul", "nonexistent"},
	}

	prompt := convCtx.buildPrompt(sess, agentCfg)

	// Should still work — missing files are skipped gracefully
	if !strings.Contains(prompt, "You are lumi, a lead assistant") {
		t.Error("prompt should contain agent identity even with missing workspace files")
	}
}