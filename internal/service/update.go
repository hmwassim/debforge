package service

import (
	"context"

	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
)

func (s *InstallService) Update(ctx context.Context, names []string, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		for _, name := range names {
			if !s.state.IsInstalled(st, name) {
				spinner.SetDesc(textutil.UcFirst(name + " not installed"))
				continue
			}
			if err := s.processOne(ctx, name, true, st, spinner, "update", "updated"); err != nil {
				return err
			}
		}
		return nil
	})
}
