// Package stage provides the response gate — a lightweight heuristic that
// decides whether a Discord message warrants a full LLM call.
//
// The goal is to avoid wasting LLM inference on low-signal messages like "+1",
// "lol", memes, and other social chatter that doesn't need a thoughtful response.
// This is NOT an LLM call itself — it's a fast keyword/length/channel heuristic.
//
// Decision logic:
//   - Manager-room: always respond (Ema expects responses there)
//   - Build-room: always respond (agent messages need acknowledgment)
//   - Fun channel: always respond (that's the whole point)
//   - Unknown channel: respond to questions and direct mentions, skip low-signal
//
// Low-signal patterns:
//   - Very short messages (<5 chars) that aren't questions
//   - Pure emoji/reaction messages
//   - Common social noise: "+1", "lol", "nice", "haha", etc.
//   - Messages that are just acknowledgments
package stage

import (
	"strings"
)

// ResponseDecision represents the gate's decision about a message.
type ResponseDecision int

const (
	// RespondFully means the message warrants a full LLM call.
	RespondFully ResponseDecision = iota
	// RespondLightly means the message warrants a short acknowledgment.
	RespondLightly
	// Skip means the message doesn't need a response.
	Skip
)

// lowSignalMessages are messages that don't warrant a full LLM response.
// These are common social noise that people type in channels.
var lowSignalMessages = map[string]bool{
	"+1":     true,
	"++":     true,
	"lol":    true,
	"lmao":   true,
	"haha":   true,
	"hehe":   true,
	"nice":   true,
	"cool":   true,
	"ok":     true,
	"okay":   true,
	"yep":    true,
	"nope":   true,
	"sure":   true,
	"thanks": true,
	"thx":    true,
	"ty":     true,
	"gg":     true,
	"rip":    true,
	"oof":    true,
	"smh":    true,
	"wdym":   true,
	"wdyt":   true,
	"idk":    true,
	"ig":     true,
	"tbh":    true,
	"nvm":    true,
	"brb":    true,
	"afk":    true,
}

// ShouldRespond decides whether a message warrants a response.
// channelRole MUST be a role name (e.g., "manager-room", "build-room", "fun"),
// NOT a channel ID. Passing a channel ID will cause the gate to treat it as
// an unknown channel and apply heuristics, potentially skipping important messages.
func ShouldRespond(message, channelRole string) ResponseDecision {
	trimmed := strings.TrimSpace(message)

	// In strategic channels (manager-room, build-room), always respond fully.
	// These channels are for work — every message matters, even empty ones.
	if channelRole == "manager-room" || channelRole == "build-room" {
		return RespondFully
	}

	// Empty messages are always skipped (except in strategic channels above)
	if trimmed == "" {
		return Skip
	}

	// In fun/social channels, always respond — that's the point.
	if channelRole == "fun" || channelRole == "social" {
		return RespondFully
	}

	// For unknown/general channels, apply heuristics.

	// Messages with direct questions always get full responses
	if containsQuestion(trimmed) {
		return RespondFully
	}

	// Messages that mention tools or code get full responses
	if containsTechIntent(trimmed) {
		return RespondFully
	}

	// Very short messages that are low-signal noise get light responses
	lower := strings.ToLower(trimmed)
	if len([]rune(trimmed)) <= 4 {
		if lowSignalMessages[lower] {
			return RespondLightly
		}
		// Short but not in the noise list — could be important
		// (e.g., "yes", "no", "go", "stop")
		return RespondLightly
	}

	// Messages that are pure emoji get light responses
	if isMostlyEmoji(trimmed) {
		return RespondLightly
	}

	// Everything else gets a full response
	return RespondFully
}

// containsQuestion checks if a message contains a question mark or question words.
func containsQuestion(s string) bool {
	if strings.Contains(s, "?") {
		return true
	}
	lower := strings.ToLower(s)
	questionStarters := []string{
		"what", "how", "why", "when", "where", "who", "which",
		"can you", "could you", "do you", "is there", "are there",
		"will you", "would you", "should i", "can i",
	}
	for _, q := range questionStarters {
		if strings.HasPrefix(lower, q+" ") || strings.Contains(lower, " "+q+" ") {
			return true
		}
	}
	return false
}

// containsTechIntent checks if a message seems to be about technical work.
// Includes short command words that signal intent even at ≤4 chars.
func containsTechIntent(s string) bool {
	lower := strings.ToLower(s)
	techWords := []string{
		"code", "file", "function", "class", "method", "variable",
		"bug", "fix", "error", "issue", "test", "deploy", "build",
		"pr", "merge", "commit", "branch", "repo", "project",
		"api", "server", "client", "database", "config", "log",
		"feature", "refactor", "implement", "debug", "review",
		// Short command words that signal tech intent even at ≤4 chars
		"new", "add", "run", "set", "get", "put", "del",
		"git", "ssh", "npm", "pip", "go",
	}
	for _, w := range techWords {
		if strings.Contains(lower, w) {
			return true
		}
	}
	return false
}

// isMostlyEmoji checks if a message is primarily emoji characters.
func isMostlyEmoji(s string) bool {
	runes := []rune(s)
	if len(runes) == 0 {
		return false
	}
	emojiCount := 0
	for _, r := range runes {
		if isEmojiRune(r) {
			emojiCount++
		}
	}
	return emojiCount > len(runes)/2
}

// isEmojiRune checks if a rune is likely an emoji.
func isEmojiRune(r rune) bool {
	// Common emoji ranges
	return (r >= 0x1F600 && r <= 0x1F64F) || // Emoticons
		(r >= 0x1F300 && r <= 0x1F5FF) || // Misc Symbols and Pictographs
		(r >= 0x1F680 && r <= 0x1F6FF) || // Transport and Map
		(r >= 0x1F1E6 && r <= 0x1F1FF) || // Flags
		(r >= 0x2600 && r <= 0x26FF) ||   // Misc symbols
		(r >= 0x2700 && r <= 0x27BF) ||   // Dingbats
		(r >= 0xFE00 && r <= 0xFE0F) ||   // Variation selectors
		(r >= 0x1F900 && r <= 0x1F9FF) || // Supplemental symbols
		(r >= 0x1FA00 && r <= 0x1FA6F) || // Chess symbols
		(r >= 0x1FA70 && r <= 0x1FAFF) || // Symbols and pictographs extended
		(r == 0x200D) // ZWJ
}