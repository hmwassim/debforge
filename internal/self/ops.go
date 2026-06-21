package self

import (
	"context"

	"github.com/hmwassim/debforge/internal/lockrun"
	"github.com/hmwassim/debforge/internal/ports"
)

func withRootAndLock(ctx context.Context, action string, locker ports.Locker, lockPath string, fn func(context.Context) error) error {
	if err := requireRoot(action); err != nil {
		return err
	}
	return lockrun.WithLock(ctx, locker, lockPath, func() error {
		return fn(ctx)
	})
}
