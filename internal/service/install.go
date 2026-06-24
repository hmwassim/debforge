package service

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
)

func (s *InstallService) Run(ctx context.Context, names []string, force bool, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		return s.processAll(ctx, names, force, force, st, spinner, "install", "installed")
	})
}
