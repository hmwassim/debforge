package lock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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

// Note: a third case - a second Acquire whose context is cancelled while
// genuinely blocked behind a held lock - is intentionally not covered
// here. Acquire's ctx.Done() branch closes its own fd but then blocks on
// <-done waiting for the background syscall.Flock(LOCK_EX) goroutine to
// return, which itself only returns once the lock is released - so a
// "cancelled while contended" Acquire call does not actually return until
// the contended lock becomes free, defeating the point of the context
// deadline, and can leak the lock to whichever goroutine's blocked
// syscall.Flock happens to be granted it after the fd was already closed.
// This is a real, pre-existing gap in FLock.Acquire that's out of scope
// for this pass (no current debforge call site constructs a Locker
// context with a deadline, so it's not hit in production today) - see
// FOLLOW-UPS.md.
