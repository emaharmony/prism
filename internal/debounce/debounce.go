// Package debounce provides per-key rate limiting for message processing.
// It prevents rapid-fire messages from the same user from triggering
// multiple concurrent LLM calls.
package debounce

import (
	"sync"
	"time"
)

// DefaultInterval is the minimum time between processed messages per key.
const DefaultInterval = 500 * time.Millisecond

// Tracker tracks the last processed time for each key and determines
// whether a new message should be processed or skipped.
type Tracker struct {
	mu        sync.RWMutex
	lastSeen  map[string]time.Time
	interval  time.Duration
	onDrop    func(key string)
}

// Option configures a Tracker.
type Option func(*Tracker)

// WithInterval sets the minimum interval between processed messages.
func WithInterval(d time.Duration) Option {
	return func(t *Tracker) {
		t.interval = d
	}
}

// WithOnDrop sets a callback for when a message is dropped.
func WithOnDrop(fn func(key string)) Option {
	return func(t *Tracker) {
		t.onDrop = fn
	}
}

// New creates a new debounce tracker with the given options.
func New(opts ...Option) *Tracker {
	t := &Tracker{
		lastSeen: make(map[string]time.Time),
		interval: DefaultInterval,
		onDrop:   func(key string) {},
	}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// Allow checks whether a message from the given key should be processed.
// Returns true if enough time has passed since the last processed message.
// Returns false if the message should be debounced (dropped).
func (t *Tracker) Allow(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := time.Now()
	last, exists := t.lastSeen[key]
	if !exists || now.Sub(last) >= t.interval {
		t.lastSeen[key] = now
		return true
	}

	// Message arrived too soon after the last one — debounce it.
	if t.onDrop != nil {
		t.onDrop(key)
	}
	return false
}

// Clean removes entries older than 10x the interval.
// Call this periodically to prevent memory leaks from inactive users.
func (t *Tracker) Clean() {
	t.mu.Lock()
	defer t.mu.Unlock()

	threshold := time.Now().Add(-10 * t.interval)
	for key, last := range t.lastSeen {
		if last.Before(threshold) {
			delete(t.lastSeen, key)
		}
	}
}

// Reset clears the debounce state for a specific key.
func (t *Tracker) Reset(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.lastSeen, key)
}