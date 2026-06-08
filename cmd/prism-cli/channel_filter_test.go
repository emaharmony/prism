package main

import (
	"testing"

	"github.com/emaharmony/prism/internal/orchestrator"
	"github.com/emaharmony/prism/internal/provider"
	"github.com/emaharmony/prism/internal/tool"
)

func TestFilterChatToolsByChannelRole(t *testing.T) {
	allTools := []provider.ChatTool{
		{Function: provider.FunctionDef{Name: "read_project"}},
		{Function: provider.FunctionDef{Name: "search_files"}},
		{Function: provider.FunctionDef{Name: "git_status"}},
		{Function: provider.FunctionDef{Name: "git_commit"}},
		{Function: provider.FunctionDef{Name: "git_push"}},
		{Function: provider.FunctionDef{Name: "plan_create"}},
		{Function: provider.FunctionDef{Name: "state_get"}},
	}

	tests := []struct {
		name        string
		role        *orchestrator.ChannelRole
		expectCount int
		expectNames []string
	}{
		{
			name:        "nil role returns all tools",
			role:        nil,
			expectCount: 7,
		},
		{
			name:        "all tools returns all",
			role:        &orchestrator.ChannelRole{Role: "manager-room", Tools: "all"},
			expectCount: 7,
		},
		{
			name:        "empty tools returns all",
			role:        &orchestrator.ChannelRole{Role: "manager-room", Tools: ""},
			expectCount: 7,
		},
		{
			name:        "none returns no tools",
			role:        &orchestrator.ChannelRole{Role: "fun", Tools: "none"},
			expectCount: 0,
		},
		{
			name:        "read-only returns only read tools",
			role:        &orchestrator.ChannelRole{Role: "casual", Tools: "read-only"},
			expectCount: 4, // read_project, search_files, git_status, state_get
			expectNames: []string{"read_project", "search_files", "git_status", "state_get"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterChatToolsByChannelRole(allTools, tt.role)
			if len(result) != tt.expectCount {
				t.Errorf("got %d tools, want %d", len(result), tt.expectCount)
			}
			if tt.expectNames != nil {
				resultNames := make(map[string]bool)
				for _, ct := range result {
					resultNames[ct.Function.Name] = true
				}
				for _, name := range tt.expectNames {
					if !resultNames[name] {
						t.Errorf("missing expected tool %q", name)
					}
				}
			}
		})
	}
}

func TestFilterToolInfosByChannelRole(t *testing.T) {
	allInfos := []tool.ToolInfo{
		{Name: "read_project", Description: "Read project files"},
		{Name: "search_files", Description: "Search files"},
		{Name: "git_status", Description: "Check git status"},
		{Name: "git_commit", Description: "Git commit"},
		{Name: "git_push", Description: "Git push"},
	}

	tests := []struct {
		name        string
		role        *orchestrator.ChannelRole
		expectCount int
	}{
		{
			name:        "nil role returns all",
			role:        nil,
			expectCount: 5,
		},
		{
			name:        "all returns all",
			role:        &orchestrator.ChannelRole{Role: "manager-room", Tools: "all"},
			expectCount: 5,
		},
		{
			name:        "none returns nil",
			role:        &orchestrator.ChannelRole{Role: "fun", Tools: "none"},
			expectCount: 0,
		},
		{
			name:        "read-only returns only read tools",
			role:        &orchestrator.ChannelRole{Role: "casual", Tools: "read-only"},
			expectCount: 3, // read_project, search_files, git_status
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterToolInfosByChannelRole(allInfos, tt.role)
			if len(result) != tt.expectCount {
				t.Errorf("got %d tool infos, want %d", len(result), tt.expectCount)
			}
		})
	}
}