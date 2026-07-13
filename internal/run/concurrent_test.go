package run

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewRunPool_DefaultConcurrency(t *testing.T) {
	pool := NewRunPool(0) // should default to 4
	if pool.MaxConcurrent != 4 {
		t.Errorf("MaxConcurrent = %d, want 4", pool.MaxConcurrent)
	}
}

func TestNewRunPool_CustomConcurrency(t *testing.T) {
	pool := NewRunPool(8)
	if pool.MaxConcurrent != 8 {
		t.Errorf("MaxConcurrent = %d, want 8", pool.MaxConcurrent)
	}
}

func TestRunPool_Execute_Success(t *testing.T) {
	pool := NewRunPool(2)
	var executed atomic.Int32

	err := pool.Execute(context.Background(), func(ctx context.Context) error {
		executed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if executed.Load() != 1 {
		t.Errorf("executed = %d, want 1", executed.Load())
	}
}

func TestRunPool_Execute_Concurrent(t *testing.T) {
	pool := NewRunPool(4)
	var wg sync.WaitGroup
	var executed atomic.Int32

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Execute(context.Background(), func(ctx context.Context) error {
				executed.Add(1)
				time.Sleep(10 * time.Millisecond) // simulate work
				return nil
			})
		}()
	}
	wg.Wait()

	if executed.Load() != 10 {
		t.Errorf("executed = %d, want 10", executed.Load())
	}
	if pool.Total() != 10 {
		t.Errorf("Total() = %d, want 10", pool.Total())
	}
}

func TestRunPool_Execute_ConcurrencyLimit(t *testing.T) {
	pool := NewRunPool(2)
	var running atomic.Int32
	var maxRunning atomic.Int32

	var wg sync.WaitGroup
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pool.Execute(context.Background(), func(ctx context.Context) error {
				current := running.Add(1)
				// Track max concurrent
				for {
					old := maxRunning.Load()
					if current <= old || maxRunning.CompareAndSwap(old, current) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				running.Add(-1)
				return nil
			})
		}()
	}
	wg.Wait()

	if maxRunning.Load() > int32(pool.MaxConcurrent) {
		t.Errorf("max concurrent = %d, exceeded limit %d", maxRunning.Load(), pool.MaxConcurrent)
	}
}

func TestRunPool_Execute_Error(t *testing.T) {
	pool := NewRunPool(2)
	expectedErr := errors.New("task failed")

	err := pool.Execute(context.Background(), func(ctx context.Context) error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Errorf("Execute() error = %v, want %v", err, expectedErr)
	}
}

func TestRunPool_Execute_ContextCancelled(t *testing.T) {
	pool := NewRunPool(1) // Only 1 slot

	// Fill the slot with a long-running task
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool.Execute(context.Background(), func(ctx context.Context) error {
			time.Sleep(200 * time.Millisecond)
			return nil
		})
	}()

	// Give the first task time to acquire the slot
	time.Sleep(20 * time.Millisecond)

	// Try to execute with a very short timeout — should timeout waiting for slot
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	err := pool.Execute(ctx, func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Error("expected error for cancelled context")
	}

	wg.Wait()
}

func TestRunPool_TryExecute_Success(t *testing.T) {
	pool := NewRunPool(2)
	var executed atomic.Int32

	err := pool.TryExecute(context.Background(), func(ctx context.Context) error {
		executed.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("TryExecute() error = %v", err)
	}
	if executed.Load() != 1 {
		t.Errorf("executed = %d, want 1", executed.Load())
	}
}

func TestRunPool_TryExecute_PoolFull(t *testing.T) {
	pool := NewRunPool(1)

	// Fill the slot
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		pool.Execute(context.Background(), func(ctx context.Context) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		})
	}()

	// Give the first task time to start
	time.Sleep(10 * time.Millisecond)

	// Try to execute another — should get ErrPoolFull
	err := pool.TryExecute(context.Background(), func(ctx context.Context) error {
		return nil
	})
	if !errors.Is(err, ErrPoolFull) {
		t.Errorf("TryExecute() error = %v, want ErrPoolFull", err)
	}

	wg.Wait()
}

func TestRunPool_RunningAndAvailable(t *testing.T) {
	pool := NewRunPool(4)

	if pool.Running() != 0 {
		t.Errorf("Running() = %d, want 0 before any tasks", pool.Running())
	}
	if pool.Available() != 4 {
		t.Errorf("Available() = %d, want 4 before any tasks", pool.Available())
	}
}

func TestRunPool_Total(t *testing.T) {
	pool := NewRunPool(2)

	for i := 0; i < 5; i++ {
		pool.Execute(context.Background(), func(ctx context.Context) error {
			return nil
		})
	}

	if pool.Total() != 5 {
		t.Errorf("Total() = %d, want 5", pool.Total())
	}
}
