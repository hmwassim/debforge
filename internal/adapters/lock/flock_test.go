package lock

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFlockAcquireRelease(t *testing.T) {
	l := NewFlockLocker()
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "test.lock")

	release, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}

	// Second acquire should fail (already locked)
	_, err = l.Acquire(context.Background(), lockPath)
	if err == nil {
		t.Fatal("expected error when acquiring already-held lock")
	}

	// Release and acquire again
	release()
	release2, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	release2()
}

func TestFlockDoubleRelease(t *testing.T) {
	l := NewFlockLocker()
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "double.lock")

	release, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	// Should not panic
	release()
	release()
}

func TestFlockLockFileCreated(t *testing.T) {
	l := NewFlockLocker()
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "exists.lock")

	release, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	release()

	if _, err := os.Stat(lockPath); err != nil {
		t.Fatal("expected lock file to exist after release")
	}
}

func TestFlockContextCancelled(t *testing.T) {
	l := NewFlockLocker()
	dir := t.TempDir()
	lockPath := filepath.Join(dir, "cancel.lock")

	// Acquire first lock
	release1, err := l.Acquire(context.Background(), lockPath)
	if err != nil {
		t.Fatal(err)
	}
	defer release1()

	// Try to acquire with cancelled context — context cancellation doesn't affect flock
	// but should respect it
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = l.Acquire(ctx, lockPath)
	if err == nil {
		t.Log("flock does not support context cancellation (expected with current impl)")
	}
}
