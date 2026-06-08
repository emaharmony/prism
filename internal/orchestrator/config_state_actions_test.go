package orchestrator

import (
	"testing"
)

func TestResolveChannelRole(t *testing.T) {
	cfg := &Config{
		ChannelRoles: []ChannelRole{
			{ID: "1491622581348864162", Role: "manager-room"},
			{ID: "1491622824991920231", Role: "build-room"},
			{ID: "1493297644821283067", Role: "fun"},
		},
	}

	tests := []struct {
		channelID string
		expected  string
	}{
		{"1491622581348864162", "manager-room"},
		{"1491622824991920231", "build-room"},
		{"1493297644821283067", "fun"},
		{"999999999999999999", ""},       // unknown channel
		{"", ""},                           // empty channel
	}

	for _, tt := range tests {
		t.Run(tt.channelID, func(t *testing.T) {
			result := cfg.ResolveChannelRole(tt.channelID)
			if result != tt.expected {
				t.Errorf("ResolveChannelRole(%q) = %q, want %q", tt.channelID, result, tt.expected)
			}
		})
	}
}

func TestResolveStateAction(t *testing.T) {
	cfg := &Config{
		Agents: []AgentConfig{
			{
				ID:   "lumi",
				Role: "lead",
				StateActions: map[string]StateAction{
					"manager-room": {
						Inject: "You are in the manager room — full dev mode, all tools, be direct.",
					},
					"fun": {
						Inject: "You are in the fun channel — no tools, no code, purely conversational.",
					},
					"agent": {
						Inject: "This message is from another AI agent. Respond with structured data.",
					},
				},
			},
		},
	}

	tests := []struct {
		agentID string
		key     string
		want    string
		wantNil bool
	}{
		{
			agentID: "lumi",
			key:     "manager-room",
			want:    "You are in the manager room — full dev mode, all tools, be direct.",
		},
		{
			agentID: "lumi",
			key:     "fun",
			want:    "You are in the fun channel — no tools, no code, purely conversational.",
		},
		{
			agentID: "lumi",
			key:     "agent",
			want:    "This message is from another AI agent. Respond with structured data.",
		},
		{
			agentID: "lumi",
			key:     "unknown-state",
			wantNil: true,
		},
		{
			agentID: "unknown-agent",
			key:     "manager-room",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.agentID+"/"+tt.key, func(t *testing.T) {
			sa := cfg.ResolveStateAction(tt.agentID, tt.key)
			if tt.wantNil {
				if sa != nil {
					t.Errorf("ResolveStateAction(%q, %q) = %v, want nil", tt.agentID, tt.key, sa)
				}
				return
			}
			if sa == nil {
				t.Fatalf("ResolveStateAction(%q, %q) = nil, want non-nil", tt.agentID, tt.key)
			}
			if sa.Inject != tt.want {
				t.Errorf("ResolveStateAction(%q, %q).Inject = %q, want %q", tt.agentID, tt.key, sa.Inject, tt.want)
			}
		})
	}
}

func TestResolveChannelRole_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	result := cfg.ResolveChannelRole("1491622581348864162")
	if result != "" {
		t.Errorf("expected empty string for missing channel role, got %q", result)
	}
}

func TestResolveStateAction_EmptyConfig(t *testing.T) {
	cfg := &Config{}
	sa := cfg.ResolveStateAction("lumi", "manager-room")
	if sa != nil {
		t.Errorf("expected nil for missing state action, got %v", sa)
	}
}

func TestStateActionYAMLParsing(t *testing.T) {
	yamlContent := `
agents:
  - id: lumi
    role: lead
    provider: ollama
    model: glm-5.1:cloud
    primary: true
    state_actions:
      manager-room:
        inject: "Full dev mode. Be direct."
      fun:
        inject: "Casual social mode. No tools."
      agent:
        inject: "Peer agent. Structured responses."

channel_roles:
  - id: "1491622581348864162"
    role: manager-room
  - id: "1493297644821283067"
    role: fun

channels:
  - type: discord
    token: "test-token"
    channels: []

sessions:
  max_context_messages: 100
  idle_timeout_minutes: 30
  compaction_strategy: truncate
  daily_reset_hour: 4

remembrance:
  enabled: false
`
	cfg, err := LoadConfigFromBytes([]byte(yamlContent))
	if err != nil {
		t.Fatalf("Failed to parse YAML: %v", err)
	}

	// Check state actions parsed correctly
	if len(cfg.Agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(cfg.Agents))
	}

	lumi := cfg.Agents[0]
	if len(lumi.StateActions) != 3 {
		t.Errorf("expected 3 state actions, got %d", len(lumi.StateActions))
	}

	if sa, ok := lumi.StateActions["manager-room"]; !ok {
		t.Error("missing manager-room state action")
	} else if sa.Inject != "Full dev mode. Be direct." {
		t.Errorf("manager-room inject = %q, want %q", sa.Inject, "Full dev mode. Be direct.")
	}

	if sa, ok := lumi.StateActions["fun"]; !ok {
		t.Error("missing fun state action")
	} else if sa.Inject != "Casual social mode. No tools." {
		t.Errorf("fun inject = %q, want %q", sa.Inject, "Casual social mode. No tools.")
	}

	if sa, ok := lumi.StateActions["agent"]; !ok {
		t.Error("missing agent state action")
	} else if sa.Inject != "Peer agent. Structured responses." {
		t.Errorf("agent inject = %q, want %q", sa.Inject, "Peer agent. Structured responses.")
	}

	// Check channel roles parsed correctly
	if len(cfg.ChannelRoles) != 2 {
		t.Fatalf("expected 2 channel roles, got %d", len(cfg.ChannelRoles))
	}

	if cfg.ChannelRoles[0].ID != "1491622581348864162" || cfg.ChannelRoles[0].Role != "manager-room" {
		t.Errorf("channel role 0 = %+v, want {ID: 1491622581348864162, Role: manager-room}", cfg.ChannelRoles[0])
	}
	if cfg.ChannelRoles[1].ID != "1493297644821283067" || cfg.ChannelRoles[1].Role != "fun" {
		t.Errorf("channel role 1 = %+v, want {ID: 1493297644821283067, Role: fun}", cfg.ChannelRoles[1])
	}
	// Test resolution
	if role := cfg.ResolveChannelRole("1491622581348864162"); role != "manager-room" {
		t.Errorf("ResolveChannelRole(manager-room) = %q, want %q", role, "manager-room")
	}
	if sa := cfg.ResolveStateAction("lumi", "fun"); sa == nil || sa.Inject != "Casual social mode. No tools." {
		t.Errorf("ResolveStateAction(lumi, fun) = %+v", sa)
	}
}

func TestResolveChannelRoleConfig(t *testing.T) {
	cfg := &Config{
		ChannelRoles: []ChannelRole{
			{
				ID:          "1491622581348864162",
				Role:        "manager-room",
				Project:     "prism",
				Tools:       "all",
				Personality: "direct",
				Context:     "You are in #manager-room, a private strategic channel with Ema.",
			},
			{
				ID:          "1493297644821283067",
				Role:        "fun",
				Project:     "none",
				Tools:       "none",
				Personality: "bubbly",
				Context:     "You are in #fun, a casual social channel. NO tools, NO code.",
			},
		},
	}

	// Test existing channel
	cr := cfg.ResolveChannelRoleConfig("1491622581348864162")
	if cr == nil {
		t.Fatal("expected ChannelRole, got nil")
	}
	if cr.Project != "prism" {
		t.Errorf("Project = %q, want prism", cr.Project)
	}
	if cr.Tools != "all" {
		t.Errorf("Tools = %q, want all", cr.Tools)
	}
	if cr.Personality != "direct" {
		t.Errorf("Personality = %q, want direct", cr.Personality)
	}

	// Test fun channel
	cr2 := cfg.ResolveChannelRoleConfig("1493297644821283067")
	if cr2 == nil {
		t.Fatal("expected ChannelRole for fun, got nil")
	}
	if cr2.Project != "none" {
		t.Errorf("Project = %q, want none", cr2.Project)
	}
	if cr2.Tools != "none" {
		t.Errorf("Tools = %q, want none", cr2.Tools)
	}

	// Test unknown channel
	cr3 := cfg.ResolveChannelRoleConfig("999999999999999999")
	if cr3 != nil {
		t.Errorf("expected nil for unknown channel, got %+v", cr3)
	}

	// Test backward compatibility — old config with just ID and Role
	cfgOld := &Config{
		ChannelRoles: []ChannelRole{
			{ID: "123", Role: "test"},
		},
	}
	cr4 := cfgOld.ResolveChannelRoleConfig("123")
	if cr4 == nil {
		t.Fatal("expected ChannelRole for old config, got nil")
	}
	if cr4.Role != "test" {
		t.Errorf("Role = %q, want test", cr4.Role)
	}
	// New fields should be empty (backward compatible)
	if cr4.Project != "" {
		t.Errorf("Project = %q, want empty", cr4.Project)
	}
	if cr4.Tools != "" {
		t.Errorf("Tools = %q, want empty", cr4.Tools)
	}
}