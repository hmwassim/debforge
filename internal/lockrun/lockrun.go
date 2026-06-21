// Package lockrun centralizes the "acquire the lock, run the callback,
// always release" pattern. Before this package existed, this exact
// sequence (acquire, wrap the error, defer release) was written by hand
// in three different places: internal/service (install/remove), and
// internal/self (updater and remover). Now all three call WithLock.
package lockrun

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/ports"
)

// WithLock acquires locker at path, runs fn, and always releases the lock
// afterwards, regardless of whether fn succeeds.
func WithLock(ctx context.Context, locker ports.Locker, path string, fn func() error) error {
	release, err := locker.Acquire(ctx, path)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer release()
	return fn()
}
