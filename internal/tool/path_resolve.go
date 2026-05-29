package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FuzzyResolvePath attempts to resolve a path that may be partial, misspelled,
// or use non-standard conventions (like ~ or /home/user instead of /Users/user).
//
// Resolution strategy:
//  1. Exact resolution via ResolveToolPath (handles relative, absolute, containment)
//  2. Tilde expansion: ~/... → home directory
//  3. Fuzzy match: search allowed roots for directories matching the last component
//     e.g., "bassbook" → /Users/ema/projects/repos/bassbook
//  4. Common path corrections: /home/user → /Users/user on macOS
//
// This allows agents to say "summarize bassbook" or "read ~/projects/repos/bassbook"
// and have the tools find the right path even if the model doesn't know the exact
// absolute path or uses Linux-style conventions on macOS.
func FuzzyResolvePath(tp ToolPaths, inputPath string) (string, error) {
	// Step 1: Try exact resolution first
	resolved, err := ResolveToolPath(tp, inputPath)
	if err == nil {
		return resolved, nil
	}

	// Step 2: Tilde expansion
	if strings.HasPrefix(inputPath, "~/") {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return "", fmt.Errorf("cannot expand ~: %w", homeErr)
		}
		expanded := filepath.Join(home, inputPath[2:])
		if resolved, err := ResolveToolPath(tp, expanded); err == nil {
			return resolved, nil
		}
		// Continue with fuzzy matching on the expanded path
		inputPath = expanded
	}

	// Step 3: Common path corrections
	// On macOS, /home/user → /Users/user is a common model hallucination
	if strings.HasPrefix(inputPath, "/home/") {
		parts := strings.SplitN(inputPath[6:], "/", 2)
		if len(parts) > 0 {
			corrected := filepath.Join("/Users", parts[0])
			if len(parts) > 1 {
				corrected = filepath.Join(corrected, parts[1])
			}
			if resolved, err := ResolveToolPath(tp, corrected); err == nil {
				return resolved, nil
			}
			inputPath = corrected
		}
	}

	// Step 3b: If the path starts with ~ after expansion failure,
	// or if it still has a tilde prefix, strip it and try fuzzy matching
	// on the last component. E.g., ~/projects/repos/bassbook → fuzzy match "bassbook".
	if strings.HasPrefix(inputPath, "~") {
		stripped := inputPath
		if strings.HasPrefix(stripped, "~/") {
			stripped = stripped[2:]
		} else {
			stripped = strings.TrimPrefix(stripped, "~")
			if strings.HasPrefix(stripped, "/") {
				stripped = stripped[1:]
			}
		}
		if stripped != "" {
			inputPath = stripped
		}
	}

	// Step 4: Fuzzy match — search allowed roots for a directory
	// matching the last path component
	targetName := filepath.Base(inputPath)
	if targetName == "" || targetName == "." || targetName == "/" {
		return "", fmt.Errorf("cannot fuzzy-match empty path: %q", inputPath)
	}

	match, err := fuzzyMatchInRoots(tp, targetName)
	if err != nil {
		// Return the original error from ResolveToolPath with context
		return "", fmt.Errorf("path %q not found (tried exact, ~, /home→/Users, and fuzzy match): %w",
			inputPath, err)
	}

	return match, nil
}

// fuzzyMatchInRoots searches all allowed root directories for immediate
// subdirectories matching the target name (case-insensitive).
//
// Priority:
//  1. Exact match (case-sensitive)
//  2. Case-insensitive match
//  3. Prefix match
//
// Returns the full absolute path of the best match, or an error if no match found.
func fuzzyMatchInRoots(tp ToolPaths, targetName string) (string, error) {
	roots := tp.AllRoots()
	lowerTarget := strings.ToLower(targetName)

	var caseInsensitiveMatch string
	var prefixMatch string

	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue // Can't read this root, skip it
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			name := entry.Name()

			// Exact case-sensitive match — return immediately
			if name == targetName {
				candidate := filepath.Join(root, name)
				// Verify containment
				if _, err := ResolveToolPath(tp, candidate); err == nil {
					return candidate, nil
				}
			}

			// Case-insensitive match — remember, keep looking for exact
			if strings.ToLower(name) == lowerTarget && caseInsensitiveMatch == "" {
				candidate := filepath.Join(root, name)
				if _, err := ResolveToolPath(tp, candidate); err == nil {
					caseInsensitiveMatch = candidate
				}
			}

			// Prefix match — remember, keep looking for better matches
			if strings.HasPrefix(strings.ToLower(name), lowerTarget) && prefixMatch == "" {
				candidate := filepath.Join(root, name)
				if _, err := ResolveToolPath(tp, candidate); err == nil {
					prefixMatch = candidate
				}
			}
		}
	}

	if caseInsensitiveMatch != "" {
		return caseInsensitiveMatch, nil
	}
	if prefixMatch != "" {
		return prefixMatch, nil
	}

	return "", fmt.Errorf("no directory named %q found in any allowed root", targetName)
}