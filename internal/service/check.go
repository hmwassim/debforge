package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

// checkInstalled verifies that name is recorded in the state and that
// the package is still installed on the system. When the state says
// installed but the system says not installed, it removes the stale
// entry from the in-memory state (without persisting) and returns
// ErrNotInstalled so the caller can decide whether to proceed.
func (s *baseService) checkInstalled(ctx context.Context, st *State, name string, p *pkg.Package, spinner ports.Spinner) (cleanedUp bool, err error) {
	if !s.state.IsInstalled(st, name) {
		spinner.SetDesc(name + " not installed")
		return false, fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	ok, err := installer.CheckInstalled(ctx, s.runner, s.fs, s.sys, p)
	if err != nil {
		return false, err
	}
	if !ok {
		s.state.Remove(st, name)
		spinner.SetDesc(name + " not installed")
		return true, fmt.Errorf("%w: %s", ErrNotInstalled, name)
	}
	return false, nil
}
