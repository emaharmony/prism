package factorymonitor

import (
	"fmt"
	"strings"
)

// FormatTaskMessage renders a concise Discord-safe task status message.
func FormatTaskMessage(kind string, st TaskStatus) string {
	prefix := "Factory status"
	if kind == EventStatusStuck {
		prefix = "Factory task may be stuck"
	}
	lines := []string{
		fmt.Sprintf("**%s**", prefix),
		fmt.Sprintf("Task: `%s`", st.TaskID),
		fmt.Sprintf("Status: `%s`", st.Status),
	}
	if st.Message != "" {
		lines = append(lines, "Message: "+truncate(st.Message, 300))
	}
	if st.UpdatedAt != "" {
		lines = append(lines, fmt.Sprintf("Updated: `%s`", st.UpdatedAt))
	}
	if kind == EventStatusStuck && st.AgeSeconds > 0 {
		lines = append(lines, fmt.Sprintf("No status update for: `%s`", formatAge(st.AgeSeconds)))
	}
	if st.ResultPath != "" {
		lines = append(lines, "Result: `"+st.ResultPath+"`")
	}
	if st.ErrorPath != "" {
		lines = append(lines, "Error: `"+st.ErrorPath+"`")
	}
	return truncate(strings.Join(lines, "\n"), 1900)
}

// FormatDigestMessage renders a concise Factory queue summary.
func FormatDigestMessage(snap Snapshot) string {
	var b strings.Builder
	b.WriteString("**Factory status digest**\n")
	b.WriteString(fmt.Sprintf("Root: `%s`\n", snap.Root))
	b.WriteString(fmt.Sprintf("Queue: inbox `%d`, processing `%d`, failed `%d`, archive `%d`\n",
		snap.Counts.Inbox, snap.Counts.Processing, snap.Counts.Failed, snap.Counts.Archive))
	b.WriteString(fmt.Sprintf("Tasks: active `%d`, completed `%d`\n", snap.Counts.Active, snap.Counts.Completed))

	active := make([]TaskStatus, 0)
	for _, st := range snap.Tasks {
		if !terminalStatuses[st.Status] {
			active = append(active, st)
		}
	}
	if len(active) == 0 {
		b.WriteString("No active Factory tasks.")
		return b.String()
	}
	b.WriteString("\nActive:\n")
	for i, st := range active {
		if i >= 8 {
			b.WriteString(fmt.Sprintf("- ...and `%d` more\n", len(active)-i))
			break
		}
		line := fmt.Sprintf("- `%s`: `%s`", st.TaskID, st.Status)
		if st.Message != "" {
			line += " - " + truncate(st.Message, 140)
		}
		if st.AgeSeconds > 0 {
			line += " (" + formatAge(st.AgeSeconds) + " since update)"
		}
		b.WriteString(line + "\n")
	}
	return truncate(b.String(), 1900)
}

func formatAge(seconds int64) string {
	if seconds < 60 {
		return fmt.Sprintf("%ds", seconds)
	}
	minutes := seconds / 60
	if minutes < 60 {
		return fmt.Sprintf("%dm", minutes)
	}
	hours := minutes / 60
	minutes = minutes % 60
	return fmt.Sprintf("%dh%02dm", hours, minutes)
}

func truncate(s string, limit int) string {
	if limit <= 0 || len(s) <= limit {
		return s
	}
	if limit <= 15 {
		return s[:limit]
	}
	return s[:limit-15] + "\n...(truncated)"
}
