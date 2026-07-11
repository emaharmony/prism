package safety

import (
	"sync"
	"testing"
	"time"
)

func TestRateLimiterBurstAndRefill(t *testing.T) {
	limiter := NewRateLimiter(2, 2)
	for range 2 {
		if !limiter.Allow() {
			t.Fatal("initial burst should be allowed")
		}
	}
	if limiter.Allow() {
		t.Fatal("request beyond burst should be denied")
	}

	limiter.mu.Lock()
	limiter.lastRefill = time.Now().Add(-time.Second)
	limiter.mu.Unlock()
	if !limiter.Allow() {
		t.Fatal("elapsed refill interval should restore tokens")
	}
}

func TestUserRateLimiterPerUserAndGlobalLimits(t *testing.T) {
	limiter := NewUserRateLimiter(1, 0, 2, 0)
	if !limiter.Allow("alice") {
		t.Fatal("alice's first request should pass")
	}
	if limiter.Allow("alice") {
		t.Fatal("alice should exceed the per-user limit")
	}
	if limiter.Allow("bob") {
		t.Fatal("denied alice request still consumes the global token, so bob should hit the global limit")
	}
}

func TestUserRateLimiterCleanupStale(t *testing.T) {
	limiter := NewUserRateLimiter(2, 1, 10, 1)
	limiter.userLimits["full"] = NewRateLimiter(2, 1)
	active := NewRateLimiter(2, 1)
	active.tokens = 1
	limiter.userLimits["active"] = active

	limiter.CleanupStale(time.Minute)
	if _, ok := limiter.userLimits["full"]; ok {
		t.Fatal("full idle limiter was not removed")
	}
	if _, ok := limiter.userLimits["active"]; !ok {
		t.Fatal("active limiter was removed")
	}
}

func TestRateLimiterConcurrentAccess(t *testing.T) {
	limiter := NewRateLimiter(50, 0)
	var wg sync.WaitGroup
	results := make(chan bool, 100)
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- limiter.Allow()
		}()
	}
	wg.Wait()
	close(results)

	allowed := 0
	for result := range results {
		if result {
			allowed++
		}
	}
	if allowed != 50 {
		t.Fatalf("allowed = %d, want 50", allowed)
	}
}
