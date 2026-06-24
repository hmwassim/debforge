package service

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
)

func (s *InstallService) Update(ctx context.Context, names []string, force bool, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		for _, name := range names {
			if err := checkInstalled(s.state, st, name, spinner); err != nil {
				spinner.DoneInfo()
				return err
			}
		}
		return s.processAll(ctx, names, force, true, st, spinner, "update", "updated")
	})
}
