package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/emaharmony/prizm/internal/context"
	"github.com/emaharmony/prizm/internal/orchestrator"
	"github.com/emaharmony/prizm/internal/session"
)

func TestResolveConversationPostfix(t *testing.T) {
	tests := []struct {
		name        string
		agentCfg    *orchestrator.AgentConfig
		channelRole *orchestrator.ChannelRole
		want        string
	}{
		{
			name:        "explicit agent postfix wins over channel personality",
			agentCfg:    &orchestrator.AgentConfig{ConversationPostfix: "Speak only in haiku."},
			channelRole: &orchestrator.ChannelRole{Personality: "bubbly"},
			want:        "Speak only in haiku.",
		},
		{
			name:        "channel personality wins when agent has no explicit postfix",
			agentCfg:    &orchestrator.AgentConfig{},
			channelRole: &orchestrator.ChannelRole{Personality: "terse"},
			want:        orchestrator.PersonalityDirective("terse"),
		},
		{
			name:        "unrecognized personality falls back to the harness default",
			agentCfg:    &orchestrator.AgentConfig{},
			channelRole: &orchestrator.ChannelRole{Personality: "nonexistent"},
			want:        defaultConversationPostfix,
		},
		{
			name:        "nil channel role falls back to the harness default",
			agentCfg:    &orchestrator.AgentConfig{},
			channelRole: nil,
			want:        defaultConversationPostfix,
		},
		{
			name:        "no agent config, no channel role — harness default",
			agentCfg:    nil,
			channelRole: nil,
			want:        defaultConversationPostfix,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveConversationPostfix(tt.agentCfg, tt.channelRole)
			if got != tt.want {
				t.Errorf("resolveConversationPostfix() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildChatPrompt_PersonalityDirective(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "SOUL.md"), []byte("# SOUL\nYou are Lumi."), 0644); err != nil {
		t.Fatal(err)
	}
	ctxBuilder := context.NewBuilder(workspace)
	cc := &chatContext{
		cfg:        &orchestrator.Config{},
		ctxBuilder: ctxBuilder,
	}
	sess := &session.Session{ID: "test", Messages: []session.Message{}}

	agentCfg := &orchestrator.AgentConfig{ID: "lumi", Role: "lead", Context: []string{"soul"}}
	cc.buildStaticSystemContent(agentCfg)

	channelRole := &orchestrator.ChannelRole{Role: "manager-room", Personality: "terse"}
	prompt := cc.buildChatPrompt(sess, agentCfg, "manager-room", channelRole)
	if !strings.Contains(prompt, "## How You Respond") {
		t.Error("expected prompt to contain a How You Respond section")
	}
	if !strings.Contains(prompt, orchestrator.PersonalityDirective("terse")) {
		t.Error("expected prompt to contain the terse personality's directive text")
	}
	if strings.Contains(prompt, defaultConversationPostfix) {
		t.Error("expected the channel personality directive to replace the harness default")
	}

	messages := cc.buildChatMessages(sess, agentCfg, "manager-room", channelRole)
	if len(messages) == 0 || messages[0].Role != "system" {
		t.Fatal("expected a system message")
	}
	if !strings.Contains(messages[0].Content, orchestrator.PersonalityDirective("terse")) {
		t.Error("expected the ChatProvider system message to contain the terse personality's directive text")
	}
}
