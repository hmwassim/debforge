package lock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/hmwassim/debforge/internal/ports"
)

type FlockLocker struct{}

func NewFlockLocker() *FlockLocker {
	return &FlockLocker{}
}

func (l *FlockLocker) Acquire(ctx context.Context, name string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, fmt.Errorf("another debforge process is already running")
		}
		return nil, err
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}

var _ ports.Locker = (*FlockLocker)(nil)
