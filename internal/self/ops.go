package self

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/lockrun"
	"github.com/hmwassim/debforge/internal/ports"
)

func requireRoot(action string, sys ports.System) error {
	if !sys.IsPrivileged() {
		return fmt.Errorf("%s must be run as root", action)
	}
	return nil
}

func withRootAndLock(ctx context.Context, action string, sys ports.System, locker ports.Locker, lockPath string, fn func(context.Context) error) error {
	if err := requireRoot(action, sys); err != nil {
		return err
	}
	return lockrun.WithLock(ctx, locker, lockPath, func() error {
		return fn(ctx)
	})
}
