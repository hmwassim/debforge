// Package lock provides the single global lock that serialises every
// mutating debforge operation (install, remove, update, setup, self-update).
package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// Lock is a held flock. The kernel releases it if the process dies, so a
// stale lock file is harmless.
type Lock struct {
	f *os.File
}

// Acquire takes an exclusive flock on path, waiting until ctx is done.
// onWait is called once if the lock is contended.
func Acquire(ctx context.Context, path string, onWait func()) (*Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("lock dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	waited := false
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			_ = f.Truncate(0)
			_, _ = f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
			return &Lock{f: f}, nil
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EINTR) {
			f.Close()
			return nil, fmt.Errorf("flock %s: %w", path, err)
		}
		if !waited {
			waited = true
			if onWait != nil {
				onWait()
			}
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// Holder returns the pid recorded in the lock file, if any.
func Holder(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(trimNL(b))
}

func trimNL(b []byte) []byte {
	for len(b) > 0 && (b[len(b)-1] == '\n' || b[len(b)-1] == ' ') {
		b = b[:len(b)-1]
	}
	return b
}

// Release drops the lock.
func (l *Lock) Release() error {
	if l == nil || l.f == nil {
		return nil
	}
	err := syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	cerr := l.f.Close()
	l.f = nil
	return errors.Join(err, cerr)
}
