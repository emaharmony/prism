package run

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRunLock_AcquireAndRelease(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "run_test")

	lock := NewRunLock(runDir)
	ctx := context.Background()

	err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	// Verify lock file exists
	lockPath := filepath.Join(runDir, ".lock")
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Error("lock file not created")
	}

	err = lock.Release()
	if err != nil {
		t.Fatalf("Release() error = %v", err)
	}
}

func TestRunLock_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "run_concurrent")

	lock1 := NewRunLock(runDir)
	lock2 := NewRunLock(runDir)
	ctx := context.Background()

	// First lock acquires
	err := lock1.Acquire(ctx)
	if err != nil {
		t.Fatalf("lock1.Acquire() error = %v", err)
	}

	// Second lock should fail
	err = lock2.Acquire(ctx)
	if err == nil {
		t.Error("expected ErrRunLocked when acquiring already-locked dir")
		lock2.Release()
	}

	// Release first lock
	lock1.Release()

	// Second lock should now succeed
	err = lock2.Acquire(ctx)
	if err != nil {
		t.Fatalf("lock2.Acquire() after release error = %v", err)
	}
	lock2.Release()
}

func TestRunLock_IsLocked(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "run_islocked")

	lock := NewRunLock(runDir)
	ctx := context.Background()

	// Before acquiring, not locked
	if lock.IsLocked() {
		t.Error("expected IsLocked() = false before acquiring")
	}

	lock.Acquire(ctx)

	// After acquiring by same process, IsLocked checks PID which is our process
	// So it will report as locked since our PID is alive
	if !lock.IsLocked() {
		t.Error("expected IsLocked() = true after acquiring")
	}

	lock.Release()
}

func TestRunLock_TryAcquire(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "run_try")

	lock := NewRunLock(runDir)

	acquired, err := lock.TryAcquire()
	if err != nil {
		t.Fatalf("TryAcquire() error = %v", err)
	}
	if !acquired {
		t.Error("expected TryAcquire() = true")
	}

	lock.Release()
}

func TestRunLock_ForceUnlock(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "run_force")

	lock := NewRunLock(runDir)
	ctx := context.Background()

	lock.Acquire(ctx)

	// Force unlock (simulating crash recovery)
	err := lock.ForceUnlock()
	if err != nil {
		t.Fatalf("ForceUnlock() error = %v", err)
	}

	// Should be able to acquire again
	lock2 := NewRunLock(runDir)
	err = lock2.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire after ForceUnlock() error = %v", err)
	}
	lock2.Release()
}

func TestRunLock_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "nested", "run_new")

	lock := NewRunLock(runDir)
	ctx := context.Background()

	err := lock.Acquire(ctx)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	lock.Release()

	if _, err := os.Stat(runDir); os.IsNotExist(err) {
		t.Error("run directory not created")
	}
}

func TestRunLock_LockInfo(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "run_info")

	lock := NewRunLock(runDir)
	ctx := context.Background()

	info := lock.Info()
	if info.Locked {
		t.Error("expected Locked = false before acquiring")
	}
	if info.RunDir != runDir {
		t.Errorf("RunDir = %q, want %q", info.RunDir, runDir)
	}

	lock.Acquire(ctx)

	info = lock.Info()
	if !info.Locked {
		t.Error("expected Locked = true after acquiring")
	}

	lock.Release()
}

func TestRunLock_Timeout(t *testing.T) {
	tmpDir := t.TempDir()
	runDir := filepath.Join(tmpDir, "run_timeout")

	lock1 := NewRunLock(runDir)
	lock2 := NewRunLock(runDir)

	ctx := context.Background()
	lock1.Acquire(ctx)

	// Second lock with short timeout
	ctx2, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := lock2.Acquire(ctx2)
	if err == nil {
		t.Error("expected timeout error when acquiring locked dir")
	}

	lock1.Release()
}
