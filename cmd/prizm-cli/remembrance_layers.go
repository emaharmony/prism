package main

import (
	"strings"

	"github.com/emaharmony/prizm/internal/orchestrator"
	"github.com/emaharmony/prizm/internal/remembrance"
	"github.com/emaharmony/prizm/internal/session"
)

func remembranceMemoryBlock(ctx *remembrance.ContextPackResponse) string {
	if ctx == nil {
		return ""
	}
	if strings.TrimSpace(ctx.ContextMarkdown) != "" {
		return "Long-term semantic memory from Remembrance. Use this below the exact recent local transcript if they conflict:\n" + ctx.ContextMarkdown
	}
	if ctx.ContextJSON == nil || len(ctx.ContextJSON.Memories) == 0 {
		return ""
	}
	var parts []string
	for _, mem := range ctx.ContextJSON.Memories {
		if strings.TrimSpace(mem.Summary) != "" {
			parts = append(parts, "- "+mem.Summary)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "Long-term semantic memory from Remembrance. Use this below the exact recent local transcript if they conflict:\n" + strings.Join(parts, "\n")
}

func localRecentSummary(sess *session.Session) string {
	if sess == nil {
		return ""
	}
	var lines []string
	for _, msg := range sess.Messages {
		if msg.Role == "system" && strings.Contains(msg.Content, "Local rolling summary") {
			lines = append(lines, msg.Content)
			continue
		}
		if msg.Role != "user" && msg.Role != "agent" {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		if len(content) > 240 {
			content = content[:240] + "..."
		}
		speaker := msg.Role
		if msg.Role == "agent" && msg.AgentID != "" {
			speaker = msg.AgentID
		}
		lines = append(lines, speaker+": "+content)
	}
	if len(lines) > 20 {
		lines = lines[len(lines)-20:]
	}
	return strings.Join(lines, "\n")
}

func channelRoleContext(role *orchestrator.ChannelRole) string {
	if role == nil {
		return ""
	}
	if role.Context != "" {
		return role.Context
	}
	return role.Role
}
