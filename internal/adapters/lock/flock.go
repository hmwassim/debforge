package lock

import (
	"context"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hmwassim/debforge/internal/ports"
)

var _ ports.Locker = (*FLock)(nil)

type FLock struct{}

func NewFLock() *FLock {
	return &FLock{}
}

func (l *FLock) Acquire(ctx context.Context, path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	type result struct {
		err error
	}
	done := make(chan result, 1)

	go func() {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		done <- result{err}
	}()

	select {
	case <-ctx.Done():
		f.Close()
		<-done
		return nil, ctx.Err()
	case r := <-done:
		if r.err != nil {
			f.Close()
			return nil, r.err
		}
		return func() {
			syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
			f.Close()
		}, nil
	}
}
