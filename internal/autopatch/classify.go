package autopatch

import "strings"

// RequestMatches detects explicit requests to diagnose and patch a bug.
func RequestMatches(message string) bool {
	lower := strings.ToLower(strings.TrimSpace(message))
	if lower == "" {
		return false
	}
	strong := []string{
		"autopatch",
		"auto patch",
		"auto-patch",
		"patch this bug",
		"patch the bug",
		"fix this bug",
		"fix the bug",
		"find the issue and fix",
		"tests are failing",
		"test is failing",
		"validation failed",
		"validation is failing",
	}
	for _, phrase := range strong {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return (strings.Contains(lower, "bug") || strings.Contains(lower, "error")) &&
		(strings.Contains(lower, "diagnose") || strings.Contains(lower, "solve") || strings.Contains(lower, "patch")) &&
		(strings.Contains(lower, "code") || strings.Contains(lower, "repo") || strings.Contains(lower, "software") || strings.Contains(lower, "system"))
}
