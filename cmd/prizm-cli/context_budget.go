package main

import (
	"fmt"
	"log"

	"github.com/emaharmony/prizm/internal/provider"
)

// estimateTokenCount provides a rough estimate of the token count for a slice
// of chat messages. Uses the 4-chars-per-token heuristic.
func estimateTokenCount(messages []provider.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += len(msg.Content) / 4
		for _, tc := range msg.ToolCalls {
			total += len(tc.Function.Name) / 4
			for _, v := range tc.Function.Arguments {
				total += len(fmt.Sprintf("%v", v)) / 4
			}
		}
		if msg.Role == "system" {
			total += len(msg.Content) / 4
		}
	}
	return total
}

// compressToolResults replaces older tool results with compressed summaries
// to keep the conversation within the token budget. Tool results older than
// keepRecent are summarized to just the tool name and first 200 chars.
// Returns the number of messages compressed.
func compressToolResults(messages []provider.ChatMessage, keepRecent int, maxResultChars int) int {
	compressed := 0

	// Walk messages in order, compressing tool results that are not in the
	// last keepRecent tool-result messages.
	toolResultIndices := []int{}
	for i, msg := range messages {
		if msg.Role == "tool" {
			toolResultIndices = append(toolResultIndices, i)
		}
	}

	// Indices to compress: all tool results except the last keepRecent
	compressFrom := len(toolResultIndices) - keepRecent
	if compressFrom < 0 {
		compressFrom = 0
	}

	for i := 0; i < compressFrom && i < len(toolResultIndices); i++ {
		idx := toolResultIndices[i]
		msg := messages[idx]
		if len(msg.Content) > maxResultChars {
			// Compress: keep first maxResultChars chars + truncation notice
			truncated := msg.Content[:maxResultChars]
			truncated += fmt.Sprintf("\n\n[... %d more characters omitted ...]", len(msg.Content)-maxResultChars)
			messages[idx].Content = truncated
			compressed++
		}
	}

	return compressed
}

// contextBudget manages context window estimation and compression in the
// agentic loop. It monitors the estimated token count and compresses older
// tool results when the conversation exceeds the budget.
type contextBudget struct {
	modelContextTokens int   // Maximum context tokens for the model
	warnThreshold      float64 // Warn when tokens exceed this fraction (0.8 = 80%)
	compressThreshold  float64 // Compress when tokens exceed this fraction (0.9 = 90%)
	keepRecent         int     // Number of recent tool results to keep uncompressed
	maxResultChars     int     // Max chars for compressed tool results
}

// defaultContextBudget returns a budget configured for the given model's context size.
func defaultContextBudget(modelContextTokens int) contextBudget {
	return contextBudget{
		modelContextTokens: modelContextTokens,
		warnThreshold:      0.60,
		compressThreshold:  0.75,
		keepRecent:         6,
		maxResultChars:     300,
	}
}

// checkAndCompress estimates the current token count and compresses if needed.
// Returns the estimated token count after any compression.
func (cb *contextBudget) checkAndCompress(messages []provider.ChatMessage, iteration int) int {
	estimated := estimateTokenCount(messages)
	budget := cb.modelContextTokens

	if budget <= 0 {
		return estimated // No budget set, skip compression
	}

	ratio := float64(estimated) / float64(budget)

	if ratio >= cb.compressThreshold {
		compressed := compressToolResults(messages, cb.keepRecent, cb.maxResultChars)
		if compressed > 0 {
			log.Printf("[CONTEXT-BUDGET] iteration %d: %.0f%% of context budget (%d/%d tokens), compressed %d older tool results", iteration, ratio*100, estimated, budget, compressed)
		}
		// Re-estimate after compression
		estimated = estimateTokenCount(messages)
		ratio = float64(estimated) / float64(budget)
	}

	if ratio >= cb.warnThreshold {
		log.Printf("[CONTEXT-BUDGET] iteration %d: %.0f%% of context budget (%d/%d tokens)", iteration, ratio*100, estimated, budget)
	}

	return estimated
}
// modelContextTokens maps known model names to their maximum context window size.
// This is used to estimate when compression is needed.
func getModelContextTokens(model string) (int, bool) {
	known := map[string]int{
		"glm-5.1:cloud":       202752,
		"glm-5.2:cloud":       202752,
		"glm-4:cloud":         131072,
		"deepseek-v4-pro:cloud": 131072,
		"deepseek-v4-flash:cloud": 131072,
		"qwen3.5:4b":         32768,
		"qwen3.5:9b":         65536,
		"qwen3.5:cloud":      131072,
		"qwen3-coder:480b-cloud": 131072,
	}
	tokens, ok := known[model]
	return tokens, ok
}
