package debounce

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestAllow(t *testing.T) {
	d := New(WithInterval(100 * time.Millisecond))

	if !d.Allow("user-1") {
		t.Error("first message should be allowed")
	}

	if d.Allow("user-1") {
		t.Error("second message within interval should be debounced")
	}

	// Different user should be allowed
	if !d.Allow("user-2") {
		t.Error("different user should be allowed")
	}
}

func TestAllowAfterInterval(t *testing.T) {
	d := New(WithInterval(50 * time.Millisecond))

	if !d.Allow("user-1") {
		t.Error("first message should be allowed")
	}

	time.Sleep(60 * time.Millisecond)

	if !d.Allow("user-1") {
		t.Error("message after interval should be allowed")
	}
}

func TestOnDrop(t *testing.T) {
	var drops atomic.Int32
	d := New(
		WithInterval(100*time.Millisecond),
		WithOnDrop(func(key string) {
			drops.Add(1)
		}),
	)

	d.Allow("user-1")
	d.Allow("user-1") // should trigger drop

	if drops.Load() != 1 {
		t.Errorf("expected 1 drop callback, got %d", drops.Load())
	}
}

func TestClean(t *testing.T) {
	d := New(WithInterval(10 * time.Millisecond))

	d.Allow("old-user")
	time.Sleep(150 * time.Millisecond)

	d.Clean()

	// After clean, old-user should be allowed again
	if !d.Allow("old-user") {
		t.Error("old user should be allowed after cleanup")
	}
}

func TestReset(t *testing.T) {
	d := New(WithInterval(100 * time.Millisecond))

	d.Allow("user-1")
	d.Reset("user-1")

	// After reset, should be allowed again immediately
	if !d.Allow("user-1") {
		t.Error("user should be allowed after reset")
	}
}

func TestDifferentUsers(t *testing.T) {
	d := New(WithInterval(100 * time.Millisecond))

	if !d.Allow("user-1") {
		t.Error("user-1 first message should be allowed")
	}
	if !d.Allow("user-2") {
		t.Error("user-2 first message should be allowed")
	}
	if d.Allow("user-1") {
		t.Error("user-1 second message should be debounced")
	}
	if d.Allow("user-2") {
		t.Error("user-2 second message should be debounced")
	}
}