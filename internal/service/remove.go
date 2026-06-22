package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
	"github.com/hmwassim/debforge/internal/textutil"
)

var ErrNotInstalled = errors.New("not installed")

type RemoveService struct {
	reg      *pkg.Registry
	instReg  *installer.Registry
	state    *StateManager
	locker   ports.Locker
	lockPath string
}

func NewRemoveService(
	reg *pkg.Registry,
	instReg *installer.Registry,
	state *StateManager,
	locker ports.Locker,
	lockPath string,
) *RemoveService {
	return &RemoveService{
		reg:      reg,
		instReg:  instReg,
		state:    state,
		locker:   locker,
		lockPath: lockPath,
	}
}

func (s *RemoveService) Run(ctx context.Context, names []string, spinner ports.Spinner) error {
	return withState(ctx, s.locker, s.lockPath, s.state, func(st *State) error {
		for _, name := range names {
			if err := s.RemoveOne(ctx, name, st, spinner); err != nil {
				return err
			}
		}
		return nil
	})
}

// RemoveOne removes a single already-resolved package from an
// already-loaded state. It is exported (rather than kept private to this
// service) so other flows that need to remove a managed package under a
// lock they already hold - such as internal/self's self-remove flow - call
// this instead of re-implementing lookup + remove + state bookkeeping by
// hand.
func (s *RemoveService) RemoveOne(ctx context.Context, name string, st *State, spinner ports.Spinner) error {
	p, err := LookupPackage(s.reg, name)
	if err != nil {
		return err
	}

	if err := checkInstalled(s.state, st, name, spinner); err != nil {
		return err
	}

	inst, err := LookupInstaller(s.instReg, p.Type)
	if err != nil {
		return err
	}
	if err := inst.Remove(ctx, p, spinner); err != nil {
		return fmt.Errorf("remove %s: %w", p.Name, err)
	}

	s.state.Remove(st, name)
	if err := saveState(s.state, st, name); err != nil {
		return err
	}

	spinner.SetDesc(textutil.UcFirst(name + " removed"))
	return nil
}
