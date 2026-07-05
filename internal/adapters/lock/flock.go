// Package lock provides a file-based locking implementation using flock(2).
package lock

import (
	"context"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/hmwassim/debforge/internal/ports"
)

var _ ports.Locker = (*FLock)(nil)

// FLock implements ports.Locker using the flock(2) system call.
type FLock struct{}

// NewFLock returns a new FLock.
func NewFLock() *FLock {
	return &FLock{}
}

// pollInterval is how long Acquire waits between non-blocking flock
// attempts when the lock is contended. Kept as a var so tests can
// shorten it.
var pollInterval = 50 * time.Millisecond

// Acquire acquires an exclusive lock on the file at path, blocking until
// the lock is acquired or the context is cancelled. It returns a release
// function that must be called to unlock.
//
// Unlike the previous implementation, this uses LOCK_NB in a poll loop
// so that context cancellation is prompt: there is no background
// goroutine that can block past cancellation on a contended flock(), and
// no risk of leaking a lock grant to a caller who already gave up.
func (l *FLock) Acquire(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	for {
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		default:
		}
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
