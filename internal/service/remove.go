package service

import (
	"context"
	"fmt"

	"github.com/hmwassim/debforge/internal/domain/installer"
	"github.com/hmwassim/debforge/internal/domain/pkg"
	"github.com/hmwassim/debforge/internal/ports"
)

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
	release, err := s.locker.Acquire(ctx, s.lockPath)
	if err != nil {
		return fmt.Errorf("acquire lock: %w", err)
	}
	defer release()

	st, err := s.state.Load()
	if err != nil {
		return fmt.Errorf("load state: %w", err)
	}

	for _, name := range names {
		if err := s.removeOne(ctx, name, st, spinner); err != nil {
			return err
		}
	}
	return nil
}

func (s *RemoveService) removeOne(ctx context.Context, name string, st *State, spinner ports.Spinner) error {
	p, ok := s.reg.Lookup(name)
	if !ok {
		return fmt.Errorf("unknown package: %s", name)
	}

	if !s.state.IsInstalled(st, name) {
		spinner.SetDesc(name + " not installed")
		return nil
	}

	inst, ok := s.instReg.Lookup(p.Type)
	if !ok {
		return fmt.Errorf("no installer for type %s", p.Type)
	}
	if err := inst.Remove(ctx, p, spinner); err != nil {
		return fmt.Errorf("remove %s: %w", p.Name, err)
	}

	s.state.Remove(st, name)
	if err := s.state.Save(st); err != nil {
		return fmt.Errorf("save state after %s: %w", name, err)
	}

	spinner.SetDesc(name + " removed")
	return nil
}
