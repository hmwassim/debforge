package lock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestAcquire_createsMissingParentDir is the regression test for the bug
// where install/remove/update could fail with a raw "no such file or
// directory" if the directory holding the lock file (var/) didn't exist
// yet - Acquire used to assume it was already there. Acquire must create
// it on demand instead of requiring a prior MkdirAll elsewhere.
func TestAcquire_createsMissingParentDir(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "does", "not", "exist", "yet", "lock")

	l := NewFLock()
	release, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer release()

	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("expected lock file to exist at %s: %v", lockPath, err)
	}
}

func TestAcquire_releaseAllowsReacquire(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	l := NewFLock()

	release1, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	release1()

	release2, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatalf("second Acquire after release: %v", err)
	}
	release2()
}

func TestAcquire_cancelledBeforeCallReturnsImmediately(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	l := NewFLock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, err := l.Acquire(ctx, lockPath)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error from already-cancelled context, got nil")
	}
	if elapsed > 100*time.Millisecond {
		t.Errorf("Acquire took %v on already-cancelled context, should be instant", elapsed)
	}
}

func TestAcquire_returnsOnContextCancelWhileContended(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), "lock")
	l := NewFLock()

	// Speed up tests by shortening the poll interval.
	orig := pollInterval
	pollInterval = 5 * time.Millisecond
	defer func() { pollInterval = orig }()

	// Acquire and hold the lock so the second Acquire contends.
	release1, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatalf("first Acquire: %v", err)
	}
	defer release1()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err = l.Acquire(ctx, lockPath)
	elapsed := time.Since(start)

	if err == nil {
		t.Error("expected error from timed-out context, got nil")
	}
	if elapsed > 300*time.Millisecond {
		t.Errorf("Acquire took %v on contended lock with short deadline, should return promptly", elapsed)
	}
}
