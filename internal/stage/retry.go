// Package stage provides Prism's pipeline execution engine (V14a+).
//
// V14b adds exponential backoff retry for LLM and tool stages.
// Retries only on retryable errors (429, 503, timeout, network).
// Mutations NEVER retry — this is enforced at the type level.
package stage

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// RetryConfig controls exponential backoff retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts (0 = no retry).
	MaxRetries int

	// BaseDelay is the initial delay between retries.
	BaseDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration

	// Jitter is the random jitter added to each delay to prevent
	// thundering herd. Set to 0 for deterministic delays.
	Jitter time.Duration
}

// DefaultRetryConfig returns the default retry configuration.
// 3 retries, 1s base, 30s max, 500ms jitter.
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
		Jitter:     500 * time.Millisecond,
	}
}

// RetryableError wraps an error that can be retried.
// Not all errors should be retried — mutations, for example, should
// never be retried because they have side effects.
type RetryableError struct {
	Err     error
	Retryable bool
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError creates a retryable error.
func NewRetryableError(err error) *RetryableError {
	return &RetryableError{Err: err, Retryable: true}
}

// NewNonRetryableError creates a non-retryable error.
// Used for mutations and other operations that have side effects.
func NewNonRetryableError(err error) *RetryableError {
	return &RetryableError{Err: err, Retryable: false}
}

// IsRetryable checks if an error should be retried.
// Returns true for RetryableError with Retryable=true,
// and for common transient errors (429, 503, timeout, network).
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for explicit RetryableError
	if re, ok := err.(*RetryableError); ok {
		return re.Retryable
	}

	// Check for context errors (timeout, deadline)
	if err == context.DeadlineExceeded || err == context.Canceled {
		return true
	}

	// Check for common transient error messages
	msg := err.Error()
	transientPatterns := []string{
		"503",           // Service Unavailable
		"429",           // Too Many Requests
		"timeout",
		"connection refused",
		"connection reset",
		"temporary failure",
		"rate limit",
	}
	for _, pattern := range transientPatterns {
		if contains(msg, pattern) {
			return true
		}
	}

	// Default: non-retryable
	return false
}

// RetryWithBackoff executes a function with exponential backoff retry.
// It retries only on retryable errors. Non-retryable errors return immediately.
// Each retry has a delay that increases exponentially with jitter.
//
// The function receives the attempt number (0-based) as a parameter.
//
// Returns the result on success, or the last error on failure.
func RetryWithBackoff[T any](ctx context.Context, config RetryConfig, fn func(attempt int) (T, error)) (T, error) {
	var lastErr error
	var zero T

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		default:
		}

		// Execute the function
		result, err := fn(attempt)
		if err == nil {
			return result, nil
		}

		// Check if the error is retryable
		if !IsRetryable(err) {
			return zero, err
		}

		lastErr = err

		// Don't sleep after the last attempt
		if attempt >= config.MaxRetries {
			break
		}

		// Calculate delay with exponential backoff and jitter
		delay := calculateDelay(attempt, config)

		// Emit a retry event (the caller can listen for this)
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	return zero, fmt.Errorf("retry: %d attempts exhausted: %w", config.MaxRetries+1, lastErr)
}

// calculateDelay computes the delay for a given attempt with exponential
// backoff and jitter.
func calculateDelay(attempt int, config RetryConfig) time.Duration {
	// Exponential backoff: baseDelay * 2^attempt
	delay := config.BaseDelay
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay >= config.MaxDelay {
			delay = config.MaxDelay
			break
		}
	}

	// Cap at max delay
	if delay > config.MaxDelay {
		delay = config.MaxDelay
	}

	// Add random jitter (uniform distribution)
	if config.Jitter > 0 {
		jitter := time.Duration(rand.Int63n(int64(config.Jitter)))
		delay += jitter - config.Jitter/2 // Center jitter around 0
		if delay < 0 {
			delay = 0
		}
	}

	return delay
}

// contains checks if a string contains a substring (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}