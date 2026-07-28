// Package linkunderstanding provides automatic URL detection and content
// fetching for incoming messages. When a user sends a link, the system
// fetches the content and injects it into the agent's system prompt so
// the agent already knows what the link contains before responding.
//
// This mirrors OpenClaw's link-understanding feature: detect URLs in
// messages, fetch readable content, inject as "## Fetched Link Content"
// in the system prompt.
package linkunderstanding

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// URLRegex matches URLs in message text.
var urlRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

// DetectedLink represents a URL found in a message with its fetched content.
type DetectedLink struct {
	URL     string `json:"url"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content,omitempty"`
	Error   string `json:"error,omitempty"`
}

// DetectURLs extracts all URLs from a text string.
// Returns URLs in order of appearance, deduplicated.
func DetectURLs(text string) []string {
	matches := urlRegex.FindAllString(text, -1)
	if len(matches) == 0 {
		return nil
	}

	// Deduplicate while preserving order
	seen := make(map[string]bool)
	var urls []string
	for _, u := range matches {
		// Strip trailing punctuation that's likely not part of the URL
		u = strings.TrimRight(u, ".,;:!?)")
		if !seen[u] {
			seen[u] = true
			urls = append(urls, u)
		}
	}
	return urls
}

// FetchContent fetches the content of a URL and extracts readable text.
// It strips HTML tags and returns plain text, truncated to maxBytes.
// Returns the page title (if found) and the content.
func FetchContent(url string, maxBytes int, timeout time.Duration) (title, content string, err error) {
	if maxBytes <= 0 {
		maxBytes = 4000
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", "", fmt.Errorf("fetch %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", "", fmt.Errorf("fetch %s: HTTP %d", url, resp.StatusCode)
	}

	// Read up to maxBytes + some headroom for truncation
	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes*2)))
	if err != nil {
		return "", "", fmt.Errorf("read %s: %w", url, err)
	}

	// Extract title
	title = extractTitle(string(body))

	// Extract readable content — strip HTML tags
	content = stripHTML(string(body))
	if len(content) > maxBytes {
		content = content[:maxBytes] + "\n[... truncated ...]"
	}

	return title, content, nil
}

// ProcessLinks detects URLs in the message text, fetches their content,
// and returns a formatted block for system prompt injection.
//
// If no URLs are found, returns an empty string.
// If fetching fails for a URL, the error is included but doesn't block
// other URLs from being fetched.
func ProcessLinks(messageText string, maxLinks int, maxBytes int, timeout time.Duration) string {
	urls := DetectURLs(messageText)
	if len(urls) == 0 {
		return ""
	}

	if maxLinks > 0 && len(urls) > maxLinks {
		urls = urls[:maxLinks]
	}

	var links []DetectedLink
	for _, u := range urls {
		link := DetectedLink{URL: u}
		title, content, err := FetchContent(u, maxBytes, timeout)
		if err != nil {
			link.Error = err.Error()
		} else {
			link.Title = title
			link.Content = content
		}
		links = append(links, link)
	}

	return FormatForPrompt(links)
}

// FormatForPrompt renders detected links as a system prompt section.
func FormatForPrompt(links []DetectedLink) string {
	if len(links) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("## Fetched Link Content\n")
	b.WriteString("The user sent links in their message. Here's the content from those pages:\n\n")

	for _, link := range links {
		b.WriteString(fmt.Sprintf("### %s\n", link.URL))
		if link.Title != "" {
			b.WriteString(fmt.Sprintf("**Title:** %s\n\n", link.Title))
		}
		if link.Error != "" {
			b.WriteString(fmt.Sprintf("_Fetch failed: %s_\n\n", link.Error))
		} else if link.Content != "" {
			b.WriteString(link.Content + "\n\n")
		}
	}

	return b.String()
}

// extractTitle finds the <title> tag content in HTML.
func extractTitle(html string) string {
	lower := strings.ToLower(html)
	start := strings.Index(lower, "<title>")
	if start < 0 {
		return ""
	}
	end := strings.Index(lower[start:], "</title>")
	if end < 0 {
		return ""
	}
	title := html[start+7 : start+end]
	return strings.TrimSpace(title)
}

// stripHTML removes HTML tags and returns plain text.
// This is a simple implementation — not a full HTML parser.
// It handles common cases: removes tags, decodes entities, collapses whitespace.
func stripHTML(html string) string {
	// Remove script and style blocks
	for _, tag := range []string{"script", "style", "nav", "footer", "header"} {
		html = removeTagBlock(html, tag)
	}

	// Remove all remaining HTML tags
	var b strings.Builder
	inTag := false
	for _, c := range html {
		if c == '<' {
			inTag = true
			continue
		}
		if c == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(c)
		}
	}

	text := b.String()

	// Decode common HTML entities
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")

	// Collapse whitespace
	text = strings.Join(strings.Fields(text), " ")

	return strings.TrimSpace(text)
}

// removeTagBlock removes everything between <tag> and </tag> (inclusive).
func removeTagBlock(html, tag string) string {
	lower := strings.ToLower(html)
	open := "<" + tag
	close := "</" + tag + ">"

	for {
		start := strings.Index(lower, open)
		if start < 0 {
			break
		}
		end := strings.Index(lower[start:], close)
		if end < 0 {
			break
		}
		end += start + len(close)
		html = html[:start] + html[end:]
		lower = strings.ToLower(html)
	}

	return html
}