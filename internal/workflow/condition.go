package workflow

import (
	"fmt"
	"strings"
)

// EvaluateCondition evaluates a simple condition expression.
//
// Supported syntax: step_id.field == "value" or step_id.field == value
// If the condition is empty, it returns true (no condition = always run).
// If the referenced step doesn't exist or the field is missing, returns false.
func EvaluateCondition(expr string, stepOutputs map[string]map[string]any) (bool, error) {
	if expr == "" {
		return true, nil
	}

	// Parse: step_id.field == "value" or step_id.field == value
	parts := strings.SplitN(expr, "==", 2)
	if len(parts) != 2 {
		return false, nil // Unknown expression format, treat as not met
	}

	lhs := strings.TrimSpace(parts[0])
	rhs := strings.TrimSpace(parts[1])

	// Parse lhs: step_id.field
	dotParts := strings.SplitN(lhs, ".", 2)
	if len(dotParts) != 2 {
		return false, nil
	}

	stepID := dotParts[0]
	field := dotParts[1]

	// Look up step output
	output, ok := stepOutputs[stepID]
	if !ok {
		return false, nil // Step hasn't run yet or doesn't exist
	}

	value, ok := output[field]
	if !ok {
		return false, nil // Field doesn't exist in step output
	}

	// Clean rhs: remove surrounding quotes if present
	rhs = strings.Trim(rhs, "\"")
	rhs = strings.Trim(rhs, "'")

	// Compare as strings
	return fmt.Sprintf("%v", value) == rhs, nil
}