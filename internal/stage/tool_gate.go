package stage

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// ToolDecision determines whether tools should be included in the LLM request.
type ToolDecision int

const (
	// ToolDecisionInclude includes all available tools.
	ToolDecisionInclude ToolDecision = iota
	// ToolDecisionExclude excludes all tools — conversational response expected.
	ToolDecisionExclude
	// ToolDecisionSubset includes only a subset of tools (filtered by name).
	ToolDecisionSubset
)

// GateResult is the output of the ToolRelevanceGate evaluation.
type GateResult struct {
	Decision   ToolDecision
	Reason     string   // Human-readable reason for the decision (logged)
	ToolFilter []string // Tool names to include (only for Subset decision)
}

// ToolRelevanceGate decides whether tools should be included in the LLM request
// based on the current user message. This prevents inappropriate tool calls
// for simple conversational messages (greetings, thanks, opinions).
//
// V1 uses heuristic keyword matching — no LLM call, zero latency, deterministic.
// The gate evaluates the RAW user message only, not conversation history,
// to prevent context bleed from prior tool-relevant discussions.
type ToolRelevanceGate struct {
	// Enabled controls whether the gate is active.
	Enabled bool
	// IncludeKeywords are words that signal the message needs tools.
	// Defaults are used if empty.
	IncludeKeywords []string
	// GitKeywords signal the message needs git tools only.
	GitKeywords []string
	// ExcludePatterns are regex patterns that signal a conversational message.
	ExcludePatterns []*regexp.Regexp
}

// DefaultIncludeKeywords are words that strongly indicate tool relevance.
var DefaultIncludeKeywords = []string{
	"read", "search", "find", "list", "show", "open", "check",
	"git", "branch", "status", "diff", "log", "commit", "push",
	"project", "file", "directory", "folder", "path", "code", "repo",
	"overview", "structure", "contents", "config", "source",
	"workspace", "scan", "inspect", "analyze",
	"image", "images", "reference", "references", "download", "generate", "caption", "collect", "asset", "assets",
}

// DefaultGitKeywords are words that signal git-only tool needs.
var DefaultGitKeywords = []string{
	"git status", "git log", "git diff", "git branch",
	"git add", "git commit", "git push",
}

// DefaultExcludePatterns match common conversational patterns.
var defaultExcludePatterns = []string{
	`^(hey|hi|hello|yo|sup|hola|howdy|greetings)\b`,           // Greetings
	`^(thanks|thank you|thx|ty|cheers|bye|goodbye|cya)\b`,     // Farewells
	`^(lol|lmao|rofl|haha|😊|😂|❤️|✨|💜|💛)$`,                     // Pure emoji/reaction
	`^(yes|no|ok|okay|sure|right|got it|cool|nice|wow|omg)\b`, // Acknowledgments
	`^(say|tell me) (hello|hi|hey|goodbye|thanks)`,            // Conversational commands
	`what do you think about`,                                 // Opinion questions
	`^(what's up|how are you|how's it going)\b`,               // Social check-ins
}

// NewToolRelevanceGate creates a gate with default settings.
func NewToolRelevanceGate(enabled bool) *ToolRelevanceGate {
	gate := &ToolRelevanceGate{
		Enabled:         enabled,
		IncludeKeywords: DefaultIncludeKeywords,
		GitKeywords:     DefaultGitKeywords,
	}

	// Compile default exclude patterns
	for _, p := range defaultExcludePatterns {
		re, err := regexp.Compile("(?i)" + p)
		if err == nil {
			gate.ExcludePatterns = append(gate.ExcludePatterns, re)
		}
	}

	return gate
}

// Evaluate determines whether tools should be included for the given message.
// It analyzes the raw user message and returns a GateResult.
//
// The evaluation is conservative: when uncertain, it includes tools
// (false positive = slight waste, false negative = broken functionality).
func (g *ToolRelevanceGate) Evaluate(message string, availableTools []string) GateResult {
	if !g.Enabled {
		return GateResult{
			Decision: ToolDecisionInclude,
			Reason:   "gate disabled",
		}
	}

	// Normalize the message for analysis
	normalized := strings.ToLower(strings.TrimSpace(message))

	// Empty or very short messages with no keywords → Exclude
	if len(message) < 15 && !containsAnyKeyword(normalized, g.IncludeKeywords) {
		return GateResult{
			Decision: ToolDecisionExclude,
			Reason:   "short message with no tool keywords",
		}
	}

	// Check exclude patterns (greetings, farewells, etc.)
	for _, pattern := range g.ExcludePatterns {
		if pattern.MatchString(normalized) {
			// But if the message also contains tool keywords, include anyway
			// e.g., "Hey can you read the file?" → Include (has "read")
			if containsAnyKeyword(normalized, g.IncludeKeywords) {
				return GateResult{
					Decision: ToolDecisionInclude,
					Reason:   "matches conversational pattern but contains tool keyword",
				}
			}
			return GateResult{
				Decision: ToolDecisionExclude,
				Reason:   "matches conversational pattern: " + pattern.String(),
			}
		}
	}

	// Check for file path patterns (absolute or relative paths)
	if containsFilePath(message) {
		return GateResult{
			Decision: ToolDecisionInclude,
			Reason:   "message contains file path",
		}
	}

	// Check for tool keywords
	hasKeyword, keywordName := containsAnyKeywordWithMatch(normalized, g.IncludeKeywords)
	if hasKeyword {
		// Check if it's git-only
		for _, gk := range g.GitKeywords {
			if strings.Contains(normalized, strings.ToLower(gk)) {
				gitTools := filterToolsByPrefix(availableTools, "git_")
				if len(gitTools) > 0 {
					return GateResult{
						Decision:   ToolDecisionSubset,
						Reason:     "git-specific keyword: " + gk,
						ToolFilter: gitTools,
					}
				}
			}
		}
		return GateResult{
			Decision: ToolDecisionInclude,
			Reason:   "contains tool keyword: " + keywordName,
		}
	}

	// Check for question patterns about code/files/projects
	if isFactualQuestionAboutCode(normalized) {
		return GateResult{
			Decision: ToolDecisionInclude,
			Reason:   "factual question about code/project",
		}
	}

	// Default: include tools (conservative)
	return GateResult{
		Decision: ToolDecisionInclude,
		Reason:   "no exclusion criteria matched (conservative default)",
	}
}

// containsAnyKeyword checks if the text contains any of the keywords.
func containsAnyKeyword(text string, keywords []string) bool {
	for _, kw := range keywords {
		if wordBoundaryMatch(text, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

// containsAnyKeywordWithMatch returns true and the matching keyword.
func containsAnyKeywordWithMatch(text string, keywords []string) (bool, string) {
	for _, kw := range keywords {
		if wordBoundaryMatch(text, strings.ToLower(kw)) {
			return true, kw
		}
	}
	return false, ""
}

// wordBoundaryMatch checks if keyword appears as a whole word in text.
func wordBoundaryMatch(text, keyword string) bool {
	idx := strings.Index(text, keyword)
	if idx == -1 {
		return false
	}
	// Check if it's a whole word
	beforeOK := idx == 0 || !isAlpha(rune(text[idx-1]))
	afterOK := idx+len(keyword) >= len(text) || !isAlpha(rune(text[idx+len(keyword)]))
	return beforeOK && afterOK
}

func isAlpha(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

// containsFilePath detects file path patterns in the message.
func containsFilePath(message string) bool {
	// Absolute paths: /Users/..., /home/..., C:\..., ~/
	if strings.Contains(message, "/") || strings.Contains(message, "\\") {
		// Check if it looks like a path (has a dot extension or multiple path segments)
		if filepath.Ext(message) != "" {
			return true
		}
		// Check for path separators with directory names
		parts := strings.Split(message, "/")
		if len(parts) >= 3 {
			return true
		}
	}
	// Home directory reference
	if strings.Contains(message, "~/") {
		return true
	}
	return false
}

// isFactualQuestionAboutCode detects questions that need file/project access.
func isFactualQuestionAboutCode(text string) bool {
	// Pattern: "what does X do", "how is X structured", "show me X"
	questionStarters := []string{"what does", "how is", "how does", "show me", "tell me about"}
	codeNouns := []string{"project", "code", "module", "function", "class", "method", "file", "structure", "architecture"}

	for _, starter := range questionStarters {
		if strings.Contains(text, starter) {
			for _, noun := range codeNouns {
				if strings.Contains(text, noun) {
					return true
				}
			}
		}
	}
	return false
}

// filterToolsByPrefix returns tool names that start with the given prefix.
func filterToolsByPrefix(tools []string, prefix string) []string {
	var filtered []string
	for _, t := range tools {
		if strings.HasPrefix(t, prefix) {
			filtered = append(filtered, t)
		}
	}
	return filtered
}
